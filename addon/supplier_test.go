package eracun_test

import (
	"testing"

	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/rules"
	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSupplierVATNumberDiffersFromOIBIdentity checks that a supplier whose
// VAT number and OIB identity carry different OIBs is refused: one of them is
// a typo.
func TestSupplierVATNumberDiffersFromOIBIdentity(t *testing.T) {
	inv := testInvoice()
	inv.Supplier.Identities = []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}}
	require.NoError(t, inv.Calculate())
	assertFault(t, inv, "GOBL-HR-ERACUN-BILL-INVOICE-17")
}

// TestSupplierVATNumberMatchesOIBIdentity checks that the OIB identity is
// compared once normalized, so separators and an HR prefix don't count.
func TestSupplierVATNumberMatchesOIBIdentity(t *testing.T) {
	inv := testInvoice()
	inv.Supplier.Identities = []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "HR 123-456-789-03"}}
	require.NoError(t, inv.Calculate())
	require.NoError(t, rules.Validate(inv))
}

// TestSupplierForeignVATNumberBesideOIBIdentity checks that a VAT number of
// another country is not compared with the OIB identity: it holds no OIB.
func TestSupplierForeignVATNumberBesideOIBIdentity(t *testing.T) {
	inv := testInvoice()
	inv.Supplier.TaxID = &tax.Identity{Country: "DE", Code: "111111125"}
	inv.Supplier.Identities = []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "12345678903"}}
	require.NoError(t, inv.Calculate())
	faults := rules.Validate(inv)
	if faults != nil {
		assert.False(t, faults.HasCode("GOBL-HR-ERACUN-BILL-INVOICE-17"), "got %s", faults)
	}
}
