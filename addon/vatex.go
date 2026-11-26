package eracun

import (
	"strconv"
	"strings"

	"github.com/invopop/gobl/catalogues/cef"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/tax"
)

// cisVATEXVersion is the last version of the CEF VATEX code list the Tax
// Administration takes when an eRačun is fiscalized. The codes of the type
// izuzecePdv of its schema (appendix 16 of Fiskalizacija eRačuna i
// eIzvještavanje v1.15) are those of the cef-vatex extension first listed in
// version 5 or earlier.
const cisVATEXVersion = 5

// VATEXCodes are the VAT exemption reason codes the Tax Administration takes,
// taken from the cef-vatex extension. Each stands for an article of the
// Croatian VAT Act: VATEX-EU-79-C, for one, for the amounts left out of the
// VAT base (Art. 33(3)). The schema takes them in upper or lower case.
var VATEXCodes = vatexCodes()

// vatexCodes provides the codes of the cef-vatex extension first listed in
// the VATEX code list version cisVATEXVersion or earlier.
func vatexCodes() []cbc.Code {
	var codes []cbc.Code
	for _, v := range tax.ExtensionForKey(cef.ExtKeyVATEX).Values {
		if n, err := strconv.Atoi(v.Meta["first-version"]); err == nil && n <= cisVATEXVersion {
			codes = append(codes, v.Code)
		}
	}
	return codes
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
