package racun

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	return data
}

// recordedClient is a sandbox client that trusts the FINA CA bundle and
// checks certificates when the accepted cassette was recorded.
func recordedClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(&Config{
		Environment: EnvironmentSandbox,
		Certificate: testCertificate(t),
		CABundle:    readTestdata(t, finaCABundle),
	})
	require.NoError(t, err)
	sentAt := acceptedCassette(t).SentAt
	c.now = func() time.Time { return sentAt }
	return c
}

func TestNew(t *testing.T) {
	bundle := readTestdata(t, finaCABundle)
	t.Run("sandbox", func(t *testing.T) {
		c, err := New(&Config{Environment: EnvironmentSandbox, Certificate: testCertificate(t), CABundle: bundle})
		require.NoError(t, err)
		assert.Equal(t, "https://cistest.apis-it.hr:8449/FiskalizacijaServiceTest", c.URL())
		assert.Equal(t, "fiskalcistest", c.signer)
	})
	t.Run("live", func(t *testing.T) {
		c, err := New(&Config{Environment: EnvironmentLive, Certificate: testCertificate(t), CABundle: bundle})
		require.NoError(t, err)
		assert.Equal(t, "https://cis.porezna-uprava.hr:8449/FiskalizacijaService", c.URL())
		assert.Equal(t, "fiskalcis", c.signer)
	})
	t.Run("unknown environment", func(t *testing.T) {
		_, err := New(&Config{Environment: "test", Certificate: testCertificate(t), CABundle: bundle})
		assert.ErrorContains(t, err, "unknown CIS environment")
	})
	t.Run("no certificate", func(t *testing.T) {
		_, err := New(&Config{Environment: EnvironmentSandbox, CABundle: bundle})
		assert.ErrorContains(t, err, "certificate is required")
	})
	t.Run("no CA bundle", func(t *testing.T) {
		_, err := New(&Config{Environment: EnvironmentSandbox, Certificate: testCertificate(t)})
		assert.ErrorContains(t, err, "no PEM certificate")
	})
}

func TestReadRecordedAnswers(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		res, err := recordedClient(t).readInvoiceAnswer(200, readTestdata(t, "testdata/cistest/accepted-answer.xml"))
		require.NoError(t, err)
		assert.Equal(t, acceptedCassette(t).JIR, res.JIR)
		assert.Equal(t, acceptedCassette(t).MessageID, res.Header.MessageID)
		assert.Empty(t, res.Errors)
	})
	t.Run("refused", func(t *testing.T) {
		// CIS answers a refusal with HTTP 500 and a signed RacunOdgovor.
		res, err := recordedClient(t).readInvoiceAnswer(500, readTestdata(t, "testdata/cistest/refused-answer.xml"))
		require.ErrorIs(t, err, ErrCIS)
		assert.Equal(t, "s013", CodeOf(err))
		assert.Equal(t, "cis: s013: Račun sadrži restriktivne greške: 179", err.Error())
		require.NotNil(t, res)
		assert.Empty(t, res.JIR)
		require.Len(t, res.Errors, 1)
		assert.Equal(t, "s013", res.Errors[0].Code)
	})
}

func TestAnswerSignature(t *testing.T) {
	accepted := string(readTestdata(t, "testdata/cistest/accepted-answer.xml"))
	signatureErr := func(t *testing.T, c *Client, answer, msg string) {
		t.Helper()
		_, err := c.readInvoiceAnswer(200, []byte(answer))
		require.ErrorIs(t, err, ErrSignature)
		var e *Error
		require.True(t, errors.As(err, &e))
		assert.True(t, e.RequestWritten())
		assert.ErrorContains(t, err, msg)
	}

	t.Run("a changed JIR", func(t *testing.T) {
		answer := strings.Replace(accepted, acceptedCassette(t).JIR, "00000000-c7a1-407f-a1b3-14118c9d1b83", 1)
		signatureErr(t, recordedClient(t), answer, "")
	})
	t.Run("no signature", func(t *testing.T) {
		i, j := strings.Index(accepted, "<Signature "), strings.Index(accepted, "</Signature>")
		answer := accepted[:i] + accepted[j+len("</Signature>"):]
		signatureErr(t, recordedClient(t), answer, "not signed")
	})
	t.Run("the production signer expected", func(t *testing.T) {
		c := recordedClient(t)
		c.signer = answerSigners[EnvironmentLive]
		signatureErr(t, c, accepted, `signed by "fiskalcistest", not fiskalcis`)
	})
	t.Run("an expired certificate", func(t *testing.T) {
		c := recordedClient(t)
		c.now = func() time.Time { return time.Date(2028, 7, 7, 0, 0, 0, 0, time.UTC) }
		signatureErr(t, c, accepted, "not trusted")
	})
	t.Run("a CA bundle without the demo chain", func(t *testing.T) {
		c := recordedClient(t)
		c.roots = certPool(t, readTestdata(t, finaCABundle), "Fina Root CA")
		signatureErr(t, c, accepted, "not trusted")
	})
}

func TestReadAnswer(t *testing.T) {
	c := recordedClient(t)
	envelope := func(body string) []byte {
		return []byte(`<soap:Envelope xmlns:soap="` + NamespaceSOAP + `"><soap:Body>` + body + `</soap:Body></soap:Envelope>`)
	}
	tests := []struct {
		name    string
		status  int
		data    []byte
		err     *Error
		code    string
		written bool
		msg     string
	}{
		{
			name:   "client fault",
			status: 500,
			data:   envelope(`<soap:Fault><faultcode>soap:Client</faultcode><faultstring>bad request</faultstring></soap:Fault>`),
			err:    ErrCIS, code: "soap:Client", msg: "bad request",
		},
		{
			name:   "server fault",
			status: 500,
			data:   envelope(`<soap:Fault><faultcode>soap:Server.Busy</faultcode><faultstring>busy</faultstring></soap:Fault>`),
			err:    ErrServer, code: "soap:Server.Busy", written: true, msg: "busy",
		},
		{
			name:   "HTTP error without a SOAP answer",
			status: 502,
			data:   []byte("<html>Bad Gateway</html>"),
			err:    ErrConnection, code: "502", written: true, msg: "Bad Gateway",
		},
		{
			name:   "redirect",
			status: 307,
			err:    ErrConnection, code: "307", written: true, msg: "redirect",
		},
		{
			name:   "empty body",
			status: 200,
			data:   envelope(``),
			err:    ErrProtocol, written: true, msg: "0 messages",
		},
		{
			name:   "two messages",
			status: 200,
			data:   envelope(`<a/><b/>`),
			err:    ErrProtocol, written: true, msg: "2 messages",
		},
		{
			name:   "another message",
			status: 200,
			data:   envelope(`<EchoResponse xmlns="` + NamespaceF73 + `">hi</EchoResponse>`),
			err:    ErrProtocol, code: "200", written: true, msg: "unexpected answer message EchoResponse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.readInvoiceAnswer(tt.status, tt.data)
			require.ErrorIs(t, err, tt.err)
			var e *Error
			require.True(t, errors.As(err, &e))
			assert.Equal(t, tt.code, e.Code())
			assert.Equal(t, tt.written, e.RequestWritten())
			assert.Contains(t, e.Message(), tt.msg)
		})
	}
}

// signingCIS is a stand-in for CIS that signs answers the real CIS would
// never send. It returns a sandbox client that trusts only its certificate,
// and a function that signs an answer message and puts it in a SOAP
// envelope.
func signingCIS(t *testing.T) (*Client, func(msg string) []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: answerSigners[EnvironmentSandbox]},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	c := recordedClient(t)
	c.roots = x509.NewCertPool()
	c.roots.AddCert(cert)
	c.now = time.Now

	sign := func(msg string) []byte {
		t.Helper()
		doc := etree.NewDocument()
		require.NoError(t, doc.ReadFromString(msg))
		ctx, err := dsig.NewSigningContext(key, [][]byte{der})
		require.NoError(t, err)
		ctx.IdAttribute = "Id"
		ctx.Canonicalizer = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
		require.NoError(t, ctx.SetSignatureMethod(dsig.RSASHA256SignatureMethod))
		signed, err := ctx.SignEnveloped(doc.Root())
		require.NoError(t, err)
		out := etree.NewDocument()
		out.SetRoot(signed)
		data, err := out.WriteToBytes()
		require.NoError(t, err)
		return envelop(data)
	}
	return c, sign
}

// answer is a RacunOdgovor with the given content, ready for signingCIS.
func answer(content string) string {
	return `<RacunOdgovor xmlns="` + NamespaceF73 + `" Id="RacunOdgovor">` + content + `</RacunOdgovor>`
}

const answerHeader = `<Zaglavlje><IdPoruke>9c608e11-1b97-4425-a89d-3ad4bcbba2e9</IdPoruke>` +
	`<DatumVrijeme>10.11.2026T16:18:23</DatumVrijeme></Zaglavlje>`

func TestAnswerBreaksSpecification(t *testing.T) {
	c, sign := signingCIS(t)
	protocolErr := func(t *testing.T, status int, data []byte, msg string) {
		t.Helper()
		_, err := c.readInvoiceAnswer(status, data)
		require.ErrorIs(t, err, ErrProtocol)
		var e *Error
		require.True(t, errors.As(err, &e))
		assert.True(t, e.RequestWritten())
		assert.ErrorContains(t, err, msg)
	}

	t.Run("the stand-in's valid answer is accepted", func(t *testing.T) {
		res, err := c.readInvoiceAnswer(200, sign(answer(answerHeader+`<Jir>00000000-c7a1-407f-a1b3-14118c9d1b83</Jir>`)))
		require.NoError(t, err)
		assert.Equal(t, "00000000-c7a1-407f-a1b3-14118c9d1b83", res.JIR)
	})
	t.Run("HTTP 200 that is not SOAP", func(t *testing.T) {
		protocolErr(t, 200, []byte("<html>maintenance</html>"), "not a SOAP envelope")
	})
	t.Run("no header", func(t *testing.T) {
		protocolErr(t, 200, sign(answer(`<Jir>00000000-c7a1-407f-a1b3-14118c9d1b83</Jir>`)), "no message ID")
	})
	t.Run("a header with no message ID", func(t *testing.T) {
		header := `<Zaglavlje><IdPoruke></IdPoruke><DatumVrijeme>10.11.2026T16:18:23</DatumVrijeme></Zaglavlje>`
		protocolErr(t, 200, sign(answer(header+`<Jir>00000000-c7a1-407f-a1b3-14118c9d1b83</Jir>`)), "no message ID")
	})
	t.Run("a refusal with no header", func(t *testing.T) {
		refusal := `<Greske><Greska><SifraGreske>s005</SifraGreske><PorukaGreske>x</PorukaGreske></Greska></Greske>`
		protocolErr(t, 500, sign(answer(refusal)), "no message ID")
	})
	t.Run("neither a JIR nor errors", func(t *testing.T) {
		protocolErr(t, 200, sign(answer(answerHeader)), "neither a JIR nor errors")
	})
}

func TestSendInvoice(t *testing.T) {
	accepted := readTestdata(t, "testdata/cistest/accepted-answer.xml")

	t.Run("posts the envelope and reads the answer", func(t *testing.T) {
		var got *http.Request
		var body []byte
		c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
			got = r
			body, _ = io.ReadAll(r.Body)
			w.Write(accepted) //nolint:errcheck
		})
		req := testRequest()
		envelope, err := c.SignInvoice(req)
		require.NoError(t, err)
		res, err := c.SendInvoice(context.Background(), envelope)
		require.NoError(t, err)
		assert.Equal(t, acceptedCassette(t).JIR, res.JIR)
		assert.Equal(t, http.MethodPost, got.Method)
		assert.Equal(t, "text/xml; charset=utf-8", got.Header.Get("Content-Type"))
		assert.Equal(t, soapActionInvoice, got.Header.Get("SOAPAction"))
		assert.Equal(t, envelope, body)
	})

	t.Run("Fiscalize refuses an answer to another message", func(t *testing.T) {
		c := serverClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Write(accepted) //nolint:errcheck
		})
		res, err := c.Fiscalize(context.Background(), testRequest())
		require.ErrorIs(t, err, ErrSignature)
		assert.ErrorContains(t, err, "the answer is for message "+acceptedCassette(t).MessageID)
		assert.NotNil(t, res)
	})

	t.Run("Fiscalize returns the JIR for its own message", func(t *testing.T) {
		c := serverClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Write(accepted) //nolint:errcheck
		})
		req := testRequest()
		req.Header.MessageID = acceptedCassette(t).MessageID
		res, err := c.Fiscalize(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, acceptedCassette(t).JIR, res.JIR)
	})

	t.Run("does not follow a redirect", func(t *testing.T) {
		var calls int
		c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
		})
		_, err := c.SendInvoice(context.Background(), envelop([]byte("<x/>")))
		require.ErrorIs(t, err, ErrConnection)
		assert.Equal(t, 1, calls)
	})

	t.Run("nothing written when the connection fails", func(t *testing.T) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := l.Addr().String()
		require.NoError(t, l.Close())
		c, err := newClient(&Config{Certificate: testCertificate(t), CABundle: readTestdata(t, finaCABundle)}, "https://"+addr+"/")
		require.NoError(t, err)
		_, err = c.SendInvoice(context.Background(), envelop([]byte("<x/>")))
		require.ErrorIs(t, err, ErrConnection)
		var e *Error
		require.True(t, errors.As(err, &e))
		assert.False(t, e.RequestWritten())
		assert.NotContains(t, err.Error(), "https://")
	})

	t.Run("written when the answer times out", func(t *testing.T) {
		old := timeout
		timeout = 100 * time.Millisecond
		t.Cleanup(func() { timeout = old })
		done := make(chan struct{})
		t.Cleanup(func() { close(done) })
		c := serverClient(t, func(_ http.ResponseWriter, r *http.Request) {
			io.ReadAll(r.Body) //nolint:errcheck
			select {
			case <-done:
			case <-r.Context().Done():
			}
		})
		_, err := c.SendInvoice(context.Background(), envelop([]byte("<x/>")))
		require.ErrorIs(t, err, ErrConnection)
		var e *Error
		require.True(t, errors.As(err, &e))
		assert.True(t, e.RequestWritten())
	})
}

// serverClient starts a TLS server with handler and returns a sandbox
// client that calls it, trusting its certificate and the FINA CA bundle,
// with certificates checked when the accepted cassette was recorded.
func serverClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	bundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	bundle = append(bundle, readTestdata(t, finaCABundle)...)
	c, err := newClient(&Config{
		Environment: EnvironmentSandbox,
		Certificate: testCertificate(t),
		CABundle:    bundle,
	}, srv.URL)
	require.NoError(t, err)
	sentAt := acceptedCassette(t).SentAt
	c.now = func() time.Time { return sentAt }
	return c
}

// certPool is a pool of the certificates in a PEM bundle whose common name
// is one of names.
func certPool(t *testing.T, bundle []byte, names ...string) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	for rest := bundle; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		for _, n := range names {
			if cert.Subject.CommonName == n {
				pool.AddCert(cert)
			}
		}
	}
	return pool
}
