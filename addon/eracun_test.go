package eracun_test

import (
	"testing"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
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
	assert.Equal(t, eracun.TaxCategoryPDV25, inv.Lines[0].Taxes[0].Ext.Get(eracun.ExtKeyTaxCategory))
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
		{"line without KPD", func(inv *bill.Invoice) { inv.Lines[0].Item.Identities = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-12"},
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
			inv.Lines[0].Taxes[0].Ext = tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxCategoryPDV25})
		}, "GOBL-HR-ERACUN-TAX-COMBO-01"},
		{"pass-through category on a standard combo", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes[0].Ext = tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxCategoryPP})
		}, "GOBL-HR-ERACUN-TAX-COMBO-02"},
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

func TestTaxComboNormalization(t *testing.T) {
	tests := []struct {
		combo *tax.Combo
		want  cbc.Code
	}{
		{&tax.Combo{Category: tax.CategoryVAT, Rate: tax.RateGeneral}, eracun.TaxCategoryPDV25},
		{&tax.Combo{Category: tax.CategoryVAT, Rate: tax.RateReduced}, eracun.TaxCategoryPDV13},
		{&tax.Combo{Category: tax.CategoryVAT, Rate: tax.RateSuperReduced}, eracun.TaxCategoryPDV5},
		{&tax.Combo{Category: tax.CategoryVAT, Percent: num.NewPercentage(250, 3)}, eracun.TaxCategoryPDV25},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyZero}, eracun.TaxCategoryZ},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyIntraCommunity}, eracun.TaxCategoryK},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyExport}, eracun.TaxCategoryG},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyReverseCharge}, eracun.TaxCategoryAE},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyExempt}, eracun.TaxCategoryE},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}, eracun.TaxCategoryO},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyExempt, Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxCategoryPP})}, eracun.TaxCategoryPP},
		{&tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyZero, Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxCategoryPDV25})}, eracun.TaxCategoryZ},
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

func TestEndpointFromInboxScheme(t *testing.T) {
	inv := testInvoice()
	inv.Customer.TaxID = &tax.Identity{Country: "DE", Code: "111111125"}
	inv.Customer.Inboxes = []*org.Inbox{{Scheme: "0088", Code: "3850001234567"}}
	require.NoError(t, inv.Calculate())
	require.Empty(t, inv.Customer.Endpoints)
	require.NoError(t, rules.Validate(inv))
}

func TestEndpointNotDerivedWhenPresent(t *testing.T) {
	inv := testInvoice()
	inv.Supplier.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::0088:1234567890123"}}
	require.NoError(t, inv.Calculate())
	require.Len(t, inv.Supplier.Endpoints, 1)
	assert.Equal(t, cbc.URI("iso6523-actorid-upis::0088:1234567890123"), inv.Supplier.Endpoints[0].URI)
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
