package racun

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"flag"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/pay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "write the expected invoices in testdata/out")

const (
	examplesDir = "testdata/examples"
	outDir      = "testdata/out"
	// fiveEuros is an amount several examples share.
	fiveEuros = "5.00"
)

// testOptions are the options of every example: premises POS1, device 1,
// Invopop's OIB as the operator.
func testOptions() *Options {
	return &Options{Premises: "POS1", Device: "1", OperatorOIB: invopopOIB}
}

// testKey is a fixed RSA key, used only by tests, so that the ZKI of an
// example does not change between runs.
func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	data, err := os.ReadFile("testdata/test-key.pem")
	require.NoError(t, err)
	block, _ := pem.Decode(data)
	require.NotNil(t, block)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)
	return key
}

// loadExample reads and calculates the example invoice in name.
func loadExample(t *testing.T, name string) *gobl.Envelope {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(examplesDir, name))
	require.NoError(t, err)
	inv := new(bill.Invoice)
	require.NoError(t, json.Unmarshal(data, inv))
	env, err := gobl.Envelop(inv)
	require.NoError(t, err)
	return env
}

// requestFor wraps an invoice in a request with a fixed header.
func requestFor(r *Invoice) *InvoiceRequest {
	return &InvoiceRequest{
		ID: InvoiceRequestID,
		Header: &Header{
			MessageID: "00000000-0000-4000-8000-000000000000",
			SentAt:    r.IssuedAt,
		},
		Invoice: r,
	}
}

func examples(t *testing.T) []string {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(examplesDir, "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, names)
	for i, n := range names {
		names[i] = filepath.Base(n)
	}
	return names
}

func outName(example string) string {
	return filepath.Join(outDir, strings.TrimSuffix(example, ".json")+".xml")
}

// TestExamples converts each example invoice and compares the request with
// the one in testdata/out. Run with -update to write them again.
func TestExamples(t *testing.T) {
	key := testKey(t)
	for _, name := range examples(t) {
		t.Run(name, func(t *testing.T) {
			r, err := NewInvoice(loadExample(t, name), testOptions(), key)
			require.NoError(t, err)
			data, err := xml.MarshalIndent(requestFor(r), "", "\t")
			require.NoError(t, err)
			data = append(data, '\n')
			if *update {
				require.NoError(t, os.MkdirAll(outDir, 0o755))
				require.NoError(t, os.WriteFile(outName(name), data, 0o644)) //nolint:gosec
				return
			}
			want, err := os.ReadFile(outName(name))
			require.NoError(t, err, "run with -update to write it")
			assert.Equal(t, string(want), string(data))
		})
	}
}

func TestExampleValues(t *testing.T) {
	key := testKey(t)
	t.Run("prices with VAT included, paid in cash", func(t *testing.T) {
		r, err := NewInvoice(loadExample(t, "cash-prices-include-vat.json"), testOptions(), key)
		require.NoError(t, err)
		assert.Equal(t, invopopOIB, r.IssuerOIB)
		assert.True(t, r.InVATSystem)
		assert.Equal(t, "06.11.2026T13:58:10", r.IssuedAt)
		assert.Equal(t, SequencePremises, r.SequenceScope)
		assert.Equal(t, &Number{Sequential: "1001", Premises: "POS1", Device: "1"}, r.Number)
		assert.Equal(t, []*Tax{
			{Rate: "25.00", Base: "4.00", Amount: "1.00"},
			{Rate: "13.00", Base: fiveEuros, Amount: "0.65"},
		}, r.VAT)
		assert.Equal(t, "10.65", r.Total)
		assert.Equal(t, PaymentCash, r.PaymentMethod)
		assert.Equal(t, invopopOIB, r.OperatorOIB)
		assert.Len(t, r.ZKI, 32)
		assert.Empty(t, r.BuyerOIB)
	})
	t.Run("an exempt line, paid by card and in cash", func(t *testing.T) {
		r, err := NewInvoice(loadExample(t, "exempt-card-and-cash.json"), testOptions(), key)
		require.NoError(t, err)
		assert.Equal(t, []*Tax{{Rate: "25.00", Base: "10.00", Amount: "2.50"}}, r.VAT)
		assert.Equal(t, "20.00", r.VATExemptAmount)
		assert.Equal(t, "32.50", r.Total)
		assert.Equal(t, PaymentOther, r.PaymentMethod)
	})
	t.Run("a Croatian business buyer, paid by card", func(t *testing.T) {
		r, err := NewInvoice(loadExample(t, "business-buyer-card.json"), testOptions(), key)
		require.NoError(t, err)
		assert.Equal(t, []*Tax{{Rate: "5.00", Base: "100.00", Amount: fiveEuros}}, r.VAT)
		assert.Equal(t, "105.00", r.Total)
		assert.Equal(t, PaymentCard, r.PaymentMethod)
		assert.Equal(t, "12345678903", r.BuyerOIB)
	})
}

func TestZKIMatchesInvoice(t *testing.T) {
	key := testKey(t)
	r, err := NewInvoice(loadExample(t, "cash-prices-include-vat.json"), testOptions(), key)
	require.NoError(t, err)
	zki, err := ZKI(key, &ZKIInput{
		IssuerOIB: invopopOIB,
		IssuedAt:  time.Date(2026, 11, 6, 13, 58, 10, 0, location),
		Number:    r.Number,
		Total:     "10.65",
	})
	require.NoError(t, err)
	assert.Equal(t, zki, r.ZKI)
}

func TestOptions(t *testing.T) {
	key := testKey(t)
	env := loadExample(t, "cash-prices-include-vat.json")
	opts := testOptions()
	opts.SequenceScope = SequenceDevice
	opts.NotInVATSystem = true
	opts.LateDelivery = true
	opts.HandwrittenNumber = "123/458/5"
	r, err := NewInvoice(env, opts, key)
	require.NoError(t, err)
	assert.Equal(t, SequenceDevice, r.SequenceScope)
	assert.False(t, r.InVATSystem)
	assert.True(t, r.LateDelivery)
	assert.Equal(t, "123/458/5", r.HandwrittenNumber)
}

func TestErrors(t *testing.T) {
	key := testKey(t)
	tests := []struct {
		name string
		inv  func(*bill.Invoice)
		opts func(*Options)
		err  error
		msg  string
	}{
		{name: "no issue time", inv: func(i *bill.Invoice) { i.IssueTime = nil }, err: ErrMissing, msg: "issue time"},
		{name: "a code with a series", inv: func(i *bill.Invoice) { i.Code = "R-1001" }, err: ErrInvalid, msg: "invoice code"},
		{name: "a code with a leading zero", inv: func(i *bill.Invoice) { i.Code = "01001" }, err: ErrInvalid, msg: "invoice code"},
		{name: "a foreign supplier", inv: func(i *bill.Invoice) { i.Supplier.TaxID.Country = "SI" }, err: ErrInvalid, msg: "the issuer must be Croatian"},
		{name: "a credit note", inv: func(i *bill.Invoice) { i.Type = bill.InvoiceTypeCreditNote }, err: ErrUnsupported, msg: "credit-note"},
		{name: "another currency", inv: func(i *bill.Invoice) { i.Currency = "USD" }, err: ErrInvalid, msg: "euros"},
		{name: "not calculated", inv: func(i *bill.Invoice) { i.Totals = nil }, err: ErrMissing, msg: "calculate"},
		{name: "no premises", opts: func(o *Options) { o.Premises = "" }, err: ErrMissing, msg: "premises"},
		{name: "a premises label with a dash", opts: func(o *Options) { o.Premises = "POS-1" }, err: ErrInvalid, msg: "premises"},
		{name: "no device", opts: func(o *Options) { o.Device = "" }, err: ErrMissing, msg: "device"},
		{name: "a device label with letters", opts: func(o *Options) { o.Device = "T1" }, err: ErrInvalid, msg: "device"},
		{name: "no operator", opts: func(o *Options) { o.OperatorOIB = "" }, err: ErrMissing, msg: "operator"},
		{name: "a short operator OIB", opts: func(o *Options) { o.OperatorOIB = "1234" }, err: ErrInvalid, msg: "operator"},
		{name: "an unknown sequence scope", opts: func(o *Options) { o.SequenceScope = "X" }, err: ErrInvalid, msg: "sequence scope"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := loadExample(t, "cash-prices-include-vat.json")
			inv := env.Extract().(*bill.Invoice)
			opts := testOptions()
			if tt.inv != nil {
				tt.inv(inv)
			}
			if tt.opts != nil {
				tt.opts(opts)
			}
			_, err := FromInvoice(inv, opts, key)
			require.ErrorIs(t, err, tt.err)
			assert.ErrorContains(t, err, tt.msg)
		})
	}

	t.Run("not an invoice", func(t *testing.T) {
		_, err := NewInvoice(nil, testOptions(), key)
		assert.ErrorIs(t, err, ErrNotInvoice)
	})
	t.Run("a reverse charge", func(t *testing.T) {
		env := loadExample(t, "business-buyer-card.json")
		inv := env.Extract().(*bill.Invoice)
		inv.Totals.Taxes.Categories[0].Rates[0].Key = "reverse-charge"
		_, err := FromInvoice(inv, testOptions(), key)
		assert.ErrorIs(t, err, ErrUnsupported)
	})
	t.Run("an issue time ZKI needs", func(t *testing.T) {
		env := loadExample(t, "cash-prices-include-vat.json")
		inv := env.Extract().(*bill.Invoice)
		inv.IssueTime = cal.NewTime(0, 0, 0)
		_, err := FromInvoice(inv, testOptions(), key)
		assert.ErrorIs(t, err, ErrMissing)
	})
}

func TestPaymentMethod(t *testing.T) {
	advances := func(keys ...cbc.Key) *bill.PaymentDetails {
		p := new(bill.PaymentDetails)
		for _, k := range keys {
			p.Advances = append(p.Advances, &pay.Record{Key: k})
		}
		return p
	}
	tests := []struct {
		name string
		p    *bill.PaymentDetails
		want PaymentMethod
	}{
		{"no payment", nil, PaymentOther},
		{"cash", advances("cash"), PaymentCash},
		{"card", advances("card"), PaymentCard},
		{"debit card", advances("card+debit"), PaymentCard},
		{"two card payments", advances("card", "card"), PaymentCard},
		{"credit transfer", advances("credit-transfer+sepa"), PaymentTransfer},
		{"direct debit", advances("direct-debit"), PaymentTransfer},
		{"online", advances("online"), PaymentOther},
		{"card and cash", advances("card", "cash"), PaymentOther},
		{"card instructions", &bill.PaymentDetails{Instructions: &pay.Instructions{Key: "card"}}, PaymentCard},
		{"advances win over instructions", &bill.PaymentDetails{
			Instructions: &pay.Instructions{Key: "credit-transfer"},
			Advances:     []*pay.Record{{Key: "cash"}},
		}, PaymentCash},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, paymentMethod(tt.p))
		})
	}
}

// TestExamplesAgainstSchema signs the expected requests in testdata/out and
// checks them with xmllint against FiskalizacijaSchema.xsd. It needs Docker
// and runs with -xsd only.
func TestExamplesAgainstSchema(t *testing.T) {
	if !*validateXSD {
		t.Skip("run with -xsd to validate against the schema (needs Docker)")
	}
	dir := t.TempDir()
	for _, name := range []string{"FiskalizacijaSchema.xsd", "xmldsig-core-schema.xsd"} {
		data, err := os.ReadFile(filepath.Join("testdata", "schema", name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644)) //nolint:gosec
	}
	cert := fixedCertificate(t)
	var files []string
	for _, name := range examples(t) {
		r, err := NewInvoice(loadExample(t, name), testOptions(), cert.Key)
		require.NoError(t, err)
		signed, err := Sign(cert, requestFor(r))
		require.NoError(t, err)
		file := strings.TrimSuffix(name, ".json") + ".xml"
		require.NoError(t, os.WriteFile(filepath.Join(dir, file), signed, 0o644)) //nolint:gosec
		files = append(files, "/w/"+file)
	}
	script := "apk add -q libxml2-utils && xmllint --noout --nonet --schema /w/FiskalizacijaSchema.xsd " + strings.Join(files, " ")
	out, err := exec.Command("docker", "run", "--rm", "-v", dir+":/w:ro", "alpine:3.20", "sh", "-c", script).CombinedOutput() //nolint:gosec
	t.Logf("xmllint:\n%s", out)
	require.NoError(t, err)
}

// fixedCertificate is a self-signed certificate for testKey.
func fixedCertificate(t *testing.T) *Certificate {
	t.Helper()
	key := testKey(t)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "FISKAL TEST", Country: []string{"HR"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return &Certificate{Cert: cert, Key: key}
}
