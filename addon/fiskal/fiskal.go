// Package fiskal implements the Croatian Fiskal addon for B2C invoices: the
// rules an invoice issued to a consumer must follow to be fiscalized with the
// Tax Administration's invoice fiscalization service (Fiscalization 1.0,
// "fiskalizacija računa u krajnjoj potrošnji", Fiscalization Act, NN 89/25).
//
// Croatian law calls the document a shop prints a račun, an invoice: it has
// no separate term for a receipt. The racun package converts an invoice with
// this addon into the message CIS (the Tax Administration's fiscalization
// service) receives.
package fiskal

import (
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/norm"
	"github.com/invopop/gobl/pkg/here"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/rules/is"
	"github.com/invopop/gobl/tax"
)

const (
	// Namespace is the rules namespace for the Croatian Fiskal B2C addon.
	Namespace rules.Code = "HR-FISKAL-B2C"

	// Key identifies the Croatian Fiskal B2C addon family.
	Key cbc.Key = "hr-fiskal-b2c"

	// V1 is the version of the addon for the specification "Fiskalizacija
	// računa u krajnjoj potrošnji" v2.7 and FiskalizacijaSchema.xsd v1.10.
	V1 cbc.Key = Key + "-v1"
)

func init() {
	tax.RegisterAddonDef(newAddon())
	rules.RegisterWithGuard(
		Key.String(),
		rules.GOBL.Add(Namespace),
		is.InContext(tax.AddonIn(V1)),
		billInvoiceRules(),
	)
	norm.RegisterWithGuard(is.InContext(tax.AddonIn(V1)),
		norm.For(normalizeInvoice),
	)
}

func newAddon() *tax.AddonDef {
	return &tax.AddonDef{
		Key: V1,
		Name: i18n.String{
			i18n.EN: "Croatia Fiskal B2C invoices",
			i18n.HR: "Hrvatska fiskalizacija računa u krajnjoj potrošnji",
		},
		Description: i18n.String{
			i18n.EN: here.Doc(`
				Support for Croatian invoices fiscalized at the time of the sale
				(Fiscalization 1.0), such as the invoices a shop prints at the till.
				Each invoice is reported to the Tax Administration's fiscalization
				service (CIS), which answers with a unique invoice identifier (JIR).

				The invoice number has three parts: the invoice code, which must be
				the bare sequential number, the business premises label and the
				device label. Set the labels, as registered in ePorezna, in the
				hr-fiskal-premises and hr-fiskal-device extensions of the invoice tax.
				The numbering scope (hr-fiskal-sequence-scope) is per business
				premises unless set to N, per device.

				The invoice can arrive with its number, from a till that numbers its
				own invoices, or without a code, for Invopop to number it. The series
				is set from the labels to match the numbering scope: the premises
				label for scope P, such as "POS1", or the premises and device labels
				for scope N, such as "POS1/1". A counter that runs per series then
				numbers each premises, or each device, on its own. A series that does
				not match the labels is refused.

				The operator is the supplier person with the "operator" key, with an
				OIB identity: the person at the till, or the supplier for a
				self-service device. It is the same convention as the hr-eracun-v1
				addon.

				An issuer outside the VAT system sets hr-fiskal-vat-system to N, and
				then must not charge VAT. An invoice from the bound paper invoice
				book, entered after the till broke down, carries its paper number in
				hr-fiskal-paper-number.

				An invoice with no issue time, dated today or not dated at all, is
				given the current date and time in Croatia. Fiscalization needs the
				issue time, as it goes into the ZKI (the issuer's protection code).

				Once fiscalized, the invoice is stamped with the ZKI, the JIR and the
				URL of the QR code to print.
			`),
		},
		Sources: []*cbc.Source{
			{
				Title: i18n.String{
					i18n.EN: "Porezna uprava - Fiscalization of invoices in final consumption",
					i18n.HR: "Porezna uprava - Fiskalizacija u krajnjoj potrošnji",
				},
				URL: "https://porezna.gov.hr/fiskalizacija/gotovinski-racuni",
			},
			{
				Title: i18n.String{
					i18n.EN: "Fiscalization of invoices in final consumption: technical specification",
					i18n.HR: "Fiskalizacija u krajnjoj potrošnji: tehnička specifikacija",
				},
				URL: "https://porezna.gov.hr/fiskalizacija/gotovinski-racuni/tehnicka-specifikacija",
			},
		},
		Extensions: extensions,
	}
}
