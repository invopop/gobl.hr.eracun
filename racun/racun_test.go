package racun

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testCertificate makes a self-signed RSA certificate shaped like a FINA
// fiscalization certificate.
func testCertificate(t *testing.T) *Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "FISKAL TEST",
			Organization: []string{"INVOPOP"},
			Country:      []string{"HR"},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &Certificate{Cert: cert, Key: key}
}

var zagreb = mustLocation("Europe/Zagreb")

// testBase is the taxable base of every tax in the test receipts.
const (
	testBase     = "10.00"
	testTotal    = "12.50"
	testPremises = "POS1"
)

// testRequest is a cash receipt of €12.50 with 25 % VAT, issued by
// Invopop's demo OIB.
func testRequest() *ReceiptRequest {
	issued := time.Date(2026, 11, 6, 13, 58, 10, 0, zagreb)
	return &ReceiptRequest{
		ID: ReceiptRequestID,
		Header: &Header{
			MessageID: "24d544c5-859a-47e2-85dc-0e1ce881b819",
			SentAt:    issued.Format(DateTimeFormat),
		},
		Receipt: &Receipt{
			IssuerOIB:     "80432461483",
			InVATSystem:   true,
			IssuedAt:      issued.Format(DateTimeFormat),
			SequenceScope: SequencePremises,
			Number:        &Number{Sequential: "1", Premises: testPremises, Device: "1"},
			VAT:           []*Tax{{Rate: "25.00", Base: testBase, Amount: "2.50"}},
			Total:         testTotal,
			PaymentMethod: PaymentCash,
			OperatorOIB:   "80432461483",
			ZKI:           "8b55b3586e66d00b33266e44a774a408",
		},
	}
}

// fullRequest is testRequest with every optional element filled.
func fullRequest() *ReceiptRequest {
	req := testRequest()
	r := req.Receipt
	r.ConsumptionTax = []*Tax{{Rate: "3.00", Base: testBase, Amount: "0.30"}}
	r.OtherTaxes = []*OtherTax{{Name: "Porez na luksuz", Rate: "15.00", Base: testBase, Amount: "1.50"}}
	r.VATExemptAmount = "1.00"
	r.MarginAmount = "2.00"
	r.NotSubjectAmount = "3.00"
	r.Fees = []*Fee{{Name: "Povratna naknada", Amount: "0.10"}}
	r.LateDelivery = true
	r.HandwrittenNumber = "123/POS1/1"
	r.SpecialPurpose = "x"
	r.BuyerOIB = "12345678903"
	return req
}
