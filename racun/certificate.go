package racun

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"software.sslmate.com/src/go-pkcs12"
)

// Certificate is the issuer's fiscalization certificate and its private key.
// It signs requests and computes the ZKI.
type Certificate struct {
	Cert *x509.Certificate
	Key  crypto.Signer
	// Chain holds the intermediate certificates, if any.
	Chain []*x509.Certificate
}

// LoadCertificate reads a certificate and its private key from a PKCS#12
// file (.p12, .pfx) opened with the password.
func LoadCertificate(path, password string) (*Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseCertificate(data, password)
}

// ParseCertificate reads a certificate and its private key from PKCS#12
// data opened with the password. The key must be RSA, which is what the
// specification allows (§7).
func ParseCertificate(data []byte, password string) (*Certificate, error) {
	key, cert, chain, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return nil, fmt.Errorf("decoding PKCS#12: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("the private key is not RSA")
	}
	return &Certificate{Cert: cert, Key: rsaKey, Chain: chain}, nil
}
