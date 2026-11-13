package eracun

import (
	"strings"

	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"
)

// A supplier in the VAT system has a tax ID with its Croatian VAT number,
// HR followed by its OIB, which the export writes as the seller's VAT
// identifier (BT-31). A supplier outside the VAT system, such as a small
// taxpayer, has no VAT number: its tax ID has only the country HR, which
// keeps the regime, and its OIB is an identity of type OIB. The export writes
// that identity as the seller's legal registration (BT-30) and as its tax
// registration (BT-32) with the scheme FRE.
//
// gobl.fr.ctc models a French supplier the same way, with a SIREN identity
// and no VAT status flag, and the hr-fiskal-b2c-v1 addon takes the same
// party, so one supplier serves both Croatian addons.

// PartyHasVATNumber says whether the party's tax ID has a VAT number. A
// Croatian supplier without one is outside the VAT system.
func PartyHasVATNumber(p *org.Party) bool {
	return p != nil && p.TaxID != nil && p.TaxID.Code != cbc.CodeEmpty
}

// PartyOIB provides the OIB of a party: the code of its Croatian VAT number,
// or else the code of its OIB identity. It is empty when the party has
// neither.
func PartyOIB(p *org.Party) cbc.Code {
	if p == nil {
		return cbc.CodeEmpty
	}
	if p.TaxID != nil && p.TaxID.Country == l10n.HR.Tax() {
		tID := *p.TaxID
		tax.NormalizeIdentity(&tID)
		if tID.Code != cbc.CodeEmpty {
			return tID.Code
		}
	}
	if id := org.IdentityForType(p.Identities, hr.IdentityTypeOIB); id != nil {
		return NormalizeOIB(id.Code.String())
	}
	return cbc.CodeEmpty
}

// NormalizeOIB removes the separators and the HR prefix of a VAT number from
// an OIB, as the hr regime does with an OIB identity.
func NormalizeOIB(code string) cbc.Code {
	c := cbc.NormalizeAlphanumericalCode(cbc.Code(code))
	return cbc.Code(strings.TrimPrefix(c.String(), l10n.HR.String()))
}

// partyOutsideVAT says whether the party is a Croatian supplier outside the
// VAT system: a tax ID with only the country HR. A party with no tax ID
// says nothing about it.
func partyOutsideVAT(p *org.Party) bool {
	return p != nil && p.TaxID != nil && p.TaxID.Country == l10n.HR.Tax() && p.TaxID.Code == cbc.CodeEmpty
}

// assignOIBScope gives an unscoped OIB identity of a supplier outside the VAT
// system the scope the export needs. With the legal scope, the export writes
// the OIB as the legal registration (BT-30), which BR-CO-26 needs from a
// seller with no VAT identifier (BT-31), and as the tax registration (BT-32).
// gobl.fr.ctc gives a SIREN the legal scope the same way. When another
// identity already has the legal scope, the OIB takes the tax scope and goes
// only to BT-32: with no scope it would be written as a party identification
// with the scheme "OIB", which is not an ISO 6523 code.
func assignOIBScope(p *org.Party) {
	if !partyOutsideVAT(p) {
		return
	}
	id := org.IdentityForType(p.Identities, hr.IdentityTypeOIB)
	if id == nil || id.Scope != cbc.KeyEmpty {
		return
	}
	id.Scope = org.IdentityScopeLegal
	for _, other := range p.Identities {
		if other != nil && other != id && other.Scope.Has(org.IdentityScopeLegal) {
			id.Scope = org.IdentityScopeTax
			return
		}
	}
}

func supplierOutsideVAT(val any) bool {
	inv, ok := val.(*bill.Invoice)
	return ok && inv != nil && partyOutsideVAT(inv.Supplier)
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
