package ubl

import (
	"slices"

	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/catalogues/untdid"
	"github.com/invopop/gobl/currency"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"
)

// consumptionTaxName is the HR-TB-2 name of the consumption tax (porez na
// potrošnju), HR:PP.
var consumptionTaxName = taxCategoryPrefix + eracun.TaxLabelPP.String()

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
// The lines say nothing of PP: the HR CIUS has no element that ties a line
// to the charge. A line that lists PP before VAT gets its VAT category back,
// since gobl.ubl writes a line's first tax.
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

	reason := eracun.TaxLabelName(eracun.TaxLabelPP)
	sum := inv.Currency.Def().Zero()
	for _, r := range pp.Rates {
		base := goblubl.NewAmount(r.Base, ccy)
		out.AllowanceCharge = append(out.AllowanceCharge, goblubl.AllowanceCharge{
			ChargeIndicator:         true,
			AllowanceChargeReason:   ptr("#" + consumptionTaxName + "#" + reason),
			MultiplierFactorNumeric: ptr(r.Percent.StringWithoutSymbol()),
			Amount:                  goblubl.NewAmount(r.Amount, ccy),
			BaseAmount:              &base,
			TaxCategory: []*goblubl.TaxCategory{{
				ID:                     &goblubl.IDType{Value: en16931.TaxCategoryExempt.String()},
				Name:                   ptr(consumptionTaxName),
				Percent:                ptr("0"),
				TaxExemptionReasonCode: ptr(vatexOutsideVAT),
				TaxExemptionReason:     ptr(reason),
				TaxScheme:              &goblubl.TaxScheme{ID: goblubl.IDType{Value: goblubl.TaxSchemeVAT}},
			}},
		})
		tt.TaxSubtotal = append(tt.TaxSubtotal, goblubl.TaxSubtotal{
			TaxableAmount: goblubl.NewAmount(r.Amount, ccy),
			TaxAmount:     goblubl.NewAmount(num.AmountZero, ccy),
			TaxCategory: goblubl.TaxCategory{
				ID:                     &goblubl.IDType{Value: en16931.TaxCategoryExempt.String()},
				Percent:                ptr("0"),
				TaxExemptionReasonCode: ptr(vatexOutsideVAT),
				TaxExemptionReason:     ptr("#" + consumptionTaxName + "#" + reason),
				TaxScheme:              &goblubl.TaxScheme{ID: goblubl.IDType{Value: goblubl.TaxSchemeVAT}},
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

// exportConsumptionTaxLines gives each line with PP its VAT category.
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

func consumptionTaxTotal(inv *bill.Invoice) *tax.CategoryTotal {
	if inv.Totals == nil || inv.Totals.Taxes == nil {
		return nil
	}
	return inv.Totals.Taxes.Category(hr.TaxCategoryPP)
}

func ptr(s string) *string {
	return &s
}
