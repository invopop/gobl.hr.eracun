package eracun_test

import (
	"testing"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/catalogues/cef"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/pay"
	_ "github.com/invopop/gobl/regimes/de"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testInvoice() *bill.Invoice {
	issueTime := cal.NewTime(12, 0, 0)
	return &bill.Invoice{
		Regime:    tax.WithRegime("HR"),
		Addons:    tax.WithAddons(eracun.V1),
		Series:    "1-P1",
		Code:      "1",
		Currency:  "EUR",
		IssueDate: cal.MakeDate(2026, 3, 1),
		IssueTime: issueTime,
		Tax: &bill.Tax{
			Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyProcess: "P1"}),
		},
		Supplier: &org.Party{
			Name:  "TVRTKA A d.o.o.",
			TaxID: &tax.Identity{Country: "HR", Code: "12345678903"},
			Ext:   tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyVATStatus: eracun.VATStatusRegistered}),
			Addresses: []*org.Address{
				{Street: "Ulica 1", Locality: "Zagreb", Code: "10000", Country: "HR"},
			},
			People: []*org.Person{
				{
					Key:        eracun.PersonKeyOperator,
					Name:       &org.Name{Given: "Operater1"},
					Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "12345678903"}},
				},
			},
		},
		Customer: &org.Party{
			Name:  "Tvrtka B d.o.o.",
			TaxID: &tax.Identity{Country: "HR", Code: "11111111119"},
			Addresses: []*org.Address{
				{Street: "Ulica 2", Locality: "Rijeka", Code: "51000", Country: "HR"},
			},
		},
		Lines: []*bill.Line{
			{
				Quantity: num.MakeAmount(1, 0),
				Item: &org.Item{
					Name:       "Proizvod",
					Price:      num.NewAmount(10000, 2),
					Identities: []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "62.20.20"}},
				},
				Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}},
			},
		},
		Payment: &bill.PaymentDetails{
			Terms: &pay.Terms{DueDates: []*pay.DueDate{{Date: cal.NewDate(2026, 3, 31), Percent: num.NewPercentage(100, 2)}}},
		},
	}
}

func assertFault(t *testing.T, inv any, code rules.Code) {
	t.Helper()
	faults := rules.Validate(inv)
	require.Error(t, faults)
	assert.True(t, faults.HasCode(code), "want %s, got %s", code, faults)
}

func TestInvoiceValid(t *testing.T) {
	inv := testInvoice()
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
	assert.Contains(t, inv.GetAddons(), en16931.V2017)
	assert.Equal(t, cbc.URI("iso6523-actorid-upis::9934:12345678903"), inv.Supplier.Endpoints[0].URI)
	assert.Equal(t, cbc.URI("iso6523-actorid-upis::9934:11111111119"), inv.Customer.Endpoints[0].URI)
	assert.Equal(t, eracun.TaxLabelPDV25, inv.Lines[0].Taxes[0].Ext.Get(eracun.ExtKeyTaxCategory))
}

func TestInvoiceRules(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*bill.Invoice)
		code   rules.Code
	}{
		{"issue date before 2026", func(inv *bill.Invoice) { inv.IssueDate = cal.MakeDate(2025, 12, 31) }, "GOBL-HR-ERACUN-BILL-INVOICE-01"},
		{"no issue time", func(inv *bill.Invoice) { inv.IssueTime = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-02"},
		{"no process", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Delete(eracun.ExtKeyProcess) }, "GOBL-HR-ERACUN-BILL-INVOICE-04"},
		{"process P13", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Set(eracun.ExtKeyProcess, "P13") }, "GOBL-HR-ERACUN-BILL-INVOICE-04"},
		{"process P99 without label", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Set(eracun.ExtKeyProcess, "P99") }, "GOBL-HR-ERACUN-BILL-INVOICE-04"},
		{"supplier without endpoint", func(inv *bill.Invoice) { inv.Supplier.TaxID = &tax.Identity{Country: "HR"} }, "GOBL-HR-ERACUN-BILL-INVOICE-05"},
		{"no operator key", func(inv *bill.Invoice) { inv.Supplier.People[0].Key = "" }, "GOBL-HR-ERACUN-BILL-INVOICE-06"},
		{"no people", func(inv *bill.Invoice) { inv.Supplier.People = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-06"},
		{"two people with the operator key", func(inv *bill.Invoice) {
			inv.Supplier.People = append(inv.Supplier.People, &org.Person{
				Key:        eracun.PersonKeyOperator,
				Name:       &org.Name{Given: "Operater2"},
				Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}},
			})
		}, "GOBL-HR-ERACUN-BILL-INVOICE-06"},
		// GOBL's person name rules already require the name HR-BR-37 needs.
		{"operator without a name", func(inv *bill.Invoice) { inv.Supplier.People[0].Name = nil }, "GOBL-ORG-PERSON-01"},
		{"operator without OIB", func(inv *bill.Invoice) { inv.Supplier.People[0].Identities = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-07"},
		{"operator without OIB while another person has one", func(inv *bill.Invoice) {
			inv.Supplier.People[0].Identities = nil
			inv.Supplier.People = append(inv.Supplier.People, &org.Person{
				Name:       &org.Name{Given: "Ana"},
				Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}},
			})
		}, "GOBL-HR-ERACUN-BILL-INVOICE-07"},
		{"no customer", func(inv *bill.Invoice) { inv.Customer = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-08"},
		{"supplier with only an email endpoint", func(inv *bill.Invoice) {
			inv.Supplier.TaxID = &tax.Identity{Country: "HR"}
			inv.Supplier.Endpoints = []*org.Endpoint{{URI: "mailto:racuni@example.com"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-05"},
		{"customer without endpoint", func(inv *bill.Invoice) { inv.Customer.TaxID = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-09"},
		{"customer with only an email endpoint", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "mailto:racuni@example.com"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-09"},
		{"customer inbox with only an email", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Inboxes = []*org.Inbox{{Email: "racuni@example.com"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-09"},
		{"customer email endpoint written before an inbox scheme", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "mailto:racuni@example.com"}}
			inv.Customer.Inboxes = []*org.Inbox{{Scheme: "0088", Code: "3850001234567"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-09"},
		{"customer endpoint not written to UBL", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "https://example.com/inbox"}}
			inv.Customer.Inboxes = []*org.Inbox{{Code: "123"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-09"},
		{"supplier address with an invalid OIB", func(inv *bill.Invoice) {
			inv.Supplier.Inboxes = []*org.Inbox{{Scheme: "9934", Code: "12345678900"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-18"},
		{"supplier address with another party's OIB", func(inv *bill.Invoice) {
			inv.Supplier.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-19"},
		{"customer with no OIB, only an address in another scheme", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Inboxes = []*org.Inbox{{Scheme: "0088", Code: "3850001234567"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-23"},
		{"customer with no OIB, only an invalid 9934 address", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Inboxes = []*org.Inbox{{Scheme: "9934", Code: "11111111118"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-23"},
		{"customer address with an invalid OIB", func(inv *bill.Invoice) {
			inv.Customer.Inboxes = []*org.Inbox{{Scheme: "9934", Code: "11111111118"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-20"},
		{"customer address that is not 11 digits", func(inv *bill.Invoice) {
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:12345"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-20"},
		{"customer address with another party's OIB", func(inv *bill.Invoice) {
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:12345678903"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-21"},
		{"customer address that differs from its OIB identity", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Identities = []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}}
			inv.Customer.Inboxes = []*org.Inbox{{Scheme: "9934", Code: "12345678903"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-21"},
		{"customer with a foreign tax ID", func(inv *bill.Invoice) {
			inv.Customer.TaxID = &tax.Identity{Country: "DE", Code: "111111125"}
			inv.Customer.Inboxes = []*org.Inbox{{Scheme: "9930", Code: "DE111111125"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-22"},
		{"customer with a foreign tax ID and an OIB address", func(inv *bill.Invoice) {
			inv.Customer.TaxID = &tax.Identity{Country: "DE", Code: "111111125"}
			inv.Customer.Inboxes = []*org.Inbox{{Scheme: "9934", Code: "11111111119"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-22"},
		{"customer without tax ID", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-10"},
		{"customer tax ID without code", func(inv *bill.Invoice) {
			inv.Customer.TaxID = &tax.Identity{Country: "HR"}
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-11"},
		{"customer without tax ID, standard charge", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
			inv.Charges = []*bill.Charge{{Amount: num.MakeAmount(500, 2), Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-10"},
		{"customer without tax ID, standard discount", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
			inv.Discounts = []*bill.Discount{{Amount: num.MakeAmount(500, 2), Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-10"},
		{"supplier without a VAT status", func(inv *bill.Invoice) {
			inv.Supplier.Ext = tax.Extensions{}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-14"},
		{"supplier with an unknown VAT status", func(inv *bill.Invoice) {
			inv.Supplier.Ext = tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyVATStatus: "exempt"})
		}, "GOBL-HR-ERACUN-BILL-INVOICE-14"},
		{"supplier tax ID without its OIB", func(inv *bill.Invoice) {
			inv.Supplier.TaxID = &tax.Identity{Country: "HR"}
			inv.Supplier.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:12345678903"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-15"},
		{"supplier without a tax ID", func(inv *bill.Invoice) {
			inv.Supplier.TaxID = nil
			inv.Supplier.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:12345678903"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-15"},
		{"customer VAT number with an O line", func(inv *bill.Invoice) {
			outsideVAT(inv)
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-16"},
		{"line without KPD", func(inv *bill.Invoice) { inv.Lines[0].Item.Identities = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-12"},
		{"KPD without dots", func(inv *bill.Invoice) {
			inv.Lines[0].Item.Identities = []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "622020"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-25"},
		{"KPD with four digits", func(inv *bill.Invoice) {
			inv.Lines[0].Item.Identities = []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "62.20"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-25"},
		{"KPD with eight digits", func(inv *bill.Invoice) {
			inv.Lines[0].Item.Identities = []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "62.20.20.01"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-25"},
		{"second KPD without dots", func(inv *bill.Invoice) {
			inv.Lines[0].Item.Identities = append(inv.Lines[0].Item.Identities,
				&org.Identity{Label: eracun.IdentityLabelKPD, Code: "622020"})
		}, "GOBL-HR-ERACUN-BILL-INVOICE-25"},
		{"rate without HR tax category", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes[0].Rate = ""
			inv.Lines[0].Taxes[0].Percent = num.NewPercentage(100, 3)
		}, "GOBL-HR-ERACUN-TAX-COMBO-01"},
		{"standard rate of another country", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes[0].Country = "DE"
		}, "GOBL-HR-ERACUN-TAX-COMBO-01"},
		{"HR tax category that doesn't match the rate", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes[0].Rate = ""
			inv.Lines[0].Taxes[0].Percent = num.NewPercentage(100, 3)
			inv.Lines[0].Taxes[0].Ext = tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxLabelPDV25})
		}, "GOBL-HR-ERACUN-TAX-COMBO-01"},
		{"pass-through category on a standard combo", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes[0].Ext = tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxLabelPP})
		}, "GOBL-HR-ERACUN-TAX-COMBO-02"},
		{"VAT from a supplier not registered for VAT", func(inv *bill.Invoice) {
			outsideVAT(inv)
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-13"},
		{"PP above 3%", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, consumptionTax(num.MakePercentage(31, 3)))
		}, "GOBL-HR-ERACUN-TAX-COMBO-03"},
		{"negative PP", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, consumptionTax(num.MakePercentage(-3, 2)))
		}, "GOBL-HR-ERACUN-TAX-COMBO-03"},
		{"PP without VAT on its line", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes = tax.Set{consumptionTax(num.MakePercentage(3, 2))}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-26"},
		{"PP on a document discount", func(inv *bill.Invoice) {
			inv.Discounts = []*bill.Discount{{Amount: num.MakeAmount(500, 2), Taxes: tax.Set{
				{Category: tax.CategoryVAT, Rate: tax.RateGeneral}, consumptionTax(num.MakePercentage(3, 2)),
			}}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-28"},
		{"PP on a document charge", func(inv *bill.Invoice) {
			inv.Charges = []*bill.Charge{{Amount: num.MakeAmount(500, 2), Taxes: tax.Set{
				{Category: tax.CategoryVAT, Rate: tax.RateGeneral}, consumptionTax(num.MakePercentage(3, 2)),
			}}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-29"},
		{"PP on a line beside a charge labelled PP", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, consumptionTax(num.MakePercentage(3, 2)))
			inv.Charges = []*bill.Charge{{Reason: "Porez na potrošnju", Amount: num.MakeAmount(300, 2), Ext: outsideVATLabel(eracun.TaxLabelPP)}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-30"},
		{"PP on a line beside a line labelled PP", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, consumptionTax(num.MakePercentage(3, 2)))
			inv.Lines = append(inv.Lines, lineOutsideVAT("Porez na potrošnju", 300, eracun.TaxLabelPP))
		}, "GOBL-HR-ERACUN-BILL-INVOICE-30"},
		{"VATEX code the Tax Administration doesn't list", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt,
				Ext: tax.ExtensionsOf(cbc.CodeMap{cef.ExtKeyVATEX: "VATEX-EU-135-1"})}}
		}, "GOBL-HR-ERACUN-TAX-COMBO-04"},
		{"VATEX code that is not a code", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt,
				Ext: tax.ExtensionsOf(cbc.CodeMap{cef.ExtKeyVATEX: "VATEX-EU-79-X"})}}
		}, "GOBL-HR-ERACUN-TAX-COMBO-04"},
		{"pass-through category on an exempt combo", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt, Ext: outsideVATLabel(eracun.TaxLabelPOVNAK)}}
			inv.Tax.Notes = []*tax.Note{{Category: tax.CategoryVAT, Key: tax.KeyExempt, Text: "Povratna naknada"}}
		}, "GOBL-HR-ERACUN-TAX-COMBO-02"},
		{"line without taxes and without a label", func(inv *bill.Invoice) {
			l := lineOutsideVAT("Povratna naknada", 500, eracun.TaxLabelPOVNAK)
			l.Ext = tax.Extensions{}
			inv.Lines = append(inv.Lines, l)
		}, "GOBL-HR-ERACUN-BILL-INVOICE-33"},
		{"line without taxes labelled as a VAT category", func(inv *bill.Invoice) {
			inv.Lines = append(inv.Lines, lineOutsideVAT("Povratna naknada", 500, eracun.TaxLabelE))
		}, "GOBL-HR-ERACUN-BILL-INVOICE-33"},
		{"charge without taxes labelled as a VAT category", func(inv *bill.Invoice) {
			inv.Charges = []*bill.Charge{{Reason: "Dostava", Amount: num.MakeAmount(500, 2), Ext: outsideVATLabel(eracun.TaxLabelPDV25)}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-34"},
		{"charge without taxes and without a reason", func(inv *bill.Invoice) {
			inv.Charges = []*bill.Charge{{Amount: num.MakeAmount(500, 2)}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-27"},
		{"line with taxes and a label of its own", func(inv *bill.Invoice) {
			inv.Lines[0].Ext = outsideVATLabel(eracun.TaxLabelPOVNAK)
		}, "GOBL-HR-ERACUN-BILL-INVOICE-35"},
		{"charge with taxes and a label of its own", func(inv *bill.Invoice) {
			inv.Charges = []*bill.Charge{{Reason: "Dostava", Amount: num.MakeAmount(500, 2), Ext: outsideVATLabel(eracun.TaxLabelN),
				Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-36"},
		{"deposit beside an O line", func(inv *bill.Invoice) {
			inv.Customer.TaxID = &tax.Identity{Country: "HR"}
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
			inv.Tax.Notes = []*tax.Note{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope, Text: "Nije predmet oporezivanja"}}
			inv.Lines = append(inv.Lines, lineOutsideVAT("Povratna naknada", 500, eracun.TaxLabelPOVNAK))
		}, "GOBL-HR-ERACUN-BILL-INVOICE-32"},
		{"deposit beside an export, customer without a tax ID", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExport}}
			inv.Tax.Notes = []*tax.Note{{Category: tax.CategoryVAT, Key: tax.KeyExport, Text: "Izvoz"}}
			inv.Lines = append(inv.Lines, lineOutsideVAT("Povratna naknada", 500, eracun.TaxLabelPOVNAK))
		}, "GOBL-HR-ERACUN-BILL-INVOICE-10"},
		{"PP beside an O line", func(inv *bill.Invoice) {
			inv.Supplier.TaxID = nil
			inv.Supplier.Identities = []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "12345678903"}}
			inv.Customer.TaxID = &tax.Identity{Country: "HR"}
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}, consumptionTax(num.MakePercentage(3, 2))}
			inv.Tax.Notes = []*tax.Note{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope, Text: "Nije predmet oporezivanja"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-32"},
		{"PP from a supplier not registered for VAT", func(inv *bill.Invoice) {
			outsideVAT(inv)
			inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, consumptionTax(num.MakePercentage(3, 2)))
		}, "GOBL-HR-ERACUN-BILL-INVOICE-31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := testInvoice()
			tt.modify(inv)
			require.NoError(t, inv.Calculate())
			assertFault(t, inv, tt.code)
		})
	}

	// The EN 16931 addon adds the tax with the document type while
	// calculating, so it's removed afterwards.
	t.Run("no tax", func(t *testing.T) {
		inv := testInvoice()
		require.NoError(t, inv.Calculate())
		inv.Tax = nil
		assertFault(t, inv, "GOBL-HR-ERACUN-BILL-INVOICE-03")
	})
}

func TestInvoiceKPDNotRequiredOnCreditNotes(t *testing.T) {
	inv := testInvoice()
	inv.Type = bill.InvoiceTypeCreditNote
	inv.Preceding = []*org.DocumentRef{{Code: "1-P1-1", IssueDate: cal.NewDate(2026, 2, 1)}}
	inv.Lines[0].Item.Identities = nil
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
}

// A document type that needs no KPD still has its KPD codes checked: the
// official code list rule (HR-BR-CL-2) applies to every CG code.
func TestInvoiceKPDFormOnCreditNotes(t *testing.T) {
	inv := testInvoice()
	inv.Type = bill.InvoiceTypeCreditNote
	inv.Preceding = []*org.DocumentRef{{Code: "1-P1-1", IssueDate: cal.NewDate(2026, 2, 1)}}
	inv.Lines[0].Item.Identities = []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "622020"}}
	require.NoError(t, inv.Calculate())
	assertFault(t, inv, "GOBL-HR-ERACUN-BILL-INVOICE-25")
}

// Only the KPD identity is held to the KPD form; the item's other
// classifications and identities keep their own.
func TestInvoiceKPDFormOnlyOnKPD(t *testing.T) {
	inv := testInvoice()
	inv.Lines[0].Item.Identities = append(inv.Lines[0].Item.Identities,
		&org.Identity{Label: "STI", Code: "8471"},
		&org.Identity{Code: "ABC-123"},
	)
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
}

func TestTaxComboNormalization(t *testing.T) {
	tests := []struct {
		combo *tax.Combo
		want  cbc.Code
	}{
		{&tax.Combo{Category: tax.CategoryVAT, Rate: tax.RateGeneral}, eracun.TaxLabelPDV25},
		{&tax.Combo{Category: tax.CategoryVAT, Rate: tax.RateReduced}, eracun.TaxLabelPDV13},
		{&tax.Combo{Category: tax.CategoryVAT, Rate: tax.RateSuperReduced}, eracun.TaxLabelPDV5},
		{&tax.Combo{Category: tax.CategoryVAT, Percent: num.NewPercentage(250, 3)}, eracun.TaxLabelPDV25},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyZero}, eracun.TaxLabelZ},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyIntraCommunity}, eracun.TaxLabelK},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyExport}, eracun.TaxLabelG},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyReverseCharge}, eracun.TaxLabelAE},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyExempt}, eracun.TaxLabelE},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}, eracun.TaxLabelO},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyExempt, Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxLabelPP})}, eracun.TaxLabelPP},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyZero, Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxLabelPDV25})}, eracun.TaxLabelZ},
	}
	for _, tt := range tests {
		t.Run(tt.want.String(), func(t *testing.T) {
			inv := testInvoice()
			inv.Lines[0].Taxes = tax.Set{tt.combo}
			require.NoError(t, inv.Calculate())
			assert.Equal(t, tt.want, inv.Lines[0].Taxes[0].Ext.Get(eracun.ExtKeyTaxCategory))
		})
	}
}

// TestEndpointFromInboxScheme checks that an inbox in another ISO 6523
// scheme than 9934, such as a GLN, which AMS also registers, is the
// address, and that a customer with an OIB in its VAT number may use it.
func TestEndpointFromInboxScheme(t *testing.T) {
	inv := testInvoice()
	inv.Customer.Inboxes = []*org.Inbox{{Scheme: "0088", Code: "3850001234567"}}
	require.NoError(t, inv.Calculate())
	require.Empty(t, inv.Customer.Endpoints)
	require.NoError(t, rules.Validate(inv))
}

// TestInvalidAddressOneFault checks that an address that is not a valid OIB
// is reported once, as such, and not also as another party's OIB.
func TestInvalidAddressOneFault(t *testing.T) {
	inv := testInvoice()
	inv.Customer.Inboxes = []*org.Inbox{{Scheme: "9934", Code: "11111111118"}}
	require.NoError(t, inv.Calculate())
	faults := rules.Validate(inv)
	require.Error(t, faults)
	assert.True(t, faults.HasCode("GOBL-HR-ERACUN-BILL-INVOICE-20"), "got %s", faults)
	assert.False(t, faults.HasCode("GOBL-HR-ERACUN-BILL-INVOICE-21"), "got %s", faults)
}

// TestAddressSkipsIncompleteEndpoint checks that the rules read the address
// the export writes: an ISO 6523 endpoint without a code is skipped, as the
// export skips it, and the inbox after it is the address. The EN 16931
// addon reports the incomplete endpoint itself.
func TestAddressSkipsIncompleteEndpoint(t *testing.T) {
	inv := testInvoice()
	inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934"}}
	inv.Customer.Inboxes = []*org.Inbox{{Scheme: "9934", Code: "11111111119"}}
	require.NoError(t, inv.Calculate())
	faults := rules.Validate(inv)
	require.Error(t, faults)
	assert.True(t, faults.HasCode("GOBL-EU-EN16931-ORG-ENDPOINT-01"), "got %s", faults)
	assert.False(t, faults.HasCode("GOBL-HR-ERACUN-BILL-INVOICE-20"), "got %s", faults)
}

func TestEndpointNotDerivedWhenPresent(t *testing.T) {
	inv := testInvoice()
	inv.Supplier.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::0088:1234567890123"}}
	require.NoError(t, inv.Calculate())
	require.Len(t, inv.Supplier.Endpoints, 1)
	assert.Equal(t, cbc.URI("iso6523-actorid-upis::0088:1234567890123"), inv.Supplier.Endpoints[0].URI)
	require.NoError(t, rules.Validate(inv))
}

func TestOperator(t *testing.T) {
	contact := &org.Person{
		Name:       &org.Name{Given: "Ana"},
		Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}},
	}
	operator := &org.Person{Key: eracun.PersonKeyOperator, Name: &org.Name{Given: "Operater1"}}
	assert.Same(t, operator, eracun.Operator([]*org.Person{nil, contact, operator}))
	assert.Nil(t, eracun.Operator([]*org.Person{contact}))
	assert.Nil(t, eracun.Operator(nil))
	other := &org.Person{Key: eracun.PersonKeyOperator, Name: &org.Name{Given: "Operater2"}}
	assert.Nil(t, eracun.Operator([]*org.Person{operator, other}))
}

// TestOperatorCompanyOIB checks that the operator may carry the supplier's
// own OIB, which Art. 8(4) of the Fiscalisation Act allows in some cases.
func TestOperatorCompanyOIB(t *testing.T) {
	inv := testInvoice()
	inv.Supplier.People[0].Name = &org.Name{Given: "Web shop"}
	inv.Supplier.People[0].Identities = []*org.Identity{{Type: hr.IdentityTypeOIB, Code: inv.Supplier.TaxID.Code}}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
}

// TestOperatorVATNumberAsOIB checks that the supplier's VAT number given as
// the operator's OIB is read as the bare OIB, as the hr regime cleans it.
func TestOperatorVATNumberAsOIB(t *testing.T) {
	inv := testInvoice()
	inv.Supplier.People[0].Identities = []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "HR 1234-5678-903"}}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
	assert.Equal(t, cbc.Code("12345678903"), inv.Supplier.People[0].Identities[0].Code)
}

func TestOperatorName(t *testing.T) {
	assert.Equal(t, "Blagajna 1", eracun.OperatorName(&org.Name{Given: "Ivan", Surname: "Kovač", Alias: "Blagajna 1"}))
	assert.Equal(t, "Ivan Kovač", eracun.OperatorName(&org.Name{Given: "Ivan", Surname: "Kovač"}))
	assert.Equal(t, "Ivan", eracun.OperatorName(&org.Name{Given: "Ivan"}))
	assert.Equal(t, "Kovač", eracun.OperatorName(&org.Name{Surname: "Kovač"}))
	assert.Empty(t, eracun.OperatorName(nil))
}

// testCorrectedInvoice provides a valid invoice with a contact on either side
// of the operator, corrected as a credit note.
func testCorrectedInvoice(t *testing.T) *bill.Invoice {
	t.Helper()
	inv := testInvoice()
	inv.Supplier.People = []*org.Person{
		{Name: &org.Name{Given: "Ana"}},
		inv.Supplier.People[0],
		{Name: &org.Name{Given: "Marko"}},
	}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
	require.NoError(t, inv.Correct(bill.Credit, bill.WithIssueDate(cal.MakeDate(2026, 3, 15))))
	return inv
}

// TestCorrectionLeavesOutOperator checks that a correction drops the original
// operator and keeps the other supplier people in order, so the corrective
// document needs its own operator.
func TestCorrectionLeavesOutOperator(t *testing.T) {
	inv := testCorrectedInvoice(t)
	require.Len(t, inv.Supplier.People, 2)
	assert.Equal(t, "Ana", inv.Supplier.People[0].Name.Given)
	assert.Equal(t, "Marko", inv.Supplier.People[1].Name.Given)
	assertFault(t, inv, "GOBL-HR-ERACUN-BILL-INVOICE-06")
}

func TestCorrectionWithNewOperator(t *testing.T) {
	inv := testCorrectedInvoice(t)
	inv.Supplier.People = append(inv.Supplier.People, &org.Person{
		Key:        eracun.PersonKeyOperator,
		Name:       &org.Name{Given: "Operater2"},
		Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}},
	})
	inv.IssueTime = cal.NewTime(9, 0, 0)
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
}

// TestCorrectionDefinition checks that the addon's correction definition
// holds only its normalizer, so the correction types, stamps and tax copy
// stay as the regime and the other addons define them.
func TestCorrectionDefinition(t *testing.T) {
	def := tax.AddonForKey(eracun.V1).Corrections.Def(bill.ShortSchemaInvoice)
	require.NotNil(t, def)
	require.NotNil(t, def.Normalizer)
	rest := *def
	rest.Normalizer = nil
	assert.Equal(t, tax.CorrectionDefinition{Schema: bill.ShortSchemaInvoice}, rest)
}

func testStatus() *bill.Status {
	return &bill.Status{
		Regime:    tax.WithRegime("HR"),
		Addons:    tax.WithAddons(eracun.V1),
		Type:      bill.StatusTypeResponse,
		Code:      "789",
		IssueDate: cal.MakeDate(2026, 7, 13),
		Supplier: &org.Party{
			TaxID: &tax.Identity{Country: "HR", Code: "12345678903"},
		},
		Customer: &org.Party{
			TaxID: &tax.Identity{Country: "HR", Code: "11111111119"},
		},
		Lines: []*bill.StatusLine{
			{
				Key:         bill.StatusLineRejected,
				Doc:         &org.DocumentRef{Code: "123-1-1", IssueDate: cal.NewDate(2026, 7, 10)},
				Description: "Tekst razloga odbijanja",
				Reasons: []*bill.Reason{
					{Key: bill.ReasonKeyOther, Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyRejectionReason: eracun.RejectionReasonU})},
				},
			},
		},
	}
}

func TestStatusValid(t *testing.T) {
	st := testStatus()
	require.NoError(t, st.Calculate())
	require.NoError(t, rules.Validate(st))
	assert.Equal(t, cbc.URI("iso6523-actorid-upis::9934:11111111119"), st.Customer.Endpoints[0].URI)
}

func TestStatusRules(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*bill.Status)
		code   rules.Code
	}{
		{"supplier without endpoint", func(st *bill.Status) { st.Supplier.TaxID = &tax.Identity{Country: "HR"} }, "GOBL-HR-ERACUN-BILL-STATUS-01"},
		{"supplier with only an email endpoint", func(st *bill.Status) {
			st.Supplier.TaxID = &tax.Identity{Country: "HR"}
			st.Supplier.Endpoints = []*org.Endpoint{{URI: "mailto:racuni@example.com"}}
		}, "GOBL-HR-ERACUN-BILL-STATUS-01"},
		{"no customer", func(st *bill.Status) { st.Customer = nil }, "GOBL-HR-ERACUN-BILL-STATUS-02"},
		{"customer without endpoint", func(st *bill.Status) { st.Customer.TaxID = &tax.Identity{Country: "HR"} }, "GOBL-HR-ERACUN-BILL-STATUS-03"},
		{"customer with only an email endpoint", func(st *bill.Status) {
			st.Customer.TaxID = &tax.Identity{Country: "HR"}
			st.Customer.Endpoints = []*org.Endpoint{{URI: "mailto:racuni@example.com"}}
		}, "GOBL-HR-ERACUN-BILL-STATUS-03"},
		{"supplier address with an invalid OIB", func(st *bill.Status) {
			st.Supplier.Inboxes = []*org.Inbox{{Scheme: "9934", Code: "12345"}}
		}, "GOBL-HR-ERACUN-BILL-STATUS-15"},
		{"supplier address with another party's OIB", func(st *bill.Status) {
			st.Supplier.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
		}, "GOBL-HR-ERACUN-BILL-STATUS-16"},
		{"customer address with an invalid OIB", func(st *bill.Status) {
			st.Customer.Inboxes = []*org.Inbox{{Scheme: "9934", Code: "11111111118"}}
		}, "GOBL-HR-ERACUN-BILL-STATUS-17"},
		{"customer address with another party's OIB", func(st *bill.Status) {
			st.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:12345678903"}}
		}, "GOBL-HR-ERACUN-BILL-STATUS-18"},
		{"error", func(st *bill.Status) { st.Lines[0].Key = bill.StatusLineError }, "GOBL-HR-ERACUN-BILL-STATUS-04"},
		{"issued", func(st *bill.Status) { st.Lines[0].Key = bill.StatusLineIssued }, "GOBL-HR-ERACUN-BILL-STATUS-04"},
		{"no description", func(st *bill.Status) { st.Lines[0].Description = "" }, "GOBL-HR-ERACUN-BILL-STATUS-05"},
		{"no rejection reason", func(st *bill.Status) { st.Lines[0].Reasons[0].Ext = tax.Extensions{} }, "GOBL-HR-ERACUN-BILL-STATUS-06"},
		{"two rejection reasons", func(st *bill.Status) {
			st.Lines[0].Reasons = append(st.Lines[0].Reasons, st.Lines[0].Reasons[0])
		}, "GOBL-HR-ERACUN-BILL-STATUS-06"},
		{"invalid rejection reason", func(st *bill.Status) {
			st.Lines[0].Reasons[0].Ext = tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyRejectionReason: "X"})
		}, "GOBL-HR-ERACUN-BILL-REASON-01"},
		{"no invoice", func(st *bill.Status) { st.Lines[0].Doc = nil }, "GOBL-HR-ERACUN-BILL-STATUS-07"},
		{"no invoice code", func(st *bill.Status) { st.Lines[0].Doc.Code = "" }, "GOBL-HR-ERACUN-BILL-STATUS-08"},
		{"no invoice issue date", func(st *bill.Status) { st.Lines[0].Doc.IssueDate = nil }, "GOBL-HR-ERACUN-BILL-STATUS-09"},
		{"rejection with an action", func(st *bill.Status) {
			st.Lines[0].Actions = []*bill.Action{{Key: bill.ActionKeyReissue}}
		}, "GOBL-HR-ERACUN-BILL-STATUS-11"},
		{"rejection with another reason", func(st *bill.Status) {
			st.Lines[0].Reasons = append(st.Lines[0].Reasons, &bill.Reason{Key: bill.ReasonKeyPrices})
		}, "GOBL-HR-ERACUN-BILL-STATUS-11"},
		{"update", func(st *bill.Status) { st.Type = bill.StatusTypeUpdate }, "GOBL-HR-ERACUN-BILL-STATUS-13"},
		{"two invoices", func(st *bill.Status) { st.Lines = append(st.Lines, testStatus().Lines[0]) }, "GOBL-HR-ERACUN-BILL-STATUS-14"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := testStatus()
			tt.modify(st)
			require.NoError(t, st.Calculate())
			assertFault(t, st, tt.code)
		})
	}

	t.Run("description on the reason", func(t *testing.T) {
		st := testStatus()
		st.Lines[0].Description = ""
		st.Lines[0].Reasons[0].Description = "Tekst razloga odbijanja"
		require.NoError(t, st.Calculate())
		require.NoError(t, rules.Validate(st))
	})
}

func TestStatusOtherResponses(t *testing.T) {
	keys := []cbc.Key{
		bill.StatusLineAccepted,
		bill.StatusLineAcknowledged,
		bill.StatusLineProcessing,
		bill.StatusLineQuerying,
		bill.StatusLinePaid,
	}
	for _, key := range keys {
		t.Run(key.String(), func(t *testing.T) {
			st := testStatus()
			st.Lines[0].Key = key
			st.Lines[0].Description = ""
			st.Lines[0].Reasons = nil
			require.NoError(t, st.Calculate())
			require.NoError(t, rules.Validate(st))
		})
	}

	t.Run("accepted with a rejection reason", func(t *testing.T) {
		st := testStatus()
		st.Lines[0].Key = bill.StatusLineAccepted
		require.NoError(t, st.Calculate())
		assertFault(t, st, "GOBL-HR-ERACUN-BILL-STATUS-10")
	})

	// The application response has one status, with a code and no text.
	query := func(reasons []*bill.Reason, actions []*bill.Action) *bill.Status {
		st := testStatus()
		st.Lines[0].Key = bill.StatusLineQuerying
		st.Lines[0].Description = "Nedostaje broj narudžbenice."
		st.Lines[0].Reasons = reasons
		st.Lines[0].Actions = actions
		return st
	}
	t.Run("querying with a reason", func(t *testing.T) {
		st := query([]*bill.Reason{{Key: bill.ReasonKeyReferences}}, nil)
		require.NoError(t, st.Calculate())
		require.NoError(t, rules.Validate(st))
	})
	t.Run("querying with an action", func(t *testing.T) {
		st := query(nil, []*bill.Action{{Key: bill.ActionKeyProvide}})
		require.NoError(t, st.Calculate())
		require.NoError(t, rules.Validate(st))
	})
	t.Run("querying with a reason and an action", func(t *testing.T) {
		st := query([]*bill.Reason{{Key: bill.ReasonKeyReferences}}, []*bill.Action{{Key: bill.ActionKeyProvide}})
		require.NoError(t, st.Calculate())
		assertFault(t, st, "GOBL-HR-ERACUN-BILL-STATUS-11")
	})
	t.Run("querying with a reason text", func(t *testing.T) {
		st := query([]*bill.Reason{{Key: bill.ReasonKeyReferences, Description: "Nedostaje broj narudžbenice."}}, nil)
		require.NoError(t, st.Calculate())
		assertFault(t, st, "GOBL-HR-ERACUN-BILL-STATUS-12")
	})
	t.Run("querying with an action text", func(t *testing.T) {
		st := query(nil, []*bill.Action{{Key: bill.ActionKeyProvide, Description: "Pošaljite broj narudžbenice."}})
		require.NoError(t, st.Calculate())
		assertFault(t, st, "GOBL-HR-ERACUN-BILL-STATUS-12")
	})
}

// nowInCroatia gives the current date and time in Croatia, as the issue time
// normalizer sees it.
func nowInCroatia() cal.DateTime {
	return cal.ThisSecondIn(tax.RegimeDefFor(l10n.HR).TimeLocation())
}

// assertIssuedNow checks the invoice has the issue date and time of a moment
// between before and now, in Croatia.
func assertIssuedNow(t *testing.T, inv *bill.Invoice, before cal.DateTime) {
	t.Helper()
	after := nowInCroatia()
	require.NotNil(t, inv.IssueTime)
	got := inv.IssueDate.WithTime(*inv.IssueTime)
	assert.False(t, got.Before(before.DateTime), "issued %s before %s", got, before)
	assert.False(t, got.After(after.DateTime), "issued %s after %s", got, after)
}

func TestInvoiceIssueTime(t *testing.T) {
	t.Run("no issue date or time", func(t *testing.T) {
		inv := testInvoice()
		inv.IssueDate = cal.Date{}
		inv.IssueTime = nil
		before := nowInCroatia()
		require.NoError(t, inv.Calculate())
		assertIssuedNow(t, inv, before)
		require.NoError(t, rules.Validate(inv))
	})
	t.Run("dated today without a time", func(t *testing.T) {
		before := nowInCroatia()
		inv := testInvoice()
		inv.IssueDate = before.Date()
		inv.IssueTime = nil
		require.NoError(t, inv.Calculate())
		assertIssuedNow(t, inv, before)
		require.NoError(t, rules.Validate(inv))
	})
	t.Run("dated another day without a time", func(t *testing.T) {
		inv := testInvoice()
		inv.IssueTime = nil
		require.NoError(t, inv.Calculate())
		assert.Equal(t, "2026-03-01", inv.IssueDate.String())
		assert.Nil(t, inv.IssueTime)
		assertFault(t, inv, "GOBL-HR-ERACUN-BILL-INVOICE-02")
	})
	t.Run("time kept", func(t *testing.T) {
		inv := testInvoice()
		inv.IssueDate = cal.Date{}
		inv.IssueTime = cal.NewTime(0, 0, 0)
		require.NoError(t, inv.Calculate())
		assert.Equal(t, "00:00:00", inv.IssueTime.String())
	})
}

// roundingApartInvoice has a 1.21 dish at 13% and a 10.03 wine at 25%. The
// VAT is 0.1573 + 2.5075: 2.66 rounded once, as the precise rule does, but
// 0.16 + 2.51 = 2.67 rounded per rate, as the VAT breakdown shows it.
func roundingApartInvoice() *bill.Invoice {
	inv := testInvoice()
	line := func(name string, cents int64, rate cbc.Key) *bill.Line {
		return &bill.Line{
			Quantity: num.MakeAmount(1, 0),
			Item: &org.Item{
				Name:       name,
				Price:      num.NewAmount(cents, 2),
				Identities: []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "56.10.11"}},
			},
			Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: rate}},
		}
	}
	inv.Lines = []*bill.Line{line("Jelo", 121, tax.RateReduced), line("Vino", 1003, tax.RateGeneral)}
	return inv
}

func TestInvoiceRounding(t *testing.T) {
	t.Run("currency when not given", func(t *testing.T) {
		inv := roundingApartInvoice()
		require.NoError(t, inv.Calculate())
		assert.Equal(t, tax.RoundingRuleCurrency, inv.Tax.Rounding)
		vat := inv.Totals.Taxes.Category(tax.CategoryVAT)
		assert.Equal(t, "2.67", vat.Amount.String())
		assert.Equal(t, "0.16", vat.Rates[0].Amount.String())
		assert.Equal(t, "2.51", vat.Rates[1].Amount.String())
		assert.Equal(t, "13.91", inv.Totals.Payable.String())
		require.NoError(t, rules.Validate(inv))
	})
	t.Run("invoice without tax", func(t *testing.T) {
		inv := testInvoice()
		inv.Tax = nil
		require.NoError(t, inv.Calculate())
		require.NotNil(t, inv.Tax)
		assert.Equal(t, tax.RoundingRuleCurrency, inv.Tax.Rounding)
	})
	t.Run("currency kept", func(t *testing.T) {
		inv := roundingApartInvoice()
		inv.Tax.Rounding = tax.RoundingRuleCurrency
		require.NoError(t, inv.Calculate())
		assert.Equal(t, tax.RoundingRuleCurrency, inv.Tax.Rounding)
		require.NoError(t, rules.Validate(inv))
	})
	t.Run("precise refused", func(t *testing.T) {
		inv := roundingApartInvoice()
		inv.Tax.Rounding = tax.RoundingRulePrecise
		require.NoError(t, inv.Calculate())
		assert.Equal(t, tax.RoundingRulePrecise, inv.Tax.Rounding)
		assert.Equal(t, "2.66", inv.Totals.Taxes.Category(tax.CategoryVAT).Amount.String())
		assertFault(t, inv, "GOBL-HR-ERACUN-BILL-INVOICE-24")
	})
}

func TestCorrectionIssueTime(t *testing.T) {
	t.Run("issued now", func(t *testing.T) {
		inv := testInvoice()
		require.NoError(t, inv.Calculate())
		before := nowInCroatia()
		require.NoError(t, inv.Correct(bill.Credit))
		assertIssuedNow(t, inv, before)
	})
	t.Run("normalizer clears the original issue date and time", func(t *testing.T) {
		inv := testInvoice()
		def := tax.AddonForKey(eracun.V1).Corrections.Def(bill.ShortSchemaInvoice)
		def.Normalizer.Normalize(&bill.CorrectionNormalize{Opts: new(bill.CorrectionOptions), Invoice: inv})
		assert.True(t, inv.IssueDate.IsZero())
		assert.Nil(t, inv.IssueTime)
	})
	t.Run("issue date option", func(t *testing.T) {
		inv := testCorrectedInvoice(t)
		assert.Equal(t, "2026-03-15", inv.IssueDate.String())
		assert.Nil(t, inv.IssueTime)
		assertFault(t, inv, "GOBL-HR-ERACUN-BILL-INVOICE-02")
	})
}

// outsideVAT makes the supplier one not registered for VAT, a small
// taxpayer: its OIB stays in its tax ID, its VAT status says so, and its
// line is exempt. The hr-fiskal-b2c-v1 addon takes the same party.
func outsideVAT(inv *bill.Invoice) {
	inv.Supplier.Ext = tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyVATStatus: eracun.VATStatusNotRegistered})
	inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt}}
	inv.Tax.Notes = []*tax.Note{{
		Category: tax.CategoryVAT,
		Key:      tax.KeyExempt,
		Text:     "Oslobođeno PDV-a temeljem čl. 90. st. 1. Zakona o porezu na dodanu vrijednost",
	}}
}

func TestSupplierNotRegisteredForVAT(t *testing.T) {
	inv := testInvoice()
	outsideVAT(inv)
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
	assert.True(t, eracun.NotRegisteredForVAT(inv.Supplier))
	assert.False(t, eracun.RegisteredForVAT(inv.Supplier))
	assert.Equal(t, cbc.Code("12345678903"), eracun.PartyOIB(inv.Supplier))
	assert.Empty(t, inv.Supplier.Identities, "the OIB stays in the tax ID")
	require.Len(t, inv.Supplier.Endpoints, 1)
	assert.Equal(t, cbc.URI("iso6523-actorid-upis::9934:12345678903"), inv.Supplier.Endpoints[0].URI)
}

// TestOutsideScopeWithoutVATNumbers checks that an invoice with an O amount
// is valid when neither party has a VAT number (BR-O-02), the shape of the
// official example "Neoporezivi racun".
func TestOutsideScopeWithoutVATNumbers(t *testing.T) {
	inv := testInvoice()
	outsideVAT(inv)
	inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
	inv.Tax.Notes = []*tax.Note{{
		Category: tax.CategoryVAT,
		Key:      tax.KeyOutsideScope,
		Text:     "Neoporezivo",
	}}
	inv.Customer.TaxID = nil
	inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
}

// TestOutsideScopeFromRegisteredSupplier checks that a supplier registered
// for VAT may issue an invoice with an O amount: the export leaves its VAT
// identifier out (BR-O-02), so the supplier doesn't have to.
func TestOutsideScopeFromRegisteredSupplier(t *testing.T) {
	inv := testInvoice()
	inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
	inv.Tax.Notes = []*tax.Note{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope, Text: "Neoporezivo"}}
	inv.Customer.TaxID = nil
	inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
}

// TestSupplierKeepsOIBInTaxID checks that the supplier's OIB is read from
// its tax ID and no identity is added for it.
func TestSupplierKeepsOIBInTaxID(t *testing.T) {
	inv := testInvoice()
	require.NoError(t, inv.Calculate())
	assert.Empty(t, inv.Supplier.Identities)
	assert.True(t, eracun.RegisteredForVAT(inv.Supplier))
	assert.Equal(t, cbc.Code("12345678903"), eracun.PartyOIB(inv.Supplier))
}

// TestEndpointNotDerivedBesideSchemeInbox checks that an OIB doesn't replace
// the ISO 6523 address a party already has in an inbox.
func TestEndpointNotDerivedBesideSchemeInbox(t *testing.T) {
	inv := testInvoice()
	outsideVAT(inv)
	inv.Supplier.Inboxes = []*org.Inbox{{Scheme: "0088", Code: "3850001234567"}}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
	assert.Empty(t, inv.Supplier.Endpoints)
}

// consumptionTax is the hr regime's consumption tax (PP) at a percent.
func consumptionTax(p num.Percentage) *tax.Combo {
	return &tax.Combo{Category: hr.TaxCategoryPP, Percent: &p}
}

func TestConsumptionTaxValid(t *testing.T) {
	for _, p := range []num.Percentage{num.MakePercentage(0, 2), num.MakePercentage(25, 3), num.MakePercentage(3, 2)} {
		t.Run(p.String(), func(t *testing.T) {
			inv := testInvoice()
			inv.Lines[0].Taxes = append(inv.Lines[0].Taxes, consumptionTax(p))
			// A line's own attributes stay.
			inv.Lines[0].Item.Attributes = []*org.Attribute{{Label: "Zapremina", Text: "0,5 l"}}
			require.NoError(t, inv.Calculate())
			require.NoError(t, rules.Validate(inv))
		})
	}
}

// outsideVATLabel is the HR tax category of an amount left out of the VAT
// base, as the extensions of the line or charge that carries it.
func outsideVATLabel(label cbc.Code) tax.Extensions {
	return tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: label})
}

// lineOutsideVAT is a line left out of the VAT base: one with no taxes and
// its HR tax category on the line.
func lineOutsideVAT(name string, cents int64, label cbc.Code) *bill.Line {
	return &bill.Line{
		Quantity: num.MakeAmount(1, 0),
		Item: &org.Item{
			Name:       name,
			Price:      num.NewAmount(cents, 2),
			Identities: []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "22.22.19"}},
		},
		Ext: outsideVATLabel(label),
	}
}

// TestAmountsOutsideVAT checks the two ways to write an amount left out of
// the VAT base: a line of its own or a document level charge, neither with
// taxes.
func TestAmountsOutsideVAT(t *testing.T) {
	inv := testInvoice()
	inv.Lines = append(inv.Lines, lineOutsideVAT("Povratna naknada", 10, eracun.TaxLabelPOVNAK))
	inv.Lines[1].Quantity = num.MakeAmount(240, 0)
	inv.Charges = []*bill.Charge{
		{Reason: "Turistička pristojba", Amount: num.MakeAmount(1000, 2)},
		{Reason: "Posebni porez na motorna vozila", Amount: num.MakeAmount(200000, 2), Ext: outsideVATLabel(eracun.TaxLabelPPMV)},
	}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
	assert.True(t, eracun.LineOutsideVAT(inv.Lines[1]))
	assert.False(t, eracun.LineOutsideVAT(inv.Lines[0]))
	assert.Equal(t, eracun.TaxLabelN, inv.Charges[0].Ext.Get(eracun.ExtKeyTaxCategory), "a charge without taxes is another fee not taxed, unless it says otherwise")
	assert.Equal(t, eracun.TaxLabelPPMV, inv.Charges[1].Ext.Get(eracun.ExtKeyTaxCategory))
	assert.True(t, eracun.ChargeOutsideVAT(inv.Charges[0]))
	// The VAT totals hold the taxed line alone.
	assert.Equal(t, "100.00", inv.Totals.Taxes.Category(tax.CategoryVAT).Rates[0].Base.String())
	assert.Equal(t, "2159.00", inv.Totals.Payable.String())
}

// TestInformativeChargeIsNotOutsideVAT checks that an informative charge,
// which adds nothing to the totals, is not taken for an amount left out of
// the VAT base.
func TestInformativeChargeIsNotOutsideVAT(t *testing.T) {
	inv := testInvoice()
	inv.Charges = []*bill.Charge{{Informative: true, Reason: "Naknada", Amount: num.MakeAmount(100, 2)}}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
	assert.False(t, eracun.ChargeOutsideVAT(inv.Charges[0]))
	assert.False(t, inv.Charges[0].Ext.Has(eracun.ExtKeyTaxCategory))
}

// TestVATEXCodes checks that a VAT combo may give any exemption reason code
// of the Tax Administration's list, and needs no VAT note then.
func TestVATEXCodes(t *testing.T) {
	assert.Len(t, eracun.VATEXCodes, 59)
	for _, code := range []cbc.Code{"VATEX-EU-132-1B", "VATEX-EU-79-C", "VATEX-EU-143-1FA", "VATEX-FR-CNWVAT"} {
		t.Run(code.String(), func(t *testing.T) {
			inv := testInvoice()
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt,
				Ext: tax.ExtensionsOf(cbc.CodeMap{cef.ExtKeyVATEX: code})}}
			require.NoError(t, inv.Calculate())
			require.NoError(t, rules.Validate(inv))
		})
	}
}

// TestTaxLabelName checks the Croatian name of an HR tax category.
func TestTaxLabelName(t *testing.T) {
	assert.Equal(t, "Porez na potrošnju", eracun.TaxLabelName(eracun.TaxLabelPP))
	assert.Equal(t, "Povratna naknada", eracun.TaxLabelName(eracun.TaxLabelPOVNAK))
	assert.Empty(t, eracun.TaxLabelName("XX"))
}
