// Package racun fiscalizes Croatian invoices (računi) with the Tax
// Administration's invoice fiscalization service (Fiscalization 1.0,
// "fiskalizacija računa u krajnjoj potrošnji": fiscalization of invoices in
// final consumption, Fiscalization Act, NN 89/25, Art. 2(12)).
//
// Croatian law calls the document a shop prints a račun, an invoice; it has
// no separate term for a receipt. Invoice here is the račun as CIS receives
// it: its elements (Art. 2(7)) in the schema's Racun element. It is not an
// eRačun (e-invoice, Art. 2(8)), which is a structured document sent to the
// buyer.
//
// NewInvoice and FromInvoice build the Invoice from a GOBL bill.Invoice and
// compute its ZKI. NewRequest wraps it in a request to send now, Sign signs
// a request, and a Client sends it to CIS and checks CIS's signed answer.
//
// The messages follow FiskalizacijaSchema.xsd v1.10 (kept in
// testdata/schema) and the technical specification "Fiskalizacija računa u
// krajnjoj potrošnji" v2.7. Elements keep the schema's Croatian names and
// order; Go names are English. Amounts, dates and times are strings,
// written exactly as the schema's patterns ask.
//
// This is a different service from the fiscalization of eRačuni
// (Fiscalization 2.0): another namespace, port and signature (plain
// XML-DSig, not XAdES).
package racun

import (
	"fmt"
	"time"

	"github.com/invopop/gobl/cal"
)

// Namespaces of the invoice messages.
const (
	NamespaceF73  = "http://www.apis-it.hr/fin/2012/types/f73"
	NamespaceDSig = "http://www.w3.org/2000/09/xmldsig#"
)

// zagreb is Croatian local time. A message's times have no zone, so a
// moment, such as when a request is sent, is written as a Croatian clock
// shows it. Loading it needs the time zone database on the host; without
// it, the package refuses to start.
var zagreb = mustLocation("Europe/Zagreb")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// DateTimeFormat is the layout of the dates and times in a message
// (DatumVrijemeType): Croatian local time, with no zone offset.
const DateTimeFormat = "02.01.2006T15:04:05"

// formatDateTime writes a date and time with no zone as "dd.mm.yyyy", sep,
// "hh:mm:ss": the time is written as it is, never converted. Messages use a
// T as sep, as in DateTimeFormat, and the ZKI input a space (§12).
func formatDateTime(dt cal.DateTime, sep string) string {
	d, t := dt.DateTime.Date, dt.DateTime.Time
	return fmt.Sprintf("%02d.%02d.%04d%s%02d:%02d:%02d", d.Day, d.Month, d.Year, sep, t.Hour, t.Minute, t.Second)
}

// SequenceScope says what an invoice number counts within (OznSlijed).
type SequenceScope string

// Sequence scopes.
const (
	// SequencePremises numbers invoices per business premises.
	SequencePremises SequenceScope = "P"
	// SequenceDevice numbers invoices per device.
	SequenceDevice SequenceScope = "N"
)

// PaymentMethod is how an invoice was paid (NacinPlac).
type PaymentMethod string

// Payment methods.
const (
	PaymentCash     PaymentMethod = "G"
	PaymentCard     PaymentMethod = "K"
	PaymentTransfer PaymentMethod = "T"
	// PaymentOther also covers invoices paid in more than one way.
	PaymentOther PaymentMethod = "O"
)
