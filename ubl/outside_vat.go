package ubl

import (
	"slices"
	"strings"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/catalogues/untdid"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/tax"
)

// An amount left out of the VAT base is a GOBL line or document level charge
// with no taxes and its HR tax category (HR-TB-2) in its own extensions. EN
// 16931 has no place for it beside taxed amounts, so the HR CIUS encodes it:
// VAT category E with the HR tax category in the core, and category O under
// its own scheme in the HR breakdown. The export writes that encoding and the
// import takes it away again.

// vatexOutsideVAT is the VATEX code of an amount left out of the VAT base:
// Article 79(c) of the VAT Directive, which the Tax Administration's list
// gives as Art. 33(3) of the VAT Act for pass-through items (Fiskalizacija
// eRačuna i eIzvještavanje v1.15, appendix 16).
const vatexOutsideVAT = "VATEX-EU-79-C"

// outsideVATTaxSchemes are the tax schemes of the amounts left out of the VAT
// base that belong to another tax, as the HR breakdown names them. The
// others, such as a fee that is not taxed (N), keep VAT.
var outsideVATTaxSchemes = map[cbc.Code]string{
	eracun.TaxLabelPP:     "LOC",
	eracun.TaxLabelPOVNAK: "OTH",
	eracun.TaxLabelPPMV:   "CAR",
}

// outsideVATAmount is what an invoice leaves out of the VAT base under one
// HR tax category.
type outsideVATAmount struct {
	label  cbc.Code
	amount num.Amount
}

// amountsOutsideVAT adds up the lines and document level charges left out of
// the VAT base by HR tax category, in the order the invoice first has them.
func amountsOutsideVAT(inv *bill.Invoice) []outsideVATAmount {
	var out []outsideVATAmount
	add := func(label cbc.Code, amount num.Amount) {
		i := slices.IndexFunc(out, func(a outsideVATAmount) bool { return a.label == label })
		if i < 0 {
			out = append(out, outsideVATAmount{label: label, amount: amount})
			return
		}
		out[i].amount = out[i].amount.Add(amount)
	}
	for _, l := range inv.Lines {
		if eracun.LineOutsideVAT(l) && l.Total != nil {
			add(l.Ext.Get(eracun.ExtKeyTaxCategory), *l.Total)
		}
	}
	for _, c := range inv.Charges {
		if eracun.ChargeOutsideVAT(c) {
			add(c.Ext.Get(eracun.ExtKeyTaxCategory), c.Amount)
		}
	}
	return out
}

// outsideVATTaxCategory is the tax category the HR CIUS gives an amount left
// out of the VAT base: exempt (E), named with its HR tax category, with the
// pass-through VATEX code as its exemption reason (HR-BR-11, HR-BR-13,
// HR-BR-16, HR-BR-36).
func outsideVATTaxCategory(label cbc.Code) *goblubl.TaxCategory {
	return &goblubl.TaxCategory{
		ID:                     &goblubl.IDType{Value: en16931.TaxCategoryExempt.String()},
		Name:                   ptr(taxCategoryPrefix + label.String()),
		Percent:                ptr("0"),
		TaxExemptionReasonCode: ptr(vatexOutsideVAT),
		TaxScheme:              &goblubl.TaxScheme{ID: goblubl.IDType{Value: goblubl.TaxSchemeVAT}},
	}
}

// exportAmountsOutsideVAT writes the lines and document level charges left
// out of the VAT base the way the HR CIUS carries them. gobl.ubl gives a
// line or charge without taxes no tax category, and the VAT breakdown no
// entry for it.
//
// A charge's exemption reason text (HR-BT-7) is its own reason (BT-104), the
// name of the fee or tax, as the official examples have it. A line has none.
// A text kept on import is written back in either.
//
// Their amounts join the one exempt entry EN 16931 allows in the VAT
// breakdown (BR-E-01): the entry added here comes after those of gobl.ubl,
// and mergeTaxSubtotals keeps the first ground it finds, so an exempt supply
// on the same invoice gives the entry its code and text, and the
// pass-through code shows only on an invoice with no exempt supply.
func exportAmountsOutsideVAT(inv *bill.Invoice, out *goblubl.Invoice) error {
	lines, err := pairedLines(inv, out)
	if err != nil {
		return err
	}
	sum := inv.Currency.Def().Zero()
	for i, l := range inv.Lines {
		if !eracun.LineOutsideVAT(l) || lines[i].Item == nil || l.Total == nil {
			continue
		}
		tc := outsideVATTaxCategory(l.Ext.Get(eracun.ExtKeyTaxCategory))
		ctc := &goblubl.ClassifiedTaxCategory{
			ID:                     tc.ID,
			Name:                   tc.Name,
			Percent:                tc.Percent,
			TaxExemptionReasonCode: tc.TaxExemptionReasonCode,
			TaxScheme:              tc.TaxScheme,
		}
		if l.Item != nil {
			if reason, ok := l.Item.Meta[MetaKeyTaxExemptionReason]; ok {
				ctc.TaxExemptionReason = &reason
			}
		}
		lines[i].Item.ClassifiedTaxCategory = ctc
		sum = sum.Add(*l.Total)
	}
	documentCharges(out, inv, func(ac *goblubl.AllowanceCharge, c documentCharge) {
		if c.charge == nil || !eracun.ChargeOutsideVAT(c.charge) {
			return
		}
		tc := outsideVATTaxCategory(c.charge.Ext.Get(eracun.ExtKeyTaxCategory))
		if reason, ok := c.charge.Meta[MetaKeyTaxExemptionReason]; ok {
			tc.TaxExemptionReason = &reason
		} else if c.charge.Reason != "" {
			tc.TaxExemptionReason = ptr(c.charge.Reason)
		}
		ac.TaxCategory = []*goblubl.TaxCategory{tc}
		sum = sum.Add(c.charge.Amount)
	})
	if len(amountsOutsideVAT(inv)) == 0 {
		return nil
	}
	// gobl.ubl writes the tax total of an invoice with no taxes too.
	ccy := inv.Currency.String()
	st := goblubl.TaxSubtotal{
		TaxableAmount: goblubl.NewAmount(sum, ccy),
		TaxAmount:     goblubl.NewAmount(num.AmountZero, ccy),
		TaxCategory:   *outsideVATTaxCategory(cbc.CodeEmpty),
	}
	st.TaxCategory.Name = nil
	if n := exemptNote(inv); n != nil {
		st.TaxCategory.TaxExemptionReason = &n.Text
	}
	out.TaxTotal[0].TaxSubtotal = append(out.TaxTotal[0].TaxSubtotal, st)
	return nil
}

// exemptNote provides the invoice's VAT note for exempt amounts, if it has
// one.
func exemptNote(inv *bill.Invoice) *tax.Note {
	return taxNote(inv, tax.ExtensionsOf(cbc.CodeMap{untdid.ExtKeyTaxCategory: en16931.TaxCategoryExempt}))
}

// importAmountsOutsideVAT reads the lines and document level charges that an
// eRačun gives the exempt category named with the HR tax category of an
// amount left out of the VAT base: HR:POVNAK, HR:PP, HR:PPMV or HR:N. The
// exempt VAT combo gobl.ubl read for each is the HR CIUS encoding, so it
// goes, and the line or charge carries the HR tax category in its own
// extensions. An exemption reason text the export wouldn't write back is kept
// in its meta data.
//
// A document level allowance named that way can't be read as one: GOBL has
// no discount without taxes (BR-32). Its VAT combo gets the HR tax category,
// which the addon refuses (TAX-COMBO-02), so the invoice doesn't pass as one
// with a discount on an exempt supply.
func importAmountsOutsideVAT(in *goblubl.Invoice, inv *bill.Invoice, lines []goblubl.InvoiceLine) {
	for i, ul := range lines {
		l := inv.Lines[i]
		if l == nil || ul.Item == nil || ul.Item.ClassifiedTaxCategory == nil {
			continue
		}
		ctc := ul.Item.ClassifiedTaxCategory
		label, ok := outsideVATLabel(ctc.ID, ctc.Name)
		if !ok {
			continue
		}
		l.Taxes = nil
		l.Ext = l.Ext.Set(eracun.ExtKeyTaxCategory, label)
		if ctc.TaxExemptionReason != nil && l.Item != nil {
			if reason := goblubl.CleanString(*ctc.TaxExemptionReason); reason != "" {
				keepExemptionReason(&l.Item.Meta, reason)
			}
		}
	}
	documentCharges(in, inv, func(ac *goblubl.AllowanceCharge, c documentCharge) {
		for _, cat := range ac.TaxCategory {
			if cat == nil {
				continue
			}
			label, ok := outsideVATLabel(cat.ID, cat.Name)
			if !ok {
				continue
			}
			if c.charge == nil {
				if combo := c.taxes.Get(tax.CategoryVAT); combo != nil {
					combo.Ext = combo.Ext.Set(eracun.ExtKeyTaxCategory, label)
				}
				return
			}
			c.charge.Taxes = nil
			c.charge.Ext = c.charge.Ext.Set(eracun.ExtKeyTaxCategory, label)
			if cat.TaxExemptionReason != nil {
				if reason := goblubl.CleanString(*cat.TaxExemptionReason); reason != "" && reason != c.charge.Reason {
					keepExemptionReason(&c.charge.Meta, reason)
				}
			}
			return
		}
	})
}

// outsideVATLabel reads the HR tax category of an amount left out of the VAT
// base from the tax category of a line or charge: the exempt category (E),
// named with the HR-TB-2 prefix. A category with VAT is what it says it is,
// whatever its name.
func outsideVATLabel(id *goblubl.IDType, name *string) (cbc.Code, bool) {
	if id == nil || id.Value != en16931.TaxCategoryExempt.String() || name == nil {
		return cbc.CodeEmpty, false
	}
	s, ok := strings.CutPrefix(*name, taxCategoryPrefix)
	if !ok {
		return cbc.CodeEmpty, false
	}
	label := cbc.Code(s)
	return label, label.In(eracun.PassThroughTaxLabels...)
}
