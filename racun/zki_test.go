package racun

import (
	"crypto"
	"crypto/md5" //nolint:gosec // the ZKI is defined with MD5
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/invopop/gobl/cal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testZKIInput() *ZKIInput {
	return &ZKIInput{
		IssuerOIB: "80432461483",
		IssuedAt:  cal.MakeDateTime(2026, 11, 6, 9, 5, 3),
		Number:    &Number{Sequential: "123", Premises: testPremises, Device: "2"},
		Total:     "1245.56",
	}
}

func TestZKIData(t *testing.T) {
	assert.Equal(t, "8043246148306.11.2026 09:05:03123POS121245.56", zkiData(testZKIInput()))
}

func TestZKI(t *testing.T) {
	cert := testCertificate(t)

	t.Run("is the MD5 of the RSA-SHA256 signature, in lowercase hex", func(t *testing.T) {
		zki, err := ZKI(cert.Key, testZKIInput())
		require.NoError(t, err)
		assert.Regexp(t, `^[0-9a-f]{32}$`, zki)

		digest := sha256.Sum256([]byte(zkiData(testZKIInput())))
		sig, err := rsa.SignPKCS1v15(nil, cert.Key.(*rsa.PrivateKey), crypto.SHA256, digest[:])
		require.NoError(t, err)
		sum := md5.Sum(sig) //nolint:gosec // see import
		assert.Equal(t, hex.EncodeToString(sum[:]), zki)
	})

	t.Run("is the same every time for the same invoice", func(t *testing.T) {
		a, err := ZKI(cert.Key, testZKIInput())
		require.NoError(t, err)
		b, err := ZKI(cert.Key, testZKIInput())
		require.NoError(t, err)
		assert.Equal(t, a, b)
	})

	t.Run("changes with the invoice data", func(t *testing.T) {
		a, err := ZKI(cert.Key, testZKIInput())
		require.NoError(t, err)
		in := testZKIInput()
		in.Total = "1245.57"
		b, err := ZKI(cert.Key, in)
		require.NoError(t, err)
		assert.NotEqual(t, a, b)
	})

	t.Run("changes with the key", func(t *testing.T) {
		a, err := ZKI(cert.Key, testZKIInput())
		require.NoError(t, err)
		b, err := ZKI(testCertificate(t).Key, testZKIInput())
		require.NoError(t, err)
		assert.NotEqual(t, a, b)
	})

	t.Run("needs a key and a number", func(t *testing.T) {
		_, err := ZKI(nil, testZKIInput())
		assert.EqualError(t, err, "no key")
		in := testZKIInput()
		in.Number = nil
		_, err = ZKI(cert.Key, in)
		assert.EqualError(t, err, "no invoice number")
	})
}
