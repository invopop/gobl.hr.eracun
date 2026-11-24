package ubl

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/catalogues/untdid"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/currency"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/tax"
)

// consumptionTaxName is the HR-TB-2 name of the consumption tax (porez na
// potrošnju), HR:PP.
var consumptionTaxName = taxCategoryPrefix + eracun.TaxLabelPP.String()

// consumptionTaxReason is the reason of a PP charge (BT-104) and of its
// exempt entry in the VAT breakdown, as the official example writes them.
var consumptionTaxReason = "#" + consumptionTaxName + "#" + passThroughReasons[eracun.TaxLabelPP]

// exportConsumptionTax restates the hr regime's consumption tax (PP) the way
// the HR CIUS carries it, as in its example "eRacun-PDV-NEOP-PP-trosak" (CIUS
// 5.2). gobl.ubl writes PP as a tax of its own, in the VAT breakdown and the
// VAT total, which EN 16931 doesn't allow. The HR CIUS has PP as an amount
// left out of the VAT base instead: here a document level charge per PP rate,
// exempt from VAT (E, HR:PP), of the PP on the lines at that rate, with their
// amounts as its base and the rate as its multiplier. The charges get their
// exempt entry in the VAT breakdown, the VAT total (BT-110) leaves PP out, and
// the tax exclusive amount (BT-109) and the charges total (BT-108) take it in.
// The HR breakdown has PP out of the VAT scope, as hrTotals writes it.
//
// Each line with PP gets the item property HR:PP with its percent, so the
// buyer can tell which lines a charge covers. A line that lists PP before
// VAT gets its VAT category back, since gobl.ubl writes a line's first tax.
//
// A PP on a document level charge or discount has no base in the lines, and
// a line with PP and no VAT no VAT category, so the export refuses both, as
// the rules do, in case the invoice was not validated first.
func exportConsumptionTax(inv *bill.Invoice, out *goblubl.Invoice) error {
	pp := consumptionTaxTotal(inv)
	if pp == nil {
		return nil
	}
	for _, c := range inv.Charges {
		if c != nil && c.Taxes.Get(hr.TaxCategoryPP) != nil {
			return gobl.ErrValidation.WithReason("PP on a document level charge can't be written in an eRačun: charge it on the lines")
		}
	}
	for _, d := range inv.Discounts {
		if d != nil && d.Taxes.Get(hr.TaxCategoryPP) != nil {
			return gobl.ErrValidation.WithReason("PP on a document level discount can't be written in an eRačun: discount the lines instead")
		}
	}
	if err := exportConsumptionTaxLines(inv, out); err != nil {
		return err
	}

	ccy := inv.Currency.String()
	tt := &out.TaxTotal[0]
	tt.TaxSubtotal = slices.DeleteFunc(tt.TaxSubtotal, func(st goblubl.TaxSubtotal) bool {
		return st.TaxCategory.TaxScheme != nil && st.TaxCategory.TaxScheme.ID.Value == hr.TaxCategoryPP.String()
	})
	vat := num.AmountZero
	if t := vatTotal(inv); t != nil {
		vat = t.Amount
	}
	tt.TaxAmount = goblubl.NewAmount(vat, ccy)
	exportAccountingVATTotal(inv, out, vat)

	reason := passThroughReasons[eracun.TaxLabelPP]
	sum := inv.Currency.Def().Zero()
	for _, r := range pp.Rates {
		base := goblubl.NewAmount(r.Base, ccy)
		out.AllowanceCharge = append(out.AllowanceCharge, goblubl.AllowanceCharge{
			ChargeIndicator:         true,
			AllowanceChargeReason:   ptr(consumptionTaxReason),
			MultiplierFactorNumeric: ptr(r.Percent.StringWithoutSymbol()),
			Amount:                  goblubl.NewAmount(r.Amount, ccy),
			BaseAmount:              &base,
			TaxCategory: []*goblubl.TaxCategory{{
				ID:                 &goblubl.IDType{Value: en16931.TaxCategoryExempt.String()},
				Name:               ptr(consumptionTaxName),
				Percent:            ptr("0"),
				TaxExemptionReason: ptr(reason),
				TaxScheme:          &goblubl.TaxScheme{ID: goblubl.IDType{Value: goblubl.TaxSchemeVAT}},
			}},
		})
		tt.TaxSubtotal = append(tt.TaxSubtotal, goblubl.TaxSubtotal{
			TaxableAmount: goblubl.NewAmount(r.Amount, ccy),
			TaxAmount:     goblubl.NewAmount(num.AmountZero, ccy),
			TaxCategory: goblubl.TaxCategory{
				ID:                 &goblubl.IDType{Value: en16931.TaxCategoryExempt.String()},
				Percent:            ptr("0"),
				TaxExemptionReason: ptr(consumptionTaxReason),
				TaxScheme:          &goblubl.TaxScheme{ID: goblubl.IDType{Value: goblubl.TaxSchemeVAT}},
			},
		})
		sum = sum.Add(r.Amount)
	}

	m := &out.LegalMonetaryTotal
	m.TaxExclusiveAmount = goblubl.NewAmount(inv.Totals.Total.Add(sum), ccy)
	charges := sum
	if inv.Totals.Charge != nil {
		charges = charges.Add(*inv.Totals.Charge)
	}
	ct := goblubl.NewAmount(charges, ccy)
	m.ChargeTotalAmount = &ct
	return nil
}

// exportConsumptionTaxLines gives each line with PP its VAT category and
// the HR:PP item property.
func exportConsumptionTaxLines(inv *bill.Invoice, out *goblubl.Invoice) error {
	lines, err := pairedLines(inv, out)
	if err != nil {
		return err
	}
	for i, l := range inv.Lines {
		if l == nil || lines[i].Item == nil {
			continue
		}
		pp := l.Taxes.Get(hr.TaxCategoryPP)
		if pp == nil {
			continue
		}
		vat := l.Taxes.Get(tax.CategoryVAT)
		if vat == nil || pp.Percent == nil {
			return gobl.ErrValidation.WithReason("line %d with PP requires its percent and a VAT combo", i+1)
		}
		it := lines[i].Item
		if ctc := it.ClassifiedTaxCategory; ctc != nil && (ctc.TaxScheme == nil || ctc.TaxScheme.ID.Value != goblubl.TaxSchemeVAT) {
			ctc.TaxScheme = &goblubl.TaxScheme{ID: goblubl.IDType{Value: goblubl.TaxSchemeVAT}}
			ctc.ID = &goblubl.IDType{Value: vat.Ext.Get(untdid.ExtKeyTaxCategory).String()}
			ctc.Percent = ptr("0")
			if vat.Percent != nil {
				ctc.Percent = ptr(vat.Percent.StringWithoutSymbol())
			}
		}
		var props []goblubl.AdditionalItemProperty
		if it.AdditionalItemProperty != nil {
			props = *it.AdditionalItemProperty
		}
		props = append(props, goblubl.AdditionalItemProperty{
			Name:  eracun.ItemAttributeConsumptionTax,
			Value: pp.Percent.StringWithoutSymbol(),
		})
		it.AdditionalItemProperty = &props
	}
	return nil
}

// exportAccountingVATTotal writes the VAT total in the regime's currency
// (BT-111) from the VAT alone, where gobl.ubl converts every tax, PP too.
func exportAccountingVATTotal(inv *bill.Invoice, out *goblubl.Invoice, vat num.Amount) {
	if len(out.TaxTotal) < 2 {
		return
	}
	acc := inv.RegimeDef().GetCurrency()
	if rate := currency.MatchExchangeRate(inv.ExchangeRates, inv.Currency, acc); rate != nil {
		out.TaxTotal[1].TaxAmount = goblubl.NewAmount(rate.Convert(vat), acc.String())
	}
}

// importConsumptionTax restates the PP charges of a received eRačun as the hr
// regime's PP tax on the lines they cover, the shape the export writes them
// from, so that an invoice exported here imports as it was issued.
//
// A PP charge moves when it is one the export would write: exempt from VAT,
// with the HR tax category PP, a base, a rate and the export's reason, and
// nothing the export wouldn't write back, such as a reason of its own. It
// moves onto the lines whose item attribute HR:PP gives its rate, which lose
// that attribute, and the exempt VAT note goes too when no exempt amount is
// left: the import read it from the VAT breakdown's exempt entry, which the
// PP charges alone made up. Nothing moves when a line has more than one
// HR:PP attribute, since the move would lose the others.
//
// The move is made on a copy of the invoice, which replaces it only when the
// copy calculates, at each rate moved, the base and amount of that rate's
// charge, keeps the VAT, the total with tax and the payable amount (which
// gobl.ubl refuses to see change), and breaks no rule more often than the
// invoice did. The lines' amounts then add up to the charge's base, and each
// of them has VAT (rule 26). The supplier isn't outside VAT (rule 31), there
// are no O amounts (rule 32), and no PP charge is left (rule 30), so the PP of
// an invoice moves whole or not at all. Otherwise the invoice stays as
// imported, such as the official example, which has no HR:PP lines.
//
// It runs once the totals are reconciled, so a move can't hide a difference
// from the declared totals, and gobl.ubl doesn't run it on an invoice tagged
// for bypass, whose declared totals a move would leave stale. The UBL
// document is only read.
func importConsumptionTax(_ *goblubl.Format, doc goblubl.Document, env *gobl.Envelope) error {
	inv, _, ok := invoices(env, doc)
	if !ok || !slices.ContainsFunc(inv.Charges, isExportedConsumptionTaxCharge) ||
		slices.ContainsFunc(inv.Lines, hasSeveralConsumptionTaxAttributes) {
		return nil
	}
	data, err := json.Marshal(inv)
	if err != nil {
		return err
	}
	moved := new(bill.Invoice)
	if err := json.Unmarshal(data, moved); err != nil {
		return err
	}
	charges := moveConsumptionTax(moved)
	if len(charges) == 0 {
		return nil
	}
	if err := moved.Calculate(); err != nil {
		return nil //nolint:nilerr // an invoice that doesn't calculate keeps its PP charges
	}
	if vat := vatTotal(moved); vat == nil || !hasExemptRate(vat) {
		removeExemptNote(moved)
	}
	if !consumptionTaxMovedExactly(inv, moved, charges) || raisesNewFaults(inv, moved) {
		return nil
	}
	*inv = *moved
	return nil
}

// moveConsumptionTax moves each PP charge the export would write onto the
// lines whose HR:PP attribute gives its rate, as the PP tax, and returns the
// charges moved.
func moveConsumptionTax(inv *bill.Invoice) []*bill.Charge {
	var moved, kept []*bill.Charge
	for _, c := range inv.Charges {
		var lines []*bill.Line
		if isExportedConsumptionTaxCharge(c) {
			for _, l := range inv.Lines {
				if l != nil && l.Item != nil && hasConsumptionTaxAttribute(l.Item, *c.Percent) {
					lines = append(lines, l)
				}
			}
		}
		if len(lines) == 0 {
			kept = append(kept, c)
			continue
		}
		for _, l := range lines {
			p := *c.Percent
			l.Taxes = append(l.Taxes, &tax.Combo{Category: hr.TaxCategoryPP, Percent: &p})
			l.Item.Attributes = slices.DeleteFunc(l.Item.Attributes, func(a *org.Attribute) bool {
				return a != nil && a.Label == eracun.ItemAttributeConsumptionTax
			})
			if len(l.Item.Attributes) == 0 {
				l.Item.Attributes = nil
			}
		}
		moved = append(moved, c)
	}
	inv.Charges = kept
	return moved
}

// isExportedConsumptionTaxCharge says whether the charge is PP at a rate
// carried as the export writes it: its reason, base, rate and amount, and a
// VAT combo exempt with the HR tax category PP, and nothing else. Moving it
// onto the lines then loses nothing the export would write back.
func isExportedConsumptionTaxCharge(c *bill.Charge) bool {
	if c.Base == nil || c.Percent == nil {
		return false
	}
	want := &bill.Charge{
		Index:   c.Index,
		Reason:  consumptionTaxReason,
		Base:    c.Base,
		Percent: c.Percent,
		Amount:  c.Amount,
		Taxes: tax.Set{{
			Category: tax.CategoryVAT,
			Key:      tax.KeyExempt,
			Ext: tax.ExtensionsOf(cbc.CodeMap{
				eracun.ExtKeyTaxCategory: eracun.TaxLabelPP,
				untdid.ExtKeyTaxCategory: en16931.TaxCategoryExempt,
			}),
		}},
	}
	got, err := json.Marshal(c)
	exp, err2 := json.Marshal(want)
	return err == nil && err2 == nil && bytes.Equal(got, exp)
}

// hasConsumptionTaxAttribute says whether the item has the HR:PP attribute
// with the rate, written as the export does, without the percent sign.
func hasConsumptionTaxAttribute(it *org.Item, rate num.Percentage) bool {
	for _, a := range it.Attributes {
		if a == nil || a.Label != eracun.ItemAttributeConsumptionTax {
			continue
		}
		if p, err := num.PercentageFromString(a.Text + "%"); err == nil && p.Equals(rate) {
			return true
		}
	}
	return false
}

// hasSeveralConsumptionTaxAttributes says whether the line has more than one
// HR:PP attribute. The export writes one, with the line's PP rate, so a move
// would lose the others.
func hasSeveralConsumptionTaxAttributes(l *bill.Line) bool {
	n := 0
	if l != nil && l.Item != nil {
		for _, a := range l.Item.Attributes {
			if a != nil && a.Label == eracun.ItemAttributeConsumptionTax {
				n++
			}
		}
	}
	return n > 1
}

// removeExemptNote removes the invoice's exempt VAT note.
func removeExemptNote(inv *bill.Invoice) {
	inv.Tax.Notes = slices.DeleteFunc(inv.Tax.Notes, func(n *tax.Note) bool {
		return n != nil && n.Category == tax.CategoryVAT && n.Ext.Get(untdid.ExtKeyTaxCategory) == en16931.TaxCategoryExempt
	})
	if len(inv.Tax.Notes) == 0 {
		inv.Tax.Notes = nil
	}
}

// consumptionTaxMovedExactly says whether the invoice with its PP moved onto
// the lines charges, at each rate moved, the base and amount of that rate's
// charge, with the VAT, the total with tax and the payable amount unchanged.
func consumptionTaxMovedExactly(inv, moved *bill.Invoice, charges []*bill.Charge) bool {
	was, is := inv.Totals, moved.Totals
	if !is.TotalWithTax.Equals(was.TotalWithTax) || !is.Payable.Equals(was.Payable) || !vatAmount(moved).Equals(vatAmount(inv)) {
		return false
	}
	var rates []*tax.RateTotal
	if pp := consumptionTaxTotal(moved); pp != nil {
		rates = pp.Rates
	}
	for _, c := range charges {
		i := slices.IndexFunc(rates, func(r *tax.RateTotal) bool {
			return r.Percent != nil && r.Percent.Equals(*c.Percent)
		})
		if i < 0 || !rates[i].Base.Equals(*c.Base) || !rates[i].Amount.Equals(c.Amount) {
			return false
		}
	}
	return true
}

func vatAmount(inv *bill.Invoice) num.Amount {
	a := num.AmountZero
	if vat := vatTotal(inv); vat != nil {
		a = vat.Amount
	}
	return a
}

// raisesNewFaults says whether the invoice with its PP moved breaks a rule
// more often than the invoice as imported. Faults are counted by code, since
// their paths shift as the charges go.
func raisesNewFaults(inv, moved *bill.Invoice) bool {
	had := faultCounts(inv)
	for code, n := range faultCounts(moved) {
		if n > had[code] {
			return true
		}
	}
	return false
}

// faultCounts provides how often the invoice breaks each rule: the number of
// places each fault code is raised at.
func faultCounts(inv *bill.Invoice) map[rules.Code]int {
	counts := make(map[rules.Code]int)
	if faults := rules.Validate(inv); faults != nil {
		for _, f := range faults.List() {
			counts[f.Code()] += len(f.Paths())
		}
	}
	return counts
}

func consumptionTaxTotal(inv *bill.Invoice) *tax.CategoryTotal {
	if inv.Totals == nil || inv.Totals.Taxes == nil {
		return nil
	}
	return inv.Totals.Taxes.Category(hr.TaxCategoryPP)
}

func ptr(s string) *string {
	return &s
}
