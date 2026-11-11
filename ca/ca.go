// Package ca holds the FINA CA certificates that clients of the Croatian Tax
// Administration's fiscalization service (CIS) trust.
//
// FINA (Financijska agencija) issues the TLS certificates of both CIS
// servers and the certificates CIS signs its answers with. Its CAs are not
// in the system certificate store, and the CIS servers send their TLS
// certificate without its intermediate, so a client needs FINA's chain
// itself. The chains are public certificates, embedded here so that every
// client trusts the same ones:
//
//   - fina-demo.pem: Fina Demo CA 2020 and Fina Demo Root CA, for CIS test.
//   - fina.pem: Fina RDC 2020 and Fina Root CA, for production CIS.
//
// A client trusts one chain only, the one of its environment. FINA issues
// demo certificates to any developer, so a production client that also
// trusted the demo chain would accept a demo certificate.
//
// The intermediates expire in 2030 and the roots in 2034 and 2035, but CIS
// renews its certificates every year and can move to a newer FINA
// intermediate before then. The Tax Administration announces such changes
// on porezna-uprava.gov.hr. testdata/chains holds the TLS certificates the
// CIS servers sent when they were last checked; TestChains says how to
// capture them again.
package ca

import (
	"crypto/x509"
	_ "embed"
)

//go:embed fina-demo.pem
var demo []byte

//go:embed fina.pem
var production []byte

// Demo returns a new pool with FINA's demo chain, for CIS test.
func Demo() *x509.CertPool {
	return pool(demo)
}

// Production returns a new pool with FINA's production chain, for
// production CIS.
func Production() *x509.CertPool {
	return pool(production)
}

func pool(data []byte) *x509.CertPool {
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(data) {
		// The files are embedded and checked by the tests.
		panic("ca: an embedded chain holds no PEM certificate")
	}
	return p
}
