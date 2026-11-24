package ubl_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/currency"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pp is the hr regime's consumption tax (porez na potrošnju) at a percent,
// such as "3%".
func pp(percent string) *tax.Combo {
	p, err := num.PercentageFromString(percent)
	if err != nil {
		panic(err)
	}
	return &tax.Combo{Category: hr.TaxCategoryPP, Percent: &p}
}

// exportInvoice calculates and exports the invoice, which must be valid.
func exportInvoice(t *testing.T, inv *bill.Invoice) (*bill.Invoice, *goblubl.Invoice) {
	t.Helper()
	env, err := gobl.Envelop(inv)
	require.NoError(t, err)
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())
	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	doc, ok := out.(*goblubl.Invoice)
	require.True(t, ok)
	return env.Extract().(*bill.Invoice), doc
}

// TestConsumptionTaxAsOfficialExample checks that the consumption tax on a
// line exports the way the official example that carries it does,
// "eRacun-PDV-NEOP-PP-trosak" (CIUS 5.2): the example imported, with its PP
// charge moved onto its line as the hr regime's PP tax, exports as the
// example itself.
func TestConsumptionTaxAsOfficialExample(t *testing.T) {
	const name = "eRacun-PDV-NEOP-PP-trosak"
	data, err := os.ReadFile(filepath.Join("testdata", "hr", name+".xml"))
	require.NoError(t, err)
	in, err := goblubl.Decode(data)
	require.NoError(t, err)
	env, err := goblubl.Import(in)
	require.NoError(t, err)

	inv := env.Extract().(*bill.Invoice)
	require.Len(t, inv.Charges, 1)
	require.Equal(t, eracun.TaxLabelPP, inv.Charges[0].Taxes.Get(tax.CategoryVAT).Ext.Get(eracun.ExtKeyTaxCategory))
	inv.Charges = nil
	inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, pp("3%"))
	// The import reads the PP charge's exemption reason as the exempt note,
	// which an invoice without exempt amounts doesn't have.
	require.Len(t, inv.Tax.Notes, 1)
	inv.Tax.Notes = nil
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())

	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	got, err := goblubl.Encode(out)
	require.NoError(t, err)

	want, err := os.ReadFile(filepath.Join("testdata", "hr", "out", name+".xml"))
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

// catering is an invoice for a beer and a wine, each with the consumption tax
// at its own rate, and a meal without it.
func catering() *bill.Invoice {
	inv := scenarioInvoice()
	inv.Lines = []*bill.Line{
		line("Pivo", 400, vat(tax.KeyStandard, tax.RateGeneral)),
		line("Vino", 1250, vat(tax.KeyStandard, tax.RateGeneral)),
		line("Jelo", 1800, vat(tax.KeyStandard, tax.RateReduced)),
	}
	inv.Lines[0].Quantity = num.MakeAmount(3, 0)
	inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, pp("3%"))
	inv.Lines[1].Taxes = append(inv.Lines[1].Taxes, pp("2%"))
	return inv
}

func TestConsumptionTaxRates(t *testing.T) {
	inv, doc := exportInvoice(t, catering())
	assert.Equal(t, "0.61", inv.Totals.Taxes.Category(hr.TaxCategoryPP).Amount.String())

	// One exempt charge per rate, on the amounts of the lines that carry it.
	require.Len(t, doc.AllowanceCharge, 2)
	for i, want := range []struct{ rate, base, amount string }{
		{"3", "12.00", "0.36"},
		{"2", "12.50", "0.25"},
	} {
		ac := doc.AllowanceCharge[i]
		assert.True(t, ac.ChargeIndicator)
		assert.Equal(t, "#HR:PP#Porez na potrošnju", *ac.AllowanceChargeReason)
		assert.Equal(t, want.rate, *ac.MultiplierFactorNumeric)
		assert.Equal(t, want.base, ac.BaseAmount.Value)
		assert.Equal(t, want.amount, ac.Amount.Value)
		require.Len(t, ac.TaxCategory, 1)
		tc := ac.TaxCategory[0]
		assert.Equal(t, "E", tc.ID.Value)
		assert.Equal(t, "HR:PP", *tc.Name)
		assert.Equal(t, "0", *tc.Percent)
		assert.Equal(t, "Porez na potrošnju", *tc.TaxExemptionReason)
		assert.Equal(t, goblubl.TaxSchemeVAT, tc.TaxScheme.ID.Value)
	}

	// The VAT breakdown and total leave PP out, but for its exempt charges.
	tt := doc.TaxTotal[0]
	assert.Equal(t, "8.47", tt.TaxAmount.Value) // 6.13 + 2.34
	require.Len(t, tt.TaxSubtotal, 3)
	for _, st := range tt.TaxSubtotal {
		assert.Equal(t, goblubl.TaxSchemeVAT, st.TaxCategory.TaxScheme.ID.Value)
	}
	e := tt.TaxSubtotal[2]
	assert.Equal(t, "E", e.TaxCategory.ID.Value)
	assert.Equal(t, "0.61", e.TaxableAmount.Value)
	assert.Equal(t, "0.00", e.TaxAmount.Value)
	assert.Equal(t, "#HR:PP#Porez na potrošnju", *e.TaxCategory.TaxExemptionReason)

	m := doc.LegalMonetaryTotal
	assert.Equal(t, "42.50", m.LineExtensionAmount.Value)
	assert.Equal(t, "43.11", m.TaxExclusiveAmount.Value)
	assert.Equal(t, "0.61", m.ChargeTotalAmount.Value)
	assert.Equal(t, "51.58", m.TaxInclusiveAmount.Value)
	assert.Equal(t, "51.58", m.PayableAmount.Value)

	// The lines say nothing of PP: the HR CIUS has no element for it.
	for i := range doc.InvoiceLines {
		assert.Nil(t, doc.InvoiceLines[i].Item.AdditionalItemProperty, "line %d", i+1)
	}

	// The HR breakdown has PP out of the VAT scope (O), under the LOC scheme.
	hrd, err := hrubl.FindHRFISK20Data(doc)
	require.NoError(t, err)
	require.NotNil(t, hrd)
	require.NotNil(t, hrd.HRTaxTotal)
	assert.Equal(t, "8.47", hrd.HRTaxTotal.TaxAmount.Value)
	// One entry for PP, whatever its rates, like each other HR category.
	rows := hrd.HRTaxTotal.HRTaxSubtotals
	require.Len(t, rows, 3)
	assert.Equal(t, "HR:PDV25", rows[0].HRTaxCategory.Name)
	assert.Equal(t, "HR:PDV13", rows[1].HRTaxCategory.Name)
	assert.Equal(t, "0.61", rows[2].TaxableAmount.Value)
	assert.Equal(t, "0.00", rows[2].TaxAmount.Value)
	assert.Equal(t, hrubl.HRTaxCategory{ID: "O", Name: "HR:PP", Percent: "0", HRTaxScheme: hrubl.HRTaxScheme{ID: "LOC"}}, rows[2].HRTaxCategory)
	require.NotNil(t, hrd.HRLegalMonetaryTotal)
	assert.Equal(t, "42.50", hrd.HRLegalMonetaryTotal.TaxExclusiveAmount.Value)
	assert.Equal(t, "0.61", hrd.HRLegalMonetaryTotal.OutOfScopeOfVATAmount.Value)
}

// TestConsumptionTaxBeforeVAT checks a line that lists PP before VAT still
// has its VAT category written: gobl.ubl writes the line's first tax.
func TestConsumptionTaxBeforeVAT(t *testing.T) {
	inv := scenarioInvoice()
	inv.Lines[0].Taxes = tax.Set{pp("3%"), vat(tax.KeyStandard, tax.RateGeneral)}
	_, doc := exportInvoice(t, inv)
	ctc := doc.InvoiceLines[0].Item.ClassifiedTaxCategory
	assert.Equal(t, "S", ctc.ID.Value)
	assert.Equal(t, "HR:PDV25", *ctc.Name)
	assert.Equal(t, "25.0", *ctc.Percent)
	assert.Equal(t, goblubl.TaxSchemeVAT, ctc.TaxScheme.ID.Value)
}

// TestConsumptionTaxExemptLine checks that an exempt line keeps its VAT
// category and its own exemption reason on a line beside PP.
func TestConsumptionTaxExemptLine(t *testing.T) {
	inv := scenarioInvoice()
	inv.Lines[0].Taxes = tax.Set{vat(tax.KeyExempt, ""), pp("3%")}
	addNote(inv, tax.KeyExempt, "Oslobođeno PDV-a")
	_, doc := exportInvoice(t, inv)
	ctc := doc.InvoiceLines[0].Item.ClassifiedTaxCategory
	assert.Equal(t, "E", ctc.ID.Value)
	assert.Equal(t, "0", *ctc.Percent)
	assert.Equal(t, "Oslobođeno PDV-a", *ctc.TaxExemptionReason)
	// One exempt breakdown entry (BR-E-01), for the line and the charge.
	tt := doc.TaxTotal[0]
	require.Len(t, tt.TaxSubtotal, 1)
	assert.Equal(t, "103.00", tt.TaxSubtotal[0].TaxableAmount.Value)
	assert.Equal(t, "Oslobođeno PDV-a", *tt.TaxSubtotal[0].TaxCategory.TaxExemptionReason)
}

// TestConsumptionTaxInAnotherCurrency checks the VAT total in the regime's
// currency (BT-111) leaves PP out too.
func TestConsumptionTaxInAnotherCurrency(t *testing.T) {
	inv := scenarioInvoice()
	inv.Currency = "USD"
	inv.ExchangeRates = []*currency.ExchangeRate{{From: "USD", To: "EUR", Amount: num.MakeAmount(9, 1)}}
	inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, pp("3%"))
	_, doc := exportInvoice(t, inv)
	require.Len(t, doc.TaxTotal, 2)
	assert.Equal(t, "25.00", doc.TaxTotal[0].TaxAmount.Value)
	assert.Equal(t, "USD", *doc.TaxTotal[0].TaxAmount.CurrencyID)
	assert.Equal(t, "22.50", doc.TaxTotal[1].TaxAmount.Value)
	assert.Equal(t, "EUR", *doc.TaxTotal[1].TaxAmount.CurrencyID)
}

// TestConsumptionTaxExportRefusals checks that the export refuses the PP it
// can't write, in case the invoice was not validated first.
func TestConsumptionTaxExportRefusals(t *testing.T) {
	tests := []struct {
		name string
		edit func(*bill.Invoice)
	}{
		{"on a discount", func(inv *bill.Invoice) {
			inv.Discounts = []*bill.Discount{{Reason: "Popust", Amount: num.MakeAmount(500, 2),
				Taxes: tax.Set{vat(tax.KeyStandard, tax.RateGeneral), pp("3%")}}}
		}},
		{"on a charge", func(inv *bill.Invoice) {
			inv.Charges = []*bill.Charge{{Reason: "Usluga", Amount: num.MakeAmount(500, 2),
				Taxes: tax.Set{vat(tax.KeyStandard, tax.RateGeneral), pp("3%")}}}
		}},
		{"without VAT", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes = tax.Set{pp("3%")}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := scenarioInvoice()
			inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, pp("3%"))
			tt.edit(inv)
			env, err := gobl.Envelop(inv)
			require.NoError(t, err)
			require.NoError(t, env.Calculate())
			_, err = goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
			require.ErrorContains(t, err, "PP")
		})
	}
}

// TestConsumptionTaxImportsAsCharge checks what the import makes of an
// exported PP: the exempt charge with the HR tax category PP, which an
// invoice received from another issuer has too.
func TestConsumptionTaxImportsAsCharge(t *testing.T) {
	_, doc := exportInvoice(t, catering())
	data, err := goblubl.Encode(doc)
	require.NoError(t, err)
	in, err := goblubl.Decode(data)
	require.NoError(t, err)
	env, err := goblubl.Import(in)
	require.NoError(t, err)
	require.NoError(t, env.Validate())
	inv := env.Extract().(*bill.Invoice)
	require.Len(t, inv.Charges, 2)
	for _, c := range inv.Charges {
		assert.Equal(t, eracun.TaxLabelPP, c.Taxes.Get(tax.CategoryVAT).Ext.Get(eracun.ExtKeyTaxCategory))
	}
	assert.Empty(t, inv.Lines[0].Item.Attributes)
	assert.Equal(t, "51.58", inv.Totals.Payable.String())
}
