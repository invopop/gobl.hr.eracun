package racun

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The answers -cistest recorded on 2026-11-06, signed by fiskalcistest.
const (
	acceptedJIR       = "fdd9fc9a-c7a1-407f-a1b3-14118c9d1b83"
	acceptedMessageID = "07474d19-f658-4cc4-af06-c308ab67c400"
)

// recordedAt is when the answers were recorded: the time their
// certificate is checked at.
var recordedAt = time.Date(2026, 11, 6, 12, 37, 0, 0, time.UTC)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	return data
}

// recordedClient is a sandbox client that trusts the FINA CA bundle and
// checks certificates at recordedAt.
func recordedClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(&Config{
		Environment: EnvironmentSandbox,
		Certificate: testCertificate(t),
		CABundle:    readTestdata(t, finaCABundle),
	})
	require.NoError(t, err)
	c.now = func() time.Time { return recordedAt }
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
		res, err := recordedClient(t).readReceiptAnswer(200, readTestdata(t, "testdata/cistest/accepted-answer.xml"))
		require.NoError(t, err)
		assert.Equal(t, acceptedJIR, res.JIR)
		assert.Equal(t, acceptedMessageID, res.Header.MessageID)
		assert.Empty(t, res.Errors)
	})
	t.Run("refused", func(t *testing.T) {
		// CIS answers a refusal with HTTP 500 and a signed RacunOdgovor.
		res, err := recordedClient(t).readReceiptAnswer(500, readTestdata(t, "testdata/cistest/refused-answer.xml"))
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
		_, err := c.readReceiptAnswer(200, []byte(answer))
		require.ErrorIs(t, err, ErrSignature)
		var e *Error
		require.True(t, errors.As(err, &e))
		assert.True(t, e.RequestWritten())
		assert.ErrorContains(t, err, msg)
	}

	t.Run("a changed JIR", func(t *testing.T) {
		answer := strings.Replace(accepted, acceptedJIR, "00000000-c7a1-407f-a1b3-14118c9d1b83", 1)
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
			err:    ErrConnection, written: true, msg: "0 messages",
		},
		{
			name:   "two messages",
			status: 200,
			data:   envelope(`<a/><b/>`),
			err:    ErrConnection, written: true, msg: "2 messages",
		},
		{
			name:   "another message",
			status: 200,
			data:   envelope(`<EchoResponse xmlns="` + NamespaceF73 + `">hi</EchoResponse>`),
			err:    ErrConnection, code: "200", written: true, msg: "unexpected answer message EchoResponse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.readReceiptAnswer(tt.status, tt.data)
			require.ErrorIs(t, err, tt.err)
			var e *Error
			require.True(t, errors.As(err, &e))
			assert.Equal(t, tt.code, e.Code())
			assert.Equal(t, tt.written, e.RequestWritten())
			assert.Contains(t, e.Message(), tt.msg)
		})
	}
}

func TestSendReceipt(t *testing.T) {
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
		envelope, err := c.SignReceipt(req)
		require.NoError(t, err)
		res, err := c.SendReceipt(context.Background(), envelope)
		require.NoError(t, err)
		assert.Equal(t, acceptedJIR, res.JIR)
		assert.Equal(t, http.MethodPost, got.Method)
		assert.Equal(t, "text/xml; charset=utf-8", got.Header.Get("Content-Type"))
		assert.Equal(t, soapActionReceipt, got.Header.Get("SOAPAction"))
		assert.Equal(t, envelope, body)
	})

	t.Run("Fiscalize refuses an answer to another message", func(t *testing.T) {
		c := serverClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Write(accepted) //nolint:errcheck
		})
		res, err := c.Fiscalize(context.Background(), testRequest())
		require.ErrorIs(t, err, ErrSignature)
		assert.ErrorContains(t, err, "the answer is for message "+acceptedMessageID)
		assert.NotNil(t, res)
	})

	t.Run("Fiscalize returns the JIR for its own message", func(t *testing.T) {
		c := serverClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Write(accepted) //nolint:errcheck
		})
		req := testRequest()
		req.Header.MessageID = acceptedMessageID
		res, err := c.Fiscalize(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, acceptedJIR, res.JIR)
	})

	t.Run("does not follow a redirect", func(t *testing.T) {
		var calls int
		c := serverClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
		})
		_, err := c.SendReceipt(context.Background(), envelop([]byte("<x/>")))
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
		_, err = c.SendReceipt(context.Background(), envelop([]byte("<x/>")))
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
		_, err := c.SendReceipt(context.Background(), envelop([]byte("<x/>")))
		require.ErrorIs(t, err, ErrConnection)
		var e *Error
		require.True(t, errors.As(err, &e))
		assert.True(t, e.RequestWritten())
	})
}

// serverClient starts a TLS server with handler and returns a sandbox
// client that calls it, trusting its certificate and the FINA CA bundle,
// with certificates checked at recordedAt.
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
	c.now = func() time.Time { return recordedAt }
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
