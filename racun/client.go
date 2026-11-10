package racun

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync/atomic"
	"time"
)

// Environment selects which CIS a client talks to.
type Environment string

const (
	// EnvironmentSandbox is the CIS test environment.
	EnvironmentSandbox Environment = "sandbox"
	// EnvironmentLive is the CIS production environment.
	EnvironmentLive Environment = "live"
)

// endpoints are the invoice fiscalization service URLs per environment (§3, §6.1). The
// port is 8449; the B2B eRačun fiscalization service is on 8509.
var endpoints = map[Environment]string{
	EnvironmentSandbox: "https://cistest.apis-it.hr:8449/FiskalizacijaServiceTest",
	EnvironmentLive:    "https://cis.porezna-uprava.hr:8449/FiskalizacijaService",
}

// answerSigners is the common name of the certificate CIS signs its answers
// with, by environment. CIS test's is issued by Fina Demo CA 2020 to
// APIS IT.
var answerSigners = map[Environment]string{
	EnvironmentSandbox: "fiskalcistest",
	EnvironmentLive:    "fiskalcis",
}

// soapActionInvoice is the SOAP action of the invoice operation (racuni)
// in the service's WSDL.
const soapActionInvoice = "http://e-porezna.porezna-uprava.hr/fiskalizacija/2012/services/FiskalizacijaService/racuni"

// maxAnswer is the most of an answer that is read. Answers are a few
// kilobytes.
const maxAnswer = 1 << 20

// timeout bounds a whole call to CIS, from connecting to reading the
// answer. CIS aims to answer within two seconds. A variable only so tests
// can shorten it.
var timeout = 30 * time.Second

// Config prepares a Client for one environment.
type Config struct {
	Environment Environment
	// Certificate is the issuer's FINA fiscalization certificate. It signs
	// every request.
	Certificate *Certificate
	// CABundle holds the PEM certificates trusted for CIS: the FINA chains
	// that issue its TLS certificate and the certificate it signs answers
	// with. Nothing else is trusted.
	CABundle []byte
}

// Client sends invoices to one CIS environment.
type Client struct {
	url    string
	signer string
	cert   *Certificate
	roots  *x509.CertPool
	conn   *http.Client
	// now is the time answers' certificates are checked at.
	now func() time.Time
}

// New checks the configuration and prepares a client.
func New(conf *Config) (*Client, error) {
	endpoint, ok := endpoints[conf.Environment]
	if !ok {
		return nil, fmt.Errorf("unknown CIS environment %q", conf.Environment)
	}
	return newClient(conf, endpoint)
}

// newClient prepares a client that calls endpoint. Tests use it directly to
// call a local server.
func newClient(conf *Config, endpoint string) (*Client, error) {
	if conf.Certificate == nil || conf.Certificate.Cert == nil || conf.Certificate.Key == nil {
		return nil, errors.New("a signing certificate is required")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(conf.CABundle) {
		return nil, errors.New("the CA bundle holds no PEM certificate")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	return &Client{
		url:    endpoint,
		signer: answerSigners[conf.Environment],
		cert:   conf.Certificate,
		roots:  roots,
		conn: &http.Client{
			Transport: transport,
			// A redirect is never followed: on a 307 or 308 net/http would
			// post the signed request again to wherever Location points.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		now: time.Now,
	}, nil
}

// URL returns the CIS endpoint the client talks to.
func (c *Client) URL() string {
	return c.url
}

// Fiscalize signs an invoice request and sends it, for callers that keep no
// record of what they sent. It also checks that the answer is for this
// request. See SignInvoice and SendInvoice.
func (c *Client) Fiscalize(ctx context.Context, req *InvoiceRequest) (*InvoiceResponse, error) {
	envelope, err := c.SignInvoice(req)
	if err != nil {
		return nil, err
	}
	res, err := c.SendInvoice(ctx, envelope)
	if res != nil && req.Header != nil && res.Header != nil && res.Header.MessageID != req.Header.MessageID {
		return res, ErrSignature.WithRequestWritten(true).WithMessage(fmt.Sprintf(
			"the answer is for message %s, not %s", res.Header.MessageID, req.Header.MessageID))
	}
	return res, err
}

// SignInvoice signs an invoice request and returns the SOAP envelope to
// send. Store it before sending: it is the record of what CIS got, and it
// must be sent exactly as returned, since the signature covers it. An error
// here is a bug or a certificate problem, never something CIS said.
func (c *Client) SignInvoice(req *InvoiceRequest) ([]byte, error) {
	signed, err := Sign(c.cert, req)
	if err != nil {
		return nil, err
	}
	return envelop(signed), nil
}

// SendInvoice posts an envelope from SignInvoice to CIS and returns its
// answer, after checking the answer's signature. A refusal returns the
// answer, with all its errors, and an ErrCIS. Other errors are ErrServer,
// ErrSignature and ErrConnection, with no answer. It does not retry, and it
// does not follow a redirect.
//
// The answer's MessageID should match the request's; Fiscalize checks it.
func (c *Client) SendInvoice(ctx context.Context, envelope []byte) (*InvoiceResponse, error) {
	status, data, err := c.post(ctx, soapActionInvoice, envelope)
	if err != nil {
		return nil, err
	}
	return c.readInvoiceAnswer(status, data)
}

// post sends a SOAP envelope and returns the HTTP status and body of the
// answer.
func (c *Client) post(ctx context.Context, action string, envelope []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// The write happens on the transport's own goroutine, so the flag is
	// read and written concurrently.
	var written atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				written.Store(true)
			}
		},
	})

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(envelope))
	if err != nil {
		return 0, nil, connError(err)
	}
	hreq.Header.Set("Content-Type", "text/xml; charset=utf-8")
	hreq.Header.Set("SOAPAction", action)
	hres, err := c.conn.Do(hreq)
	if err != nil {
		return 0, nil, connError(err).WithRequestWritten(written.Load())
	}
	defer hres.Body.Close() //nolint:errcheck

	// The answer's headers arrived, so the request was written even if
	// reading the body fails.
	data, err := io.ReadAll(io.LimitReader(hres.Body, maxAnswer+1))
	if err != nil {
		return 0, nil, connError(err).WithRequestWritten(true)
	}
	if len(data) > maxAnswer {
		return 0, nil, ErrConnection.WithRequestWritten(true).WithMessage(fmt.Sprintf("the answer exceeds %d bytes", maxAnswer))
	}
	return hres.StatusCode, data, nil
}

// connError wraps a transport error, leaving the URL out of the message.
func connError(err error) *Error {
	msg := err.Error()
	var uerr *url.Error
	if errors.As(err, &uerr) {
		msg = uerr.Err.Error()
	}
	return ErrConnection.WithMessage(msg).WithCause(err)
}
