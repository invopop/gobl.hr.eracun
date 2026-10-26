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

				The operator is the supplier person with the "operator" key, or else
				the first supplier person with an OIB identity, and must have an OIB
				identity. A person with the "operator" key is not the seller's
				contact. Parties
				with a Croatian tax ID and no ISO 6523 endpoint are given the
				eRačun network address 9934:<OIB>.
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
		},
		Extensions: extensions,
		Tags:       []*tax.TagSet{invoiceTags},
	}
}
