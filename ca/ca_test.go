package ca

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedAt is when testdata/chains was captured. The servers' certificates
// last a year, so they are checked at that time.
var capturedAt = time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)

// TestChains checks that each environment's pool trusts the TLS certificate
// its CIS server sends, and that the other environment's pool does not.
// Each server sends its certificate alone, so only the embedded chain can
// complete it. The same certificate serves every port of a host.
//
// To capture the chains again, after CIS renews its certificates, run for
// each host:
//
//	openssl s_client -connect <host>:8449 -servername <host> -showcerts </dev/null \
//	  | awk '/BEGIN CERT/,/END CERT/' > testdata/chains/<host>.pem
//
// and set capturedAt to the time of the capture.
func TestChains(t *testing.T) {
	tests := []struct {
		host        string
		trusted     *x509.CertPool
		untrusted   *x509.CertPool
		environment string
	}{
		{"cistest.apis-it.hr", Demo(), Production(), "test"},
		{"cis.porezna-uprava.hr", Production(), Demo(), "production"},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			certs := loadCerts(t, filepath.Join("testdata", "chains", tt.host+".pem"))
			leaf := certs[0]
			intermediates := x509.NewCertPool()
			for _, c := range certs[1:] {
				intermediates.AddCert(c)
			}
			opts := x509.VerifyOptions{
				DNSName:       tt.host,
				Intermediates: intermediates,
				CurrentTime:   capturedAt,
			}

			opts.Roots = tt.trusted
			_, err := leaf.Verify(opts)
			assert.NoError(t, err, "the %s chain does not trust %s", tt.environment, tt.host)

			opts.Roots = tt.untrusted
			_, err = leaf.Verify(opts)
			assert.Error(t, err, "the other environment's chain trusts %s", tt.host)
		})
	}
}

// TestContents checks that each embedded file holds the CAs it names, and
// no leaf certificate.
func TestContents(t *testing.T) {
	tests := map[string][]string{
		"fina-demo.pem": {"Fina Demo CA 2020", "Fina Demo Root CA"},
		"fina.pem":      {"Fina RDC 2020", "Fina Root CA"},
	}
	for file, want := range tests {
		t.Run(file, func(t *testing.T) {
			var names []string
			for _, c := range loadCerts(t, file) {
				names = append(names, c.Subject.CommonName)
				assert.True(t, c.IsCA, "%s is not a CA", c.Subject.CommonName)
				assert.Equal(t, "Financijska agencija", c.Subject.Organization[0])
			}
			assert.ElementsMatch(t, want, names)
		})
	}
}

// loadCerts parses every PEM certificate in a file, in order.
func loadCerts(t *testing.T, path string) []*x509.Certificate {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		c, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		certs = append(certs, c)
	}
	require.NotEmpty(t, certs, "no certificate in %s", path)
	return certs
}
