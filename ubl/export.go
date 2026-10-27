package ubl

import (
	"slices"

	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/catalogues/cef"
	"github.com/invopop/gobl/catalogues/untdid"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"
)

// taxCategoryPrefix starts every HR tax category name (HR-TB-2).
const taxCategoryPrefix = "HR:"

// passThroughTaxSchemes are the tax schemes of the amounts left out of the
// VAT base that belong to another tax, as the HR extension breaks them down.
// The others, such as a non-taxable fee (N), keep VAT.
var passThroughTaxSchemes = map[cbc.Code]string{
	eracun.TaxCategoryPP:     "LOC",
	eracun.TaxCategoryPOVNAK: "OTH",
	eracun.TaxCategoryPPMV:   "CAR",
}

// exportHR applies the HR CIUS rules to an exported invoice.
func exportHR(_ *goblubl.Format, env *gobl.Envelope, doc goblubl.Document) error {
	inv, out, ok := invoices(env, doc)
	if !ok {
		return nil
	}
	// HR-BR-2
	if inv.IssueTime != nil {
		out.IssueTime = inv.IssueTime.String()
	}
	// BT-23
	if p := inv.Tax.GetExt(eracun.ExtKeyProcess); p != cbc.CodeEmpty {
		out.ProfileID = &goblubl.IDType{Value: p.String()}
	}
	if err := exportSellerContact(inv, out); err != nil {
		return err
	}
	exportSellerContactPoint(inv, out)
	exportSellerTaxRegistration(out)
	exportPaymentMeansText(inv, out)
	removeOrderReferenceFiller(inv, out)
	exportLineTaxCategories(inv, out)
	exportChargeTaxCategories(inv, out)
	if err := mergeTaxSubtotals(out); err != nil {
		return err
	}
	return exportHRFISK20Data(inv, out)
}

// mergeTaxSubtotals joins the VAT breakdown entries of the same category and
// rate. GOBL keeps a separate rate total for each HR tax category, so an
// exempt supply and an amount left out of the VAT base would otherwise be
// two exempt entries, where EN 16931 allows one (BR-E-01). The first
// exemption reason and code found are kept.
func mergeTaxSubtotals(out *goblubl.Invoice) error {
	for i := range out.TaxTotal {
		var merged []goblubl.TaxSubtotal
		for _, st := range out.TaxTotal[i].TaxSubtotal {
			j := slices.IndexFunc(merged, func(m goblubl.TaxSubtotal) bool {
				return sameTaxCategory(m.TaxCategory, st.TaxCategory)
			})
			if j < 0 {
				merged = append(merged, st)
				continue
			}
			var err error
			if merged[j].TaxableAmount, err = addAmounts(merged[j].TaxableAmount, st.TaxableAmount); err != nil {
				return err
			}
			if merged[j].TaxAmount, err = addAmounts(merged[j].TaxAmount, st.TaxAmount); err != nil {
				return err
			}
			mc := &merged[j].TaxCategory
			if mc.TaxExemptionReasonCode == nil {
				mc.TaxExemptionReasonCode = st.TaxCategory.TaxExemptionReasonCode
			}
			if mc.TaxExemptionReason == nil {
				mc.TaxExemptionReason = st.TaxCategory.TaxExemptionReason
			}
		}
		out.TaxTotal[i].TaxSubtotal = merged
	}
	return nil
}

func sameTaxCategory(a, b goblubl.TaxCategory) bool {
	if a.ID == nil || b.ID == nil || a.TaxScheme == nil || b.TaxScheme == nil {
		return false
	}
	return a.ID.Value == b.ID.Value && a.TaxScheme.ID.Value == b.TaxScheme.ID.Value &&
		ptrValue(a.Percent) == ptrValue(b.Percent)
}

func ptrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func addAmounts(a, b goblubl.Amount) (goblubl.Amount, error) {
	x, err := num.AmountFromString(a.Value)
	if err != nil {
		return a, err
	}
	y, err := num.AmountFromString(b.Value)
	if err != nil {
		return a, err
	}
	return goblubl.Amount{CurrencyID: a.CurrencyID, Value: x.Add(y).String()}, nil
}

// exportSellerContact writes the operator (HR-BT-4, HR-BT-5). An invoice
// without one, which the rules reject, is refused rather than written without
// the SellerContact (HR-BR-9, HR-BR-37), in case it was not validated first.
func exportSellerContact(inv *bill.Invoice, out *goblubl.Invoice) error {
	var op *org.Person
	if inv.Supplier != nil {
		op = eracun.Operator(inv.Supplier.People)
	}
	var id *org.Identity
	name := ""
	if op != nil {
		id = org.IdentityForType(op.Identities, hr.IdentityTypeOIB)
		name = eracun.OperatorName(op.Name)
	}
	if id == nil || id.Code == cbc.CodeEmpty || name == "" {
		return gobl.ErrValidation.WithReason("invoice supplier requires exactly one person with the operator key, a name and an OIB identity (HR-BR-9, HR-BR-37)")
	}
	oib := id.Code.String()
	out.AccountingSupplierParty.SellerContact = &goblubl.Contact{
		ID:   &oib,
		Name: &name,
	}
	return nil
}

// exportSellerContactPoint keeps a person with the operator key out of the
// seller's contact (BG-6), which gobl.ubl fills from the first supplier
// person: the contact is the first person without the key, if any.
func exportSellerContactPoint(inv *bill.Invoice, out *goblubl.Invoice) {
	party := out.AccountingSupplierParty.Party
	if inv.Supplier == nil || len(inv.Supplier.People) == 0 || party == nil || party.Contact == nil {
		return
	}
	if p := inv.Supplier.People[0]; p == nil || p.Key != eracun.PersonKeyOperator {
		return
	}
	c := party.Contact
	c.Name = nil
	for _, p := range inv.Supplier.People {
		if p != nil && p.Key != eracun.PersonKeyOperator {
			if n := goblubl.ContactName(p.Name); n != "" {
				c.Name = &n
			}
			break
		}
	}
	if c.ID == nil && c.Name == nil && c.Telephone == nil && c.ElectronicMail == nil {
		party.Contact = nil
	}
}

// taxSchemeOutsideVAT is the tax scheme of the seller's tax registration
// (BT-32), which the HR CIUS uses for a seller outside the VAT system.
const taxSchemeOutsideVAT = "FRE"

// exportSellerTaxRegistration gives the tax registration (BT-32) of a seller
// outside the VAT system, one without a VAT identifier (BT-31), the tax
// scheme the HR CIUS uses, where gobl.ubl writes LOC.
func exportSellerTaxRegistration(out *goblubl.Invoice) {
	p := out.AccountingSupplierParty.Party
	if p == nil {
		return
	}
	for _, pts := range p.PartyTaxScheme {
		if pts.TaxScheme != nil && pts.TaxScheme.ID.Value == goblubl.TaxSchemeVAT {
			return
		}
	}
	for _, pts := range p.PartyTaxScheme {
		if pts.TaxScheme != nil {
			pts.TaxScheme.ID.Value = taxSchemeOutsideVAT
		}
	}
}

// exportPaymentMeansText writes the payment means text (BT-82) where the HR
// CIUS puts it, the instruction note, instead of the name of the payment
// means code that EN 16931 uses.
func exportPaymentMeansText(inv *bill.Invoice, out *goblubl.Invoice) {
	if inv.Payment == nil || inv.Payment.Instructions == nil || inv.Payment.Instructions.Detail == "" || len(out.PaymentMeans) == 0 {
		return
	}
	pm := &out.PaymentMeans[0]
	pm.PaymentMeansCode.Name = nil
	pm.InstructionNote = []string{inv.Payment.Instructions.Detail}
}

// removeOrderReferenceFiller drops the "NA" purchase order reference that
// gobl.ubl adds for PEPPOL-EN16931-R003, a rule HR invoices don't have. It
// stays when it carries other references, which UBL can't write without it,
// and when "NA" is the purchase order code of the GOBL invoice. An imported
// "NA" order reference is not kept: gobl.ubl reads it as no order, so as not
// to make up a purchase order.
func removeOrderReferenceFiller(inv *bill.Invoice, out *goblubl.Invoice) {
	o := out.OrderReference
	if o == nil || o.ID != goblubl.OrderReferenceNotApplicable || o.SalesOrderID != "" || o.IssueDate != "" || o.CustomerReference != "" {
		return
	}
	if inv.Ordering != nil && len(inv.Ordering.Purchases) > 0 {
		return
	}
	out.OrderReference = nil
}

// exportLineTaxCategories names the tax category of each line, with the
// exemption reason the HR CIUS requires for E and O lines (HR-BR-16,
// HR-BR-36).
func exportLineTaxCategories(inv *bill.Invoice, out *goblubl.Invoice) {
	lines := ublLines(out)
	for i, l := range inv.Lines {
		if i >= len(lines) || l == nil || lines[i].Item == nil || lines[i].Item.ClassifiedTaxCategory == nil {
			continue
		}
		tc := l.Taxes.Get(tax.CategoryVAT)
		if tc == nil {
			continue
		}
		ctc := lines[i].Item.ClassifiedTaxCategory
		ctc.Name = taxCategoryName(tc.Ext)
		if tc.Ext.Get(untdid.ExtKeyTaxCategory).In(reasonRequiredCategories...) {
			if v := tc.Ext.Get(cef.ExtKeyVATEX).String(); v != "" {
				ctc.TaxExemptionReasonCode = &v
			}
			if n := taxNote(inv, tc.Ext); n != nil {
				ctc.TaxExemptionReason = &n.Text
			}
		}
		if reason, ok := keptExemptionReason(l.Item.Meta, tc.Ext); ok {
			ctc.TaxExemptionReason = &reason
		}
	}
}

// keptExemptionReason provides the exemption reason kept on import for a
// line or charge, if its VAT category is one that has exemption reasons.
func keptExemptionReason(meta cbc.Meta, ext tax.Extensions) (string, bool) {
	reason, ok := meta[MetaKeyTaxExemptionReason]
	if !ok || !ext.Get(untdid.ExtKeyTaxCategory).In(reasonAllowedCategories...) {
		return "", false
	}
	return reason, true
}

// exportChargeTaxCategories names the tax category of each document level
// charge and allowance (HR-BR-11), and writes back an exemption reason kept
// on import (HR-BT-7).
func exportChargeTaxCategories(inv *bill.Invoice, out *goblubl.Invoice) {
	documentCharges(out, inv, func(ac *goblubl.AllowanceCharge, ts tax.Set, meta *cbc.Meta) {
		tc := ts.Get(tax.CategoryVAT)
		if tc == nil {
			return
		}
		for _, cat := range ac.TaxCategory {
			if cat == nil || cat.TaxScheme == nil || cat.TaxScheme.ID.Value != goblubl.TaxSchemeVAT {
				continue
			}
			cat.Name = taxCategoryName(tc.Ext)
			if reason, ok := keptExemptionReason(*meta, tc.Ext); ok {
				cat.TaxExemptionReason = &reason
			}
		}
	})
}

func taxCategoryName(ext tax.Extensions) *string {
	c := ext.Get(eracun.ExtKeyTaxCategory)
	if c == cbc.CodeEmpty {
		return nil
	}
	n := taxCategoryPrefix + c.String()
	return &n
}

// exportHRFISK20Data adds the HR extension data: the cash basis VAT flag,
// and the HR VAT breakdown and totals when the invoice has exempt amounts
// (HR-BR-26), which is how amounts left out of the VAT base are carried.
func exportHRFISK20Data(inv *bill.Invoice, out *goblubl.Invoice) error {
	data := new(HRFISK20Data)
	if inv.HasTags(hr.TagCashBasis) {
		data.HRObracunPDVPoNaplati = cashBasisText
	}
	if vat := vatTotal(inv); vat != nil && hasExemptRate(vat) {
		data.HRTaxTotal, data.HRLegalMonetaryTotal = hrTotals(inv, vat)
	}
	if data.HRObracunPDVPoNaplati == "" && data.HRTaxTotal == nil {
		return nil
	}
	return addHRFISK20Data(out, data)
}

func vatTotal(inv *bill.Invoice) *tax.CategoryTotal {
	if inv.Totals == nil || inv.Totals.Taxes == nil {
		return nil
	}
	return inv.Totals.Taxes.Category(tax.CategoryVAT)
}

func hasExemptRate(vat *tax.CategoryTotal) bool {
	for _, r := range vat.Rates {
		if r.Ext.Get(untdid.ExtKeyTaxCategory) == en16931.TaxCategoryExempt {
			return true
		}
	}
	return false
}

// hrTotals breaks the VAT totals down by HR tax category. Amounts left out
// of the VAT base move to category O (HR-BR-29, HR-BR-32), and so does any
// other O amount.
func hrTotals(inv *bill.Invoice, vat *tax.CategoryTotal) (*HRTaxTotal, *HRLegalMonetaryTotal) {
	ccy := inv.Currency
	amount := func(a num.Amount) goblubl.Amount {
		id := ccy.String()
		return goblubl.Amount{CurrencyID: &id, Value: ccy.Def().Rescale(a).String()}
	}
	taxAmount := ccy.Def().Zero()
	outOfScope := ccy.Def().Zero()
	tt := new(HRTaxTotal)
	for _, r := range vat.Rates {
		label := r.Ext.Get(eracun.ExtKeyTaxCategory)
		cat := HRTaxCategory{
			ID:          r.Ext.Get(untdid.ExtKeyTaxCategory).String(),
			Name:        taxCategoryPrefix + label.String(),
			Percent:     "0",
			HRTaxScheme: HRTaxScheme{ID: goblubl.TaxSchemeVAT},
		}
		if label.In(eracun.PassThroughTaxCategories...) {
			cat.ID = en16931.TaxCategoryOutsideScope.String()
			if scheme, ok := passThroughTaxSchemes[label]; ok {
				cat.HRTaxScheme.ID = scheme
			}
		} else {
			if r.Percent != nil {
				cat.Percent = r.Percent.StringWithoutSymbol()
			}
			if cat.ID == en16931.TaxCategoryExempt.String() {
				if n := taxNote(inv, r.Ext); n != nil {
					cat.TaxExemptionReason = n.Text
				}
			}
		}
		if cat.ID == en16931.TaxCategoryOutsideScope.String() {
			outOfScope = outOfScope.Add(r.Base)
		}
		taxAmount = taxAmount.Add(r.Amount)
		tt.HRTaxSubtotals = append(tt.HRTaxSubtotals, HRTaxSubtotal{
			TaxableAmount: amount(r.Base),
			TaxAmount:     amount(r.Amount),
			HRTaxCategory: cat,
		})
	}
	tt.TaxAmount = amount(taxAmount)
	return tt, &HRLegalMonetaryTotal{
		TaxExclusiveAmount:    amount(inv.Totals.Total.Subtract(outOfScope)),
		OutOfScopeOfVATAmount: amount(outOfScope),
	}
}
