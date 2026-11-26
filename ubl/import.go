package ubl

import (
	"slices"
	"strings"

	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/catalogues/untdid"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/l10n"
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
	importSellerTaxRegistration(in, inv)
	importPaymentMeansText(in, inv)
	lines, err := pairedLines(inv, in)
	if err != nil {
		return err
	}
	importAmountsOutsideVAT(in, inv, lines)

	data, err := FindHRFISK20Data(in)
	if err != nil {
		return err
	}
	if data != nil && data.HRObracunPDVPoNaplati != "" {
		inv.SetTags(append(inv.GetTags(), hr.TagCashBasis)...)
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
// people, as a person with the operator key. The person gobl.ubl read from
// the party contact (BG-6) stays apart, even with the same name, so that the
// export writes both back. A SellerContact without an ID or a name adds
// nobody, so that the missing operator is the fault.
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
	inv.Supplier.People = append(inv.Supplier.People, &org.Person{
		Key:        eracun.PersonKeyOperator,
		Name:       &org.Name{Given: name},
		Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: cbc.Code(oib)}},
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

// importSellerTaxRegistration reads the seller's VAT status from where its
// OIB is written, and its OIB into its tax ID:
//
//   - a seller with a VAT identifier (BT-31), which gobl.ubl reads as its tax
//     ID, is registered for VAT;
//   - a Croatian seller with a tax registration (BT-32) and no VAT identifier
//     is not: its tax registration is its OIB (HR-BR-53);
//   - a Croatian seller with neither, as on an invoice with an amount not
//     subject to VAT (O), which shows no VAT identifier (BR-O-02), has its
//     OIB as its legal registration (BT-30). A seller that isn't registered
//     gives a tax registration, so this one is taken as registered.
//
// gobl.ubl reads BT-32 as an identity with the tax scope and BT-30 as one
// with the legal scope: those that repeat the OIB go, since the export writes
// both back from the tax ID.
func importSellerTaxRegistration(in *goblubl.Invoice, inv *bill.Invoice) {
	sup := inv.Supplier
	p := in.AccountingSupplierParty.Party
	if sup == nil || p == nil {
		return
	}
	if sup.TaxID != nil {
		if sup.TaxID.Country == l10n.HR.Tax() && sup.TaxID.Code != cbc.CodeEmpty {
			sup.Ext = sup.Ext.Set(eracun.ExtKeyVATStatus, eracun.VATStatusRegistered)
		}
		return
	}
	if !croatianAddress(p) {
		return
	}
	oib, status := cbc.CodeEmpty, eracun.VATStatusNotRegistered
	for _, pts := range p.PartyTaxScheme {
		if pts.TaxScheme == nil || pts.CompanyID == nil {
			continue
		}
		if pts.TaxScheme.ID.Value == goblubl.TaxSchemeVAT {
			return
		}
		if oib == cbc.CodeEmpty {
			oib = eracun.NormalizeOIB(pts.CompanyID.Value)
		}
	}
	if oib == cbc.CodeEmpty {
		if le := p.PartyLegalEntity; le != nil && le.CompanyID != nil {
			oib, status = eracun.NormalizeOIB(le.CompanyID.Value), eracun.VATStatusRegistered
		}
		if !eracun.ValidOIB(oib) {
			return
		}
	}
	sup.TaxID = &tax.Identity{Country: l10n.HR.Tax(), Code: oib}
	sup.Ext = sup.Ext.Set(eracun.ExtKeyVATStatus, status)
	sup.Identities = slices.DeleteFunc(sup.Identities, func(id *org.Identity) bool {
		return id != nil && id.Type == cbc.CodeEmpty && eracun.NormalizeOIB(id.Code.String()) == oib &&
			(id.Scope == org.IdentityScopeTax || id.Scope == org.IdentityScopeLegal)
	})
}

// croatianAddress says whether the party's postal address is in Croatia.
func croatianAddress(p *goblubl.Party) bool {
	return p.PostalAddress != nil && p.PostalAddress.Country != nil &&
		p.PostalAddress.Country.IdentificationCode == l10n.HR.String()
}
