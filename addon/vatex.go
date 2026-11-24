package eracun

import (
	"strings"

	"github.com/invopop/gobl/catalogues/cef"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/tax"
)

// VATEXCodes are the VAT exemption reason codes the Tax Administration takes
// when an eRačun is fiscalized: appendix 16 of its specification
// (Fiskalizacija eRačuna i eIzvještavanje v1.15), the type izuzecePdv of its
// schema. They are part of the CEF VATEX list of the cef-vatex extension,
// and each stands for an article of the Croatian VAT Act: VATEX-EU-79-C, for
// one, for the amounts left out of the VAT base (Art. 33(3)). The schema
// takes them in upper or lower case.
var VATEXCodes = []cbc.Code{
	"VATEX-EU-79-C", "VATEX-EU-132", "VATEX-EU-132-1A", "VATEX-EU-132-1B",
	"VATEX-EU-132-1C", "VATEX-EU-132-1D", "VATEX-EU-132-1E", "VATEX-EU-132-1F",
	"VATEX-EU-132-1G", "VATEX-EU-132-1H", "VATEX-EU-132-1I", "VATEX-EU-132-1J",
	"VATEX-EU-132-1K", "VATEX-EU-132-1L", "VATEX-EU-132-1M", "VATEX-EU-132-1N",
	"VATEX-EU-132-1O", "VATEX-EU-132-1P", "VATEX-EU-132-1Q", "VATEX-EU-143",
	"VATEX-EU-143-1A", "VATEX-EU-143-1B", "VATEX-EU-143-1C", "VATEX-EU-143-1D",
	"VATEX-EU-143-1E", "VATEX-EU-143-1F", "VATEX-EU-143-1FA", "VATEX-EU-143-1G",
	"VATEX-EU-143-1H", "VATEX-EU-143-1I", "VATEX-EU-143-1J", "VATEX-EU-143-1K",
	"VATEX-EU-143-1L", "VATEX-EU-148", "VATEX-EU-148-A", "VATEX-EU-148-B",
	"VATEX-EU-148-C", "VATEX-EU-148-D", "VATEX-EU-148-E", "VATEX-EU-148-F",
	"VATEX-EU-148-G", "VATEX-EU-151", "VATEX-EU-151-1A", "VATEX-EU-151-1AA",
	"VATEX-EU-151-1B", "VATEX-EU-151-1C", "VATEX-EU-151-1D", "VATEX-EU-151-1E",
	"VATEX-EU-309", "VATEX-EU-AE", "VATEX-EU-D", "VATEX-EU-F",
	"VATEX-EU-G", "VATEX-EU-I", "VATEX-EU-IC", "VATEX-EU-J",
	"VATEX-EU-O", "VATEX-FR-FRANCHISE", "VATEX-FR-CNWVAT",
}

// extensionsHaveListedVATEX checks that the VATEX code of a tax combo, if it
// has one, is one of VATEXCodes.
func extensionsHaveListedVATEX(val any) bool {
	ext, ok := tax.ExtensionsFromValue(val)
	if !ok {
		return true
	}
	code := ext.Get(cef.ExtKeyVATEX)
	return code == cbc.CodeEmpty || cbc.Code(strings.ToUpper(code.String())).In(VATEXCodes...)
}

// TaxLabelName provides the Croatian name of an HR tax category (HR-TB-2),
// such as "Porez na potrošnju" for PP, from the definition of the
// ExtKeyTaxCategory extension. It is empty for a code that is not one.
func TaxLabelName(label cbc.Code) string {
	for _, def := range extensions {
		if def.Key != ExtKeyTaxCategory {
			continue
		}
		for _, v := range def.Values {
			if v.Code == label {
				return v.Name[i18n.HR]
			}
		}
	}
	return ""
}
