package racun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/invopop/gobl.hr.eracun/addon/fiskal"
)

// Invopop numbers an invoice before it is fiscalized, so an invoice the
// addon accepts must always convert: a numbered invoice that is never
// issued leaves a gap in the sequence (Art. 9(2) of the Fiscalization Act).

// TestAddonRefusesUnsupported checks that the addon refuses, before the
// invoice is numbered, each invoice the conversion does not support.
func TestAddonRefusesUnsupported(t *testing.T) {
	general := tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}
	tests := []struct {
		name string
		edit func(*bill.Invoice)
	}{
		{"a credit note", func(i *bill.Invoice) { i.Type = bill.InvoiceTypeCreditNote }},
		{"a negative total", func(i *bill.Invoice) {
			for _, l := range i.Lines {
				l.Item.Price = num.NewAmount(-100, 2)
			}
		}},
		{"a rounding amount", func(i *bill.Invoice) {
			r := num.MakeAmount(-5, 2)
			i.Totals.Rounding = &r
		}},
		{"a charge without taxes", func(i *bill.Invoice) {
			i.Charges = []*bill.Charge{{Amount: num.MakeAmount(10, 2), Reason: "Povratna naknada"}}
		}},
		{"a discount without taxes", func(i *bill.Invoice) {
			i.Discounts = []*bill.Discount{{Amount: num.MakeAmount(10, 2), Reason: "Promo"}}
		}},
		{"a reverse charge", func(i *bill.Invoice) {
			i.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyReverseCharge}}
		}},
		{"a charge with VAT is supported", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := loadExample(t, "cash-prices-include-vat.json").Extract().(*bill.Invoice)
			if tt.edit == nil {
				inv.Charges = []*bill.Charge{{Amount: num.MakeAmount(100, 2), Reason: "Service", Taxes: general}}
			} else {
				tt.edit(inv)
			}
			require.NoError(t, inv.Calculate())
			verr := rules.Validate(inv)
			_, cerr := FromInvoice(inv, testOptions(), testKey(t))
			if tt.edit == nil {
				assert.NoError(t, verr)
				assert.NoError(t, cerr)
				return
			}
			require.Error(t, verr)
			assert.Contains(t, verr.Error(), string(fiskal.Namespace))
			assert.ErrorIs(t, cerr, ErrUnsupported)
		})
	}
}

// TestAddonExamplesConvert converts every example of the addon in the
// module's examples, as calculated and validated envelopes.
func TestAddonExamplesConvert(t *testing.T) {
	names, err := filepath.Glob("../examples/out/invoice-hr-hr-fiskal-*.json")
	require.NoError(t, err)
	require.NotEmpty(t, names)
	for _, name := range names {
		t.Run(filepath.Base(name), func(t *testing.T) {
			data, err := os.ReadFile(name)
			require.NoError(t, err)
			env := new(gobl.Envelope)
			require.NoError(t, json.Unmarshal(data, env))
			require.NoError(t, env.Validate())
			_, err = NewInvoice(env, testOptions(), testKey(t))
			assert.NoError(t, err)
		})
	}
}
