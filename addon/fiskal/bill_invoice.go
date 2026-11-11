package fiskal

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
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
	inv.Code = normalizeCode(inv.Code)
	if inv.Tax == nil {
		inv.Tax = new(bill.Tax)
	}
	if !inv.Tax.Ext.Has(ExtKeySequenceScope) {
		inv.Tax.Ext = inv.Tax.Ext.Set(ExtKeySequenceScope, SequencePremises)
	}
	if inv.Supplier != nil && !inv.Supplier.Ext.Has(ExtKeyVATSystem) {
		inv.Supplier.Ext = inv.Supplier.Ext.Set(ExtKeyVATSystem, VATSystemIn)
	}
	if inv.Series == cbc.CodeEmpty {
		inv.Series = Series(inv.Tax.Ext)
	}
}

// normalizeCode removes the leading zeros of a code made only of digits,
// so "000042" becomes "42". CIS refuses leading zeros in the sequential
// number (BrOznRac), and Invopop's sequence step pads its numbers to six
// digits unless set otherwise. The invoice is numbered before it is
// validated, so a padded code would use up a number that is never issued.
// A code with other characters, or with only zeros, is left for rule 01
// to refuse.
func normalizeCode(code cbc.Code) cbc.Code {
	s := code.String()
	for _, c := range s {
		if c < '0' || c > '9' {
			return code
		}
	}
	if t := strings.TrimLeft(s, "0"); t != "" {
		return cbc.Code(t)
	}
	return code
}

// Series provides the invoice series that matches the numbering scope: the
// business premises label for scope P, such as "POS1", or the premises and
// device labels for scope N, such as "POS1/1". It is empty without the
// labels.
//
// An invoice counter belongs to one series. When Invopop numbers the
// invoices, its sequence service keeps one counter for each supplier and
// series, so this series gives one counter for each business premises, or
// for each device, as the numbering scope requires. A till that numbers its
// own invoices can leave the series empty.
func Series(ext tax.Extensions) cbc.Code {
	premises := ext.Get(ExtKeyPremises)
	if premises == cbc.CodeEmpty {
		return cbc.CodeEmpty
	}
	if ext.Get(ExtKeySequenceScope) != SequenceDevice {
		return premises
	}
	device := ext.Get(ExtKeyDevice)
	if device == cbc.CodeEmpty {
		return cbc.CodeEmpty
	}
	return premises + "/" + device
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
		// The code is not required: a draft has none until Invopop numbers it,
		// and GOBL signs an invoice only with a code.
		rules.Field("code",
			rules.AssertIfPresent("01", "invoice code must be the sequential number: digits with no leading zero (BrOznRac)",
				is.MatchesRegexp(sequentialPattern),
			),
		),
		rules.Assert("17", "invoice series must match the numbering scope: the premises label for scope P, or <premises>/<device> for scope N, so that each counter numbers one premises or one device (OznSlijed)",
			is.Func("series matches the numbering scope", seriesMatchesScope),
		),
		rules.Field("issue_time",
			rules.Assert("02", "invoice issue time is required for an invoice not dated today in Croatia (DatVrijeme)", is.Present),
		),
		// CIS receives the amounts in euros (spec v2.7, p. 10), but the VAT Act
		// (Art. 81) lets an invoice show them in another currency too. The
		// racun conversion converts the invoice with its exchange rate.
		rules.Assert("03", "invoice must be in EUR or provide an exchange rate to EUR: CIS receives the amounts in euros",
			currency.CanConvertTo(currency.EUR),
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
				rules.Assert("10", fmt.Sprintf("invoice tax ext %s must be up to 100 characters (ParagonBrRac)", ExtKeyPaperNumber),
					tax.ExtensionHasValidCode(ExtKeyPaperNumber),
				),
			),
		),
		rules.Field("supplier",
			rules.Field("ext",
				rules.Assert("09", fmt.Sprintf("invoice supplier ext %s must be Y or N (USustPdv)", ExtKeyVATSystem),
					tax.ExtensionHasValidCode(ExtKeyVATSystem),
				),
			),
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
				rules.Assert("16", fmt.Sprintf("invoice from a supplier outside the VAT system (%s %s) must not charge VAT: use the VAT key exempt (Art. 90.g of the VAT Act)", ExtKeyVATSystem, VATSystemOut),
					is.Func("charges no VAT", totalsChargeNoVAT),
				),
			),
		),
		// Rules 18 to 23 refuse what the racun conversion refuses. A valid
		// invoice must always convert: Invopop numbers it before it is
		// fiscalized, and a numbered invoice that is never issued leaves a
		// gap in the sequence, which Art. 9(2) of the Fiscalization Act
		// forbids.
		//
		// Rules 18, 19 and 21 are cases racun does not support yet:
		// corrections (EU3-29) and fees (EU3-34). Drop a rule when racun
		// supports its case.
		//
		// Rules 20, 22 and 23 stay: the invoice request has no element for a
		// rounding amount, a discount without taxes or a line without taxes.
		// Each changes the total but no tax breakdown, and CIS checks that
		// the total is the sum of the breakdowns (check v166, spec v2.7,
		// p. 212). A line must say how VAT applies to it, even when it is
		// exempt or outside its scope, for racun to put its amount in the
		// right breakdown.
		rules.Field("type",
			rules.Assert("18", "invoice type must be standard: corrections are not supported yet", is.In(bill.InvoiceTypeStandard)),
		),
		rules.Field("totals",
			rules.Assert("19", "invoice total must not be negative: corrections are not supported yet",
				is.Func("payable not negative", totalsPayableNotNegative),
			),
			rules.Assert("20", "invoice rounding is not supported: the invoice request has no element for it",
				is.Func("no rounding", totalsHaveNoRounding),
			),
		),
		rules.Field("charges",
			rules.Each(
				rules.Field("taxes",
					rules.Assert("21", "invoice charge requires VAT: fees without taxes (Naknade) are not supported yet", is.Present),
				),
			),
		),
		rules.Field("discounts",
			rules.Each(
				rules.Field("taxes",
					rules.Assert("22", "invoice discount requires VAT: the invoice request has no element for a discount without taxes", is.Present),
				),
			),
		),
		rules.Field("lines",
			rules.Each(
				rules.Field("taxes",
					rules.Assert("23", "invoice line requires VAT, with the key exempt or outside-scope when no VAT applies: the invoice request has no element for a line without taxes", is.Present),
				),
			),
		),
	)
}

// taxComboRules refuse the taxes that the racun conversion does not support
// yet, for the same reason as rules 18 to 22 of the invoice: taxes other
// than VAT (EU3-34), and reverse charge, whose report to CIS is still to
// confirm.
func taxComboRules() *rules.Set {
	return rules.For(new(tax.Combo),
		rules.Field("cat",
			rules.Assert("01", "tax category must be VAT: other taxes are not supported yet", is.In(tax.CategoryVAT)),
		),
		rules.Field("key",
			rules.Assert("02", "VAT key must not be reverse-charge or intra-community: not supported yet",
				is.Func("not reverse charge", keyNotReverseCharge),
			),
		),
	)
}

// -- Predicates -----------------------------------------------------------

func totalsPayableNotNegative(val any) bool {
	t, ok := val.(*bill.Totals)
	return !ok || t == nil || !t.Payable.IsNegative()
}

func totalsHaveNoRounding(val any) bool {
	t, ok := val.(*bill.Totals)
	return !ok || t == nil || t.Rounding == nil || t.Rounding.IsZero()
}

func keyNotReverseCharge(val any) bool {
	k, ok := val.(cbc.Key)
	return !ok || !k.In(tax.KeyReverseCharge, tax.KeyIntraCommunity)
}

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

func seriesMatchesScope(val any) bool {
	inv, ok := val.(*bill.Invoice)
	if !ok || inv == nil || inv.Tax == nil || inv.Series == cbc.CodeEmpty {
		return true
	}
	return inv.Series == Series(inv.Tax.Ext)
}

func outsideVATSystem(val any) bool {
	inv, ok := val.(*bill.Invoice)
	return ok && inv != nil && inv.Supplier != nil && inv.Supplier.Ext.Get(ExtKeyVATSystem) == VATSystemOut
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
