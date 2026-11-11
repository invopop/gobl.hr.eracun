package fiskal

import (
	"fmt"
	"regexp"

	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/currency"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/rules/is"
	"github.com/invopop/gobl/tax"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
)

// sequentialPattern is the sequential number of an invoice, its code: digits
// with no leading zero (BrOznRac).
var sequentialPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)

// -- Normalization --------------------------------------------------------

func normalizeInvoice(inv *bill.Invoice) {
	if inv == nil {
		return
	}
	normalizeIssueTime(inv)
	if inv.Tax == nil {
		inv.Tax = new(bill.Tax)
	}
	if !inv.Tax.Ext.Has(ExtKeySequenceScope) {
		inv.Tax.Ext = inv.Tax.Ext.Set(ExtKeySequenceScope, SequencePremises)
	}
	if !inv.Tax.Ext.Has(ExtKeyVATSystem) {
		inv.Tax.Ext = inv.Tax.Ext.Set(ExtKeyVATSystem, VATSystemIn)
	}
}

// normalizeIssueTime sets the issue time fiscalization needs to the current
// time in Croatia when the invoice is issued now: it has no issue time, and
// its issue date is empty or today. The issue date is set from the same
// moment. GOBL fills in only the date. An invoice dated another day keeps its
// empty time, which the rules then ask for.
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

// -- Rule set -------------------------------------------------------------

func billInvoiceRules() *rules.Set {
	return rules.For(new(bill.Invoice),
		rules.Field("code",
			rules.Assert("01", "invoice code is required and must be the sequential number: digits with no leading zero (BrOznRac)",
				is.Present,
				is.MatchesRegexp(sequentialPattern),
			),
		),
		rules.Field("issue_time",
			rules.Assert("02", "invoice issue time is required for an invoice not dated today in Croatia (DatVrijeme)", is.Present),
		),
		rules.Field("currency",
			rules.Assert("03", "invoice currency must be EUR", is.In(currency.EUR)),
		),
		rules.Field("tax",
			rules.Assert("04", "invoice tax is required", is.Present),
			rules.Field("ext",
				rules.Assert("05", fmt.Sprintf("invoice tax ext requires the business premises (%s) and device (%s) labels (BrRac)", ExtKeyPremises, ExtKeyDevice),
					tax.ExtensionsRequire(ExtKeyPremises, ExtKeyDevice),
				),
				rules.Assert("06", fmt.Sprintf("invoice tax ext %s must be up to 20 letters and digits (OznPosPr)", ExtKeyPremises),
					tax.ExtensionHasValidCode(ExtKeyPremises),
				),
				rules.Assert("07", fmt.Sprintf("invoice tax ext %s must be up to 20 digits with no leading zero (OznNapUr)", ExtKeyDevice),
					tax.ExtensionHasValidCode(ExtKeyDevice),
				),
				rules.Assert("08", fmt.Sprintf("invoice tax ext %s must be P or N (OznSlijed)", ExtKeySequenceScope),
					tax.ExtensionHasValidCode(ExtKeySequenceScope),
				),
				rules.Assert("09", fmt.Sprintf("invoice tax ext %s must be Y or N (USustPdv)", ExtKeyVATSystem),
					tax.ExtensionHasValidCode(ExtKeyVATSystem),
				),
				rules.Assert("10", fmt.Sprintf("invoice tax ext %s must be up to 100 characters (ParagonBrRac)", ExtKeyPaperNumber),
					tax.ExtensionHasValidCode(ExtKeyPaperNumber),
				),
			),
		),
		rules.Field("supplier",
			rules.Field("tax_id",
				rules.Assert("11", "invoice supplier tax ID is required: the issuer's OIB (Oib)", is.Present),
				rules.Field("country",
					rules.Assert("12", "invoice supplier tax ID must be Croatian (Oib)", is.In(l10n.HR.Tax())),
				),
				rules.Field("code",
					rules.Assert("13", "invoice supplier tax ID code is required: the issuer's OIB (Oib)", is.Present),
				),
			),
			rules.Field("people",
				rules.Assert("14", "invoice supplier requires exactly one person with the operator key: the person at the till, or the supplier for a self-service device (OibOper)",
					is.Func("has one operator", peopleHaveOneOperator),
				),
				rules.Each(
					rules.When(is.Func("operator", isOperator),
						rules.Field("identities",
							rules.Assert("15", "invoice supplier operator requires an identity of type OIB (OibOper)",
								is.Func("has OIB", identitiesHaveOIB),
							),
						),
					),
				),
			),
		),
		rules.When(is.Func("outside the VAT system", outsideVATSystem),
			rules.Field("totals",
				rules.Assert("16", fmt.Sprintf("invoice from an issuer outside the VAT system (%s %s) must not charge VAT (USustPdv)", ExtKeyVATSystem, VATSystemOut),
					is.Func("charges no VAT", totalsChargeNoVAT),
				),
			),
		),
	)
}

// -- Predicates -----------------------------------------------------------

func peopleHaveOneOperator(val any) bool {
	people, _ := val.([]*org.Person)
	return eracun.Operator(people) != nil
}

func isOperator(val any) bool {
	p, ok := val.(*org.Person)
	return ok && p != nil && p.Key == eracun.PersonKeyOperator
}

func identitiesHaveOIB(val any) bool {
	ids, _ := val.([]*org.Identity)
	return org.IdentityForType(ids, hr.IdentityTypeOIB) != nil
}

func outsideVATSystem(val any) bool {
	inv, ok := val.(*bill.Invoice)
	return ok && inv != nil && inv.Tax != nil && inv.Tax.Ext.Get(ExtKeyVATSystem) == VATSystemOut
}

// totalsChargeNoVAT is true when no VAT rate in the totals has a percentage
// above zero. Exempt and outside-scope amounts are allowed.
func totalsChargeNoVAT(val any) bool {
	t, ok := val.(*bill.Totals)
	if !ok || t == nil || t.Taxes == nil {
		return true
	}
	ct := t.Taxes.Category(tax.CategoryVAT)
	if ct == nil {
		return true
	}
	for _, r := range ct.Rates {
		if r.Percent != nil && !r.Percent.IsZero() {
			return false
		}
	}
	return true
}
