// Package ubl adds the Croatian eRačun UBL format (HR CIUS 2025 with the
// national extension) on top of the gobl.ubl base import and export, and
// registers it with gobl.ubl and the GOBL convert register. Import it for its
// side effects:
//
//	import _ "github.com/invopop/gobl.hr.eracun/ubl"
package ubl

import (
	"encoding/xml"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/schema"
)

// KeyHRCIUS identifies the Croatian eRačun UBL format.
const KeyHRCIUS cbc.Key = "ubl+hr-eracun-v1"

// CustomizationID identifies HR CIUS 2025 with the national extension
// (BT-24, HR-BR-5).
const CustomizationID = "urn:cen.eu:en16931:2017#compliant#urn:mfin.gov.hr:cius-2025:1.0#conformant#urn:mfin.gov.hr:ext-2025:1.0"

// FormatHRCIUS defines the format of Croatian eRačun invoices, and of the
// application responses buyers send about them. The ProfileID of an invoice
// is its business process, so it is not part of the format.
var FormatHRCIUS = goblubl.Format{
	Key:             KeyHRCIUS,
	Name:            i18n.NewString("UBL HR CIUS 2025"),
	Countries:       []l10n.Code{l10n.HR},
	Import:          []schema.ID{schema.Lookup(bill.Invoice{}), schema.Lookup(bill.Status{})},
	Export:          []schema.ID{schema.Lookup(bill.Invoice{}), schema.Lookup(bill.Status{})},
	CustomizationID: CustomizationID,
	Addons:          []cbc.Key{eracun.V1},
	VESIDs: goblubl.VESIDMapping{
		Invoice:    "hr.gov.porezna.eracun:ubl-invoice:1.0.4",
		CreditNote: "hr.gov.porezna.eracun:ubl-creditnote:1.0.4",
	},
	Specifications: []goblubl.Specification{SpecificationApplicationResponse},
	ExportFuncs:    []goblubl.ExportFunc{exportHR, exportApplicationResponse},
	ImportFuncs:    []goblubl.ImportFunc{importHR, importApplicationResponse},
}

func init() {
	goblubl.RegisterFormats(FormatHRCIUS)
	goblubl.RegisterExtensionData(xml.Name{Space: NamespaceHRExtension, Local: "HRFISK20Data"}, func() goblubl.ExtensionData { return new(HRFISK20Data) })
}
