package ubl

import (
	"bytes"
	"encoding/xml"

	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/xmlctx"
)

// NamespaceHRExtension is the namespace of the HR extension elements.
const NamespaceHRExtension = "urn:mfin.gov.hr:schema:xsd:HRExtensionAggregateComponents-1"

// cashBasisText is the HRObracunPDVPoNaplati text of an invoice whose VAT is
// accounted for when it is paid, in the wording the VAT Act prescribes and
// the regime's cash basis note uses. The official example has a shorter
// variant; only the element's presence carries meaning.
const cashBasisText = "Obračun prema naplaćenim naknadama"

// HRFISK20Data holds the HR extension data of an invoice (HR-BG-1): the VAT
// breakdown that puts POVNAK, PP, PPMV and N amounts outside the VAT base,
// the totals that follow from it, and the cash basis VAT flag.
type HRFISK20Data struct {
	XMLName               xml.Name              `xml:"hrextac:HRFISK20Data"`
	Namespace             string                `xml:"xmlns:hrextac,attr"`
	HRObracunPDVPoNaplati string                `xml:"hrextac:HRObracunPDVPoNaplati,omitempty"`
	HRTaxTotal            *HRTaxTotal           `xml:"hrextac:HRTaxTotal,omitempty"`
	HRLegalMonetaryTotal  *HRLegalMonetaryTotal `xml:"hrextac:HRLegalMonetaryTotal,omitempty"`
}

// HRTaxTotal is the HR VAT breakdown (HR-BG-2).
type HRTaxTotal struct {
	TaxAmount      goblubl.Amount  `xml:"cbc:TaxAmount"`
	HRTaxSubtotals []HRTaxSubtotal `xml:"hrextac:HRTaxSubtotal"`
}

// HRTaxSubtotal is one HR tax category of the breakdown.
type HRTaxSubtotal struct {
	TaxableAmount goblubl.Amount `xml:"cbc:TaxableAmount"`
	TaxAmount     goblubl.Amount `xml:"cbc:TaxAmount"`
	HRTaxCategory HRTaxCategory  `xml:"hrextac:HRTaxCategory"`
}

// HRTaxCategory identifies an HR tax category.
type HRTaxCategory struct {
	ID                 string      `xml:"cbc:ID"`
	Name               string      `xml:"cbc:Name"`
	Percent            string      `xml:"cbc:Percent"`
	TaxExemptionReason string      `xml:"cbc:TaxExemptionReason,omitempty"`
	HRTaxScheme        HRTaxScheme `xml:"hrextac:HRTaxScheme"`
}

// HRTaxScheme identifies the tax scheme of an HR tax category.
type HRTaxScheme struct {
	ID string `xml:"cbc:ID"`
}

// HRLegalMonetaryTotal holds the HR totals (HR-BG-3).
type HRLegalMonetaryTotal struct {
	TaxExclusiveAmount    goblubl.Amount `xml:"cbc:TaxExclusiveAmount"`
	OutOfScopeOfVATAmount goblubl.Amount `xml:"hrextac:OutOfScopeOfVATAmount"`
}

// addHRFISK20Data adds the HR extension data to the invoice as its own UBL
// extension.
func addHRFISK20Data(out *goblubl.Invoice, data *HRFISK20Data) error {
	data.Namespace = NamespaceHRExtension
	raw, err := xml.Marshal(data)
	if err != nil {
		return err
	}
	out.AddExtension(goblubl.NewRawExtension(raw))
	return nil
}

// findHRFISK20Data provides the HR extension data of an imported invoice, if
// any.
func findHRFISK20Data(in *goblubl.Invoice) (*HRFISK20Data, error) {
	if in.Extensions == nil {
		return nil, nil
	}
	for _, e := range in.Extensions.Extension {
		if e.ExtensionContent == nil || !bytes.Contains(e.ExtensionContent.Raw, []byte(NamespaceHRExtension)) {
			continue
		}
		data := new(HRFISK20Data)
		err := xmlctx.Unmarshal(e.ExtensionContent.Raw, data, xmlctx.WithNamespaces(map[string]string{
			"hrextac": NamespaceHRExtension,
			"cbc":     goblubl.NamespaceCBC,
		}))
		if err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, nil
}
