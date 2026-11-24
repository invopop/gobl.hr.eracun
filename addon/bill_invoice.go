package eracun

import (
	"slices"
	"strings"

	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/catalogues/iso"
	"github.com/invopop/gobl/catalogues/untdid"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/rules/is"
	"github.com/invopop/gobl/tax"
)

// SchemeIDOIB is the ISO 6523 ICD of the Croatian OIB, used for the eRačun
// network address of a party.
const SchemeIDOIB = "9934"

// IdentityLabelKPD is the item identity label of the KPD classification
// (Klasifikacija proizvoda po djelatnostima), written as the UBL
// ItemClassificationCode listID.
const IdentityLabelKPD = "CG"

// kpdCodePattern is the form of a KPD 2025 code as the official code list
// writes it (HR-BR-CL-2): three pairs of digits joined by dots. The form
// alone is checked, not the list.
const kpdCodePattern = `^\d{2}\.\d{2}\.\d{2}$`

// firstIssueDate is the earliest issue date of an eRačun (HR-BR-40).
var firstIssueDate = cal.MakeDate(2026, 1, 1)

// kpdExemptDocumentTypes are the UNTDID 1001 document types whose lines need
// no KPD classification: advance invoices and credit notes (HR-BR-25).
var kpdExemptDocumentTypes = []cbc.Code{
	"386", "81", "83", "261", "262", "296", "308", "381", "396", "420", "458", "532",
}

// buyerVATTaxCategories are the EN 16931 tax categories that require the
// buyer's VAT identifier, on a line, a document level discount or a document
// level charge alike (HR-BR-S-1 to HR-BR-AE-3).
var buyerVATTaxCategories = []cbc.Code{
	en16931.TaxCategoryStandard, en16931.TaxCategoryZero,
	en16931.TaxCategoryExempt, en16931.TaxCategoryReverseCharge,
}

var invoiceCorrectionDefinitions = tax.CorrectionSet{
	{
		Schema:     bill.ShortSchemaInvoice,
		Normalizer: new(billCorrectionNormalizer),
	},
}

// billCorrectionNormalizer leaves the original operator out of a corrective
// document, which names the person who issued it (Art. 8(1) of the
// Fiscalisation Act), so the rules ask for a new one. The whole person goes,
// so they aren't written as the seller's contact (BG-6) instead.
//
// The corrective document is issued now, so the original's issue time goes
// too. Without an issue date option GOBL dates it today in UTC, so that date
// goes as well, and both are set again from the time in Croatia.
type billCorrectionNormalizer struct{}

func (*billCorrectionNormalizer) Normalize(doc any) {
	in, ok := doc.(*bill.CorrectionNormalize)
	if !ok {
		return
	}
	in.Invoice.IssueTime = nil
	if in.Opts.IssueDate == nil {
		in.Invoice.IssueDate = cal.Date{}
	}
	if sup := in.Invoice.Supplier; sup != nil {
		sup.People = slices.DeleteFunc(sup.People, func(p *org.Person) bool {
			return isOperator(p)
		})
	}
}

func normalizeInvoice(inv *bill.Invoice) {
	if inv == nil {
		return
	}
	normalizeIssueTime(inv)
	normalizeRounding(inv)
	normalizePartyEndpoint(inv.Supplier)
	normalizePartyEndpoint(inv.Customer)
}

// normalizeRounding sets the currency rounding rule on an invoice that gives
// none. EN 16931 checks the amounts the invoice shows: each VAT breakdown
// amount is its base times its rate, rounded (BR-CO-17), and the VAT total is
// their sum (BR-CO-14). GOBL's default precise rule rounds the sum of the
// unrounded amounts instead, which can leave the total a cent off the
// breakdown on an invoice with more than one VAT rate. gobl.dk.oioubl sets
// the rule the same way for its own sum rule (F-INV126). The eu-en16931-v2017
// addon this one requires gives the invoice its tax object first.
func normalizeRounding(inv *bill.Invoice) {
	if inv.Tax.Rounding == cbc.KeyEmpty {
		inv.Tax.Rounding = tax.RoundingRuleCurrency
	}
}

// normalizeIssueTime sets the issue time HR-BR-2 requires to the current
// time in Croatia when the invoice is issued now: it has no issue time, and
// its issue date is empty or today. The issue date is set from the same
// moment. GOBL fills in only the date. An invoice dated another day keeps its
// empty time, which rule 02 then asks for.
func normalizeIssueTime(inv *bill.Invoice) {
	if inv.IssueTime != nil {
		return
	}
	now := cal.ThisSecondIn(tax.RegimeDefFor(l10n.HR).TimeLocation())
	if !inv.IssueDate.IsZero() && inv.IssueDate != now.Date() {
		return
	}
	t := now.Time()
	inv.IssueDate = now.Date()
	inv.IssueTime = &t
}

// normalizePartyEndpoint gives a party with an OIB its eRačun network
// address, 9934:<OIB>, unless it already has an ISO 6523 address: an
// endpoint, or an inbox with a scheme, which the export writes otherwise.
// The OIB is the code of the party's Croatian tax ID or its OIB identity.
func normalizePartyEndpoint(p *org.Party) {
	if p == nil || p.Endpoint(iso.ActorIDScheme) != nil || hasSchemeInbox(p) {
		return
	}
	oib := PartyOIB(p)
	if oib == cbc.CodeEmpty {
		return
	}
	p.Endpoints = append(p.Endpoints, &org.Endpoint{
		URI: cbc.URI(iso.ActorIDScheme + "::" + SchemeIDOIB + ":" + oib.String()),
	})
}

func hasSchemeInbox(p *org.Party) bool {
	for _, ib := range p.Inboxes {
		if ib != nil && ib.Scheme != cbc.CodeEmpty {
			return true
		}
	}
	return false
}

func billInvoiceRules() *rules.Set {
	return rules.For(new(bill.Invoice),
		rules.Field("issue_date",
			rules.Assert("01", "invoice issue date must be on or after 2026-01-01 (HR-BR-40)",
				cal.DateAfter(firstIssueDate),
			),
		),
		rules.Field("issue_time",
			rules.Assert("02", "invoice issue time is required for an invoice not dated today in Croatia (HR-BR-2)", is.Present),
		),
		rules.Field("tax",
			rules.Assert("03", "invoice tax is required", is.Present),
			rules.Field("ext",
				rules.Assert("04", "invoice tax ext hr-eracun-process is required and must be P1 to P12 or P99:<label> (HR-BR-34)",
					tax.ExtensionsRequire(ExtKeyProcess),
					tax.ExtensionHasValidCode(ExtKeyProcess),
				),
			),
			rules.Field("rounding",
				rules.AssertIfPresent("24", "invoice tax rounding must be currency, so the VAT breakdown amounts add up to the VAT total (BR-CO-14); leave it empty to have it set",
					is.In(tax.RoundingRuleCurrency),
				),
			),
		),
		rules.Field("supplier",
			rules.Assert("05", "invoice supplier requires an eRačun address registered in AMS: an OIB, from its Croatian tax ID or an OIB identity, or an ISO 6523 endpoint, not an email (HR-BR-7)",
				is.Func("has endpoint", partyHasEndpoint),
			),
			rules.Assert("18", "invoice supplier eRačun address with scheme 9934 must be a valid OIB (HR-BR-7)",
				is.Func("9934 address is an OIB", partyOIBAddressValid),
			),
			rules.Assert("19", "invoice supplier eRačun address must be the supplier's own OIB, the one in its Croatian tax ID or OIB identity (HR-BR-7)",
				is.Func("address matches OIB", partyAddressMatchesOIB),
			),
			rules.Assert("17", "invoice supplier OIB identity must be the OIB in its tax ID code",
				is.Func("tax ID matches OIB identity", vatNumberMatchesOIBIdentity),
			),
			rules.Assert("15", "invoice supplier tax ID code is required: its OIB, whether or not it is registered for VAT",
				is.Func("has tax ID code", partyHasTaxIDCode),
			),
			rules.Field("ext",
				rules.Assert("14", "invoice supplier ext hr-eracun-vat-status is required: registered when the supplier is registered for VAT, not-registered when it is not, such as a small taxpayer",
					tax.ExtensionsRequire(ExtKeyVATStatus),
					tax.ExtensionHasValidCode(ExtKeyVATStatus),
				),
			),
			rules.Field("people",
				rules.Assert("06", "invoice supplier requires exactly one person with the operator key, the person who issued the invoice (HR-BR-9, HR-BR-37)",
					is.Func("has one operator", peopleHaveOneOperator),
				),
				// The operator label (HR-BT-4, HR-BR-37) needs no rule here:
				// GOBL requires a given name or surname on every person, so
				// OperatorName always has a label to write.
				rules.Each(
					rules.When(is.Func("operator", isOperator),
						rules.Field("identities",
							rules.Assert("07", "invoice supplier operator requires an identity of type OIB: their own OIB, or the supplier's where Art. 8(4) of the Fiscalisation Act allows it (HR-BR-9)",
								is.Func("has OIB", identitiesHaveOIB),
							),
						),
					),
				),
			),
		),
		rules.Field("customer",
			rules.Assert("08", "invoice customer is required", is.Present),
			rules.Assert("09", "invoice customer requires an eRačun address registered in AMS: a Croatian tax ID or an ISO 6523 endpoint, not an email (HR-BR-10)",
				is.Func("has endpoint", partyHasEndpoint),
			),
			rules.Assert("20", "invoice customer eRačun address with scheme 9934 must be a valid OIB (HR-BR-10)",
				is.Func("9934 address is an OIB", partyOIBAddressValid),
			),
			rules.Assert("21", "invoice customer eRačun address must be the customer's own OIB, the one in its Croatian VAT number or OIB identity (HR-BR-10)",
				is.Func("address matches OIB", partyAddressMatchesOIB),
			),
			rules.Assert("23", "invoice customer requires its OIB, which CIS records for the buyer: a Croatian VAT number, an OIB identity or a 9934:<OIB> address",
				is.Func("has OIB", partyHasOIB),
			),
			rules.Field("tax_id",
				rules.Assert("22", "invoice customer tax ID must be Croatian: an eRačun recipient is a taxpayer established in Croatia (Art. 2(24) of the Fiscalisation Act), so invoice a foreign buyer another way",
					is.Func("Croatian", taxIDIsCroatian),
				),
			),
		),
		rules.When(is.Func("has S, Z, E or AE taxes", invoiceNeedsBuyerVAT),
			rules.Field("customer",
				rules.Field("tax_id",
					rules.Assert("10", "invoice customer tax ID is required for S, Z, E and AE taxes (HR-BR-S-1 to HR-BR-AE-3)", is.Present),
					rules.Field("code",
						rules.Assert("11", "invoice customer tax ID code is required for S, Z, E and AE taxes (HR-BR-S-1 to HR-BR-AE-3)", is.Present),
					),
				),
			),
		),
		// An invoice with an O (not subject to VAT) amount shows no VAT
		// identifier (BR-O-02). The export leaves the supplier's out; GOBL
		// writes a customer's tax ID code as its VAT identifier, so the
		// customer gives its OIB another way.
		rules.When(is.Func("has O taxes", invoiceHasOutsideScope),
			rules.Field("totals",
				rules.Assert("32", "invoice with O taxes must not charge PP: the eRačun states PP as an exempt amount, which EN 16931 doesn't allow beside O (BR-O-11)",
					is.Func("charges no PP", totalsChargeNoConsumptionTax),
				),
			),
			rules.Field("customer",
				rules.Field("tax_id",
					rules.Field("code",
						rules.Assert("16", "invoice customer tax ID code must be empty for O taxes (BR-O-02 to BR-O-04)", is.Empty),
					),
				),
			),
		),
		// The HR schematron accepts a supplier's tax registration (BT-32)
		// wherever a seller tax ID is required, so this rule is the only
		// check that a supplier not registered for VAT charges none.
		rules.When(is.Func("supplier not registered for VAT", supplierNotRegisteredForVAT),
			rules.Field("totals",
				rules.Assert("13", "invoice from a supplier not registered for VAT (supplier ext hr-eracun-vat-status) must not charge VAT (Art. 90.g of the VAT Act)",
					is.Func("charges no VAT", totalsChargeNoVAT),
				),
				// How such a supplier states PP in an eRačun is not settled yet.
				rules.Assert("31", "invoice from a supplier not registered for VAT (supplier ext hr-eracun-vat-status) can't charge PP in an eRačun yet",
					is.Func("charges no PP", totalsChargeNoConsumptionTax),
				),
			),
		),
		rules.When(is.Func("requires KPD", invoiceRequiresKPD),
			rules.Field("lines",
				rules.Each(
					rules.Field("item",
						rules.Assert("12", "line item requires a KPD classification: an identity with label CG (HR-BR-25)",
							is.Func("has KPD", itemHasKPD),
						),
					),
				),
			),
		),
		// Every document type, including those that need no KPD: the code
		// list rule checks every CG code.
		rules.Field("lines",
			rules.Each(
				rules.Field("item",
					rules.Field("identities",
						rules.Each(
							rules.When(is.Func("KPD identity", identityIsKPD),
								rules.Field("code",
									rules.Assert("25", "line item KPD code (identity with label CG) must be a KPD 2025 code written as three pairs of digits joined by dots, such as 62.20.20 (HR-BR-CL-2)",
										is.Matches(kpdCodePattern),
									),
								),
							),
						),
					),
				),
			),
		),
		// The consumption tax (PP), which the export writes as an exempt
		// document level charge per rate, on the amounts of the lines that
		// carry it (CIUS example 5.2).
		rules.Field("lines",
			rules.Each(
				rules.When(is.Func("has PP", lineHasConsumptionTax),
					rules.Field("taxes",
						rules.Assert("26", "line taxes with PP require a VAT combo too, the line's VAT category",
							is.Func("has VAT", taxesHaveVAT),
						),
					),
				),
			),
		),
		rules.Field("discounts",
			rules.Each(
				rules.Field("taxes",
					rules.Assert("28", "discount taxes must not include PP: the eRačun states PP on the amounts of the lines that carry it, so discount those lines instead",
						is.Func("no PP", taxesHaveNoConsumptionTax),
					),
				),
			),
		),
		rules.Field("charges",
			rules.Each(
				rules.Field("taxes",
					rules.Assert("29", "charge taxes must not include PP: the eRačun states PP on the amounts of the lines that carry it, so charge it on those lines instead",
						is.Func("no PP", taxesHaveNoConsumptionTax),
					),
				),
			),
		),
		rules.When(is.Func("lines have PP", invoiceHasConsumptionTax),
			rules.Assert("30", "invoice with the PP tax on its lines must not also have a line, charge or discount with the HR tax category PP: state PP once, as the tax on the lines",
				is.Func("PP stated once", invoiceHasNoPassThroughPP),
			),
		),
	)
}

// partyAddress provides the party's eRačun address as gobl.ubl's export
// writes it to the UBL EndpointID (BT-34, BT-49): the first of an ISO 6523
// endpoint with a scheme and a code, an email endpoint with an address, or an
// inbox with an email or a scheme. found is false when that is an email,
// which is not an eRačun address, or nothing.
func partyAddress(p *org.Party) (scheme, code cbc.Code, found bool) {
	if e := p.Endpoint(iso.ActorIDScheme); e != nil {
		s, c, ok := strings.Cut(strings.TrimPrefix(e.URI.Opaque(), ":"), ":")
		if ok && s != "" && c != "" {
			return cbc.Code(s), cbc.Code(c), true
		}
	}
	if e := p.Endpoint("mailto"); e != nil && e.URI.Opaque() != "" {
		return cbc.CodeEmpty, cbc.CodeEmpty, false
	}
	for _, ib := range p.Inboxes {
		if ib == nil {
			continue
		}
		if ib.Email != "" {
			return cbc.CodeEmpty, cbc.CodeEmpty, false
		}
		if ib.Scheme != cbc.CodeEmpty {
			return ib.Scheme, ib.Code, true
		}
	}
	return cbc.CodeEmpty, cbc.CodeEmpty, false
}

// partyHasEndpoint checks the party has an eRačun address.
func partyHasEndpoint(val any) bool {
	p, ok := val.(*org.Party)
	if !ok || p == nil {
		return true
	}
	_, _, found := partyAddress(p)
	return found
}

// partyOIBAddressValid checks that an eRačun address with scheme 9934 holds a
// valid OIB, as AMS registers it: iso6523-actorid-upis::9934:<OIB>. AMS also
// registers other ISO 6523 identifiers, such as a GLN (0088), besides the
// OIB every participant has (AMS specification v1.4, MPS specification
// v1.3), so an address in another scheme is left alone. A party with no
// address is left to rules 05 and 09.
func partyOIBAddressValid(val any) bool {
	p, ok := val.(*org.Party)
	if !ok || p == nil {
		return true
	}
	scheme, code, found := partyAddress(p)
	return !found || scheme != SchemeIDOIB || validOIB(code.String())
}

// partyHasOIB checks the party has an OIB: from its Croatian VAT number, its
// OIB identity, or a valid 9934 address. CIS records the buyer's OIB with
// every eRačun (Primatelj.oibPorezniBroj), and every eRačun recipient has
// one, whatever other identifiers it uses (MPS specification v1.3).
func partyHasOIB(val any) bool {
	p, ok := val.(*org.Party)
	if !ok || p == nil || PartyOIB(p) != cbc.CodeEmpty {
		return true
	}
	scheme, code, found := partyAddress(p)
	return found && scheme == SchemeIDOIB && validOIB(code.String())
}

// partyAddressMatchesOIB checks that a party with an OIB, from its Croatian
// VAT number or its OIB identity, has it as its 9934 address too: the
// eRačun would otherwise go to another participant than the one it names.
// An address that is not a valid OIB is left to partyOIBAddressValid.
func partyAddressMatchesOIB(val any) bool {
	p, ok := val.(*org.Party)
	if !ok || p == nil {
		return true
	}
	scheme, code, found := partyAddress(p)
	if !found || scheme != SchemeIDOIB || !validOIB(code.String()) {
		return true
	}
	oib := PartyOIB(p)
	return oib == cbc.CodeEmpty || code == oib
}

// taxIDIsCroatian checks a tax ID belongs to Croatia. An eRačun is exchanged
// only between taxpayers established in Croatia (Art. 2(24) and 38 of the
// Fiscalisation Act), so a buyer with a foreign tax ID gets its invoice
// another way: on paper or as another electronic invoice.
func taxIDIsCroatian(val any) bool {
	tID, ok := val.(*tax.Identity)
	return !ok || tID == nil || tID.Country == l10n.HR.Tax()
}

func peopleHaveOneOperator(val any) bool {
	people, _ := val.([]*org.Person)
	n := 0
	for _, p := range people {
		if isOperator(p) {
			n++
		}
	}
	return n == 1
}

func isOperator(val any) bool {
	p, ok := val.(*org.Person)
	return ok && p != nil && p.Key == PersonKeyOperator
}

func identitiesHaveOIB(val any) bool {
	ids, _ := val.([]*org.Identity)
	return org.IdentityForType(ids, hr.IdentityTypeOIB) != nil
}

// PersonKeyOperator marks the supplier person who issued the invoice, the
// operator, who is not also the seller's contact (BG-6). Exactly one
// supplier person has it.
const PersonKeyOperator cbc.Key = "operator"

// Operator provides the person who issued the invoice: the supplier person
// with the operator key, when exactly one has it. No other person is taken
// in their place; the rules require exactly one, with a name and an OIB
// identity.
func Operator(people []*org.Person) *org.Person {
	var op *org.Person
	for _, p := range people {
		if !isOperator(p) {
			continue
		}
		if op != nil {
			return nil
		}
		op = p
	}
	return op
}

// OperatorName provides the operator label (HR-BT-4) of a person: the alias,
// or else the given name and surname.
func OperatorName(n *org.Name) string {
	if n == nil {
		return ""
	}
	if n.Alias != "" {
		return n.Alias
	}
	return strings.TrimSpace(n.Given + " " + n.Surname)
}

func invoiceNeedsBuyerVAT(val any) bool {
	inv, ok := val.(*bill.Invoice)
	return ok && inv != nil && invoiceHasTaxCategory(inv, buyerVATTaxCategories)
}

func invoiceHasOutsideScope(val any) bool {
	inv, ok := val.(*bill.Invoice)
	return ok && inv != nil && invoiceHasTaxCategory(inv, []cbc.Code{en16931.TaxCategoryOutsideScope})
}

// invoiceHasTaxCategory checks for a VAT combo with one of the EN 16931 tax
// categories on a line, a document level charge or a document level discount.
func invoiceHasTaxCategory(inv *bill.Invoice, codes []cbc.Code) bool {
	for _, l := range inv.Lines {
		if l != nil && taxesHaveCategory(l.Taxes, codes) {
			return true
		}
	}
	for _, c := range inv.Charges {
		if c != nil && taxesHaveCategory(c.Taxes, codes) {
			return true
		}
	}
	for _, d := range inv.Discounts {
		if d != nil && taxesHaveCategory(d.Taxes, codes) {
			return true
		}
	}
	return false
}

func taxesHaveCategory(ts tax.Set, codes []cbc.Code) bool {
	for _, tc := range ts {
		if tc != nil && tc.Category == tax.CategoryVAT && tc.Ext.Get(untdid.ExtKeyTaxCategory).In(codes...) {
			return true
		}
	}
	return false
}

func invoiceRequiresKPD(val any) bool {
	inv, ok := val.(*bill.Invoice)
	if !ok || inv == nil {
		return false
	}
	return inv.Tax == nil || !inv.Tax.Ext.Get(untdid.ExtKeyDocumentType).In(kpdExemptDocumentTypes...)
}

func identityIsKPD(val any) bool {
	id, ok := val.(*org.Identity)
	return ok && id != nil && id.Label == IdentityLabelKPD
}

func itemHasKPD(val any) bool {
	item, ok := val.(*org.Item)
	if !ok || item == nil {
		return true
	}
	for _, id := range item.Identities {
		if id != nil && id.Label == IdentityLabelKPD && id.Code != cbc.CodeEmpty {
			return true
		}
	}
	return false
}
