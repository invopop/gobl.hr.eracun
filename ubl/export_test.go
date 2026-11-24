package ubl_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/invopop/gobl"
	eracun "github.com/invopop/gobl.hr.eracun/addon"
	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/pay"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"
	"github.com/invopop/phorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testInvoice is an invoice written in GOBL, as an issuer sends it: the HR
// tax categories follow from the tax keys and rates, and the amount left out
// of the VAT base is a charge without taxes that names its own.
func testInvoice() *bill.Invoice {
	return &bill.Invoice{
		Regime:    tax.WithRegime("HR"),
		Addons:    tax.WithAddons(eracun.V1),
		Tags:      tax.WithTags(hr.TagCashBasis),
		Series:    "1-P1",
		Code:      "2",
		Currency:  "EUR",
		IssueDate: cal.MakeDate(2026, 3, 1),
		IssueTime: cal.NewTime(9, 30, 0),
		Tax: &bill.Tax{
			Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyProcess: "P99:Narudzba"}),
			Notes: []*tax.Note{
				{Category: tax.CategoryVAT, Key: tax.KeyExempt, Text: "Oslobođeno PDV-a"},
			},
		},
		Supplier: &org.Party{
			Name:  "TVRTKA A d.o.o.",
			TaxID: &tax.Identity{Country: "HR", Code: "12345678903"},
			Ext:   tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyVATStatus: eracun.VATStatusRegistered}),
			Addresses: []*org.Address{
				{Street: "Ulica 1", Locality: "Zagreb", Code: "10000", Country: "HR"},
			},
			People: []*org.Person{
				{Name: &org.Name{Given: "Ana", Surname: "Horvat"}},
				{
					Key:        eracun.PersonKeyOperator,
					Name:       &org.Name{Given: "Ivan", Surname: "Kovač", Alias: "Blagajna 1"},
					Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}},
				},
			},
		},
		Customer: &org.Party{
			Name:  "Tvrtka B d.o.o.",
			TaxID: &tax.Identity{Country: "HR", Code: "11111111119"},
			Addresses: []*org.Address{
				{Street: "Ulica 2", Locality: "Rijeka", Code: "51000", Country: "HR"},
			},
		},
		Lines: []*bill.Line{
			{
				Quantity: num.MakeAmount(2, 0),
				Item: &org.Item{
					Name:       "Proizvod",
					Price:      num.NewAmount(5000, 2),
					Identities: []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "62.20.20"}},
				},
				Taxes: tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}},
			},
			{
				Quantity: num.MakeAmount(1, 0),
				Item: &org.Item{
					Name:       "Usluga",
					Price:      num.NewAmount(2000, 2),
					Identities: []*org.Identity{{Label: eracun.IdentityLabelKPD, Code: "62.20.20"}},
				},
				Taxes: tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt}},
			},
		},
		Charges: []*bill.Charge{
			{
				Reason: "Porez na potrošnju",
				Amount: num.MakeAmount(300, 2),
				Ext:    outsideVAT(eracun.TaxLabelPP),
			},
		},
		Discounts: []*bill.Discount{
			{
				Reason: "Popust",
				Amount: num.MakeAmount(1000, 2),
				Taxes:  tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}},
			},
		},
		Payment: &bill.PaymentDetails{
			Terms: &pay.Terms{DueDates: []*pay.DueDate{{Date: cal.NewDate(2026, 3, 31), Percent: num.NewPercentage(100, 2)}}},
		},
	}
}

func TestExportInvoice(t *testing.T) {
	inv := testInvoice()
	inv.UUID = staticUUID
	env, err := gobl.Envelop(inv)
	require.NoError(t, err)
	env.Head.UUID = staticUUID
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())

	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	doc, ok := out.(*goblubl.Invoice)
	require.True(t, ok)

	assert.Equal(t, "09:30:00", doc.IssueTime)
	assert.Equal(t, "P99:Narudzba", doc.ProfileID.Value)
	sc := doc.AccountingSupplierParty.SellerContact
	require.NotNil(t, sc)
	assert.Equal(t, "11111111119", *sc.ID)
	assert.Equal(t, "Blagajna 1", *sc.Name)
	ctc := doc.InvoiceLines[1].Item.ClassifiedTaxCategory
	assert.Equal(t, "HR:E", *ctc.Name)
	require.Len(t, doc.AllowanceCharge, 2)
	assert.Equal(t, "HR:PP", *doc.AllowanceCharge[0].TaxCategory[0].Name)
	assert.Equal(t, "HR:PDV25", *doc.AllowanceCharge[1].TaxCategory[0].Name)

	data, err := goblubl.Encode(out)
	require.NoError(t, err)
	outPath := filepath.Join("testdata", "convert", "invoice.xml")
	if *updateOut {
		require.NoError(t, os.WriteFile(outPath, data, 0644))
	}
	if *validate {
		validateXML(t, phormClient(t), hrubl.FormatHRCIUS.VESID(env.Extract()), data)
	}
	expected, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, string(expected), string(data), "Output should match the expected XML. Update with --update flag.")

	t.Run("imports back", func(t *testing.T) {
		in, err := goblubl.Decode(data)
		require.NoError(t, err)
		env2, err := goblubl.Import(in)
		require.NoError(t, err)
		require.NoError(t, env2.Validate())
		inv := env2.Extract().(*bill.Invoice)
		assert.True(t, inv.HasTags(hr.TagCashBasis))
		assert.Equal(t, cbc.Code("P99:Narudzba"), inv.Tax.GetExt(eracun.ExtKeyProcess))
		assert.Empty(t, inv.Charges[0].Taxes, "an amount left out of the VAT base has no VAT combo")
		assert.Equal(t, eracun.TaxLabelPP, inv.Charges[0].Ext.Get(eracun.ExtKeyTaxCategory))
		op := eracun.Operator(inv.Supplier.People)
		require.NotNil(t, op)
		assert.Equal(t, "Blagajna 1", op.Name.Given)
	})
}

// TestImportOperatorIsSellerContact checks that an operator who is also the
// seller contact (BG-6) is read as two people, the contact and the person
// with the operator key, so that the export writes both back.
func TestImportOperatorIsSellerContact(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "hr", "eRacun-PDV25.xml"))
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), "\t\t\t</cac:PartyLegalEntity>\n\t\t</cac:Party>\n\t\t<cac:SellerContact>",
		"\t\t\t</cac:PartyLegalEntity>\n\t\t\t<cac:Contact>\n\t\t\t\t<cbc:Name>Operater1</cbc:Name>\n\t\t\t</cac:Contact>\n\t\t</cac:Party>\n\t\t<cac:SellerContact>", 1))
	require.Contains(t, string(data), "<cac:Contact>")
	doc, err := goblubl.Decode(data)
	require.NoError(t, err)
	env, err := goblubl.Import(doc)
	require.NoError(t, err)
	inv := env.Extract().(*bill.Invoice)
	require.Len(t, inv.Supplier.People, 2)
	assert.Empty(t, inv.Supplier.People[0].Key)
	assert.Same(t, inv.Supplier.People[1], eracun.Operator(inv.Supplier.People))

	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())
	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	party := out.(*goblubl.Invoice).AccountingSupplierParty
	require.NotNil(t, party.Party.Contact)
	assert.Equal(t, "Operater1", *party.Party.Contact.Name)
	assert.Equal(t, "12345678903", *party.SellerContact.ID)
	assert.Equal(t, "Operater1", *party.SellerContact.Name)
}

// TestImportOperatorVATPrefix checks that an operator OIB with the HR prefix
// of the VAT number, which HR-BR-9 accepts, is read as the bare OIB.
func TestImportOperatorVATPrefix(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "hr", "eRacun-PDV25.xml"))
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), "<cbc:ID>12345678903</cbc:ID>\n\t\t\t<cbc:Name>Operater1</cbc:Name>",
		"<cbc:ID>HR12345678903</cbc:ID>\n\t\t\t<cbc:Name>Operater1</cbc:Name>", 1))
	require.Contains(t, string(data), "<cbc:ID>HR12345678903</cbc:ID>")
	doc, err := goblubl.Decode(data)
	require.NoError(t, err)
	env, err := goblubl.Import(doc)
	require.NoError(t, err)
	inv := env.Extract().(*bill.Invoice)
	op := eracun.Operator(inv.Supplier.People)
	require.NotNil(t, op)
	assert.Equal(t, cbc.Code("12345678903"), op.Identities[0].Code)
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())
}

// TestImportOperatorEmptyID checks that a SellerContact whose ID carries no
// text adds no operator, so the fault is the missing operator rather than
// an identity with an empty code.
func TestImportOperatorEmptyID(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "hr", "eRacun-PDV25.xml"))
	require.NoError(t, err)
	data = []byte(strings.Replace(string(data), "<cbc:ID>12345678903</cbc:ID>\n\t\t\t<cbc:Name>Operater1</cbc:Name>",
		"<cbc:ID> </cbc:ID>\n\t\t\t<cbc:Name>Operater1</cbc:Name>", 1))
	require.Contains(t, string(data), "<cbc:ID> </cbc:ID>")
	doc, err := goblubl.Decode(data)
	require.NoError(t, err)
	env, err := goblubl.Import(doc)
	require.NoError(t, err)
	inv := env.Extract().(*bill.Invoice)
	assert.Empty(t, inv.Supplier.People)
	require.NoError(t, env.Calculate())
	err = env.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GOBL-HR-ERACUN-BILL-INVOICE-06")
}

// TestImportTaxCategoryNeedsHRPrefix checks that the HR tax category of an
// amount left out of the VAT base is read only from a name with the HR-TB-2
// prefix, so that a plain "PPMV" leaves the charge as the exempt one it says
// it is.
func TestImportTaxCategoryNeedsHRPrefix(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "hr", "Racun za leasing PDV PPMV-aktivacija.xml"))
	require.NoError(t, err)
	labels := func(data []byte) []cbc.Code {
		t.Helper()
		doc, err := goblubl.Decode(data)
		require.NoError(t, err)
		env, err := goblubl.Import(doc)
		require.NoError(t, err)
		inv := env.Extract().(*bill.Invoice)
		var out []cbc.Code
		for _, l := range inv.Lines {
			if eracun.LineOutsideVAT(l) {
				out = append(out, l.Ext.Get(eracun.ExtKeyTaxCategory))
			}
		}
		for _, c := range inv.Charges {
			if eracun.ChargeOutsideVAT(c) {
				out = append(out, c.Ext.Get(eracun.ExtKeyTaxCategory))
			}
		}
		return out
	}
	assert.Contains(t, labels(data), eracun.TaxLabelPPMV)

	plain := strings.ReplaceAll(string(data), "<cbc:Name>HR:PPMV</cbc:Name>", "<cbc:Name>PPMV</cbc:Name>")
	require.False(t, strings.Contains(plain, "<cbc:Name>HR:PPMV</cbc:Name>"))
	assert.NotContains(t, labels([]byte(plain)), eracun.TaxLabelPPMV)
}

// TestExemptSupplyGivesTheBreakdownReason checks the one exempt entry EN
// 16931 allows in the VAT breakdown (BR-E-01) on an invoice with an exempt
// supply and an amount left out of the VAT base: it has both amounts, and the
// exempt supply's ground. The HR breakdown keeps them apart.
func TestExemptSupplyGivesTheBreakdownReason(t *testing.T) {
	inv := testInvoice()
	inv.Charges = nil
	inv.Tax.Notes = nil
	inv.Lines[0].Taxes = nil
	inv.Lines[0].Ext = outsideVAT(eracun.TaxLabelPOVNAK)
	inv.Lines[1].Taxes = tax.Set{vatex("VATEX-EU-132")}
	inv.Discounts = nil
	doc := exportTestInvoice(t, inv)
	var exempt []goblubl.TaxSubtotal
	for _, st := range doc.TaxTotal[0].TaxSubtotal {
		if st.TaxCategory.ID.Value == "E" {
			exempt = append(exempt, st)
		}
	}
	require.Len(t, exempt, 1)
	assert.Equal(t, "120.00", exempt[0].TaxableAmount.Value)
	require.NotNil(t, exempt[0].TaxCategory.TaxExemptionReasonCode)
	assert.Equal(t, "VATEX-EU-132", *exempt[0].TaxCategory.TaxExemptionReasonCode)

	hrd, err := hrubl.FindHRFISK20Data(doc)
	require.NoError(t, err)
	require.NotNil(t, hrd)
	var rows []string
	for _, r := range hrd.HRTaxTotal.HRTaxSubtotals {
		rows = append(rows, r.HRTaxCategory.ID+" "+r.HRTaxCategory.Name+" "+r.HRTaxCategory.HRTaxScheme.ID+" "+r.TaxableAmount.Value)
	}
	assert.ElementsMatch(t, []string{"E HR:E VAT 20.00", "O HR:POVNAK OTH 100.00"}, rows)
	assert.Equal(t, "100.00", hrd.HRLegalMonetaryTotal.OutOfScopeOfVATAmount.Value)
	assert.Equal(t, "20.00", hrd.HRLegalMonetaryTotal.TaxExclusiveAmount.Value)
}

func exportTestInvoice(t *testing.T, inv *bill.Invoice) *goblubl.Invoice {
	t.Helper()
	env, err := gobl.Envelop(inv)
	require.NoError(t, err)
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())
	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	doc, ok := out.(*goblubl.Invoice)
	require.True(t, ok)
	return doc
}

// TestExportOrderReference checks that the "NA" purchase order reference
// gobl.ubl adds is left out, unless the invoice needs it.
func TestExportOrderReference(t *testing.T) {
	t.Run("no ordering", func(t *testing.T) {
		doc := exportTestInvoice(t, testInvoice())
		assert.Nil(t, doc.OrderReference)
	})
	t.Run("sales order only", func(t *testing.T) {
		inv := testInvoice()
		inv.Ordering = &bill.Ordering{Sales: []*org.DocumentRef{{Code: "SO-1"}}}
		doc := exportTestInvoice(t, inv)
		require.NotNil(t, doc.OrderReference)
		assert.Equal(t, goblubl.OrderReferenceNotApplicable, doc.OrderReference.ID)
		assert.Equal(t, "SO-1", doc.OrderReference.SalesOrderID)
	})
	t.Run("purchase order NA", func(t *testing.T) {
		inv := testInvoice()
		inv.Ordering = &bill.Ordering{Purchases: []*org.DocumentRef{{Code: "NA"}}}
		doc := exportTestInvoice(t, inv)
		require.NotNil(t, doc.OrderReference)
		assert.Equal(t, "NA", doc.OrderReference.ID)
	})
}

// TestPaymentMeansText checks that the payment means text (BT-82) goes to
// the instruction note, as the HR CIUS binds it, and comes back.
func TestPaymentMeansText(t *testing.T) {
	inv := testInvoice()
	inv.Payment.Instructions = &pay.Instructions{
		Key:            pay.MeansKeyCreditTransfer,
		Detail:         "Virman",
		CreditTransfer: []*pay.CreditTransfer{{IBAN: "HR1210010051863000160"}},
	}
	doc := exportTestInvoice(t, inv)
	require.Len(t, doc.PaymentMeans, 1)
	assert.Equal(t, []string{"Virman"}, doc.PaymentMeans[0].InstructionNote)
	assert.Nil(t, doc.PaymentMeans[0].PaymentMeansCode.Name)

	data, err := goblubl.Encode(doc)
	require.NoError(t, err)
	in, err := goblubl.Decode(data)
	require.NoError(t, err)
	env, err := goblubl.Import(in)
	require.NoError(t, err)
	inv2 := env.Extract().(*bill.Invoice)
	require.NotNil(t, inv2.Payment.Instructions)
	assert.Equal(t, "Virman", inv2.Payment.Instructions.Detail)
}

// TestExportRequiresOperator checks that an invoice that skipped validation
// is refused rather than written without the operator (HR-BR-9, HR-BR-37).
func TestExportRequiresOperator(t *testing.T) {
	tests := []struct {
		name   string
		modify func(people []*org.Person) []*org.Person
	}{
		{"no operator key", func(p []*org.Person) []*org.Person { p[1].Key = ""; return p }},
		{"two operator keys", func(p []*org.Person) []*org.Person { p[0].Key = eracun.PersonKeyOperator; return p }},
		{"operator without OIB", func(p []*org.Person) []*org.Person { p[1].Identities = nil; return p }},
		{"operator without a name", func(p []*org.Person) []*org.Person { p[1].Name = &org.Name{}; return p }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv := testInvoice()
			inv.Supplier.People = tt.modify(inv.Supplier.People)
			env, err := gobl.Envelop(inv)
			require.NoError(t, err)
			require.NoError(t, env.Calculate())
			_, err = goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
			require.Error(t, err)
			assert.ErrorIs(t, err, gobl.ErrValidation)
			assert.ErrorContains(t, err, "operator")
		})
	}
}

// TestSellerContactPoint checks that a person with the operator key is not
// written as the seller contact (BG-6).
func TestSellerContactPoint(t *testing.T) {
	operator := func() *org.Person {
		return &org.Person{
			Key:        eracun.PersonKeyOperator,
			Name:       &org.Name{Given: "Operater1"},
			Identities: []*org.Identity{{Type: hr.IdentityTypeOIB, Code: "11111111119"}},
		}
	}
	t.Run("operator before the contact", func(t *testing.T) {
		inv := testInvoice()
		inv.Supplier.People = []*org.Person{operator(), {Name: &org.Name{Given: "Ana", Surname: "Horvat"}}}
		doc := exportTestInvoice(t, inv)
		c := doc.AccountingSupplierParty.Party.Contact
		require.NotNil(t, c)
		assert.Equal(t, "Ana Horvat", *c.Name)
		assert.Equal(t, "Operater1", *doc.AccountingSupplierParty.SellerContact.Name)
	})
	t.Run("operator only", func(t *testing.T) {
		inv := testInvoice()
		inv.Supplier.People = []*org.Person{operator()}
		doc := exportTestInvoice(t, inv)
		assert.Nil(t, doc.AccountingSupplierParty.Party.Contact)
	})
	t.Run("operator only with an email", func(t *testing.T) {
		inv := testInvoice()
		inv.Supplier.People = []*org.Person{operator()}
		inv.Supplier.Emails = []*org.Email{{Address: "racuni@example.com"}}
		doc := exportTestInvoice(t, inv)
		c := doc.AccountingSupplierParty.Party.Contact
		require.NotNil(t, c)
		assert.Nil(t, c.Name)
		assert.Equal(t, "racuni@example.com", *c.ElectronicMail)
	})
}

// TestChargeExemptionReason checks that the exemption reason of a document
// level charge or allowance (HR-BT-7) is kept when the export wouldn't give
// it back, and only then. A charge left out of the VAT base gets its own
// reason (BT-104) as its exemption reason.
func TestChargeExemptionReason(t *testing.T) {
	importExample := func(t *testing.T, edit func(string) string) *bill.Invoice {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", "hr", "eRacun-PDV-NEOP-PP-trosak.xml"))
		require.NoError(t, err)
		doc, err := goblubl.Decode([]byte(edit(string(data))))
		require.NoError(t, err)
		env, err := goblubl.Import(doc)
		require.NoError(t, err)
		return env.Extract().(*bill.Invoice)
	}
	t.Run("differs from the charge's reason", func(t *testing.T) {
		// The official example: "#HR:PP#Porez na potrošnju" as the charge's
		// reason, "Porez na potrošnju" as its exemption reason.
		inv := importExample(t, func(s string) string { return s })
		assert.Equal(t, "Porez na potrošnju", inv.Charges[0].Meta[hrubl.MetaKeyTaxExemptionReason])
		doc := exportTestInvoice(t, inv)
		assert.Equal(t, "#HR:PP#Porez na potrošnju", *doc.AllowanceCharge[0].AllowanceChargeReason)
		assert.Equal(t, "Porez na potrošnju", *doc.AllowanceCharge[0].TaxCategory[0].TaxExemptionReason)
	})
	t.Run("same as the charge's reason", func(t *testing.T) {
		inv := importExample(t, func(s string) string {
			return strings.Replace(s, "<cbc:AllowanceChargeReason>#HR:PP#Porez na potrošnju<", "<cbc:AllowanceChargeReason>Porez na potrošnju<", 1)
		})
		assert.Empty(t, inv.Charges[0].Meta)
	})
	t.Run("discount", func(t *testing.T) {
		inv := testInvoice()
		inv.Discounts[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt}}
		inv.Discounts[0].Meta = cbc.Meta{hrubl.MetaKeyTaxExemptionReason: "Popust na oslobođenu isporuku"}
		doc := exportTestInvoice(t, inv)
		require.Len(t, doc.AllowanceCharge, 2)
		assert.False(t, doc.AllowanceCharge[1].ChargeIndicator)
		assert.Equal(t, "Popust na oslobođenu isporuku", *doc.AllowanceCharge[1].TaxCategory[0].TaxExemptionReason)
	})
}

// TestSellerTaxRegistrationScheme checks that only the tax registration
// (BT-32) of a seller outside the VAT system gets the FRE tax scheme.
func TestSellerTaxRegistrationScheme(t *testing.T) {
	inv := testInvoice()
	inv.Supplier.Identities = []*org.Identity{{Scope: org.IdentityScopeTax, Code: "12345678903"}}
	doc := exportTestInvoice(t, inv)
	var schemes []string
	for _, pts := range doc.AccountingSupplierParty.Party.PartyTaxScheme {
		schemes = append(schemes, pts.TaxScheme.ID.Value)
	}
	assert.Equal(t, []string{"VAT", "LOC"}, schemes)
}

// TestLineExemptionReason checks that a line's exemption reason (HR-BT-13)
// that isn't the VAT breakdown's comes back.
func TestLineExemptionReason(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "hr", "eRacun-PDV-NEOP-POVRATNA-NAKNADA-stavka.xml"))
	require.NoError(t, err)
	i := strings.LastIndex(string(data), "#HR:POVNAK#Povratna naknada")
	require.Positive(t, i)
	data = []byte(string(data[:i]) + "Povratna naknada za ambalažu" + string(data[i+len("#HR:POVNAK#Povratna naknada"):]))
	doc, err := goblubl.Decode(data)
	require.NoError(t, err)
	env, err := goblubl.Import(doc)
	require.NoError(t, err)
	inv := env.Extract().(*bill.Invoice)
	assert.Equal(t, "Povratna naknada za ambalažu", inv.Lines[1].Item.Meta[hrubl.MetaKeyTaxExemptionReason])
	assert.Empty(t, inv.Lines[0].Item.Meta)

	require.NoError(t, env.Calculate())
	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	ctc := out.(*goblubl.Invoice).InvoiceLines[1].Item.ClassifiedTaxCategory
	assert.Equal(t, "Povratna naknada za ambalažu", *ctc.TaxExemptionReason)
	assert.Equal(t, "#HR:POVNAK#Povratna naknada", *out.(*goblubl.Invoice).TaxTotal[0].TaxSubtotal[1].TaxCategory.TaxExemptionReason)
}

// TestAmountsOutsideVATReasons checks the reasons of the amounts left out of
// the VAT base beside an exempt supply: each has the pass-through VATEX code,
// a charge has its own reason as the text, and the exempt supply keeps the
// text of the VAT note, which is also the breakdown's.
func TestAmountsOutsideVATReasons(t *testing.T) {
	inv := testInvoice()
	l := lineOutsideVAT("Povratna naknada", 7, eracun.TaxLabelPOVNAK)
	l.Quantity = num.MakeAmount(4, 0)
	inv.Lines = append(inv.Lines, l)
	doc := exportTestInvoice(t, inv)
	require.Len(t, doc.InvoiceLines, 3)
	exempt := doc.InvoiceLines[1].Item.ClassifiedTaxCategory
	require.NotNil(t, exempt.TaxExemptionReason)
	assert.Equal(t, "Oslobođeno PDV-a", *exempt.TaxExemptionReason, "exempt service")
	assert.Nil(t, exempt.TaxExemptionReasonCode)

	deposit := doc.InvoiceLines[2].Item.ClassifiedTaxCategory
	require.NotNil(t, deposit)
	assert.Equal(t, "E", deposit.ID.Value)
	assert.Equal(t, "HR:POVNAK", *deposit.Name)
	assert.Equal(t, "VATEX-EU-79-C", *deposit.TaxExemptionReasonCode)
	assert.Nil(t, deposit.TaxExemptionReason, "a line left out of the VAT base has no reason text of its own")

	require.NotEmpty(t, doc.AllowanceCharge)
	pp := doc.AllowanceCharge[0].TaxCategory[0]
	assert.Equal(t, "VATEX-EU-79-C", *pp.TaxExemptionReasonCode)
	assert.Equal(t, "Porez na potrošnju", *pp.TaxExemptionReason, "the charge's reason")

	var exemptRows []goblubl.TaxSubtotal
	for _, st := range doc.TaxTotal[0].TaxSubtotal {
		if st.TaxCategory.ID.Value == "E" {
			exemptRows = append(exemptRows, st)
		}
	}
	require.Len(t, exemptRows, 1)
	assert.Equal(t, "23.28", exemptRows[0].TaxableAmount.Value)
	assert.Equal(t, "Oslobođeno PDV-a", *exemptRows[0].TaxCategory.TaxExemptionReason)
}

// TestKeptExemptionReasonNeedsExemptCategory checks that a kept exemption
// reason is not written for a category without exemption reasons.
func TestKeptExemptionReasonNeedsExemptCategory(t *testing.T) {
	inv := testInvoice()
	inv.Charges[0].Ext = tax.Extensions{}
	inv.Charges[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}
	inv.Charges[0].Meta = cbc.Meta{hrubl.MetaKeyTaxExemptionReason: "Porez na potrošnju"}
	doc := exportTestInvoice(t, inv)
	assert.Nil(t, doc.AllowanceCharge[0].TaxCategory[0].TaxExemptionReason)
}

// TestExportRefusesUnpairedLines checks that the export refuses an invoice
// whose UBL lines don't pair one to one with the GOBL lines, instead of
// naming the tax categories of the wrong lines: here a format that writes
// sub-invoice lines, which the HR format never does.
func TestExportRefusesUnpairedLines(t *testing.T) {
	inv := testInvoice()
	inv.Lines[0].Breakdown = []*bill.SubLine{{
		Quantity: num.MakeAmount(2, 0),
		Item:     &org.Item{Name: "Proizvod", Price: num.NewAmount(5000, 2)},
	}}
	env, err := gobl.Envelop(inv)
	require.NoError(t, err)
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())

	f := hrubl.FormatHRCIUS
	f.SubLines = true
	_, err = goblubl.Export(env, goblubl.WithFormat(f))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 in UBL and 2 in GOBL")

	_, err = goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	assert.NoError(t, err)
}

// TestImportRefusesUnpairedLines checks that the import refuses a document
// with a line gobl.ubl leaves out, one without a price, instead of reading
// its exemption reason into the next line.
func TestImportRefusesUnpairedLines(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "hr", "eRacun-PDV25.xml"))
	require.NoError(t, err)
	s := string(data)
	i := strings.Index(s, "\t<cac:InvoiceLine>")
	j := strings.Index(s, "</cac:InvoiceLine>") + len("</cac:InvoiceLine>")
	require.Positive(t, i)
	require.Greater(t, j, i)
	line := s[i:j]
	p := strings.Index(line, "\t\t<cac:Price>")
	require.Positive(t, p)
	priceless := line[:p] + "\t</cac:InvoiceLine>"
	priceless = strings.Replace(priceless, "<cbc:ID>1</cbc:ID>", "<cbc:ID>0</cbc:ID>", 1)
	require.Contains(t, priceless, "<cbc:ID>0</cbc:ID>")
	standard := "<cbc:ID>S</cbc:ID>\n\t\t\t\t<cbc:Name>HR:PDV25</cbc:Name>\n\t\t\t\t<cbc:Percent>25</cbc:Percent>"
	require.Contains(t, priceless, standard)
	priceless = strings.Replace(priceless, standard,
		"<cbc:ID>E</cbc:ID>\n\t\t\t\t<cbc:Name>HR:E</cbc:Name>\n\t\t\t\t<cbc:Percent>0</cbc:Percent>\n\t\t\t\t<cbc:TaxExemptionReason>Oslobođeno</cbc:TaxExemptionReason>", 1)
	s = s[:i] + priceless + "\n" + s[i:]

	doc, err := goblubl.Decode([]byte(s))
	require.NoError(t, err)
	_, err = goblubl.Import(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 in UBL and 1 in GOBL")
}

// outsideVATInvoice is testInvoice from a supplier not registered for VAT:
// its OIB in its tax ID, its VAT status saying so, and no VAT.
func outsideVATInvoice() *bill.Invoice {
	inv := testInvoice()
	inv.Tags = tax.Tags{}
	inv.Supplier.Ext = tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyVATStatus: eracun.VATStatusNotRegistered})
	for _, l := range inv.Lines {
		l.Taxes = tax.Set{{Category: tax.CategoryVAT, Key: tax.KeyExempt}}
	}
	inv.Discounts = nil
	return inv
}

// TestSellerOutsideVAT checks the parties of a seller outside the VAT system,
// as in the official example "eRacun-Nije u sustavu PDV": its OIB as the
// legal registration (BT-30) and as the tax registration (BT-32) with the
// scheme FRE, and no VAT identifier (BT-31).
func TestSellerOutsideVAT(t *testing.T) {
	doc := exportTestInvoice(t, outsideVATInvoice())
	p := doc.AccountingSupplierParty.Party
	require.Len(t, p.PartyTaxScheme, 1)
	assert.Equal(t, "12345678903", p.PartyTaxScheme[0].CompanyID.Value)
	assert.Equal(t, "FRE", p.PartyTaxScheme[0].TaxScheme.ID.Value)
	require.NotNil(t, p.PartyLegalEntity)
	require.NotNil(t, p.PartyLegalEntity.CompanyID)
	assert.Equal(t, "12345678903", p.PartyLegalEntity.CompanyID.Value)
	assert.Nil(t, p.PartyLegalEntity.CompanyID.SchemeID)
	assert.Empty(t, p.PartyIdentification)
	require.NotNil(t, p.EndpointID)
	assert.Equal(t, "9934", p.EndpointID.SchemeID)
	assert.Equal(t, "12345678903", p.EndpointID.Value)
}

// importExample imports one of the official examples in testdata/hr, after
// edit changes the decoded UBL.
func importExample(t *testing.T, name string, edit func(*goblubl.Invoice)) *bill.Invoice {
	t.Helper()
	data, err := os.ReadFile("testdata/hr/" + name + ".xml")
	require.NoError(t, err)
	doc, err := goblubl.Decode(data)
	require.NoError(t, err)
	if edit != nil {
		ublInv, ok := doc.(*goblubl.Invoice)
		require.True(t, ok)
		edit(ublInv)
	}
	env, err := goblubl.Import(doc)
	require.NoError(t, err)
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())
	inv, ok := env.Extract().(*bill.Invoice)
	require.True(t, ok)
	return inv
}

// TestImportSellerOutsideVAT checks that a seller outside the VAT system
// comes in as the party the addon takes: its OIB in its tax ID, read from
// its tax registration (BT-32), and its VAT status saying it isn't
// registered. The legal registration (BT-30) that repeats the OIB adds no
// identity.
func TestImportSellerOutsideVAT(t *testing.T) {
	inv := importExample(t, "eRacun-Nije u sustavu PDV", nil)
	assert.Equal(t, "HR", inv.GetRegime().String())
	assert.Equal(t, &tax.Identity{Country: "HR", Code: "12345678903"}, inv.Supplier.TaxID)
	assert.True(t, eracun.NotRegisteredForVAT(inv.Supplier))
	assert.Empty(t, inv.Supplier.Identities)
}

// TestImportSellerRegisteredForVAT checks that a seller with a VAT
// identifier (BT-31) comes in as registered for VAT.
func TestImportSellerRegisteredForVAT(t *testing.T) {
	inv := importExample(t, "eRacun-PDV25", nil)
	assert.Equal(t, &tax.Identity{Country: "HR", Code: "12345678903"}, inv.Supplier.TaxID)
	assert.True(t, eracun.RegisteredForVAT(inv.Supplier))
}

// TestImportSellerTaxRegistrationBesideLegalID checks a seller whose legal
// registration (BT-30) is not its OIB: it stays an identity, so the export
// writes both back where they were.
func TestImportSellerTaxRegistrationBesideLegalID(t *testing.T) {
	inv := importExample(t, "eRacun-Nije u sustavu PDV", func(doc *goblubl.Invoice) {
		doc.AccountingSupplierParty.Party.PartyLegalEntity.CompanyID.Value = "080000001"
	})
	assert.Equal(t, &tax.Identity{Country: "HR", Code: "12345678903"}, inv.Supplier.TaxID)
	require.Len(t, inv.Supplier.Identities, 1)
	assert.Equal(t, cbc.Code("080000001"), inv.Supplier.Identities[0].Code)
	doc := exportTestInvoice(t, inv)
	p := doc.AccountingSupplierParty.Party
	assert.Equal(t, "080000001", p.PartyLegalEntity.CompanyID.Value)
	require.Len(t, p.PartyTaxScheme, 1)
	assert.Equal(t, "12345678903", p.PartyTaxScheme[0].CompanyID.Value)
	assert.Equal(t, "FRE", p.PartyTaxScheme[0].TaxScheme.ID.Value)
}

// TestSellerWithOutsideScopeAmounts checks that an invoice with an amount
// not subject to VAT (O) from a seller registered for VAT shows no VAT
// identifier (BR-O-02): no tax registration at all, and the OIB as the legal
// registration (BT-30), as in the official example "Neoporezivi racun".
func TestSellerWithOutsideScopeAmounts(t *testing.T) {
	inv := scenarioInvoice()
	inv.Customer = customerWithoutVATNumber()
	inv.Lines[0].Taxes = tax.Set{vat(tax.KeyOutsideScope, "")}
	addNote(inv, tax.KeyOutsideScope, "Nije predmet oporezivanja")
	p := exportTestInvoice(t, inv).AccountingSupplierParty.Party
	assert.Empty(t, p.PartyTaxScheme)
	require.NotNil(t, p.PartyLegalEntity.CompanyID)
	assert.Equal(t, "12345678903", p.PartyLegalEntity.CompanyID.Value)
}

// TestImportSellerOnOutsideScopeInvoice checks the seller of the official
// example "Neoporezivi racun", which has no tax registration at all: its OIB
// comes from its legal registration (BT-30) into its tax ID. A seller that is
// not registered for VAT gives a tax registration (BT-32), so this one comes
// in as registered, and exports as it came.
func TestImportSellerOnOutsideScopeInvoice(t *testing.T) {
	inv := importExample(t, "Neoporezivi racun", nil)
	assert.Equal(t, "HR", inv.GetRegime().String())
	assert.Equal(t, &tax.Identity{Country: "HR", Code: "12345678903"}, inv.Supplier.TaxID)
	assert.True(t, eracun.RegisteredForVAT(inv.Supplier))
	assert.Empty(t, inv.Supplier.Identities)
	p := exportTestInvoice(t, inv).AccountingSupplierParty.Party
	assert.Empty(t, p.PartyTaxScheme)
	assert.Equal(t, "12345678903", p.PartyLegalEntity.CompanyID.Value)
}

// TestSellerOutsideVATKeepsOIBInTaxRegistration checks that the OIB is the
// seller's tax registration (BT-32) even when it has another identity with
// the tax scope.
func TestSellerOutsideVATKeepsOIBInTaxRegistration(t *testing.T) {
	inv := outsideVATInvoice()
	inv.Supplier.Identities = []*org.Identity{{Scope: org.IdentityScopeTax, Code: "XYZ123"}}
	doc := exportTestInvoice(t, inv)
	var ids []string
	for _, pts := range doc.AccountingSupplierParty.Party.PartyTaxScheme {
		assert.Equal(t, "FRE", pts.TaxScheme.ID.Value)
		ids = append(ids, pts.CompanyID.Value)
	}
	assert.Contains(t, ids, "12345678903")
}

// TestSellerOutsideVATBesideLegalID checks a seller whose legal registration
// (BT-30) is another number: the OIB goes only to its tax registration
// (BT-32), and never to a party identification.
func TestSellerOutsideVATBesideLegalID(t *testing.T) {
	inv := outsideVATInvoice()
	inv.Supplier.Identities = []*org.Identity{{Scope: org.IdentityScopeLegal, Code: "080000001"}}
	p := exportTestInvoice(t, inv).AccountingSupplierParty.Party
	assert.Equal(t, "080000001", p.PartyLegalEntity.CompanyID.Value)
	require.Len(t, p.PartyTaxScheme, 1)
	assert.Equal(t, "12345678903", p.PartyTaxScheme[0].CompanyID.Value)
	assert.Equal(t, "FRE", p.PartyTaxScheme[0].TaxScheme.ID.Value)
	assert.Empty(t, p.PartyIdentification)
}

// TestImportSellerTaxRegistrationWithPrefix checks that BT-30 and BT-32 are
// taken for the same OIB when BT-32 is written with the HR prefix.
func TestImportSellerTaxRegistrationWithPrefix(t *testing.T) {
	inv := importExample(t, "eRacun-Nije u sustavu PDV", func(doc *goblubl.Invoice) {
		doc.AccountingSupplierParty.Party.PartyTaxScheme[0].CompanyID.Value = "HR12345678903"
	})
	assert.Equal(t, &tax.Identity{Country: "HR", Code: "12345678903"}, inv.Supplier.TaxID)
	assert.Empty(t, inv.Supplier.Identities)
}

// TestImportForeignSellerTaxRegistration checks that a seller outside
// Croatia keeps its tax registration as it came: it is not an OIB.
func TestImportForeignSellerTaxRegistration(t *testing.T) {
	data, err := os.ReadFile("testdata/hr/eRacun-Nije u sustavu PDV.xml")
	require.NoError(t, err)
	doc, err := goblubl.Decode(data)
	require.NoError(t, err)
	ublInv, ok := doc.(*goblubl.Invoice)
	require.True(t, ok)
	ublInv.AccountingSupplierParty.Party.PostalAddress.Country.IdentificationCode = "DE"
	env, err := goblubl.Import(doc)
	require.NoError(t, err)
	inv, ok := env.Extract().(*bill.Invoice)
	require.True(t, ok)
	assert.Nil(t, inv.Supplier.TaxID)
	assert.False(t, inv.Supplier.Ext.Has(eracun.ExtKeyVATStatus))
}

// TestGOBLExamples exports the GOBL example invoices in ../examples/out, the
// documents issuers start from, and checks the UBL against the schematron
// (with -validate). Their GOBL side is checked by the module's TestExamples.
func TestGOBLExamples(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "examples", "out", "*.json"))
	require.NoError(t, err)
	var pc *phorm.Client
	if *validate {
		pc = phormClient(t)
	}
	n := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		env := new(gobl.Envelope)
		require.NoError(t, json.Unmarshal(data, env))
		if _, ok := env.Extract().(*bill.Invoice); !ok {
			continue
		}
		n++
		t.Run(filepath.Base(f), func(t *testing.T) {
			out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
			require.NoError(t, err)
			x, err := goblubl.Encode(out)
			require.NoError(t, err)
			if pc != nil {
				validateXML(t, pc, hrubl.FormatHRCIUS.VESID(env.Extract()), x)
			}
		})
	}
	require.NotZero(t, n)
}
