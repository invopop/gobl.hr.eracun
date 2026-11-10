package racun

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

// NamespaceSOAP is the SOAP 1.1 envelope namespace.
const NamespaceSOAP = "http://schemas.xmlsoap.org/soap/envelope/"

// envelop places a signed request in a SOAP envelope. The request is
// signed with exclusive canonicalization, so the envelope's namespace does
// not change its signature.
func envelop(signed []byte) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<soapenv:Envelope xmlns:soapenv="` + NamespaceSOAP + `"><soapenv:Body>`)
	b.Write(signed)
	b.WriteString(`</soapenv:Body></soapenv:Envelope>`)
	return []byte(b.String())
}

// readInvoiceAnswer reads an HTTP answer to an invoice request. CIS answers a
// refusal with HTTP 500 and a signed RacunOdgovor, not a SOAP fault, so the
// message in the body decides, not the status.
func (c *Client) readInvoiceAnswer(status int, data []byte) (*InvoiceResponse, error) {
	if status >= 300 && status < 400 {
		// The client follows no redirect, so the request stopped here.
		// Where it pointed is left out of the message.
		return nil, ErrConnection.WithRequestWritten(true).WithCode(strconv.Itoa(status)).
			WithMessage("CIS answered with a redirect, which is never followed; check the CIS URL")
	}
	msg, err := answerMessage(data)
	if err != nil {
		if status != 200 {
			return nil, ErrConnection.WithRequestWritten(true).WithCode(strconv.Itoa(status)).WithMessage(snippet(data))
		}
		return nil, ErrConnection.WithRequestWritten(true).WithMessage(err.Error())
	}

	switch {
	case msg.NamespaceURI() == NamespaceSOAP && msg.Tag == "Fault":
		return nil, faultError(msg)
	case msg.NamespaceURI() != NamespaceF73 || msg.Tag != "RacunOdgovor":
		return nil, ErrConnection.WithRequestWritten(true).WithCode(strconv.Itoa(status)).
			WithMessage(fmt.Sprintf("unexpected answer message %s", msg.Tag))
	}

	signed, err := c.verifyAnswer(msg)
	if err != nil {
		return nil, err
	}
	// Read the answer from what the signature covers, never from the raw
	// document.
	doc := etree.NewDocument()
	doc.SetRoot(signed)
	out, err := doc.WriteToBytes()
	if err != nil {
		return nil, ErrConnection.WithRequestWritten(true).WithMessage("writing the signed answer: " + err.Error())
	}
	res := new(InvoiceResponse)
	if err := xml.Unmarshal(out, res); err != nil {
		return nil, ErrConnection.WithRequestWritten(true).WithMessage("reading the answer: " + err.Error()).WithCause(err)
	}

	switch {
	case len(res.Errors) > 0:
		return res, refusal(res.Errors)
	case res.JIR == "":
		return nil, ErrConnection.WithRequestWritten(true).WithMessage("the answer has neither a JIR nor errors")
	}
	return res, nil
}

// answerMessage returns the single element in the body of a SOAP answer.
func answerMessage(data []byte) (*etree.Element, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("reading the answer: %w", err)
	}
	env := doc.Root()
	if env == nil || env.Tag != "Envelope" || env.NamespaceURI() != NamespaceSOAP {
		return nil, fmt.Errorf("the answer is not a SOAP envelope")
	}
	var body *etree.Element
	for _, el := range env.ChildElements() {
		if el.Tag == "Body" && el.NamespaceURI() == NamespaceSOAP {
			body = el
		}
	}
	if body == nil {
		return nil, fmt.Errorf("the answer has no SOAP body")
	}
	msgs := body.ChildElements()
	if len(msgs) != 1 {
		return nil, fmt.Errorf("the answer body holds %d messages, want 1", len(msgs))
	}
	return msgs[0], nil
}

// faultError turns a SOAP 1.1 fault into an error: ErrServer for a fault on
// CIS's side, ErrCIS for one blamed on the request.
func faultError(fault *etree.Element) error {
	var code, text string
	if el := fault.SelectElement("faultcode"); el != nil {
		code = strings.TrimSpace(el.Text())
	}
	if el := fault.SelectElement("faultstring"); el != nil {
		text = strings.TrimSpace(el.Text())
	}
	local := code
	if i := strings.LastIndex(local, ":"); i >= 0 {
		local = local[i+1:]
	}
	if local == "Server" || strings.HasPrefix(local, "Server.") {
		return ErrServer.WithRequestWritten(true).WithCode(code).WithMessage(text)
	}
	return ErrCIS.WithCode(code).WithMessage(text)
}

// refusal is the ErrCIS for the errors in an answer: the first error's code
// and message, or every error's code and message when there are more.
func refusal(errs []*ResponseError) error {
	if len(errs) == 1 {
		return ErrCIS.WithCode(errs[0].Code).WithMessage(errs[0].Message)
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Code + " " + e.Message
	}
	return ErrCIS.WithCode(errs[0].Code).WithMessage(strings.Join(msgs, "; "))
}

// verifyAnswer checks the enveloped signature of an answer message: that it
// covers the message, that the certificate in KeyInfo has the CIS common name
// for the environment and chains to the CA bundle, and that the digest and
// signature value are right. It returns the signed message.
func (c *Client) verifyAnswer(msg *etree.Element) (*etree.Element, error) {
	cert, err := keyInfoCertificate(msg)
	if err != nil {
		return nil, ErrSignature.WithRequestWritten(true).WithMessage(err.Error())
	}
	if cert.Subject.CommonName != c.signer {
		return nil, ErrSignature.WithRequestWritten(true).
			WithMessage(fmt.Sprintf("the answer is signed by %q, not %s", cert.Subject.CommonName, c.signer))
	}
	now := c.now()
	opts := x509.VerifyOptions{Roots: c.roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}
	if _, err := cert.Verify(opts); err != nil {
		return nil, ErrSignature.WithRequestWritten(true).
			WithMessage("the answer's certificate is not trusted: " + err.Error()).WithCause(err)
	}

	ctx := dsig.NewDefaultValidationContext(&dsig.MemoryX509CertificateStore{Roots: []*x509.Certificate{cert}})
	ctx.IdAttribute = "Id"
	ctx.Clock = dsig.NewFakeClockAt(now)
	signed, err := ctx.Validate(withScopeNamespaces(msg))
	if err != nil {
		return nil, ErrSignature.WithRequestWritten(true).WithMessage(err.Error()).WithCause(err)
	}
	return signed, nil
}

// keyInfoCertificate returns the certificate in the KeyInfo of the
// signature among el's children.
func keyInfoCertificate(el *etree.Element) (*x509.Certificate, error) {
	var sig *etree.Element
	for _, ch := range el.ChildElements() {
		if ch.Tag == "Signature" && ch.NamespaceURI() == NamespaceDSig {
			if sig != nil {
				return nil, fmt.Errorf("the answer has more than one signature")
			}
			sig = ch
		}
	}
	if sig == nil {
		return nil, fmt.Errorf("the answer is not signed")
	}
	data := sig.FindElement("./KeyInfo/X509Data/X509Certificate")
	if data == nil {
		return nil, fmt.Errorf("the answer's signature has no certificate")
	}
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(data.Text()), ""))
	if err != nil {
		return nil, fmt.Errorf("decoding the answer's certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parsing the answer's certificate: %w", err)
	}
	return cert, nil
}

// withScopeNamespaces returns a copy of el that declares every namespace in
// scope at el, the nearest declaration of each prefix winning. CIS signs
// answers with inclusive canonicalization, which writes all of them, and
// goxmldsig validates a copy of the element that has lost its ancestors.
func withScopeNamespaces(el *etree.Element) *etree.Element {
	cp := el.Copy()
	declared := make(map[string]bool)
	for _, a := range cp.Attr {
		if isNamespaceDecl(a) {
			declared[a.FullKey()] = true
		}
	}
	for p := el.Parent(); p != nil; p = p.Parent() {
		for _, a := range p.Attr {
			if !isNamespaceDecl(a) || declared[a.FullKey()] {
				continue
			}
			declared[a.FullKey()] = true
			cp.CreateAttr(a.FullKey(), a.Value)
		}
	}
	return cp
}

// isNamespaceDecl reports whether an attribute declares a namespace.
func isNamespaceDecl(a etree.Attr) bool {
	const xmlns = "xmlns"
	return a.Space == xmlns || (a.Space == "" && a.Key == xmlns)
}

// snippet is the start of an answer, for an error message.
func snippet(data []byte) string {
	const n = 512
	if len(data) <= n {
		return string(data)
	}
	return strings.ToValidUTF8(string(data[:n]), "") + "…"
}
