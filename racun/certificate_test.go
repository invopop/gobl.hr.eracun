package racun

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCertificateCheck(t *testing.T) {
	cert := testCertificate(t)
	now := time.Now()

	t.Run("a valid certificate", func(t *testing.T) {
		assert.NoError(t, cert.check(now))
	})
	t.Run("no certificate", func(t *testing.T) {
		var nilCert *Certificate
		assert.EqualError(t, nilCert.check(now), "no certificate")
		assert.EqualError(t, (&Certificate{Key: cert.Key}).check(now), "no certificate")
		assert.EqualError(t, (&Certificate{Cert: cert.Cert}).check(now), "no certificate")
	})
	t.Run("a key that is not RSA", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		assert.EqualError(t, (&Certificate{Cert: cert.Cert, Key: key}).check(now), "the private key is not RSA")
	})
	t.Run("a key from another certificate", func(t *testing.T) {
		other := &Certificate{Cert: cert.Cert, Key: testCertificate(t).Key}
		assert.EqualError(t, other.check(now), "the private key does not belong to the certificate")
	})
	t.Run("not valid yet", func(t *testing.T) {
		assert.ErrorContains(t, cert.check(cert.Cert.NotBefore.Add(-time.Second)), "the certificate is not valid until")
	})
	t.Run("expired", func(t *testing.T) {
		assert.ErrorContains(t, cert.check(cert.Cert.NotAfter.Add(time.Second)), "the certificate expired at")
	})
}

// New, Sign and ZKI refuse a certificate or key they can't use, before
// anything reaches CIS.
func TestCertificateUse(t *testing.T) {
	expired := testCertificate(t)
	expired.Cert.NotAfter = time.Now().Add(-time.Minute)

	t.Run("New refuses an expired certificate", func(t *testing.T) {
		_, err := New(&Config{Environment: EnvironmentSandbox, Certificate: expired})
		assert.ErrorContains(t, err, "signing certificate: the certificate expired at")
	})
	t.Run("Sign refuses an expired certificate", func(t *testing.T) {
		_, err := Sign(expired, testRequest())
		assert.ErrorContains(t, err, "the certificate expired at")
	})
	t.Run("ZKI refuses a key that is not RSA", func(t *testing.T) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		_, err = ZKI(key, testZKIInput())
		assert.EqualError(t, err, "the key is not RSA")
	})
	t.Run("ZKI takes any RSA signer", func(t *testing.T) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		_, err = ZKI(signer{key}, testZKIInput())
		assert.NoError(t, err)
	})
}

// signer is an RSA key behind the crypto.Signer interface alone, as a key
// in a KMS or an HSM is.
type signer struct{ *rsa.PrivateKey }
