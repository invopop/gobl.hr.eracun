package ubl

import (
	"testing"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRaisesNewFaultsCountsEachCode checks that a move that breaks a rule on
// one more line counts as a new fault, though the invoice already broke that
// rule on another line.
func TestRaisesNewFaultsCountsEachCode(t *testing.T) {
	// invoice has a beer line with the HR:PP attribute per entry, with the
	// PP tax when the entry is true, which rule 27 refuses.
	invoice := func(pp ...bool) *bill.Invoice {
		inv := &bill.Invoice{
			Regime:    tax.WithRegime("HR"),
			Addons:    tax.WithAddons(eracun.V1),
			Code:      "1",
			Currency:  "EUR",
			IssueDate: cal.MakeDate(2026, 3, 1),
			Tax:       &bill.Tax{Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyProcess: "P1"})},
			Supplier:  &org.Party{Name: "TVRTKA A d.o.o.", TaxID: &tax.Identity{Country: "HR", Code: "12345678903"}},
			Customer:  &org.Party{Name: "Tvrtka B d.o.o.", TaxID: &tax.Identity{Country: "HR", Code: "11111111119"}},
		}
		percent := num.MakePercentage(3, 2)
		for _, withPP := range pp {
			l := &bill.Line{
				Quantity: num.MakeAmount(1, 0),
				Item: &org.Item{
					Name:       "Pivo",
					Price:      num.NewAmount(400, 2),
					Attributes: []*org.Attribute{{Label: eracun.ItemAttributeConsumptionTax, Text: "3"}},
				},
				Taxes: tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyStandard, Rate: tax.RateGeneral}},
			}
			if withPP {
				l.Taxes = append(l.Taxes, &tax.Combo{Category: hr.TaxCategoryPP, Percent: &percent})
			}
			inv.Lines = append(inv.Lines, l)
		}
		require.NoError(t, inv.Calculate())
		return inv
	}
	imported, moved := invoice(true, false), invoice(true, true)
	assert.True(t, raisesNewFaults(imported, moved))
	assert.False(t, raisesNewFaults(moved, moved))
}
