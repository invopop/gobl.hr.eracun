package ubl

import (
	"github.com/invopop/gobl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/catalogues/untdid"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/tax"
)

// reasonRequiredCategories are the EN 16931 VAT categories whose lines and
// document level charges must carry an exemption reason in the HR CIUS
// (HR-BR-13, HR-BR-36): the export writes the VAT note's text for them.
var reasonRequiredCategories = []cbc.Code{en16931.TaxCategoryExempt, en16931.TaxCategoryOutsideScope}

// reasonAllowedCategories are the EN 16931 VAT categories that may carry an
// exemption reason: a reason kept on import is written back for them.
var reasonAllowedCategories = []cbc.Code{
	en16931.TaxCategoryExempt, en16931.TaxCategoryReverseCharge, en16931.TaxCategoryIntraCommunity,
	en16931.TaxCategoryExport, en16931.TaxCategoryOutsideScope,
}

// invoices provides the GOBL and UBL invoices of an export or import, if
// both are invoices.
func invoices(env *gobl.Envelope, doc goblubl.Document) (*bill.Invoice, *goblubl.Invoice, bool) {
	inv, ok := env.Extract().(*bill.Invoice)
	out, ok2 := doc.(*goblubl.Invoice)
	return inv, out, ok && ok2
}

// pairedLines provides the lines of a UBL invoice or credit note, each at the
// position of the GOBL line it corresponds to: with no sub-invoice lines,
// which the HR format never writes, gobl.ubl writes one UBL line per GOBL
// line and reads one GOBL line per UBL line, in order. A document where the
// counts differ, such as an imported one with a line gobl.ubl left out for
// having no price (BT-146), is refused rather than paired by position, which
// would put the tax data of one line on another.
func pairedLines(inv *bill.Invoice, doc *goblubl.Invoice) ([]goblubl.InvoiceLine, error) {
	lines := doc.InvoiceLines
	if len(doc.CreditNoteLines) > 0 {
		lines = doc.CreditNoteLines
	}
	if len(lines) != len(inv.Lines) {
		return nil, gobl.ErrValidation.WithReason("line count differs, %d in UBL and %d in GOBL: the lines cannot be paired", len(lines), len(inv.Lines))
	}
	return lines, nil
}

// documentCharges calls fn with each document level charge and allowance of
// the UBL invoice, and the taxes and meta data of the GOBL charge or discount
// it corresponds to. Charges and allowances are counted apart, which pairs
// them in both directions: gobl.ubl writes the charges and then the
// discounts, and reads the charges and the allowances of a document, in its
// order, into the charges and the discounts.
func documentCharges(doc *goblubl.Invoice, inv *bill.Invoice, fn func(ac *goblubl.AllowanceCharge, ts tax.Set, meta *cbc.Meta)) {
	var charges, discounts int
	for i := range doc.AllowanceCharge {
		ac := &doc.AllowanceCharge[i]
		if ac.ChargeIndicator {
			if charges < len(inv.Charges) && inv.Charges[charges] != nil {
				fn(ac, inv.Charges[charges].Taxes, &inv.Charges[charges].Meta)
			}
			charges++
			continue
		}
		if discounts < len(inv.Discounts) && inv.Discounts[discounts] != nil {
			fn(ac, inv.Discounts[discounts].Taxes, &inv.Discounts[discounts].Meta)
		}
		discounts++
	}
}

// taxNote provides the VAT note with the exemption reason of the EN 16931
// tax category, the way gobl.ubl finds it for the tax breakdown. GOBL has one
// note per category, so every E line and charge shares one reason text, an
// exempt supply and an amount left out of the VAT base included: a reason of
// its own is kept under MetaKeyTaxExemptionReason.
func taxNote(inv *bill.Invoice, ext tax.Extensions) *tax.Note {
	cat := ext.Get(untdid.ExtKeyTaxCategory)
	if inv.Tax == nil || cat == cbc.CodeEmpty {
		return nil
	}
	for _, n := range inv.Tax.Notes {
		if n != nil && n.Category == tax.CategoryVAT && n.Ext.Get(untdid.ExtKeyTaxCategory) == cat {
			return n
		}
	}
	return nil
}

// setTaxExt sets a tax extension of the invoice, giving it a tax object if
// it has none.
func setTaxExt(inv *bill.Invoice, key cbc.Key, value cbc.Code) {
	if inv.Tax == nil {
		inv.Tax = new(bill.Tax)
	}
	inv.Tax.Ext = inv.Tax.Ext.Set(key, value)
}

func setMeta(meta *cbc.Meta, key cbc.Key, value string) {
	if *meta == nil {
		*meta = make(cbc.Meta)
	}
	(*meta)[key] = value
}
