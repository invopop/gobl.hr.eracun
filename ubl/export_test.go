package ubl_test

import (
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
	"github.com/invopop/gobl/catalogues/cef"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/pay"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testInvoice is an invoice written in GOBL, as an issuer sends it: the HR
// tax categories follow from the tax keys and rates, except for the amount
// collected for a third party.
func testInvoice() *bill.Invoice {
	return &bill.Invoice{
		Regime:    tax.WithRegime("HR"),
		Addons:    tax.WithAddons(eracun.V1),
		Tags:      tax.WithTags(eracun.TagCashBasis),
		Series:    "1-P1",
		Code:      "2",
		Currency:  "EUR",
		IssueDate: cal.MakeDate(2026, 3, 1),
		IssueTime: cal.NewTime(9, 30, 0),
		Tax: &bill.Tax{
			Ext: tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyProcess: "P99:Narudzba"}),
			Notes: []*tax.Note{
				{Category: tax.CategoryVAT, Key: tax.KeyExempt, Text: "Porez na potrošnju"},
			},
		},
		Supplier: &org.Party{
			Name:  "TVRTKA A d.o.o.",
			TaxID: &tax.Identity{Country: "HR", Code: "12345678903"},
			Addresses: []*org.Address{
				{Street: "Ulica 1", Locality: "Zagreb", Code: "10000", Country: "HR"},
			},
			People: []*org.Person{
				{Name: &org.Name{Given: "Ana", Surname: "Horvat"}},
				{
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
				Reason: "#HR:PP#Porez na potrošnju",
				Amount: num.MakeAmount(300, 2),
				Taxes: tax.Set{{
					Category: tax.CategoryVAT,
					Key:      tax.KeyExempt,
					Ext:      tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxCategoryPP}),
				}},
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
		assert.True(t, inv.HasTags(eracun.TagCashBasis))
		assert.Equal(t, cbc.Code("P99:Narudzba"), inv.Tax.GetExt(eracun.ExtKeyProcess))
		assert.Equal(t, eracun.TaxCategoryPP, inv.Charges[0].Taxes[0].Ext.Get(eracun.ExtKeyTaxCategory))
		op := eracun.Operator(inv.Supplier.People)
		require.NotNil(t, op)
		assert.Equal(t, "Blagajna 1", op.Name.Given)
	})
}

// TestImportOperatorIsSellerContact checks that an operator who is also the
// seller contact (BG-6) stays one person.
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
	require.Len(t, inv.Supplier.People, 1)
	assert.Same(t, inv.Supplier.People[0], eracun.Operator(inv.Supplier.People))
}

// TestMergedExemptionKeepsVATEX checks that joining the exempt entries of a
// pass-through amount and an exempt supply keeps the VATEX code of either.
func TestMergedExemptionKeepsVATEX(t *testing.T) {
	inv := testInvoice()
	inv.Charges = nil
	inv.Lines[0].Taxes = tax.Set{{
		Category: tax.CategoryVAT,
		Key:      tax.KeyExempt,
		Ext:      tax.ExtensionsOf(cbc.CodeMap{eracun.ExtKeyTaxCategory: eracun.TaxCategoryPP}),
	}}
	inv.Lines[1].Taxes = tax.Set{{
		Category: tax.CategoryVAT,
		Key:      tax.KeyExempt,
		Ext:      tax.ExtensionsOf(cbc.CodeMap{cef.ExtKeyVATEX: "VATEX-EU-132"}),
	}}
	env, err := gobl.Envelop(inv)
	require.NoError(t, err)
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())

	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	doc := out.(*goblubl.Invoice)
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
// level charge or allowance (HR-BT-7) is kept when the VAT note doesn't give
// it back, and only then.
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
	t.Run("differs from the note", func(t *testing.T) {
		inv := importExample(t, func(s string) string { return s })
		assert.Equal(t, "Porez na potrošnju", inv.Charges[0].Meta[hrubl.MetaKeyTaxExemptionReason])
	})
	t.Run("same as the note", func(t *testing.T) {
		inv := importExample(t, func(s string) string {
			return strings.Replace(s, "<cbc:TaxExemptionReason>Porez na potrošnju<", "<cbc:TaxExemptionReason>#HR:PP#Porez na potrošnju<", 1)
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

// TestKeptExemptionReasonNeedsExemptCategory checks that a kept exemption
// reason is not written for a category without exemption reasons.
func TestKeptExemptionReasonNeedsExemptCategory(t *testing.T) {
	inv := testInvoice()
	inv.Charges[0].Taxes = tax.Set{{Category: tax.CategoryVAT, Rate: tax.RateGeneral}}
	inv.Charges[0].Meta = cbc.Meta{hrubl.MetaKeyTaxExemptionReason: "Porez na potrošnju"}
	doc := exportTestInvoice(t, inv)
	assert.Nil(t, doc.AllowanceCharge[0].TaxCategory[0].TaxExemptionReason)
}
