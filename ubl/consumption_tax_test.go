package ubl_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/currency"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
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

// officialPPExample is the official example that carries the consumption
// tax, "eRacun-PDV-NEOP-PP-trosak" (CIUS 5.2): a 100.00 line at 25% with a
// 3% PP charge.
const officialPPExample = "eRacun-PDV-NEOP-PP-trosak"

// hrPPProperty is the HR:PP item property the export writes on a line with
// PP at 3%, after the line's tax category.
const hrPPProperty = `</cac:ClassifiedTaxCategory>
      <cac:AdditionalItemProperty>
        <cbc:Name>HR:PP</cbc:Name>
        <cbc:Value>3</cbc:Value>
      </cac:AdditionalItemProperty>`

// TestConsumptionTaxAsOfficialExample checks that the consumption tax on a
// line exports the way the official example that carries it does: the
// example imported, with its PP charge moved onto its line as the hr
// regime's PP tax, exports as the example itself, plus the HR:PP item
// property on the line. The import leaves the charge as it is, since nothing
// in the example ties it to its line, so the test moves it.
func TestConsumptionTaxAsOfficialExample(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "hr", officialPPExample+".xml"))
	require.NoError(t, err)
	env := importUBL(t, data)

	inv := env.Extract().(*bill.Invoice)
	require.Len(t, inv.Charges, 1)
	require.Equal(t, eracun.TaxLabelPP, inv.Charges[0].Taxes.Get(tax.CategoryVAT).Ext.Get(eracun.ExtKeyTaxCategory))
	require.Nil(t, inv.Lines[0].Taxes.Get(hr.TaxCategoryPP))
	inv.Charges = nil
	inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, pp("3%"))
	// The import reads the PP charge's exemption reason as the exempt note,
	// which an invoice without exempt amounts doesn't have.
	require.Len(t, inv.Tax.Notes, 1)
	inv.Tax.Notes = nil
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())

	assert.Equal(t, officialPPExampleWithProperty(t), exportXML(t, env))
}

// TestConsumptionTaxImportsOfficialExampleWithProperty checks that the
// official example, given the HR:PP property the export writes on its line,
// imports with its PP charge moved onto the line, and exports back as it
// came in.
func TestConsumptionTaxImportsOfficialExampleWithProperty(t *testing.T) {
	want := officialPPExampleWithProperty(t)
	data, err := os.ReadFile(filepath.Join("testdata", "hr", officialPPExample+".xml"))
	require.NoError(t, err)
	in := strings.Replace(string(data), "</cac:ClassifiedTaxCategory>", hrPPProperty, 1)
	env := importUBL(t, []byte(in))
	require.NoError(t, env.Validate())

	inv := env.Extract().(*bill.Invoice)
	assert.Empty(t, inv.Charges)
	assert.Nil(t, inv.Tax.Notes)
	assert.Nil(t, inv.Lines[0].Item.Attributes)
	require.NotNil(t, inv.Lines[0].Taxes.Get(hr.TaxCategoryPP))
	assert.Equal(t, "3%", inv.Lines[0].Taxes.Get(hr.TaxCategoryPP).Percent.String())
	assert.Equal(t, "3.00", inv.Totals.Taxes.Category(hr.TaxCategoryPP).Amount.String())
	assert.Equal(t, "128.00", inv.Totals.Payable.String())

	assert.Equal(t, want, exportXML(t, env))
}

// officialPPExampleWithProperty is the golden file of the official PP example
// with the HR:PP property on its line.
func officialPPExampleWithProperty(t *testing.T) string {
	t.Helper()
	want, err := os.ReadFile(filepath.Join("testdata", "hr", "out", officialPPExample+".xml"))
	require.NoError(t, err)
	const end = "</cac:ClassifiedTaxCategory>"
	require.Equal(t, 1, strings.Count(string(want), end))
	return strings.Replace(string(want), end, hrPPProperty, 1)
}

func exportXML(t *testing.T, env *gobl.Envelope) string {
	t.Helper()
	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	data, err := goblubl.Encode(out)
	require.NoError(t, err)
	return string(data)
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

	// The rate on each line that carries it.
	for i, want := range []string{"3", "2", ""} {
		it := doc.InvoiceLines[i].Item
		var got string
		if it.AdditionalItemProperty != nil {
			for _, p := range *it.AdditionalItemProperty {
				if p.Name == "HR:PP" {
					got = p.Value
				}
			}
		}
		assert.Equal(t, want, got, "line %d", i+1)
	}

	// The HR breakdown has PP out of the VAT scope (O), under the LOC scheme.
	hrd, ok := goblubl.FindExtensionData[*hrubl.HRFISK20Data](doc.Extensions)
	require.True(t, ok)
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

// TestConsumptionTaxRoundTrip checks that every shape of PP on the lines
// that TestScenarios accepts imports back as it was issued: the PP charges
// the export writes move back onto the lines they came from, at the same
// rates, the amounts stay the same, and the invoice exports to the same
// eRačun again.
func TestConsumptionTaxRoundTrip(t *testing.T) {
	var rows int
	for _, sc := range scenarios {
		inv := scenarioInvoice()
		if sc.edit != nil {
			sc.edit(inv)
		}
		if sc.fault != "" || !slices.ContainsFunc(inv.Lines, func(l *bill.Line) bool { return l.Taxes.Get(hr.TaxCategoryPP) != nil }) {
			continue
		}
		rows++
		t.Run(sc.name, func(t *testing.T) {
			env, err := gobl.Envelop(inv)
			require.NoError(t, err)
			require.NoError(t, env.Validate())
			// The export takes VAT out of prices that include it, so what
			// the import must give back is read first.
			want := roundTripped(env.Extract().(*bill.Invoice))

			first := exportXML(t, env)
			got := importUBL(t, []byte(first))
			require.NoError(t, got.Validate())
			// And exports as it did, which TestScenarios checks against the
			// schematron.
			assert.Equal(t, first, exportXML(t, got))
			inv := got.Extract().(*bill.Invoice)
			assert.Equal(t, want, roundTripped(inv))
			assert.Empty(t, inv.Charges)
			for i, l := range inv.Lines {
				for _, a := range l.Item.Attributes {
					assert.NotEqual(t, "HR:PP", a.Label, "line %d", i+1)
				}
			}
		})
	}
	assert.Equal(t, 11, rows, "accepted PP rows in TestScenarios")
}

// TestConsumptionTaxStaysCharge checks that the import leaves a PP charge,
// and the lines it would cover, exactly as an import that never moves PP
// does, whenever the lines don't account for it exactly or moving it would
// give an invoice the addon refuses.
func TestConsumptionTaxStaysCharge(t *testing.T) {
	// removeProperty takes the HR:PP item property off line i.
	removeProperty := func(i int) func(*goblubl.Invoice) {
		return func(doc *goblubl.Invoice) {
			doc.InvoiceLines[i].Item.AdditionalItemProperty = nil
		}
	}
	tests := []struct {
		name string
		inv  func() *bill.Invoice
		edit func(*goblubl.Invoice)
	}{
		{
			// The 3% charge is on 1.21 + 10.03, but only the first line
			// says it has PP.
			name: "lines that don't add up to the base",
			inv: func() *bill.Invoice {
				inv := scenarioInvoice()
				roundingApart(inv)
				for _, l := range inv.Lines {
					l.Taxes = append(l.Taxes, pp("3%"))
				}
				return inv
			},
			edit: removeProperty(1),
		},
		{
			// The 3% charge matches the beer, but nothing says which line
			// the 2% one is on, and the addon refuses PP on the lines beside
			// a PP charge.
			name: "one of two rates without its lines",
			inv:  catering,
			edit: removeProperty(1),
		},
		{
			// 3% of 12.01 is the 0.36 of the beer's 12.00 too, so the
			// amounts would stay, but the base wouldn't.
			name: "a base a cent off the lines",
			inv:  catering,
			edit: func(doc *goblubl.Invoice) {
				doc.AllowanceCharge[0].BaseAmount.Value = "12.01"
			},
		},
		{
			name: "a rate the line doesn't give",
			inv:  catering,
			edit: func(doc *goblubl.Invoice) {
				(*doc.InvoiceLines[0].Item.AdditionalItemProperty)[0].Value = "2"
			},
		},
		{
			// The HR:PP property gives only the rate, so two charges at one
			// rate can't be told apart: both are 3% of the line's 100.00.
			name: "two charges at one rate",
			inv: func() *bill.Invoice {
				inv := scenarioInvoice()
				inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, pp("3%"))
				return inv
			},
			edit: func(doc *goblubl.Invoice) {
				doc.AllowanceCharge = append(doc.AllowanceCharge, doc.AllowanceCharge[0])
				doc.TaxTotal[0].TaxSubtotal[1].TaxableAmount.Value = "6.00"
				m := &doc.LegalMonetaryTotal
				m.TaxExclusiveAmount.Value = "106.00"
				m.ChargeTotalAmount.Value = "6.00"
				m.TaxInclusiveAmount.Value = "131.00"
				m.PayableAmount.Value = "131.00"
			},
		},
		{
			name: "totals that differ from the declared ones",
			inv:  catering,
			edit: func(doc *goblubl.Invoice) {
				doc.LegalMonetaryTotal.PayableAmount.Value = "51.59"
			},
		},
		{
			name: "a supplier outside VAT",
			inv: func() *bill.Invoice {
				inv := scenarioInvoice()
				inv.Supplier = supplierOutsideVAT()
				inv.Lines[0].Taxes = tax.Set{vat(tax.KeyExempt, ""), pp("3%")}
				addNote(inv, tax.KeyExempt, "Nije u sustavu PDV čl. 90. st. 1. Zakona o PDV-u")
				return inv
			},
		},
		{
			// The addon refuses the supplier's VAT number beside O amounts
			// already, and PP beside them on the lines too (BR-O-11).
			name: "an amount outside the VAT scope",
			inv: func() *bill.Invoice {
				inv := scenarioInvoice()
				inv.Customer = customerWithoutVATNumber()
				inv.Lines[0].Taxes = tax.Set{vat(tax.KeyOutsideScope, ""), pp("3%")}
				addNote(inv, tax.KeyOutsideScope, "Nije predmet oporezivanja")
				return inv
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := gobl.Envelop(tt.inv())
			require.NoError(t, err)
			require.NoError(t, env.Calculate())
			out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
			require.NoError(t, err)
			doc := out.(*goblubl.Invoice)
			if tt.edit != nil {
				tt.edit(doc)
			}
			data, err := goblubl.Encode(doc)
			require.NoError(t, err)

			got := importUBL(t, data)
			inv := got.Extract().(*bill.Invoice)
			require.NotEmpty(t, inv.Charges)
			assert.Equal(t, invoiceJSON(t, importUBLAsCharge(t, data)), invoiceJSON(t, got))
		})
	}
}

// TestConsumptionTaxKeepsOtherAttributes checks that a line whose PP moves
// keeps its other item attributes.
func TestConsumptionTaxKeepsOtherAttributes(t *testing.T) {
	inv := catering()
	inv.Lines[0].Item.Attributes = []*org.Attribute{{Label: "Volumen", Text: "0,5 l"}}
	_, doc := exportInvoice(t, inv)
	data, err := goblubl.Encode(doc)
	require.NoError(t, err)
	got := importUBL(t, data).Extract().(*bill.Invoice)
	assert.Empty(t, got.Charges)
	assert.Equal(t, []*org.Attribute{{Label: "Volumen", Text: "0,5 l"}}, got.Lines[0].Item.Attributes)
	assert.Equal(t, "3%", got.Lines[0].Taxes.Get(hr.TaxCategoryPP).Percent.String())
}

// TestConsumptionTaxMovesBesideOtherFaults checks that the PP of an invoice
// the addon refuses for another reason moves all the same: only a fault the
// move would add keeps it a charge.
func TestConsumptionTaxMovesBesideOtherFaults(t *testing.T) {
	_, doc := exportInvoice(t, catering())
	doc.AccountingSupplierParty.SellerContact = nil
	data, err := goblubl.Encode(doc)
	require.NoError(t, err)
	env := importUBL(t, data)
	assert.Empty(t, env.Extract().(*bill.Invoice).Charges)
	err = env.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GOBL-HR-ERACUN-BILL-INVOICE-06")
}

// TestConsumptionTaxImportTwice checks that importing the same decoded
// document twice gives the same invoice, with its PP moved both times: the
// import leaves the UBL document as it was.
func TestConsumptionTaxImportTwice(t *testing.T) {
	_, doc := exportInvoice(t, catering())
	data, err := goblubl.Encode(doc)
	require.NoError(t, err)
	in, err := goblubl.Decode(data)
	require.NoError(t, err)
	first, second := importDoc(t, in), importDoc(t, in)
	assert.Empty(t, first.Extract().(*bill.Invoice).Charges)
	assert.Equal(t, invoiceJSON(t, first), invoiceJSON(t, second))
}

// importUBL imports the UBL data the way a received eRačun is.
func importUBL(t *testing.T, data []byte) *gobl.Envelope {
	t.Helper()
	in, err := goblubl.Decode(data)
	require.NoError(t, err)
	return importDoc(t, in)
}

func importDoc(t *testing.T, in goblubl.Document) *gobl.Envelope {
	t.Helper()
	env, err := goblubl.Import(in)
	require.NoError(t, err)
	return env
}

// importUBLAsCharge imports the UBL data without moving PP, as the import
// did before it read PP back into the hr regime's PP tax.
func importUBLAsCharge(t *testing.T, data []byte) *gobl.Envelope {
	t.Helper()
	in, err := goblubl.Decode(data)
	require.NoError(t, err)
	f := hrubl.FormatHRCIUS
	f.ReconciledImportFuncs = nil
	env, err := goblubl.Import(in, goblubl.WithFormat(f))
	require.NoError(t, err)
	return env
}

// invoiceJSON is the invoice in the envelope, with a fixed UUID.
func invoiceJSON(t *testing.T, env *gobl.Envelope) string {
	t.Helper()
	inv := *env.Extract().(*bill.Invoice)
	inv.UUID = staticUUID
	data, err := json.Marshal(inv)
	require.NoError(t, err)
	return string(data)
}

// ppRoundTrip is what an invoice with PP on its lines must keep through an
// export and an import.
type ppRoundTrip struct {
	// LinePP is the PP percent of each line, or "" for none.
	LinePP                []string
	Payable, Tax, VAT, PP string
	// ExemptNotes are the texts of the exempt VAT notes.
	ExemptNotes []string
}

func roundTripped(inv *bill.Invoice) ppRoundTrip {
	rt := ppRoundTrip{
		LinePP:  make([]string, len(inv.Lines)),
		Payable: inv.Totals.Payable.String(),
		Tax:     inv.Totals.Tax.String(),
		VAT:     inv.Totals.Taxes.Category(tax.CategoryVAT).Amount.String(),
		PP:      inv.Totals.Taxes.Category(hr.TaxCategoryPP).Amount.String(),
	}
	for i, l := range inv.Lines {
		if c := l.Taxes.Get(hr.TaxCategoryPP); c != nil && c.Percent != nil {
			rt.LinePP[i] = c.Percent.String()
		}
	}
	for _, n := range inv.Tax.Notes {
		if n.Category == tax.CategoryVAT && n.Key == tax.KeyExempt {
			rt.ExemptNotes = append(rt.ExemptNotes, n.Text)
		}
	}
	return rt
}
