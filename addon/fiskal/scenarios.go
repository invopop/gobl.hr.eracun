package fiskal

import (
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/tax"
)

// NoteSmallTaxpayer is the note an invoice from a small taxpayer must carry
// (Art. 79(3) of the VAT Act): its supplies are exempt from VAT under
// Art. 90(1), which since 1 January 2025 holds the small-business
// exemption.
const NoteSmallTaxpayer = "Oslobođeno PDV-a temeljem čl. 90. st. 1. Zakona o porezu na dodanu vrijednost"

var invoiceScenarios = &tax.ScenarioSet{
	Schema: bill.ShortSchemaInvoice,
	List: []*tax.Scenario{
		// Supplier outside the VAT system: a small taxpayer.
		{
			Filter:     func(doc any) bool { return outsideVATSystem(doc) },
			Categories: []cbc.Code{tax.CategoryVAT},
			Note: &tax.Note{
				Category: tax.CategoryVAT,
				Key:      tax.KeyExempt,
				Text:     NoteSmallTaxpayer,
			},
		},
	},
}
