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

// Every Croatian business has an OIB, whether or not it is registered for
// VAT, and a Croatian VAT number is HR followed by the OIB. So the supplier's
// tax ID always holds its OIB and can't say whether it is registered for VAT:
// the supplier says it in ExtKeyVATStatus. Invoices are also numbered per
// supplier tax ID, which a tax ID with only the country would not tell apart.
//
// The UBL export writes the OIB of a supplier registered for VAT as its VAT
// identifier (BT-31), and the OIB of one that is not as its tax registration
// (BT-32) with the scheme FRE. The hr-fiskal-b2c-v1 addon takes the same
// party, with its own extension key.

// RegisteredForVAT says whether the supplier states that it is registered
// for VAT (ExtKeyVATStatus).
func RegisteredForVAT(p *org.Party) bool {
	return p != nil && p.Ext.Get(ExtKeyVATStatus) == VATStatusRegistered
}

// NotRegisteredForVAT says whether the supplier states that it is not
// registered for VAT (ExtKeyVATStatus), as a small taxpayer does.
func NotRegisteredForVAT(p *org.Party) bool {
	return p != nil && p.Ext.Get(ExtKeyVATStatus) == VATStatusNotRegistered
}

// taxIDOIB provides the OIB in a party's Croatian tax ID, empty when it has
// none.
func taxIDOIB(p *org.Party) cbc.Code {
	if p == nil || p.TaxID == nil || p.TaxID.Country != l10n.HR.Tax() {
		return cbc.CodeEmpty
	}
	tID := *p.TaxID
	tax.NormalizeIdentity(&tID)
	return tID.Code
}

// PartyOIB provides the OIB of a party: the code of its Croatian tax ID, or
// else the code of its OIB identity, as a customer may give it. It is empty
// when the party has neither.
func PartyOIB(p *org.Party) cbc.Code {
	if oib := taxIDOIB(p); oib != cbc.CodeEmpty {
		return oib
	}
	if p == nil {
		return cbc.CodeEmpty
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

// ValidOIB checks an OIB, as validOIB does.
func ValidOIB(code cbc.Code) bool {
	return validOIB(code.String())
}

// validOIB checks an OIB: 11 digits, the last of them the ISO 7064 MOD 11,10
// check digit. It is the check the hr regime makes on a tax ID or an OIB
// identity, which the regime doesn't export, and the u:ctrlOIB function of
// the HR schematron.
func validOIB(code string) bool {
	if len(code) != 11 {
		return false
	}
	for i := range len(code) {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	r := 10
	for i := range 10 {
		a := (r + int(code[i]-'0')) % 10
		if a == 0 {
			a = 10
		}
		r = (a * 2) % 11
	}
	check := 11 - r
	if check == 10 {
		check = 0
	}
	return check == int(code[10]-'0')
}

// vatNumberMatchesOIBIdentity checks that a party with an OIB in its tax ID
// and an OIB identity carries the same OIB in each: when they differ, one of
// them is a typo. A party with only one of them passes.
func vatNumberMatchesOIBIdentity(val any) bool {
	p, ok := val.(*org.Party)
	if !ok || taxIDOIB(p) == cbc.CodeEmpty {
		return true
	}
	id := org.IdentityForType(p.Identities, hr.IdentityTypeOIB)
	return id == nil || NormalizeOIB(id.Code.String()) == taxIDOIB(p)
}

// partyHasTaxIDCode checks that the party's tax ID has a code.
func partyHasTaxIDCode(val any) bool {
	p, ok := val.(*org.Party)
	return !ok || p == nil || (p.TaxID != nil && p.TaxID.Code != cbc.CodeEmpty)
}

func supplierNotRegisteredForVAT(val any) bool {
	inv, ok := val.(*bill.Invoice)
	return ok && inv != nil && NotRegisteredForVAT(inv.Supplier)
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
