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
	exportSellerTaxRegistration(inv, out)
	exportPaymentMeansText(inv, out)
	removeOrderReferenceFiller(inv, out)
	if err := exportLineTaxCategories(inv, out); err != nil {
		return err
	}
	exportChargeTaxCategories(inv, out)
	if err := exportConsumptionTax(inv, out); err != nil {
		return err
	}
	if err := exportAmountsOutsideVAT(inv, out); err != nil {
		return err
	}
	if err := mergeTaxSubtotals(out); err != nil {
		return err
	}
	return exportHRFISK20Data(inv, out)
}

// mergeTaxSubtotals joins the VAT breakdown entries of the same category and
// rate. GOBL keeps a rate total for each set of extensions, so two exempt
// supplies with different VATEX codes are two exempt entries, and the amounts
// left out of the VAT base add another, where EN 16931 allows one (BR-E-01).
// The first exemption reason and code found are kept: those of an exempt
// supply, which come before those of the amounts left out of the VAT base.
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
// (BT-32), which the HR CIUS uses for a seller with no VAT identifier.
const taxSchemeOutsideVAT = "FRE"

// exportSellerTaxRegistration writes the seller's OIB where its VAT status
// puts it. A seller registered for VAT keeps the VAT identifier (BT-31) that
// gobl.ubl writes from its tax ID, except on an invoice with an amount not
// subject to VAT (O), which shows no VAT identifier (BR-O-02): as in the
// official example "Neoporezivi racun", it then has no tax registration. A
// seller that is not registered has no VAT identifier either: its OIB is its
// tax registration (BT-32) with the scheme the HR CIUS uses, as in the
// official example "eRacun-Nije u sustavu PDV". gobl.ubl writes other
// identities with the tax scope there too, with their type or LOC as the
// scheme, which becomes the same scheme. Without a VAT identifier the seller
// needs a legal registration (BR-CO-26): its OIB, unless an identity already
// gives one (BT-30).
func exportSellerTaxRegistration(inv *bill.Invoice, out *goblubl.Invoice) {
	p := out.AccountingSupplierParty.Party
	if p == nil || inv.Supplier == nil {
		return
	}
	notRegistered := eracun.NotRegisteredForVAT(inv.Supplier)
	if !notRegistered && !hasOutsideScopeRate(vatTotal(inv)) {
		return
	}
	p.PartyTaxScheme = slices.DeleteFunc(p.PartyTaxScheme, func(pts goblubl.PartyTaxScheme) bool {
		return pts.TaxScheme != nil && pts.TaxScheme.ID.Value == goblubl.TaxSchemeVAT
	})
	oib := eracun.PartyOIB(inv.Supplier)
	written := false
	for _, pts := range p.PartyTaxScheme {
		if pts.TaxScheme != nil {
			pts.TaxScheme.ID.Value = taxSchemeOutsideVAT
		}
		if pts.CompanyID != nil && eracun.NormalizeOIB(pts.CompanyID.Value) == oib {
			written = true
		}
	}
	if oib == cbc.CodeEmpty {
		return
	}
	if notRegistered && !written {
		p.PartyTaxScheme = append(p.PartyTaxScheme, goblubl.PartyTaxScheme{
			CompanyID: &goblubl.IDType{Value: oib.String()},
			TaxScheme: &goblubl.TaxScheme{ID: goblubl.IDType{Value: taxSchemeOutsideVAT}},
		})
	}
	if p.PartyLegalEntity == nil {
		p.PartyLegalEntity = new(goblubl.PartyLegalEntity)
	}
	if p.PartyLegalEntity.CompanyID == nil {
		p.PartyLegalEntity.CompanyID = &goblubl.IDType{Value: oib.String()}
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
func exportLineTaxCategories(inv *bill.Invoice, out *goblubl.Invoice) error {
	lines, err := pairedLines(inv, out)
	if err != nil {
		return err
	}
	for i, l := range inv.Lines {
		if l == nil || lines[i].Item == nil || lines[i].Item.ClassifiedTaxCategory == nil {
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
			if r, ok := exemptionReason(inv, tc.Ext); ok {
				ctc.TaxExemptionReason = &r
			}
		}
		if reason, ok := keptExemptionReason(l.Item.Meta, tc.Ext); ok {
			ctc.TaxExemptionReason = &reason
		}
	}
	return nil
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
// charge and allowance (HR-BR-11), writes exemptionReason in place of the VAT
// note gobl.ubl writes, which differs only for an amount left out of the VAT
// base, and writes back an exemption reason of its own (HR-BT-7).
func exportChargeTaxCategories(inv *bill.Invoice, out *goblubl.Invoice) {
	documentCharges(out, inv, func(ac *goblubl.AllowanceCharge, c documentCharge) {
		tc := c.taxes.Get(tax.CategoryVAT)
		if tc == nil {
			return
		}
		for _, cat := range ac.TaxCategory {
			if cat == nil || cat.TaxScheme == nil || cat.TaxScheme.ID.Value != goblubl.TaxSchemeVAT {
				continue
			}
			cat.Name = taxCategoryName(tc.Ext)
			if r, ok := exemptionReason(inv, tc.Ext); ok {
				cat.TaxExemptionReason = &r
			}
			if reason, ok := keptExemptionReason(*c.meta, tc.Ext); ok {
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
// (HR-BR-26). The amounts left out of the VAT base and the consumption tax
// are among them, as the core states both as exempt.
func exportHRFISK20Data(inv *bill.Invoice, out *goblubl.Invoice) error {
	data := new(HRFISK20Data)
	if inv.HasTags(hr.TagCashBasis) {
		data.HRObracunPDVPoNaplati = cashBasisText
	}
	vat := vatTotal(inv)
	pp := consumptionTaxTotal(inv)
	outside := amountsOutsideVAT(inv)
	if hasExemptRate(vat) || pp != nil || len(outside) > 0 {
		data.HRTaxTotal, data.HRLegalMonetaryTotal = hrTotals(inv, vat, pp, outside)
	}
	if data.HRObracunPDVPoNaplati == "" && data.HRTaxTotal == nil {
		return nil
	}
	addHRFISK20Data(out, data)
	return nil
}

func vatTotal(inv *bill.Invoice) *tax.CategoryTotal {
	if inv.Totals == nil || inv.Totals.Taxes == nil {
		return nil
	}
	return inv.Totals.Taxes.Category(tax.CategoryVAT)
}

// hasOutsideScopeRate says whether the VAT totals have an amount not subject
// to VAT (O).
func hasOutsideScopeRate(vat *tax.CategoryTotal) bool {
	return hasRateCategory(vat, en16931.TaxCategoryOutsideScope)
}

func hasExemptRate(vat *tax.CategoryTotal) bool {
	return hasRateCategory(vat, en16931.TaxCategoryExempt)
}

func hasRateCategory(vat *tax.CategoryTotal, category cbc.Code) bool {
	if vat == nil {
		return false
	}
	for _, r := range vat.Rates {
		if r.Ext.Get(untdid.ExtKeyTaxCategory) == category {
			return true
		}
	}
	return false
}

// hrTotals breaks the totals down by HR tax category: an entry for each VAT
// rate total, then an entry out of the VAT scope (O) for each HR tax category
// of the amounts left out of the VAT base, under its own scheme (HR-BR-29,
// HR-BR-32). The consumption tax follows, as in the official example: one O
// entry under its LOC scheme for all its rates.
func hrTotals(inv *bill.Invoice, vat, pp *tax.CategoryTotal, outside []outsideVATAmount) (*HRTaxTotal, *HRLegalMonetaryTotal) {
	ccy := inv.Currency
	amount := func(a num.Amount) goblubl.Amount {
		id := ccy.String()
		return goblubl.Amount{CurrencyID: &id, Value: ccy.Def().Rescale(a).String()}
	}
	outsideEntry := func(label cbc.Code, a num.Amount) HRTaxSubtotal {
		scheme, ok := outsideVATTaxSchemes[label]
		if !ok {
			scheme = goblubl.TaxSchemeVAT
		}
		return HRTaxSubtotal{
			TaxableAmount: amount(a),
			TaxAmount:     amount(num.AmountZero),
			HRTaxCategory: HRTaxCategory{
				ID:          en16931.TaxCategoryOutsideScope.String(),
				Name:        taxCategoryPrefix + label.String(),
				Percent:     "0",
				HRTaxScheme: HRTaxScheme{ID: scheme},
			},
		}
	}
	taxAmount := ccy.Def().Zero()
	outOfScope := ccy.Def().Zero()
	tt := new(HRTaxTotal)
	if vat != nil {
		for _, r := range vat.Rates {
			cat := HRTaxCategory{
				ID:          r.Ext.Get(untdid.ExtKeyTaxCategory).String(),
				Name:        taxCategoryPrefix + r.Ext.Get(eracun.ExtKeyTaxCategory).String(),
				Percent:     "0",
				HRTaxScheme: HRTaxScheme{ID: goblubl.TaxSchemeVAT},
			}
			if r.Percent != nil {
				cat.Percent = r.Percent.StringWithoutSymbol()
			}
			if cat.ID == en16931.TaxCategoryExempt.String() {
				if n := taxNote(inv, r.Ext); n != nil {
					cat.TaxExemptionReason = n.Text
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
	}
	for _, a := range outside {
		tt.HRTaxSubtotals = append(tt.HRTaxSubtotals, outsideEntry(a.label, a.amount))
		outOfScope = outOfScope.Add(a.amount)
	}
	// The tax exclusive amount as written (BT-109), with the PP charges.
	total := inv.Totals.Total
	if pp != nil {
		ppAmount := ccy.Def().Zero()
		for _, r := range pp.Rates {
			ppAmount = ppAmount.Add(r.Amount)
		}
		tt.HRTaxSubtotals = append(tt.HRTaxSubtotals, outsideEntry(eracun.TaxLabelPP, ppAmount))
		outOfScope = outOfScope.Add(ppAmount)
		total = total.Add(ppAmount)
	}
	tt.TaxAmount = amount(taxAmount)
	return tt, &HRLegalMonetaryTotal{
		TaxExclusiveAmount:    amount(total.Subtract(outOfScope)),
		OutOfScopeOfVATAmount: amount(outOfScope),
	}
}
