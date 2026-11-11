package racun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every call to CIS in the tests is a cassette: CIS test's real, signed
// answer to one scenario, kept in testdata/cistest. A normal run replays the
// cassettes through the whole client, offline. With -record, the scenarios
// go to CIS test with the demo certificate and their answers replace the
// cassettes.
var record = flag.Bool("record", false, "send the scenarios to CIS test and save the answers as cassettes in "+
	cisTestDir+"; needs "+envCertificate+" and "+envPassword+" (see README)")

// The environment -record reads the demo certificate from. Neither the
// certificate nor its password is ever in the repository.
const (
	// envCertificate is the path of the PKCS#12 demo certificate.
	envCertificate = "CIS_TEST_CERT"
	// envPassword is the certificate's password.
	envPassword = "CIS_TEST_PASSWORD"
)

// cisTestDir is where the cassettes live.
const cisTestDir = "testdata/cistest"

// cassette is what a replay needs besides CIS's answer, which is kept next
// to it in <name>-answer.xml.
type cassette struct {
	// MessageID is the request's message ID. The answer repeats it, and
	// Fiscalize refuses an answer to another message.
	MessageID string `json:"message_id"`
	// SentAt is when the request was sent. A replay rebuilds the request
	// for this time and checks CIS's certificate at it.
	SentAt time.Time `json:"sent_at"`
	// Status is the HTTP status CIS answered with.
	Status int `json:"status"`
	// JIR is the invoice's JIR when CIS accepted it.
	JIR string `json:"jir,omitempty"`
	// ErrorCode is CIS's error code when it refused the invoice.
	ErrorCode string `json:"error_code,omitempty"`
}

// readCassette reads a cassette and its answer.
func readCassette(t *testing.T, name string) (*cassette, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cisTestDir, name+".json"))
	require.NoError(t, err, "no cassette %q: record it with -record (see README)", name)
	c := new(cassette)
	require.NoError(t, json.Unmarshal(data, c))
	return c, readTestdata(t, filepath.Join(cisTestDir, name+"-answer.xml"))
}

func writeCassette(t *testing.T, name string, c *cassette, answer []byte) {
	t.Helper()
	data, err := json.MarshalIndent(c, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(cisTestDir, 0o755))                                                   //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Join(cisTestDir, name+".json"), append(data, '\n'), 0o644)) //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Join(cisTestDir, name+"-answer.xml"), answer, 0o644))       //nolint:gosec
}

// acceptedCassette is the cassette of the accepted scenario, whose answer the
// offline tests take apart.
func acceptedCassette(t *testing.T) *cassette {
	t.Helper()
	c, _ := readCassette(t, "accepted")
	return c
}

// buildRequest builds a scenario's request for the time it is sent at and
// its message ID, signing the ZKI with cert's key.
type buildRequest func(t *testing.T, cert *Certificate, sentAt time.Time, messageID string) *InvoiceRequest

// fiscalizeCassette fiscalizes a scenario's request through the whole
// client and checks the outcome. It replays the cassette, or with -record
// sends the request to CIS test and saves the answer as the cassette.
func fiscalizeCassette(t *testing.T, name string, wantAccepted bool, build buildRequest) {
	t.Helper()
	if *record {
		recordCassette(t, name, wantAccepted, build)
		return
	}
	c, answer := readCassette(t, name)
	client := recordedClient(t)
	client.now = func() time.Time { return c.SentAt }
	client.conn.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != client.URL() || r.Header.Get("SOAPAction") != soapActionInvoice {
			return nil, fmt.Errorf("no cassette for %s %s (SOAPAction %q)", r.Method, r.URL, r.Header.Get("SOAPAction"))
		}
		return &http.Response{
			StatusCode: c.Status,
			Header:     http.Header{"Content-Type": {"text/xml; charset=utf-8"}},
			Body:       io.NopCloser(bytes.NewReader(answer)),
			Request:    r,
		}, nil
	})

	res, err := client.Fiscalize(context.Background(), build(t, client.cert, c.SentAt, c.MessageID))
	if c.JIR != "" {
		require.NoError(t, err)
		assert.Equal(t, c.JIR, res.JIR)
		return
	}
	require.ErrorIs(t, err, ErrCIS)
	assert.Equal(t, c.ErrorCode, CodeOf(err))
}

func recordCassette(t *testing.T, name string, wantAccepted bool, build buildRequest) {
	t.Helper()
	path, password := os.Getenv(envCertificate), os.Getenv(envPassword)
	if path == "" || password == "" {
		t.Fatalf("-record needs %s (path of the PKCS#12 demo certificate) and %s (its password)", envCertificate, envPassword)
	}
	cert, err := LoadCertificate(path, password)
	require.NoError(t, err)
	client, err := New(&Config{Environment: EnvironmentSandbox, Certificate: cert})
	require.NoError(t, err)
	rec := &recorder{next: client.conn.Transport}
	client.conn.Transport = rec

	c := &cassette{MessageID: uuid.NewString(), SentAt: time.Now().UTC().Truncate(time.Second)}
	res, err := client.Fiscalize(context.Background(), build(t, cert, c.SentAt, c.MessageID))
	switch {
	case err == nil && wantAccepted:
		c.JIR = res.JIR
	case errors.Is(err, ErrCIS) && !wantAccepted:
		c.ErrorCode = CodeOf(err)
	default:
		t.Fatalf("unexpected outcome, nothing recorded: %v", err)
	}
	c.Status = rec.status
	writeCassette(t, name, c, rec.body)
	t.Logf("recorded %s: HTTP %d, JIR %q, error %q", name, c.Status, c.JIR, c.ErrorCode)
}

// recorder keeps the last answer that passes through it.
type recorder struct {
	next   http.RoundTripper
	status int
	body   []byte
}

func (r *recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	res, err := r.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close() //nolint:errcheck
	if err != nil {
		return nil, err
	}
	r.status, r.body = res.StatusCode, body
	res.Body = io.NopCloser(bytes.NewReader(body))
	return res, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
