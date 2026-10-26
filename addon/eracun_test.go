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
		{"no operator", func(inv *bill.Invoice) { inv.Supplier.People[0].Identities = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-06"},
		{"operator without a name", func(inv *bill.Invoice) { inv.Supplier.People[0].Name = &org.Name{} }, "GOBL-HR-ERACUN-BILL-INVOICE-06"},
		{"operator key without OIB", func(inv *bill.Invoice) {
			inv.Supplier.People = append(inv.Supplier.People, &org.Person{Key: eracun.PersonKeyOperator, Name: &org.Name{Given: "Operater2"}})
		}, "GOBL-HR-ERACUN-BILL-INVOICE-06"},
		{"no customer", func(inv *bill.Invoice) { inv.Customer = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-07"},
		{"customer without endpoint", func(inv *bill.Invoice) { inv.Customer.TaxID = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-08"},
		{"customer endpoint not written to UBL", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "https://example.com/inbox"}}
			inv.Customer.Inboxes = []*org.Inbox{{Code: "123"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-08"},
		{"customer without tax ID", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-09"},
		{"customer tax ID without code", func(inv *bill.Invoice) {
			inv.Customer.TaxID = &tax.Identity{Country: "HR"}
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-10"},
		{"customer without tax ID, standard charge", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
			inv.Charges = []*bill.Charge{{Amount: num.MakeAmount(500, 2), Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-09"},
		{"customer without tax ID, standard discount", func(inv *bill.Invoice) {
			inv.Customer.TaxID = nil
			inv.Customer.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::9934:11111111119"}}
			inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}}
			inv.Discounts = []*bill.Discount{{Amount: num.MakeAmount(500, 2), Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}}}
		}, "GOBL-HR-ERACUN-BILL-INVOICE-09"},
		{"line without KPD", func(inv *bill.Invoice) { inv.Lines[0].Item.Identities = nil }, "GOBL-HR-ERACUN-BILL-INVOICE-11"},
		{"rate without HR tax category", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes[0].Rate = ""
			inv.Lines[0].Taxes[0].Percent = num.NewPercentage(100, 3)
		}, "GOBL-HR-ERACUN-TAX-COMBO-01"},
		{"standard rate of another country", func(inv *bill.Invoice) {
			inv.Lines[0].Taxes[0].Country = "DE"
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

func TestEndpointNotDerivedWhenPresent(t *testing.T) {
	inv := testInvoice()
	inv.Supplier.Endpoints = []*org.Endpoint{{URI: "iso6523-actorid-upis::0088:1234567890123"}}
	require.NoError(t, inv.Calculate())
	require.Len(t, inv.Supplier.Endpoints, 1)
	assert.Equal(t, cbc.URI("iso6523-actorid-upis::0088:1234567890123"), inv.Supplier.Endpoints[0].URI)
}

func TestOperator(t *testing.T) {
	contact := &org.Person{Name: &org.Name{Given: "Contact"}}
	operator := &org.Person{
		Name:       &org.Name{Given: "Operater1"},
		Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "12345678903"}},
	}
	unnamed := &org.Person{
		Name:       &org.Name{},
		Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "12345678903"}},
	}
	assert.Same(t, operator, eracun.Operator([]*org.Person{nil, contact, unnamed, operator}))
	assert.Nil(t, eracun.Operator([]*org.Person{contact}))
	assert.Nil(t, eracun.Operator([]*org.Person{unnamed}))
}

func TestOperatorName(t *testing.T) {
	assert.Equal(t, "Blagajna 1", eracun.OperatorName(&org.Name{Given: "Ivan", Surname: "Kovač", Alias: "Blagajna 1"}))
	assert.Equal(t, "Ivan Kovač", eracun.OperatorName(&org.Name{Given: "Ivan", Surname: "Kovač"}))
	assert.Equal(t, "Ivan", eracun.OperatorName(&org.Name{Given: "Ivan"}))
	assert.Equal(t, "Kovač", eracun.OperatorName(&org.Name{Surname: "Kovač"}))
	assert.Empty(t, eracun.OperatorName(nil))
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
		{"customer without endpoint", func(st *bill.Status) { st.Customer.TaxID = &tax.Identity{Country: "HR"} }, "GOBL-HR-ERACUN-BILL-STATUS-03"},
		{"accepted", func(st *bill.Status) { st.Lines[0].Key = bill.StatusLineAccepted }, "GOBL-HR-ERACUN-BILL-STATUS-04"},
		{"no description", func(st *bill.Status) { st.Lines[0].Description = "" }, "GOBL-HR-ERACUN-BILL-STATUS-05"},
		{"no rejection reason", func(st *bill.Status) { st.Lines[0].Reasons[0].Ext = tax.Extensions{} }, "GOBL-HR-ERACUN-BILL-STATUS-06"},
		{"two rejection reasons", func(st *bill.Status) {
			st.Lines[0].Reasons = append(st.Lines[0].Reasons, st.Lines[0].Reasons[0])
		}, "GOBL-HR-ERACUN-BILL-STATUS-06"},
		{"invalid rejection reason", func(st *bill.Status) {
			st.Lines[0].Reasons[0].Ext = tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyRejectionReason: "X"})
		}, "GOBL-HR-ERACUN-BILL-REASON-01"},
		{"no invoice", func(st *bill.Status) { st.Lines[0].Doc = nil }, "GOBL-HR-ERACUN-BILL-STATUS-07"},
		{"no invoice issue date", func(st *bill.Status) { st.Lines[0].Doc.IssueDate = nil }, "GOBL-HR-ERACUN-BILL-STATUS-09"},
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

func TestOperatorKey(t *testing.T) {
	keyed := &org.Person{Key: eracun.PersonKeyOperator, Name: &org.Name{Given: "Operater1"}}
	other := &org.Person{
		Name:       &org.Name{Given: "Ana"},
		Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}},
	}
	assert.Same(t, keyed, eracun.Operator([]*org.Person{other, keyed}))
	assert.Same(t, other, eracun.Operator([]*org.Person{other}))
	assert.Nil(t, eracun.Operator([]*org.Person{{Name: &org.Name{Given: "Bez OIB"}}}))
}
