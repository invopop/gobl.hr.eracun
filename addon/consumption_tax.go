package eracun

import (
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"
)

// The consumption tax (porez na potrošnju, PP) is the hr regime's tax
// category on drinks served in catering, charged on a line beside its VAT.
// The HR CIUS has no tax of that kind: it carries PP as an amount left out
// of the VAT base, an exempt document level charge per rate (CIUS example
// 5.2), and the UBL export writes it that way. These rules refuse the PP the
// export can't write.

// maxConsumptionTaxPercent is the highest consumption tax rate a
// municipality or city may set (Local Taxes Act).
var maxConsumptionTaxPercent = num.MakePercentage(3, 2)

func taxComboIsConsumptionTax(val any) bool {
	tc, ok := val.(*tax.Combo)
	return ok && tc != nil && tc.Category == hr.TaxCategoryPP
}

// consumptionTaxPercentValid checks a consumption tax percent is within the
// rates a municipality may set. GOBL itself requires the percent, since the
// regime has no rates for PP.
func consumptionTaxPercentValid(val any) bool {
	p, ok := val.(*num.Percentage)
	if !ok || p == nil {
		return true
	}
	return !p.IsNegative() && p.Compare(maxConsumptionTaxPercent) <= 0
}

func lineHasConsumptionTax(val any) bool {
	l, ok := val.(*bill.Line)
	return ok && l != nil && l.Taxes.Get(hr.TaxCategoryPP) != nil
}

func taxesHaveVAT(val any) bool {
	ts, _ := val.(tax.Set)
	return ts.Get(tax.CategoryVAT) != nil
}

func taxesHaveNoConsumptionTax(val any) bool {
	ts, _ := val.(tax.Set)
	return ts.Get(hr.TaxCategoryPP) == nil
}

// invoiceHasNoPassThroughPP checks that no line, charge or discount has a
// VAT combo with the HR tax category PP, consumption tax stated as an exempt
// amount.
func invoiceHasNoPassThroughPP(val any) bool {
	inv, ok := val.(*bill.Invoice)
	if !ok || inv == nil {
		return true
	}
	for _, l := range inv.Lines {
		if l != nil && taxesHavePassThroughPP(l.Taxes) {
			return false
		}
	}
	for _, c := range inv.Charges {
		if c != nil && taxesHavePassThroughPP(c.Taxes) {
			return false
		}
	}
	for _, d := range inv.Discounts {
		if d != nil && taxesHavePassThroughPP(d.Taxes) {
			return false
		}
	}
	return true
}

func taxesHavePassThroughPP(ts tax.Set) bool {
	tc := ts.Get(tax.CategoryVAT)
	return tc != nil && tc.Ext.Get(ExtKeyTaxCategory) == TaxLabelPP
}

func totalsChargeNoConsumptionTax(val any) bool {
	t, ok := val.(*bill.Totals)
	return !ok || t == nil || t.Taxes == nil || t.Taxes.Category(hr.TaxCategoryPP) == nil
}

func invoiceHasConsumptionTax(val any) bool {
	inv, ok := val.(*bill.Invoice)
	if !ok || inv == nil {
		return false
	}
	for _, l := range inv.Lines {
		if lineHasConsumptionTax(l) {
			return true
		}
	}
	return false
}
