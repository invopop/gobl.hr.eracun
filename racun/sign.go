package racun

import (
	"encoding/xml"
	"errors"
	"fmt"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

// Sign marshals a request and adds an enveloped XML-DSig signature as the
// last child of its root (§7): exclusive canonicalization, RSA-SHA256, a
// SHA-256 digest, and a reference to the root's Id. The certificate goes in
// KeyInfo. It returns the signed request, ready for a SOAP body.
func Sign(cert *Certificate, req *InvoiceRequest) ([]byte, error) {
	if cert == nil || cert.Cert == nil || cert.Key == nil {
		return nil, errors.New("no certificate")
	}
	if req.ID == "" {
		return nil, errors.New("the request has no Id to reference")
	}
	data, err := xml.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(data); err != nil {
		return nil, fmt.Errorf("reading request: %w", err)
	}

	ctx, err := dsig.NewSigningContext(cert.Key, [][]byte{cert.Cert.Raw})
	if err != nil {
		return nil, fmt.Errorf("signing context: %w", err)
	}
	ctx.IdAttribute = "Id"
	ctx.Canonicalizer = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
	if err := ctx.SetSignatureMethod(dsig.RSASHA256SignatureMethod); err != nil {
		return nil, err
	}
	signed, err := ctx.SignEnveloped(doc.Root())
	if err != nil {
		return nil, fmt.Errorf("signing request: %w", err)
	}

	out := etree.NewDocument()
	out.SetRoot(signed)
	return out.WriteToBytes()
}
