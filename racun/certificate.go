package racun

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

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

// check returns an error unless the certificate can sign at now: it has a
// certificate and a key, the key is RSA and belongs to the certificate, and
// now is within the certificate's validity dates. CIS would refuse a request
// signed otherwise (s002, s004), but only after the invoice was issued.
func (c *Certificate) check(now time.Time) error {
	if c == nil || c.Cert == nil || c.Key == nil {
		return errors.New("no certificate")
	}
	pub, ok := c.Key.Public().(*rsa.PublicKey)
	if !ok {
		return errors.New("the private key is not RSA")
	}
	if !pub.Equal(c.Cert.PublicKey) {
		return errors.New("the private key does not belong to the certificate")
	}
	if now.Before(c.Cert.NotBefore) {
		return fmt.Errorf("the certificate is not valid until %s", c.Cert.NotBefore.Format(time.RFC3339))
	}
	if now.After(c.Cert.NotAfter) {
		return fmt.Errorf("the certificate expired at %s", c.Cert.NotAfter.Format(time.RFC3339))
	}
	return nil
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
