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
	{tax.RateGeneral, num.MakePercentage(25, 2), TaxLabelPDV25},
	{tax.RateReduced, num.MakePercentage(13, 2), TaxLabelPDV13},
	{tax.RateSuperReduced, num.MakePercentage(5, 2), TaxLabelPDV5},
}

// keyTaxCategories maps the other VAT keys to their HR tax category.
var keyTaxCategories = map[cbc.Key]cbc.Code{
	tax.KeyZero:           TaxLabelZ,
	tax.KeyIntraCommunity: TaxLabelK,
	tax.KeyExport:         TaxLabelG,
	tax.KeyReverseCharge:  TaxLabelAE,
	tax.KeyExempt:         TaxLabelE,
	tax.KeyOutsideScope:   TaxLabelO,
}

// normalizeTaxCombo sets the HR tax category of a VAT combo from its key and
// rate. The category of an amount left out of the VAT base is not a VAT
// category: it is left as given, for rule 02 to refuse.
func normalizeTaxCombo(tc *tax.Combo) {
	if tc == nil || tc.Category != tax.CategoryVAT {
		return
	}
	if tc.Ext.Get(ExtKeyTaxCategory).In(PassThroughTaxLabels...) {
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
		rules.Field("ext",
			rules.Assert("02", "tax combo must not have the HR tax category POVNAK, PP, PPMV or N: an amount left out of the VAT base is a line or charge without taxes, with the category in its own ext",
				tax.ExtensionsExcludeCodes(ExtKeyTaxCategory, PassThroughTaxLabels...),
			),
		),
		rules.When(is.Func("is consumption tax", taxComboIsConsumptionTax),
			rules.Field("percent",
				rules.AssertIfPresent("03", "PP combo percent must be from 0% to 3%, the most a municipality may set (Local Taxes Act)",
					is.Func("0% to 3%", consumptionTaxPercentValid),
				),
			),
		),
	)
}

func taxComboIsVAT(val any) bool {
	tc, ok := val.(*tax.Combo)
	return ok && tc != nil && tc.Category == tax.CategoryVAT
}
