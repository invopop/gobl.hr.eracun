package ubl

import (
	"strings"

	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/catalogues/untdid"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"
)

// importHR reads the HR CIUS data of an imported invoice back into GOBL.
func importHR(_ *goblubl.Format, doc goblubl.Document, env *gobl.Envelope) error {
	inv, in, ok := invoices(env, doc)
	if !ok {
		return nil
	}
	importDocumentType(in, inv)
	if in.ProfileID != nil && in.ProfileID.Value != "" {
		setTaxExt(inv, eracun.ExtKeyProcess, cbc.Code(in.ProfileID.Value))
	}
	importOperator(in, inv)
	importPaymentMeansText(in, inv)
	importPassThroughTaxCategories(in, inv)
	importChargeExemptionReasons(in, inv)
	importLineExemptionReasons(in, inv)

	data, err := findHRFISK20Data(in)
	if err != nil {
		return err
	}
	if data != nil && data.HRObracunPDVPoNaplati != "" {
		inv.SetTags(append(inv.GetTags(), eracun.TagCashBasis)...)
	}
	return nil
}

// importDocumentType keeps the UNTDID 1001 code of a document GOBL has no
// invoice type for, such as an advance invoice (386), so it is written back
// unchanged.
func importDocumentType(in *goblubl.Invoice, inv *bill.Invoice) {
	if inv.Type != bill.InvoiceTypeOther {
		return
	}
	code := in.InvoiceTypeCode
	if code == nil {
		code = in.CreditNoteTypeCode
	}
	if code != nil && code.Value != "" {
		setTaxExt(inv, untdid.ExtKeyDocumentType, cbc.Code(code.Value))
	}
}

// importOperator adds the operator from the SellerContact to the supplier's
// people: the OIB goes to the person gobl.ubl read from the party contact
// (BG-6) when the names match, or to a new person. A SellerContact without
// an ID or a name adds nobody, so that the missing operator is the fault.
func importOperator(in *goblubl.Invoice, inv *bill.Invoice) {
	sc := in.AccountingSupplierParty.SellerContact
	if sc == nil || sc.ID == nil || sc.Name == nil || inv.Supplier == nil {
		return
	}
	oib := goblubl.CleanString(*sc.ID)
	name := goblubl.CleanString(*sc.Name)
	if oib == "" || name == "" {
		return
	}
	id := &org.Identity{Type: hr.IdentityTypeOIB, Code: cbc.Code(oib)}
	for _, p := range inv.Supplier.People {
		if p != nil && eracun.OperatorName(p.Name) == name {
			p.Identities = org.AddIdentity(p.Identities, id)
			return
		}
	}
	inv.Supplier.People = append(inv.Supplier.People, &org.Person{
		Key:        eracun.PersonKeyOperator,
		Name:       &org.Name{Given: name},
		Identities: []*org.Identity{id},
	})
}

// importPaymentMeansText reads the payment means text (BT-82) from where
// the HR CIUS puts it, the instruction note.
func importPaymentMeansText(in *goblubl.Invoice, inv *bill.Invoice) {
	if len(in.PaymentMeans) == 0 || inv.Payment == nil || inv.Payment.Instructions == nil {
		return
	}
	var notes []string
	for _, n := range in.PaymentMeans[0].InstructionNote {
		if n = goblubl.CleanString(n); n != "" {
			notes = append(notes, n)
		}
	}
	if len(notes) > 0 {
		inv.Payment.Instructions.Detail = strings.Join(notes, "\n")
	}
}

// importPassThroughTaxCategories sets the HR tax category of the lines and
// document level charges that carry an amount collected for a third party.
// The other categories follow from the tax key and rate.
func importPassThroughTaxCategories(in *goblubl.Invoice, inv *bill.Invoice) {
	for i, l := range ublLines(in) {
		if i >= len(inv.Lines) || inv.Lines[i] == nil || l.Item == nil || l.Item.ClassifiedTaxCategory == nil {
			continue
		}
		setPassThroughTaxCategory(inv.Lines[i].Taxes, l.Item.ClassifiedTaxCategory.Name)
	}
	documentCharges(in, inv, func(ac *goblubl.AllowanceCharge, ts tax.Set, _ *cbc.Meta) {
		for _, cat := range ac.TaxCategory {
			if cat != nil {
				setPassThroughTaxCategory(ts, cat.Name)
			}
		}
	})
}

// MetaKeyTaxExemptionReason keeps the VAT exemption reason of a document
// level charge or allowance (HR-BT-7), in its meta data, or of a line
// (HR-BT-13), in its item's, when the export would not write it back from the
// VAT note for its category.
const MetaKeyTaxExemptionReason cbc.Key = "hr-eracun-tax-exemption-reason"

// importChargeExemptionReasons keeps the exemption reasons of the document
// level charges and allowances that the VAT notes don't give back. GOBL has
// one exemption note per tax category, and no field for the reason of a
// single charge or allowance. The Tax Administration receives a charge's
// reason on fiscalization.
func importChargeExemptionReasons(in *goblubl.Invoice, inv *bill.Invoice) {
	documentCharges(in, inv, func(ac *goblubl.AllowanceCharge, ts tax.Set, meta *cbc.Meta) {
		tc := ts.Get(tax.CategoryVAT)
		if tc == nil {
			return
		}
		for _, cat := range ac.TaxCategory {
			if cat == nil || cat.TaxScheme == nil || cat.TaxScheme.ID.Value != goblubl.TaxSchemeVAT || cat.TaxExemptionReason == nil {
				continue
			}
			reason := goblubl.CleanString(*cat.TaxExemptionReason)
			if n := taxNote(inv, tc.Ext); reason == "" || (n != nil && n.Text == reason) {
				continue
			}
			setMeta(meta, MetaKeyTaxExemptionReason, reason)
		}
	})
}

// importLineExemptionReasons keeps the exemption reasons of the lines that
// the VAT notes don't give back. The export only writes the note's text for
// categories E and O (HR-BR-16, HR-BR-36).
func importLineExemptionReasons(in *goblubl.Invoice, inv *bill.Invoice) {
	for i, l := range ublLines(in) {
		if i >= len(inv.Lines) || inv.Lines[i] == nil || inv.Lines[i].Item == nil || l.Item == nil || l.Item.ClassifiedTaxCategory == nil {
			continue
		}
		ctc := l.Item.ClassifiedTaxCategory
		tc := inv.Lines[i].Taxes.Get(tax.CategoryVAT)
		if tc == nil || ctc.TaxExemptionReason == nil {
			continue
		}
		reason := goblubl.CleanString(*ctc.TaxExemptionReason)
		if reason == "" {
			continue
		}
		if n := taxNote(inv, tc.Ext); n != nil && n.Text == reason && tc.Ext.Get(untdid.ExtKeyTaxCategory).In(reasonRequiredCategories...) {
			continue
		}
		setMeta(&inv.Lines[i].Item.Meta, MetaKeyTaxExemptionReason, reason)
	}
}

func setPassThroughTaxCategory(ts tax.Set, name *string) {
	tc := ts.Get(tax.CategoryVAT)
	if tc == nil || name == nil {
		return
	}
	c := cbc.Code(strings.TrimPrefix(*name, taxCategoryPrefix))
	if c.In(eracun.PassThroughTaxCategories...) {
		tc.Ext = tc.Ext.Set(eracun.ExtKeyTaxCategory, c)
	}
}
