package fiskal_test

import (
	"testing"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	"github.com/invopop/gobl.hr.eracun/addon/fiskal"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/pay"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const supplierOIB = "80432461483"

func testInvoice() *bill.Invoice {
	return &bill.Invoice{
		Regime:    tax.WithRegime("HR"),
		Addons:    tax.WithAddons(fiskal.V1),
		Code:      "1001",
		Currency:  "EUR",
		IssueDate: cal.MakeDate(2026, 11, 6),
		IssueTime: cal.NewTime(13, 58, 10),
		Tax: &bill.Tax{
			Ext: tax.ExtensionsOf(cbc.CodeMap{
				fiskal.ExtKeyPremises: "POS1",
				fiskal.ExtKeyDevice:   "1",
			}),
		},
		Supplier: &org.Party{
			Name:  "Invopop d.o.o.",
			TaxID: &tax.Identity{Country: "HR", Code: supplierOIB},
			People: []*org.Person{
				{
					Key:        eracun.PersonKeyOperator,
					Name:       &org.Name{Given: "Operater"},
					Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: supplierOIB}},
				},
			},
		},
		Lines: []*bill.Line{
			{
				Quantity: num.MakeAmount(2, 0),
				Item:     &org.Item{Name: "Kava", Price: num.NewAmount(250, 2)},
				Taxes:    tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}},
			},
		},
		Payment: &bill.PaymentDetails{
			Advances: []*pay.Record{{Key: pay.MeansKeyCash, Description: "Gotovina", Percent: num.NewPercentage(100, 2)}},
		},
	}
}

func assertFault(t *testing.T, inv *bill.Invoice, code rules.Code) {
	t.Helper()
	require.NoError(t, inv.Calculate())
	faults := rules.Validate(inv)
	require.Error(t, faults)
	assert.True(t, faults.HasCode(code), "want %s, got %s", code, faults)
}

func faultCode(n string) rules.Code {
	return rules.GOBL.Add(fiskal.Namespace).Add("BILL-INVOICE").Add(rules.Code(n))
}

func TestAddon(t *testing.T) {
	ad := tax.AddonForKey(fiskal.V1)
	require.NotNil(t, ad)
	assert.Equal(t, fiskal.V1, ad.Key)
}

func TestInvoiceValid(t *testing.T) {
	inv := testInvoice()
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
	assert.Equal(t, fiskal.SequencePremises, inv.Tax.Ext.Get(fiskal.ExtKeySequenceScope))
	assert.Equal(t, fiskal.VATSystemIn, inv.Tax.Ext.Get(fiskal.ExtKeyVATSystem))
}

func TestNormalizeKeepsExtensions(t *testing.T) {
	inv := testInvoice()
	inv.Tax.Ext = inv.Tax.Ext.
		Set(fiskal.ExtKeySequenceScope, fiskal.SequenceDevice).
		Set(fiskal.ExtKeyVATSystem, fiskal.VATSystemOut)
	inv.Lines[0].Taxes[0] = &tax.Combo{Category: tax.CategoryVAT, Key: tax.KeyOutsideScope}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
	assert.Equal(t, fiskal.SequenceDevice, inv.Tax.Ext.Get(fiskal.ExtKeySequenceScope))
	assert.Equal(t, fiskal.VATSystemOut, inv.Tax.Ext.Get(fiskal.ExtKeyVATSystem))
}

func TestNormalizeIssueTime(t *testing.T) {
	loc := tax.RegimeDefFor(l10n.HR).TimeLocation()

	t.Run("no date and no time", func(t *testing.T) {
		inv := testInvoice()
		inv.IssueDate = cal.Date{}
		inv.IssueTime = nil
		require.NoError(t, inv.Calculate())
		require.NotNil(t, inv.IssueTime)
		assert.Equal(t, cal.TodayIn(loc), inv.IssueDate)
		require.NoError(t, rules.Validate(inv))
	})

	t.Run("another day without a time", func(t *testing.T) {
		inv := testInvoice()
		inv.IssueTime = nil
		assertFault(t, inv, faultCode("02"))
	})
}

func TestInvoiceRules(t *testing.T) {
	tests := []struct {
		name string
		edit func(*bill.Invoice)
		code string
	}{
		{"code with a series letter", func(inv *bill.Invoice) { inv.Code = "A1" }, "01"},
		{"code with a leading zero", func(inv *bill.Invoice) { inv.Code = "0101" }, "01"},
		{"no premises", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Delete(fiskal.ExtKeyPremises) }, "05"},
		{"no device", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Delete(fiskal.ExtKeyDevice) }, "05"},
		{"premises with a dash", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Set(fiskal.ExtKeyPremises, "POS-1") }, "06"},
		{"device with letters", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Set(fiskal.ExtKeyDevice, "T1") }, "07"},
		{"device with a leading zero", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Set(fiskal.ExtKeyDevice, "01") }, "07"},
		{"unknown sequence scope", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Set(fiskal.ExtKeySequenceScope, "X") }, "08"},
		{"unknown VAT system", func(inv *bill.Invoice) { inv.Tax.Ext = inv.Tax.Ext.Set(fiskal.ExtKeyVATSystem, "X") }, "09"},
		{"foreign supplier", func(inv *bill.Invoice) {
			inv.Supplier.TaxID = &tax.Identity{Country: "SI", Code: "92950663"}
		}, "12"},
		{"supplier without a tax ID code", func(inv *bill.Invoice) { inv.Supplier.TaxID.Code = "" }, "13"},
		{"no operator", func(inv *bill.Invoice) { inv.Supplier.People = nil }, "14"},
		{"two operators", func(inv *bill.Invoice) {
			inv.Supplier.People = append(inv.Supplier.People, inv.Supplier.People[0])
		}, "14"},
		{"operator without an OIB", func(inv *bill.Invoice) { inv.Supplier.People[0].Identities = nil }, "15"},
		{"VAT outside the VAT system", func(inv *bill.Invoice) {
			inv.Tax.Ext = inv.Tax.Ext.Set(fiskal.ExtKeyVATSystem, fiskal.VATSystemOut)
		}, "16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := testInvoice()
			tt.edit(inv)
			assertFault(t, inv, faultCode(tt.code))
		})
	}

	t.Run("operator OIB with a wrong control digit", func(t *testing.T) {
		inv := testInvoice()
		inv.Supplier.People[0].Identities[0].Code = "80432461484"
		require.NoError(t, inv.Calculate())
		err := rules.Validate(inv)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "checksum")
	})

	t.Run("paper number", func(t *testing.T) {
		inv := testInvoice()
		inv.Tax.Ext = inv.Tax.Ext.Set(fiskal.ExtKeyPaperNumber, "123/458/5")
		require.NoError(t, inv.Calculate())
		require.NoError(t, rules.Validate(inv))
	})
}
