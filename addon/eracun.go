// Package eracun implements the Croatian eRačun addon: the rules of the HR
// CIUS 2025 specification and its national extension, which domestic B2B
// invoices follow under Fiscalization 2.0 (Zakon o fiskalizaciji, NN 89/25).
// Built on the EU EN 16931 addon, which it declares as a requirement.
package eracun

import (
	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/norm"
	"github.com/invopop/gobl/pkg/here"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/rules/is"
	"github.com/invopop/gobl/tax"
)

const (
	// Namespace is the rules namespace for the Croatian eRačun addon.
	Namespace rules.Code = "HR-ERACUN"

	// Key identifies the Croatian eRačun addon family.
	Key cbc.Key = "hr-eracun"

	// V1 is the version of the addon for HR CIUS 2025 (cius-2025:1.0) with the
	// national extension ext-2025:1.0.
	V1 cbc.Key = Key + "-v1"
)

// contentTypePDF is the content type of the specifications listed as sources.
const contentTypePDF = "application/pdf"

func init() {
	tax.RegisterAddonDef(newAddon())
	rules.RegisterWithGuard(
		Key.String(),
		rules.GOBL.Add(Namespace),
		is.InContext(tax.AddonIn(V1)),
		billInvoiceRules(),
		billStatusRules(),
		reasonRules(),
		taxComboRules(),
	)
	norm.RegisterWithGuard(is.InContext(tax.AddonIn(V1)),
		norm.For(normalizeInvoice),
		norm.For(normalizeStatus),
		norm.For(normalizeTaxCombo),
	)
}

func newAddon() *tax.AddonDef {
	return &tax.AddonDef{
		Key: V1,
		Requires: []cbc.Key{
			en16931.V2017,
		},
		Name: i18n.String{
			i18n.EN: "Croatia eRačun (HR CIUS 2025)",
			i18n.HR: "Hrvatska eRačun (HR CIUS 2025)",
		},
		Description: i18n.String{
			i18n.EN: here.Doc(`
				Support for Croatian eRačun invoices exchanged between businesses
				under Fiscalization 2.0. Invoices follow EN 16931 with the HR CIUS 2025
				specification and its national extension, which add a business process
				code, the operator who issued the invoice, an HR tax category on every
				tax, a KPD classification on each line and extra HR tax totals.

				The operator is the supplier person with the "operator" key: exactly
				one person must have it, with a name and an OIB identity, and no other
				person is taken in their place. Where Art. 8(4) of the Fiscalisation
				Act allows it, the operator's OIB may be the supplier's own. The
				operator is not the seller's contact. Correcting an invoice with GOBL
				leaves out the original operator, so the corrective document needs its
				own. Parties with a Croatian tax ID and no ISO 6523 endpoint are given
				the eRačun network address 9934:<OIB>.

				An invoice with no issue time, dated today or not dated at all, is
				given the current date and time in Croatia. An invoice dated another
				day needs its issue time. A corrective document is issued now, so it
				doesn't keep the original's issue time.

				Statuses are application responses the buyer sends the supplier,
				each with one line about one invoice. A rejection needs a
				description and one reason with an N, U or O rejection reason code,
				and nothing else. The other Peppol invoice response keys (accepted,
				acknowledged, processing, querying and paid) are sent only by
				agreement with the supplier, with an optional description and at
				most one Peppol status reason or action, which has no text of its
				own.
			`),
		},
		Sources: []*cbc.Source{
			{
				Title: i18n.String{
					i18n.EN: "Porezna uprava - eRačun",
					i18n.HR: "Porezna uprava - eRačun",
				},
				URL: "https://porezna.gov.hr/fiskalizacija/bezgotovinski-racuni/eracun",
			},
			{
				Title: i18n.String{
					i18n.EN: "Specification for the core use of eRačun with extensions v1.8",
					i18n.HR: "Specifikacija osnovne uporabe eRačuna s proširenjima v1.8",
				},
				URL:         "https://porezna.gov.hr/fiskalizacija/api/dokumenti/196",
				ContentType: contentTypePDF,
			},
			{
				Title: i18n.String{
					i18n.EN: "Technical specification: eRačun fiscalization and eReporting v1.15",
					i18n.HR: "Tehnička specifikacija: Fiskalizacija eRačuna i eIzvještavanje v1.15",
				},
				URL:         "https://porezna.gov.hr/fiskalizacija/api/dokumenti/186",
				ContentType: contentTypePDF,
			},
			{
				Title: i18n.String{
					i18n.EN: "Application response v1.1",
					i18n.HR: "Aplikacijski odgovor v1.1",
				},
				URL:         "https://porezna.gov.hr/fiskalizacija/api/dokumenti/206",
				ContentType: contentTypePDF,
			},
		},
		Extensions:  extensions,
		Corrections: invoiceCorrectionDefinitions,
	}
}
