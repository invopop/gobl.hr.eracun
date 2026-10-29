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
	normalizePartyEndpoint(inv.Supplier)
	normalizePartyEndpoint(inv.Customer)
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

// normalizePartyEndpoint gives a party with a Croatian tax ID its eRačun
// network address, 9934:<OIB>, unless it already has an ISO 6523 endpoint.
func normalizePartyEndpoint(p *org.Party) {
	if p == nil || p.TaxID == nil || p.TaxID.Country != l10n.HR.Tax() {
		return
	}
	if p.Endpoint(iso.ActorIDScheme) != nil {
		return
	}
	tID := *p.TaxID
	tax.NormalizeIdentity(&tID)
	if tID.Code == cbc.CodeEmpty {
		return
	}
	p.Endpoints = append(p.Endpoints, &org.Endpoint{
		URI: cbc.URI(iso.ActorIDScheme + "::" + SchemeIDOIB + ":" + tID.Code.String()),
	})
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
		),
		rules.Field("supplier",
			rules.Assert("05", "invoice supplier requires an eRačun address registered in AMS: a Croatian tax ID or an ISO 6523 endpoint, not an email (HR-BR-7)",
				is.Func("has endpoint", partyHasEndpoint),
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
	)
}

// partyHasEndpoint checks the party has an eRačun address, the ISO 6523
// identifier registered in AMS, as the UBL EndpointID it is written to. The
// export writes the first of an ISO 6523 endpoint, an email endpoint, or an
// inbox with an email or a scheme, and an email is not an eRačun address.
func partyHasEndpoint(val any) bool {
	p, ok := val.(*org.Party)
	if !ok || p == nil || p.Endpoint(iso.ActorIDScheme) != nil {
		return true
	}
	if p.Endpoint("mailto") != nil {
		return false
	}
	for _, ib := range p.Inboxes {
		if ib == nil {
			continue
		}
		if ib.Email != "" {
			return false
		}
		if ib.Scheme != cbc.CodeEmpty {
			return true
		}
	}
	return false
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
	if !ok || inv == nil {
		return false
	}
	for _, l := range inv.Lines {
		if l != nil && taxesHaveCategory(l.Taxes, buyerVATTaxCategories) {
			return true
		}
	}
	for _, c := range inv.Charges {
		if c != nil && taxesHaveCategory(c.Taxes, buyerVATTaxCategories) {
			return true
		}
	}
	for _, d := range inv.Discounts {
		if d != nil && taxesHaveCategory(d.Taxes, buyerVATTaxCategories) {
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
