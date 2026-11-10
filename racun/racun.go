// Package racun fiscalizes Croatian B2C receipts with the Tax
// Administration's receipt service (Fiscalization 1.0, "fiskalizacija u
// krajnjoj potrošnji").
//
// NewReceipt and FromInvoice build the receipt (Racun) from a GOBL invoice
// and compute its ZKI. Sign signs a request, and a Client sends it to CIS
// and checks CIS's signed answer.
//
// The messages follow FiskalizacijaSchema.xsd v1.10 (kept in
// testdata/schema) and the technical specification "Fiskalizacija računa u
// krajnjoj potrošnji" v2.7. Elements keep the schema's Croatian names and
// order; Go names are English. Amounts, dates and times are strings,
// written exactly as the schema's patterns ask.
//
// This is a different service from the fiscalization of B2B eRačuni
// (Fiscalization 2.0): another namespace, port and signature (plain
// XML-DSig, not XAdES).
package racun

// Namespaces of the receipt messages.
const (
	NamespaceF73  = "http://www.apis-it.hr/fin/2012/types/f73"
	NamespaceDSig = "http://www.w3.org/2000/09/xmldsig#"
)

// DateTimeFormat is the layout of the dates and times in a message
// (DatumVrijemeType): Croatian local time, with no zone offset.
const DateTimeFormat = "02.01.2006T15:04:05"

// zkiDateTimeFormat is the layout of the issue time in the ZKI input. It has
// a space where DateTimeFormat has a T (§12).
const zkiDateTimeFormat = "02.01.2006 15:04:05"

// SequenceScope says what a receipt number counts within (OznSlijed).
type SequenceScope string

// Sequence scopes.
const (
	// SequencePremises numbers receipts per business premises.
	SequencePremises SequenceScope = "P"
	// SequenceDevice numbers receipts per device.
	SequenceDevice SequenceScope = "N"
)

// PaymentMethod is how a receipt was paid (NacinPlac).
type PaymentMethod string

// Payment methods.
const (
	PaymentCash     PaymentMethod = "G"
	PaymentCard     PaymentMethod = "K"
	PaymentTransfer PaymentMethod = "T"
	// PaymentOther also covers receipts paid in more than one way.
	PaymentOther PaymentMethod = "O"
)
