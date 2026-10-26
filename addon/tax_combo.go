package eracun

import (
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/rules/is"
	"github.com/invopop/gobl/tax"
)

// standardTaxCategories maps the rates of the Croatian regime to the HR tax
// category of the standard key.
var standardTaxCategories = []struct {
	rate     cbc.Key
	percent  num.Percentage
	category cbc.Code
}{
	{tax.RateGeneral, num.MakePercentage(25, 2), TaxCategoryPDV25},
	{tax.RateReduced, num.MakePercentage(13, 2), TaxCategoryPDV13},
	{tax.RateSuperReduced, num.MakePercentage(5, 2), TaxCategoryPDV5},
}

// keyTaxCategories maps the other VAT keys to their HR tax category.
var keyTaxCategories = map[cbc.Key]cbc.Code{
	tax.KeyZero:           TaxCategoryZ,
	tax.KeyIntraCommunity: TaxCategoryK,
	tax.KeyExport:         TaxCategoryG,
	tax.KeyReverseCharge:  TaxCategoryAE,
	tax.KeyExempt:         TaxCategoryE,
	tax.KeyOutsideScope:   TaxCategoryO,
}

// normalizeTaxCombo sets the HR tax category of a VAT combo from its key and
// rate. A pass-through category is chosen by the user, so it is kept.
func normalizeTaxCombo(tc *tax.Combo) {
	if tc == nil || tc.Category != tax.CategoryVAT {
		return
	}
	if tc.Ext.Get(ExtKeyTaxCategory).In(PassThroughTaxCategories...) {
		return
	}
	if c := taxCategoryFor(tc); c != cbc.CodeEmpty {
		tc.Ext = tc.Ext.Set(ExtKeyTaxCategory, c)
	} else {
		tc.Ext = tc.Ext.Delete(ExtKeyTaxCategory)
	}
}

func taxCategoryFor(tc *tax.Combo) cbc.Code {
	if tc.Key != tax.KeyStandard {
		return keyTaxCategories[tc.Key]
	}
	if tc.Country != "" && tc.Country != l10n.HR.Tax() {
		return cbc.CodeEmpty
	}
	for _, sc := range standardTaxCategories {
		if tc.Rate == sc.rate || (tc.Rate.IsEmpty() && tc.Percent != nil && tc.Percent.Equals(sc.percent)) {
			return sc.category
		}
	}
	return cbc.CodeEmpty
}

func taxComboRules() *rules.Set {
	return rules.For(new(tax.Combo),
		rules.When(is.Func("is VAT", taxComboIsVAT),
			rules.Field("ext",
				rules.Assert("01", "VAT combo requires an HR tax category: standard VAT must be the Croatian 25%, 13% or 5% rate (HR-BR-CL-1)",
					tax.ExtensionsRequire(ExtKeyTaxCategory),
				),
			),
		),
		rules.When(is.Func("has pass-through category", taxComboIsPassThrough),
			rules.Field("key",
				rules.Assert("02", "HR tax categories POVNAK, PP, PPMV and N require the exempt VAT key",
					is.In(tax.KeyExempt),
				),
			),
		),
	)
}

func taxComboIsVAT(val any) bool {
	tc, ok := val.(*tax.Combo)
	return ok && tc != nil && tc.Category == tax.CategoryVAT
}

func taxComboIsPassThrough(val any) bool {
	tc, ok := val.(*tax.Combo)
	return ok && tc != nil && tc.Ext.Get(ExtKeyTaxCategory).In(PassThroughTaxCategories...)
}
