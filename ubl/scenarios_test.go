package ubl_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/pay"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/tax"
	"github.com/invopop/phorm"
	"github.com/stretchr/testify/require"
)

// TestScenarios builds invoices in GOBL, the way an issuer sends them, one
// shape per row, and holds the addon to three promises:
//
//   - an invoice the addon accepts exports to UBL that the Tax
//     Administration's rules accept (checked with -validate);
//   - an invoice the addon refuses is refused for the expected reason, and
//     the official rule the addon's rule stands for refuses the export too
//     (checked with -validate);
//   - the content the schematron can't judge, such as which exemption reason
//     a line carries, is right.
//
// TestExamples covers the other direction: the official samples imported
// into GOBL. Those samples are valid by construction, so they can't show the
// addon letting an invalid invoice through to the schematron.
//
// The rows where GOBL refuses an invoice and phorm must raise the matching
// rule double as a canary: a phorm that checks nothing fails them.
//
// A row that fails today says why in pending, and what its failure shows.
// It is skipped while it fails that way, fails if it fails any other way,
// and fails once it passes, so the marker goes with the fix.
func TestScenarios(t *testing.T) {
	var pc *phorm.Client
	if *validate {
		pc = phormClient(t)
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			sc.report(t, sc.run(t, pc), pc != nil)
		})
	}
}

type scenario struct {
	name string
	// edit turns the base invoice into the row's shape.
	edit func(*bill.Invoice)
	// fault is what GOBL must refuse the invoice with: a fault code, or, for
	// a rule GOBL doesn't have yet, the business rule its message would cite.
	// Empty when GOBL must accept the invoice.
	fault string
	// rule is the official rule phorm must raise on the export of an invoice
	// GOBL refuses, to show that the addon's rule stands for it. The invoice
	// must still export; leave rule empty when it can't.
	rule string
	// check returns what is wrong with the legal content of the export.
	check func(*goblubl.Invoice) []string
	// pending says why the row fails today.
	pending *knownFailure
}

// knownFailure is why a row fails today, and the text each of its problems
// shows while it fails that way.
type knownFailure struct {
	why   string
	shows string
}

var scenarios = []scenario{
	// The VAT categories of HR-TB-2, from an issuer in the VAT system.
	{name: "standard 25%"},
	{name: "standard 13%", edit: func(inv *bill.Invoice) {
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyStandard, tax.RateReduced)}
	}},
	{name: "standard 5%", edit: func(inv *bill.Invoice) {
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyStandard, tax.RateSuperReduced)}
	}},
	{name: "standard 25%, 13% and 5% lines", edit: func(inv *bill.Invoice) {
		inv.Lines = append(inv.Lines,
			line("Knjiga", 3000, vat(tax.KeyStandard, tax.RateReduced)),
			line("Kruh", 200, vat(tax.KeyStandard, tax.RateSuperReduced)),
		)
	}},
	{name: "prices include VAT", edit: func(inv *bill.Invoice) {
		inv.Tax.PricesInclude = tax.CategoryVAT
	}},
	{name: "zero rated", edit: func(inv *bill.Invoice) {
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyZero, "")}
	}},
	{name: "exempt with a reason", edit: func(inv *bill.Invoice) {
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyExempt, "")}
		addNote(inv, tax.KeyExempt, "Oslobođeno PDV čl. 39. Zakona o PDV-u")
	}},
	{name: "reverse charge", edit: func(inv *bill.Invoice) {
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyReverseCharge, "")}
		addNote(inv, tax.KeyReverseCharge, "Prijenos porezne obveze čl. 75. st. 3. Zakona o PDV-u")
	}},
	{name: "export outside the EU", edit: export},

	// An eRačun goes only to a taxpayer established in Croatia (Art. 2(24)
	// of the Fiscalisation Act), so the HR schematron's foreign buyers,
	// which it accepts, are refused by GOBL alone. An intra-community supply
	// always has one, so BR-IC-11 and BR-IC-12, which the en16931 addon
	// leaves to the schematron, can't be reached here.
	{name: "intra-community supply", edit: intraCommunity, fault: "GOBL-HR-ERACUN-BILL-INVOICE-22"},
	{name: "export to a buyer outside Croatia", edit: func(inv *bill.Invoice) {
		export(inv)
		inv.Customer = customerNonEU()
	}, fault: "GOBL-HR-ERACUN-BILL-INVOICE-23"},

	// Amounts collected for a third party, outside the VAT base (HR-TB-2
	// labels POVNAK, PP, PPMV and N).
	{name: "pass-through POVNAK on a line", edit: func(inv *bill.Invoice) {
		inv.Lines = append(inv.Lines, line("Povratna naknada", 7, passThrough(eracun.TaxLabelPOVNAK)))
		addNote(inv, tax.KeyExempt, "Povratna naknada")
	}, check: lineLabel(1, "HR:POVNAK")},
	{name: "pass-through PP charge", edit: func(inv *bill.Invoice) {
		inv.Charges = []*bill.Charge{{
			Reason: "Porez na potrošnju",
			Amount: num.MakeAmount(300, 2),
			Taxes:  tax.Set{passThrough(eracun.TaxLabelPP)},
		}}
		addNote(inv, tax.KeyExempt, "Porez na potrošnju")
	}},
	{name: "exempt line beside a pass-through PP charge", edit: func(inv *bill.Invoice) {
		inv.Lines = append(inv.Lines, line("Usluga", 2000, vat(tax.KeyExempt, "")))
		inv.Charges = []*bill.Charge{{
			Reason: "Porez na potrošnju",
			Amount: num.MakeAmount(300, 2),
			Taxes:  tax.Set{passThrough(eracun.TaxLabelPP)},
		}}
		addNote(inv, tax.KeyExempt, "Oslobođeno PDV-a")
	},
		check: checks(lineReason(1, "Oslobođeno PDV-a"), chargeReason(0, "Porez na potrošnju")),
	},

	// An issuer outside the VAT system: a tax ID with only the country and its
	// OIB as an identity.
	{name: "issuer outside VAT, exempt as in the official sample", edit: func(inv *bill.Invoice) {
		inv.Supplier = supplierOutsideVAT()
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyExempt, "")}
		addNote(inv, tax.KeyExempt, "Nije u sustavu PDV čl. 90. st. 1. Zakona o PDV-u")
	}},
	{name: "issuer outside VAT charging VAT", edit: func(inv *bill.Invoice) {
		inv.Supplier = supplierOutsideVAT()
	}, fault: "GOBL-HR-ERACUN-BILL-INVOICE-13"},
	{name: "issuer outside VAT with no OIB identity", edit: func(inv *bill.Invoice) {
		inv.Supplier = supplierOutsideVAT()
		inv.Supplier.Identities = nil
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyExempt, "")}
		addNote(inv, tax.KeyExempt, "Nije u sustavu PDV čl. 90. st. 1. Zakona o PDV-u")
	}, fault: "GOBL-HR-ERACUN-BILL-INVOICE-14"},

	// Not subject to VAT (O): BR-O-02 forbids the seller's and the buyer's
	// VAT identifiers.
	{name: "outside scope, no VAT identifiers", edit: func(inv *bill.Invoice) {
		inv.Supplier = supplierOutsideVAT()
		inv.Customer = customerWithoutVATNumber()
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyOutsideScope, "")}
		addNote(inv, tax.KeyOutsideScope, "Nije predmet oporezivanja")
	}},
	{name: "outside scope from an issuer in the VAT system", edit: func(inv *bill.Invoice) {
		inv.Customer = customerWithoutVATNumber()
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyOutsideScope, "")}
		addNote(inv, tax.KeyOutsideScope, "Nije predmet oporezivanja")
	},
		fault: "GOBL-HR-ERACUN-BILL-INVOICE-15", rule: "BR-O-02",
	},
	{name: "outside scope to a buyer with a VAT number", edit: func(inv *bill.Invoice) {
		inv.Supplier = supplierOutsideVAT()
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyOutsideScope, "")}
		addNote(inv, tax.KeyOutsideScope, "Nije predmet oporezivanja")
	},
		fault: "GOBL-HR-ERACUN-BILL-INVOICE-16", rule: "BR-O-02",
	},

	// Document types (BT-3) and business processes (BT-23).
	{name: "credit note without KPD", edit: func(inv *bill.Invoice) {
		inv.Type = bill.InvoiceTypeCreditNote
		inv.Tax.Ext = inv.Tax.Ext.Set(eracun.ExtKeyProcess, "P10")
		inv.Preceding = []*org.DocumentRef{{Code: "1-P1-1", IssueDate: cal.NewDate(2026, 2, 1)}}
		inv.Lines[0].Item.Identities = nil
	}},
	{name: "corrective invoice", edit: func(inv *bill.Invoice) {
		inv.Type = bill.InvoiceTypeCorrective
		inv.Tax.Ext = inv.Tax.Ext.Set(eracun.ExtKeyProcess, "P10")
		inv.Preceding = []*org.DocumentRef{{Code: "1-P1-1", IssueDate: cal.NewDate(2026, 2, 1)}}
		inv.Lines[0].Quantity = num.MakeAmount(-1, 0)
		inv.Payment = nil
	}},
	{name: "advance invoice without KPD", edit: func(inv *bill.Invoice) {
		inv.Tags = tax.WithTags(tax.TagPrepayment)
		inv.Tax.Ext = inv.Tax.Ext.Set(eracun.ExtKeyProcess, "P4")
		inv.Lines[0].Item.Identities = nil
	}},
	{name: "final invoice after an advance", edit: func(inv *bill.Invoice) {
		inv.Tax.Ext = inv.Tax.Ext.Set(eracun.ExtKeyProcess, "P11")
		inv.Preceding = []*org.DocumentRef{{Code: "1-P1-1", IssueDate: cal.NewDate(2026, 2, 1)}}
	}},
	{name: "cash basis", edit: func(inv *bill.Invoice) {
		inv.Tags = tax.WithTags(hr.TagCashBasis)
	}},

	// What the addon refuses, and the official rule each refusal stands for.
	{name: "issued before 2026", edit: func(inv *bill.Invoice) {
		inv.IssueDate = cal.MakeDate(2025, 12, 31)
	}, fault: "GOBL-HR-ERACUN-BILL-INVOICE-01", rule: "HR-BR-40"},
	{name: "dated another day with no issue time", edit: func(inv *bill.Invoice) {
		inv.IssueTime = nil
	}, fault: "GOBL-HR-ERACUN-BILL-INVOICE-02", rule: "HR-BR-2"},
	{name: "no business process", edit: func(inv *bill.Invoice) {
		inv.Tax.Ext = inv.Tax.Ext.Delete(eracun.ExtKeyProcess)
	}, fault: "GOBL-HR-ERACUN-BILL-INVOICE-04", rule: "HR-BR-34"},
	{name: "line without KPD", edit: func(inv *bill.Invoice) {
		inv.Lines[0].Item.Identities = nil
	}, fault: "GOBL-HR-ERACUN-BILL-INVOICE-12", rule: "HR-BR-25"},
	{name: "buyer without a VAT number, standard line", edit: func(inv *bill.Invoice) {
		inv.Customer = customerWithoutVATNumber()
	}, fault: "GOBL-HR-ERACUN-BILL-INVOICE-11", rule: "HR-BR-S-1"},
	{name: "standard rate that isn't Croatian", edit: func(inv *bill.Invoice) {
		inv.Lines[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Percent: num.NewPercentage(22, 2)}}
	}, fault: "GOBL-HR-ERACUN-TAX-COMBO-01"},
	{name: "exempt with no reason", edit: func(inv *bill.Invoice) {
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyExempt, "")}
	}, fault: "GOBL-EU-EN16931-BILL-INVOICE-08", rule: "BR-E-10"},
	{name: "reverse charge with no reason", edit: func(inv *bill.Invoice) {
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyReverseCharge, "")}
	}, fault: "GOBL-EU-EN16931-BILL-INVOICE-08", rule: "BR-AE-10"},
	{name: "outside scope with no reason", edit: func(inv *bill.Invoice) {
		inv.Supplier = supplierOutsideVAT()
		inv.Customer = customerWithoutVATNumber()
		inv.Lines[0].Taxes = tax.Set{vat(tax.KeyOutsideScope, "")}
	}, fault: "GOBL-EU-EN16931-BILL-INVOICE-08", rule: "BR-O-10"},
	{name: "amount due with no payment terms", edit: func(inv *bill.Invoice) {
		inv.Payment = nil
	}, fault: "GOBL-EU-EN16931-BILL-INVOICE-06", rule: "BR-CO-25"},
}

func export(inv *bill.Invoice) {
	inv.Lines[0].Taxes = tax.Set{vat(tax.KeyExport, "")}
	addNote(inv, tax.KeyExport, "Oslobođeno PDV čl. 45. Zakona o PDV-u")
}

func intraCommunity(inv *bill.Invoice) {
	inv.Customer = customerEU()
	inv.Lines[0].Taxes = tax.Set{vat(tax.KeyIntraCommunity, "")}
	addNote(inv, tax.KeyIntraCommunity, "Oslobođeno PDV čl. 41. Zakona o PDV-u")
	inv.Delivery = &bill.DeliveryDetails{
		Date:     cal.NewDate(2026, 3, 1),
		Receiver: &org.Party{Name: "Firma C GmbH", Addresses: []*org.Address{{Locality: "Berlin", Country: "DE"}}},
	}
}

// run builds the row's invoice and returns what is wrong with how GOBL and,
// when pc is set, phorm treat it.
func (sc scenario) run(t *testing.T, pc *phorm.Client) []string {
	t.Helper()
	inv := scenarioInvoice()
	if sc.edit != nil {
		sc.edit(inv)
	}
	env, err := gobl.Envelop(inv)
	require.NoError(t, err)
	var gerr error
	var faults rules.Faults
	if gerr = env.Calculate(); gerr == nil {
		if faults = rules.Validate(env); faults != nil {
			gerr = faults
		}
	}

	var problems []string
	switch {
	case sc.fault == "" && gerr != nil:
		return []string{"GOBL refuses a valid invoice: " + gerr.Error()}
	case sc.fault != "" && gerr == nil:
		problems = append(problems, "GOBL accepts it, want a fault with "+sc.fault)
	case sc.fault != "" && !refusedWith(gerr, faults, sc.fault):
		problems = append(problems, fmt.Sprintf("GOBL refuses it without %s: %s", sc.fault, gerr))
	}

	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	if err != nil {
		if sc.fault == "" {
			problems = append(problems, "export: "+err.Error())
		} else if pc != nil && sc.rule != "" {
			problems = append(problems, fmt.Sprintf("can't export the refused invoice to check %s: %s", sc.rule, err))
		}
		return problems
	}
	if sc.fault == "" && sc.check != nil {
		if doc, ok := out.(*goblubl.Invoice); ok {
			problems = append(problems, sc.check(doc)...)
		} else {
			problems = append(problems, fmt.Sprintf("export is a %T, not an invoice", out))
		}
	}
	if pc == nil {
		return problems
	}

	data, err := goblubl.Encode(out)
	require.NoError(t, err)
	errs := phormErrors(t, pc, hrubl.FormatHRCIUS.VESID(env.Extract()), data)
	switch {
	case sc.fault == "":
		for _, f := range errs {
			problems = append(problems, "phorm refuses what GOBL accepts: "+f.String())
		}
	case sc.rule != "" && !hasRule(errs, sc.rule):
		problems = append(problems, fmt.Sprintf("phorm doesn't raise %s, the rule GOBL's fault stands for; it raises %v", sc.rule, errs))
	}
	return problems
}

// report fails the row on its problems, or skips it while it fails the way
// its pending says. A pending row that passes is reported as fixed only when
// phorm ran, since a row may fail on phorm's side alone.
func (sc scenario) report(t *testing.T, problems []string, phormRan bool) {
	t.Helper()
	kf := sc.pending
	switch {
	case len(problems) == 0 && kf == nil:
	case len(problems) == 0 && phormRan:
		t.Errorf("passes now: remove pending %q", kf.why)
	case len(problems) == 0:
		t.Skipf("pending: %s (passes without phorm; run with -validate to see whether it is fixed)", kf.why)
	case kf != nil && allShow(problems, kf.shows):
		t.Skipf("pending: %s\n%s", kf.why, strings.Join(problems, "\n"))
	case kf != nil:
		t.Errorf("fails another way than pending says (%s):\n%s", kf.shows, strings.Join(problems, "\n"))
	default:
		t.Error(strings.Join(problems, "\n"))
	}
}

func allShow(problems []string, text string) bool {
	for _, p := range problems {
		if !strings.Contains(p, text) {
			return false
		}
	}
	return true
}

// phormErrors returns the errors phorm raises on the document, leaving out
// warnings and the rules in ignoredRules.
func phormErrors(t *testing.T, pc *phorm.Client, vesid string, data []byte) []finding {
	t.Helper()
	var out []finding
	for _, f := range phormValidate(t, pc, vesid, data) {
		if f.Level == "WARN" {
			continue
		}
		if _, ok := ignoredRules[f.Rule]; ok {
			continue
		}
		out = append(out, f)
	}
	return out
}

// refusedWith says whether GOBL refused the invoice with the fault: by its
// code when it is one, or else by the business rule its message cites.
func refusedWith(err error, faults rules.Faults, fault string) bool {
	if strings.HasPrefix(fault, "GOBL-") {
		return faults != nil && faults.HasCode(rules.Code(fault))
	}
	return strings.Contains(err.Error(), fault)
}

func hasRule(fs []finding, rule string) bool {
	for _, f := range fs {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

// scenarioInvoice is the base of every row: one line at 25% from an issuer
// in the VAT system to a Croatian business buyer, dated in the past with its
// issue time. It isn't testInvoice from export_test.go, which carries the
// cash-basis tag and a PP charge on top, so a row would test those too.
func scenarioInvoice() *bill.Invoice {
	return &bill.Invoice{
		Regime:    tax.WithRegime("HR"),
		Addons:    tax.WithAddons(eracun.V1),
		Series:    "1-P1",
		Code:      "1",
		Currency:  "EUR",
		IssueDate: cal.MakeDate(2026, 3, 1),
		IssueTime: cal.NewTime(12, 0, 0),
		Tax: &bill.Tax{
			Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyProcess: "P1"}),
		},
		Supplier: &org.Party{
			Name:      "TVRTKA A d.o.o.",
			TaxID:     &tax.Identity{Country: "HR", Code: "12345678903"},
			Addresses: []*org.Address{{Street: "Ulica 1", Locality: "Zagreb", Code: "10000", Country: "HR"}},
			People:    []*org.Person{operator()},
		},
		Customer: &org.Party{
			Name:      "Tvrtka B d.o.o.",
			TaxID:     &tax.Identity{Country: "HR", Code: "11111111119"},
			Addresses: []*org.Address{{Street: "Ulica 2", Locality: "Rijeka", Code: "51000", Country: "HR"}},
		},
		Lines: []*bill.Line{line("Proizvod", 10000, vat(tax.KeyStandard, tax.RateGeneral))},
		Payment: &bill.PaymentDetails{
			Terms: &pay.Terms{DueDates: []*pay.DueDate{{Date: cal.NewDate(2026, 3, 31), Percent: num.NewPercentage(100, 2)}}},
		},
	}
}

func operator() *org.Person {
	return &org.Person{
		Key:        eracun.PersonKeyOperator,
		Name:       &org.Name{Given: "Ivan", Surname: "Kovač"},
		Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}},
	}
}

// supplierOutsideVAT is an issuer outside the VAT system: a tax ID with only
// the country, and its OIB as an identity.
func supplierOutsideVAT() *org.Party {
	return &org.Party{
		Name:       "Obrt A",
		TaxID:      &tax.Identity{Country: "HR"},
		Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "12345678903"}},
		Addresses:  []*org.Address{{Street: "Ulica 1", Locality: "Zagreb", Code: "10000", Country: "HR"}},
		People:     []*org.Person{operator()},
	}
}

// customerWithoutVATNumber is a Croatian buyer reached through its eRačun
// address, with no VAT number.
func customerWithoutVATNumber() *org.Party {
	return &org.Party{
		Name:      "Udruga B",
		TaxID:     &tax.Identity{Country: "HR"},
		Inboxes:   []*org.Inbox{{Scheme: "9934", Code: "11111111119"}},
		Addresses: []*org.Address{{Street: "Ulica 2", Locality: "Rijeka", Code: "51000", Country: "HR"}},
	}
}

func customerEU() *org.Party {
	return &org.Party{
		Name:      "Firma C GmbH",
		TaxID:     &tax.Identity{Country: "DE", Code: "111111125"},
		Inboxes:   []*org.Inbox{{Scheme: "9930", Code: "DE111111125"}},
		Addresses: []*org.Address{{Street: "Straße 3", Locality: "Berlin", Code: "10115", Country: "DE"}},
	}
}

func customerNonEU() *org.Party {
	return &org.Party{
		Name:      "Company D Inc.",
		Inboxes:   []*org.Inbox{{Scheme: "0088", Code: "1234567890128"}},
		Addresses: []*org.Address{{Street: "Street 4", Locality: "New York", Code: "10001", Country: "US"}},
	}
}

// line is a line with a KPD classification, which most document types need
// (HR-BR-25).
func line(name string, cents int64, combo *tax.Combo) *bill.Line {
	return &bill.Line{
		Quantity: num.MakeAmount(1, 0),
		Item: &org.Item{
			Name:       name,
			Price:      num.NewAmount(cents, 2),
			Identities: []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "62.20.20"}},
		},
		Taxes: tax.Set{combo},
	}
}

func vat(key, rate cbc.Key) *tax.Combo {
	return &tax.Combo{Category: tax.CategoryVAT, Key: key, Rate: rate}
}

// passThrough is an amount collected for a third party: exempt in VAT, with
// its HR-TB-2 label.
func passThrough(label cbc.Code) *tax.Combo {
	return &tax.Combo{
		Category: tax.CategoryVAT,
		Key:      tax.KeyExempt,
		Ext:      tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: label}),
	}
}

func addNote(inv *bill.Invoice, key cbc.Key, text string) {
	inv.Tax.Notes = append(inv.Tax.Notes, &tax.Note{Category: tax.CategoryVAT, Key: key, Text: text})
}

// lineLabel checks the HR-TB-2 label written on a line's tax category.
func lineLabel(i int, want string) func(*goblubl.Invoice) []string {
	return func(doc *goblubl.Invoice) []string {
		ctc := lineTaxCategory(doc, i)
		if ctc == nil || ctc.Name == nil || *ctc.Name != want {
			return []string{fmt.Sprintf("line %d label is not %s", i+1, want)}
		}
		return nil
	}
}

// checks runs every check on the document.
func checks(cs ...func(*goblubl.Invoice) []string) func(*goblubl.Invoice) []string {
	return func(doc *goblubl.Invoice) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c(doc)...)
		}
		return out
	}
}

// lineReason checks the exemption reason of line i.
func lineReason(i int, want string) func(*goblubl.Invoice) []string {
	return func(doc *goblubl.Invoice) []string {
		ctc := lineTaxCategory(doc, i)
		if ctc == nil || ctc.TaxExemptionReason == nil || *ctc.TaxExemptionReason != want {
			return []string{fmt.Sprintf("line %d doesn't carry the exemption reason %q", i+1, want)}
		}
		return nil
	}
}

// chargeReason checks the exemption reason of document level charge or
// allowance i.
func chargeReason(i int, want string) func(*goblubl.Invoice) []string {
	return func(doc *goblubl.Invoice) []string {
		if i >= len(doc.AllowanceCharge) || len(doc.AllowanceCharge[i].TaxCategory) == 0 ||
			doc.AllowanceCharge[i].TaxCategory[0].TaxExemptionReason == nil ||
			*doc.AllowanceCharge[i].TaxCategory[0].TaxExemptionReason != want {
			return []string{fmt.Sprintf("charge %d doesn't carry the exemption reason %q", i+1, want)}
		}
		return nil
	}
}

func lineTaxCategory(doc *goblubl.Invoice, i int) *goblubl.ClassifiedTaxCategory {
	lines := doc.InvoiceLines
	if lines == nil {
		lines = doc.CreditNoteLines
	}
	if i >= len(lines) || lines[i].Item == nil {
		return nil
	}
	return lines[i].Item.ClassifiedTaxCategory
}
