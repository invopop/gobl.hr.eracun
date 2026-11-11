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
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/pay"
	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	"github.com/invopop/gobl.hr.eracun/addon/fiskal"
)

var update = flag.Bool("update", false, "write the expected invoices in testdata/out")

const (
	examplesDir = "testdata/examples"
	outDir      = "testdata/out"
	// fiveEuros is an amount several examples share.
	fiveEuros = "5.00"
)

// testOptions are the options of every example: sent on time. The examples
// carry the premises POS1, the device 1 and Invopop's OIB as the operator.
func testOptions() *Options {
	return new(Options)
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

// withCharges is the cash-prices-include-vat example with document-level
// charges and discounts added, calculated again.
func withCharges(t *testing.T, charges []*bill.Charge, discounts []*bill.Discount) *bill.Invoice {
	t.Helper()
	inv := loadExample(t, "cash-prices-include-vat.json").Extract().(*bill.Invoice)
	inv.Charges = charges
	inv.Discounts = discounts
	require.NoError(t, inv.Calculate())
	return inv
}

func TestChargesAndDiscountsWithVAT(t *testing.T) {
	key := testKey(t)
	general := tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}
	t.Run("a charge", func(t *testing.T) {
		inv := withCharges(t, []*bill.Charge{{Amount: num.MakeAmount(100, 2), Reason: "Service", Taxes: general}}, nil)
		r, err := FromInvoice(inv, testOptions(), key)
		require.NoError(t, err)
		assert.Equal(t, []*Tax{
			{Rate: "25.00", Base: "4.80", Amount: "1.20"},
			{Rate: "13.00", Base: fiveEuros, Amount: "0.65"},
		}, r.VAT)
		assert.Equal(t, "11.65", r.Total)
	})
	t.Run("a discount", func(t *testing.T) {
		inv := withCharges(t, nil, []*bill.Discount{{Amount: num.MakeAmount(100, 2), Reason: "Promo", Taxes: general}})
		r, err := FromInvoice(inv, testOptions(), key)
		require.NoError(t, err)
		assert.Equal(t, []*Tax{
			{Rate: "25.00", Base: "3.20", Amount: "0.80"},
			{Rate: "13.00", Base: fiveEuros, Amount: "0.65"},
		}, r.VAT)
		assert.Equal(t, "9.65", r.Total)
	})
}

func TestZKIMatchesInvoice(t *testing.T) {
	key := testKey(t)
	r, err := NewInvoice(loadExample(t, "cash-prices-include-vat.json"), testOptions(), key)
	require.NoError(t, err)
	zki, err := ZKI(key, &ZKIInput{
		IssuerOIB: invopopOIB,
		IssuedAt:  cal.MakeDateTime(2026, 11, 6, 13, 58, 10),
		Number:    r.Number,
		Total:     "10.65",
	})
	require.NoError(t, err)
	assert.Equal(t, zki, r.ZKI)
}

// The issue time is copied as it is, never converted: 02:30 on 29 March
// 2026 does not exist in Croatia, since clocks go from 02:00 to 03:00, and
// a time.Time in Europe/Zagreb would move it to 03:30.
func TestIssueTimeInClockChange(t *testing.T) {
	key := testKey(t)
	inv := loadExample(t, "cash-prices-include-vat.json").Extract().(*bill.Invoice)
	inv.IssueDate = cal.MakeDate(2026, 3, 29)
	inv.IssueTime = cal.NewTime(2, 30, 0)
	r, err := FromInvoice(inv, testOptions(), key)
	require.NoError(t, err)
	assert.Equal(t, "29.03.2026T02:30:00", r.IssuedAt)
	zki, err := ZKI(key, &ZKIInput{
		IssuerOIB: invopopOIB,
		IssuedAt:  cal.MakeDateTime(2026, 3, 29, 2, 30, 0),
		Number:    r.Number,
		Total:     r.Total,
	})
	require.NoError(t, err)
	assert.Equal(t, zki, r.ZKI)
}

// The exempt total keeps its cents.
func TestExemptCents(t *testing.T) {
	inv := loadExample(t, "cash-prices-include-vat.json").Extract().(*bill.Invoice)
	for _, l := range inv.Lines {
		l.Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt}}
	}
	require.NoError(t, inv.Calculate())
	r, err := FromInvoice(inv, testOptions(), testKey(t))
	require.NoError(t, err)
	assert.Empty(t, r.VAT)
	assert.Equal(t, "10.65", r.VATExemptAmount)
	assert.Equal(t, "10.65", r.Total)
}

func TestAddonExtensions(t *testing.T) {
	key := testKey(t)
	inv := loadExample(t, "cash-prices-include-vat.json").Extract().(*bill.Invoice)
	inv.Tax.Ext = inv.Tax.Ext.
		Set(fiskal.ExtKeySequenceScope, fiskal.SequenceDevice).
		Set(fiskal.ExtKeyPaperNumber, "123/458/5")
	inv.Supplier.Ext = inv.Supplier.Ext.Set(fiskal.ExtKeyVATSystem, fiskal.VATSystemOut)
	r, err := FromInvoice(inv, testOptions(), key)
	require.NoError(t, err)
	assert.Equal(t, SequenceDevice, r.SequenceScope)
	assert.False(t, r.InVATSystem)
	assert.Equal(t, "123/458/5", r.HandwrittenNumber)
	assert.True(t, r.LateDelivery, "an invoice from the paper book is always late")
}

// A small taxpayer is exempt from VAT (Art. 90(1) of the VAT Act), so its
// sales are reported as the exempt amount, with no VAT breakdown. The amount
// has cents, which the exempt total once rounded away.
func TestSmallTaxpayer(t *testing.T) {
	inv := loadExample(t, "cash-prices-include-vat.json").Extract().(*bill.Invoice)
	inv.Supplier.Ext = inv.Supplier.Ext.Set(fiskal.ExtKeyVATSystem, fiskal.VATSystemOut)
	inv.Tax.PricesInclude = ""
	for _, l := range inv.Lines {
		l.Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt}}
	}
	require.NoError(t, inv.Calculate())
	r, err := FromInvoice(inv, testOptions(), testKey(t))
	require.NoError(t, err)
	assert.False(t, r.InVATSystem)
	assert.Empty(t, r.VAT)
	assert.Equal(t, "10.65", r.VATExemptAmount)
	assert.Equal(t, "10.65", r.Total)
}

func TestLateDelivery(t *testing.T) {
	r, err := NewInvoice(loadExample(t, "cash-prices-include-vat.json"), &Options{LateDelivery: true}, testKey(t))
	require.NoError(t, err)
	assert.True(t, r.LateDelivery)
	assert.Empty(t, r.HandwrittenNumber)
}

func TestBuyerOIBNotWithTransfer(t *testing.T) {
	inv := loadExample(t, "business-buyer-card.json").Extract().(*bill.Invoice)
	inv.Payment.Instructions.Key = pay.MeansKeyCreditTransfer
	r, err := FromInvoice(inv, testOptions(), testKey(t))
	require.NoError(t, err)
	assert.Equal(t, PaymentTransfer, r.PaymentMethod)
	assert.Empty(t, r.BuyerOIB)
}

func TestErrors(t *testing.T) {
	key := testKey(t)
	tests := []struct {
		name string
		inv  func(*bill.Invoice)
		err  error
		msg  string
	}{
		{name: "no issue time", inv: func(i *bill.Invoice) { i.IssueTime = nil }, err: ErrMissing, msg: "issue time"},
		{name: "no code", inv: func(i *bill.Invoice) { i.Code = "" }, err: ErrMissing, msg: "invoice code"},
		{name: "no addon", inv: func(i *bill.Invoice) { i.Addons = tax.Addons{} }, err: ErrMissing, msg: "hr-fiskal-b2c-v1"},
		{name: "a foreign supplier", inv: func(i *bill.Invoice) { i.Supplier.TaxID.Country = "SI" }, err: ErrInvalid, msg: "the issuer must be Croatian"},
		{name: "a credit note", inv: func(i *bill.Invoice) { i.Type = bill.InvoiceTypeCreditNote }, err: ErrUnsupported, msg: "credit-note"},
		{name: "another currency", inv: func(i *bill.Invoice) { i.Currency = "USD" }, err: ErrInvalid, msg: "euros"},
		{name: "not calculated", inv: func(i *bill.Invoice) { i.Totals = nil }, err: ErrMissing, msg: "calculate"},
		{name: "no premises", inv: func(i *bill.Invoice) { i.Tax.Ext = i.Tax.Ext.Delete(fiskal.ExtKeyPremises) }, err: ErrMissing, msg: "premises"},
		{name: "no device", inv: func(i *bill.Invoice) { i.Tax.Ext = i.Tax.Ext.Delete(fiskal.ExtKeyDevice) }, err: ErrMissing, msg: "device"},
		{name: "no operator", inv: func(i *bill.Invoice) { i.Supplier.People = nil }, err: ErrMissing, msg: "operator"},
		{name: "an operator without an OIB", inv: func(i *bill.Invoice) { i.Supplier.People[0].Identities = nil }, err: ErrMissing, msg: "operator OIB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := loadExample(t, "cash-prices-include-vat.json")
			inv := env.Extract().(*bill.Invoice)
			tt.inv(inv)
			_, err := FromInvoice(inv, testOptions(), key)
			require.ErrorIs(t, err, tt.err)
			assert.ErrorContains(t, err, tt.msg)
		})
	}

	t.Run("a charge without taxes", func(t *testing.T) {
		inv := withCharges(t, []*bill.Charge{{Amount: num.MakeAmount(10, 2), Reason: "Povratna naknada"}}, nil)
		_, err := FromInvoice(inv, testOptions(), key)
		require.ErrorIs(t, err, ErrUnsupported)
		assert.ErrorContains(t, err, "Povratna naknada")
	})
	for _, c := range eracun.PassThroughTaxCategories {
		t.Run("HR tax category "+c.String(), func(t *testing.T) {
			exempt := tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt, Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: c})}}
			inv := withCharges(t, []*bill.Charge{{Amount: num.MakeAmount(30, 2), Reason: "Fee", Taxes: exempt}}, nil)
			_, err := FromInvoice(inv, testOptions(), key)
			require.ErrorIs(t, err, ErrUnsupported)
			assert.ErrorContains(t, err, `HR tax category "`+c.String()+`"`)
		})
	}
	t.Run("a discount without taxes", func(t *testing.T) {
		inv := withCharges(t, nil, []*bill.Discount{{Amount: num.MakeAmount(10, 2), Reason: "Promo"}})
		_, err := FromInvoice(inv, testOptions(), key)
		require.ErrorIs(t, err, ErrUnsupported)
		assert.ErrorContains(t, err, "Promo")
	})
	t.Run("a rounding amount", func(t *testing.T) {
		inv := loadExample(t, "cash-prices-include-vat.json").Extract().(*bill.Invoice)
		rounding := num.MakeAmount(-5, 2)
		inv.Totals.Rounding = &rounding
		require.NoError(t, inv.Calculate())
		require.Equal(t, "10.60", inv.Totals.Payable.String())
		_, err := FromInvoice(inv, testOptions(), key)
		require.ErrorIs(t, err, ErrUnsupported)
		assert.ErrorContains(t, err, "rounding -0.05")
	})
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
