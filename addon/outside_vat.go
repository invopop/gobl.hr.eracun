package eracun

import (
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/regimes/hr"
)

// An amount left out of the VAT base is money the supplier collects with the
// invoice that is not the price of a supply: the return fee on packaging
// (POVNAK), the special tax on motor vehicles (PPMV), or any other fee that
// is not taxed (N), such as a tourist tax. The HR CIUS calls them
// pass-through items (Art. 33(3) of the VAT Act).
//
// In GOBL such an amount has no VAT combo: it is a line of its own, when it
// has a quantity and a unit price, or a document level charge, and it carries
// its HR tax category (HR-TB-2) in its own extensions. The hr-fiskal-b2c-v1
// addon takes a charge without taxes the same way, as a fee. EN 16931 has no
// place for an amount outside the VAT base beside taxed ones, so the UBL
// export writes it as the HR CIUS does: VAT category E with its HR tax
// category in the core, and category O in the HR breakdown.
//
// A charge on a line is not such an amount: it is part of the line's net
// amount and takes the line's VAT (BG-28).

// LineOutsideVAT says whether a line is an amount left out of the VAT base:
// a line with no taxes.
func LineOutsideVAT(l *bill.Line) bool {
	return l != nil && len(l.Taxes) == 0
}

// ChargeOutsideVAT says whether a document level charge is an amount left
// out of the VAT base: a charge with no taxes. An informative charge adds
// nothing to the totals, so it is not one.
func ChargeOutsideVAT(c *bill.Charge) bool {
	return c != nil && !c.Informative && len(c.Taxes) == 0
}

// normalizeAmountsOutsideVAT gives a charge without taxes the HR tax category
// N, any other fee that is not taxed, unless it names its own. A line without
// taxes has to name its category: a line that only forgot its VAT must not
// turn into an amount that is not taxed.
func normalizeAmountsOutsideVAT(inv *bill.Invoice) {
	for _, c := range inv.Charges {
		if ChargeOutsideVAT(c) && !c.Ext.Has(ExtKeyTaxCategory) {
			c.Ext = c.Ext.Set(ExtKeyTaxCategory, TaxLabelN)
		}
	}
}

func lineOutsideVAT(val any) bool {
	l, ok := val.(*bill.Line)
	return ok && LineOutsideVAT(l)
}

func lineHasTaxes(val any) bool {
	l, ok := val.(*bill.Line)
	return ok && l != nil && len(l.Taxes) > 0
}

func chargeOutsideVAT(val any) bool {
	c, ok := val.(*bill.Charge)
	return ok && ChargeOutsideVAT(c)
}

func chargeHasTaxes(val any) bool {
	c, ok := val.(*bill.Charge)
	return ok && c != nil && len(c.Taxes) > 0
}

// invoiceHasAmountsOutsideVAT says whether the invoice has a line or a
// document level charge left out of the VAT base.
func invoiceHasAmountsOutsideVAT(inv *bill.Invoice) bool {
	for _, l := range inv.Lines {
		if LineOutsideVAT(l) {
			return true
		}
	}
	for _, c := range inv.Charges {
		if ChargeOutsideVAT(c) {
			return true
		}
	}
	return false
}

// invoiceStatesNoExemptAmounts checks that the invoice has neither the PP
// tax nor an amount left out of the VAT base, both of which the eRačun
// states as exempt (E) amounts.
func invoiceStatesNoExemptAmounts(val any) bool {
	inv, ok := val.(*bill.Invoice)
	if !ok || inv == nil {
		return true
	}
	if invoiceHasAmountsOutsideVAT(inv) {
		return false
	}
	return inv.Totals == nil || inv.Totals.Taxes == nil || inv.Totals.Taxes.Category(hr.TaxCategoryPP) == nil
}

// keyNotTax checks that a charge's key is not tax.
func keyNotTax(val any) bool {
	k, ok := val.(cbc.Key)
	return !ok || k != bill.ChargeKeyTax
}
