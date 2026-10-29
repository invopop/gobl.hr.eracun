package eracun

import (
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/i18n"
	"github.com/invopop/gobl/pkg/here"
)

// Extension keys
const (
	// ExtKeyProcess is the business process of the invoice (BT-23), written to
	// the UBL ProfileID.
	ExtKeyProcess cbc.Key = "hr-eracun-process"

	// ExtKeyTaxCategory is the HR tax category of a tax combo (HR-TB-2),
	// written with the "HR:" prefix to the tax category name.
	ExtKeyTaxCategory cbc.Key = "hr-eracun-tax-category"

	// ExtKeyRejectionReason is the kind of reason a buyer rejects an invoice
	// for (AR-BT-12), the same code the buyer reports to the Tax
	// Administration.
	ExtKeyRejectionReason cbc.Key = "hr-eracun-rejection-reason"
)

// HR tax categories (HR-TB-2)
const (
	TaxCategoryPDV25  cbc.Code = "PDV25"
	TaxCategoryPDV13  cbc.Code = "PDV13"
	TaxCategoryPDV5   cbc.Code = "PDV5"
	TaxCategoryZ      cbc.Code = "Z"
	TaxCategoryK      cbc.Code = "K"
	TaxCategoryG      cbc.Code = "G"
	TaxCategoryAE     cbc.Code = "AE"
	TaxCategoryE      cbc.Code = "E"
	TaxCategoryPOVNAK cbc.Code = "POVNAK"
	TaxCategoryPP     cbc.Code = "PP"
	TaxCategoryPPMV   cbc.Code = "PPMV"
	TaxCategoryO      cbc.Code = "O"
	TaxCategoryN      cbc.Code = "N"
)

// Rejection reasons (VRSTARAZLOGAODBIJANJA)
const (
	RejectionReasonN cbc.Code = "N"
	RejectionReasonU cbc.Code = "U"
	RejectionReasonO cbc.Code = "O"
)

// PassThroughTaxCategories are amounts left out of the VAT base: POVNAK, PP
// and PPMV, which the supplier collects for a third party, and N, any other
// non-taxable fee charged with taxable items. They are exempt (E) in the
// EN 16931 core and outside the VAT base (O) in the HR extension.
var PassThroughTaxCategories = []cbc.Code{
	TaxCategoryPOVNAK, TaxCategoryPP, TaxCategoryPPMV, TaxCategoryN,
}

var extensions = []*cbc.Definition{
	{
		Key: ExtKeyProcess,
		Name: i18n.String{
			i18n.EN: "Business Process",
			i18n.HR: "Oznaka procesa",
		},
		Desc: i18n.String{
			i18n.EN: here.Doc(`
				Business process of the invoice (BT-23): one of the codes P1 to P12
				defined by the HR CIUS, or P99 followed by a colon and a label agreed
				with the buyer, such as "P99:Narudžba".
			`),
		},
		// HR-BR-34, and at most 200 characters (HR-BR-42).
		Pattern: `^(P[1-9]|P1[0-2]|P99:.{1,196})$`,
	},
	{
		Key: ExtKeyTaxCategory,
		Name: i18n.String{
			i18n.EN: "HR Tax Category",
			i18n.HR: "HR oznaka kategorije PDV-a",
		},
		Desc: i18n.String{
			i18n.EN: here.Doc(`
				HR tax category of the tax combo (HR-TB-2). Set automatically from
				the tax key and rate, except for amounts collected for a third party
				(POVNAK, PP and PPMV) and any other non-taxable fee charged with
				taxable items (N), which must be set on an exempt combo.
			`),
		},
		Values: []*cbc.Definition{
			{Code: TaxCategoryPDV25, Name: i18n.String{i18n.EN: "VAT 25%", i18n.HR: "PDV 25%"}},
			{Code: TaxCategoryPDV13, Name: i18n.String{i18n.EN: "VAT 13%", i18n.HR: "PDV 13%"}},
			{Code: TaxCategoryPDV5, Name: i18n.String{i18n.EN: "VAT 5%", i18n.HR: "PDV 5%"}},
			{Code: TaxCategoryZ, Name: i18n.String{i18n.EN: "Zero rated", i18n.HR: "Nulta stopa"}},
			{Code: TaxCategoryK, Name: i18n.String{i18n.EN: "Intra-community supply", i18n.HR: "Isporuka unutar EU"}},
			{Code: TaxCategoryG, Name: i18n.String{i18n.EN: "Export", i18n.HR: "Izvoz"}},
			{Code: TaxCategoryAE, Name: i18n.String{i18n.EN: "Reverse charge", i18n.HR: "Prijenos porezne obveze"}},
			{Code: TaxCategoryE, Name: i18n.String{i18n.EN: "Exempt", i18n.HR: "Oslobođeno PDV-a"}},
			{Code: TaxCategoryPOVNAK, Name: i18n.String{i18n.EN: "Packaging return fee", i18n.HR: "Povratna naknada"}},
			{Code: TaxCategoryPP, Name: i18n.String{i18n.EN: "Consumption tax", i18n.HR: "Porez na potrošnju"}},
			{Code: TaxCategoryPPMV, Name: i18n.String{i18n.EN: "Special tax on motor vehicles", i18n.HR: "Posebni porez na motorna vozila"}},
			{Code: TaxCategoryO, Name: i18n.String{i18n.EN: "Not subject to VAT", i18n.HR: "Neoporezivo"}},
			{Code: TaxCategoryN, Name: i18n.String{i18n.EN: "Non-taxable fee", i18n.HR: "Neoporeziva naknada"}},
		},
	}, {
		Key: ExtKeyRejectionReason,
		Name: i18n.String{
			i18n.EN: "Rejection Reason",
			i18n.HR: "Vrsta razloga odbijanja",
		},
		Desc: i18n.String{
			i18n.EN: here.Doc(`
				Kind of reason for rejecting an invoice (AR-BT-12), reported to the
				Tax Administration with the same code.
			`),
		},
		Values: []*cbc.Definition{
			{Code: RejectionReasonN, Name: i18n.String{
				i18n.EN: "Data mismatch that does not affect the tax calculation",
				i18n.HR: "Neusklađenost podataka koji ne utječu na obračun poreza",
			}},
			{Code: RejectionReasonU, Name: i18n.String{
				i18n.EN: "Data mismatch that affects the tax calculation",
				i18n.HR: "Neusklađenost podataka koji utječu na obračun poreza",
			}},
			{Code: RejectionReasonO, Name: i18n.String{
				i18n.EN: "Other",
				i18n.HR: "Ostalo",
			}},
		},
	},
}
