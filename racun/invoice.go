package racun

import (
	"encoding/xml"
	"time"

	"github.com/google/uuid"
)

// InvoiceRequestID is the Id attribute of an InvoiceRequest, which its
// signature references as "#RacunZahtjev" (§7).
const InvoiceRequestID = "RacunZahtjev"

// InvoiceRequest fiscalizes one invoice (RacunZahtjev).
type InvoiceRequest struct {
	XMLName struct{} `xml:"http://www.apis-it.hr/fin/2012/types/f73 RacunZahtjev"`
	// ID is the request's Id attribute; set it to InvoiceRequestID.
	ID      string   `xml:"Id,attr"`
	Header  *Header  `xml:"Zaglavlje"`
	Invoice *Invoice `xml:"Racun"`
}

// NewRequest wraps an Invoice in a request to send now: the request's Id, a
// new message ID, and the time on a Croatian clock as the send time. Build
// a new request for every send, retries included: CIS needs a new message
// ID each time, and the send time is when it is built.
func NewRequest(inv *Invoice) *InvoiceRequest {
	return &InvoiceRequest{
		ID: InvoiceRequestID,
		Header: &Header{
			MessageID: uuid.NewString(),
			SentAt:    time.Now().In(zagreb).Format(DateTimeFormat),
		},
		Invoice: inv,
	}
}

// Header heads every request (Zaglavlje).
type Header struct {
	// MessageID is a new UUID for every message, retries included.
	MessageID string `xml:"IdPoruke"`
	// SentAt is when the message is sent, in DateTimeFormat.
	SentAt string `xml:"DatumVrijeme"`
}

// Invoice holds the elements of one račun (Racun) that CIS receives:
// issuer, date and time, number, taxes, total, payment method, operator
// and ZKI. FromInvoice builds it from a GOBL bill.Invoice.
type Invoice struct {
	// IssuerOIB is the OIB of the business that issues the invoice.
	IssuerOIB string `xml:"Oib"`
	// InVATSystem says whether the issuer is in the VAT system.
	InVATSystem bool `xml:"USustPdv"`
	// IssuedAt is the issue date and time printed on the invoice, in
	// DateTimeFormat.
	IssuedAt      string        `xml:"DatVrijeme"`
	SequenceScope SequenceScope `xml:"OznSlijed"`
	Number        *Number       `xml:"BrRac"`
	// VAT has one entry per rate.
	VAT []*Tax `xml:"Pdv>Porez,omitempty"`
	// ConsumptionTax (porez na potrošnju) has one entry per rate.
	ConsumptionTax []*Tax      `xml:"Pnp>Porez,omitempty"`
	OtherTaxes     []*OtherTax `xml:"OstaliPor>Porez,omitempty"`
	// VATExemptAmount is the total of supplies exempt from VAT.
	VATExemptAmount string `xml:"IznosOslobPdv,omitempty"`
	// MarginAmount is the total under the margin scheme (used goods, art,
	// antiques).
	MarginAmount string `xml:"IznosMarza,omitempty"`
	// NotSubjectAmount is the total not subject to tax.
	NotSubjectAmount string `xml:"IznosNePodlOpor,omitempty"`
	// Fees are charges such as a returnable packaging deposit.
	Fees          []*Fee        `xml:"Naknade>Naknada,omitempty"`
	Total         string        `xml:"IznosUkupno"`
	PaymentMethod PaymentMethod `xml:"NacinPlac"`
	// OperatorOIB is the OIB of the person at the till, or the issuer's OIB
	// for self-service devices.
	OperatorOIB string `xml:"OibOper"`
	// ZKI is the issuer's protection code (ZastKod), from ZKI.
	ZKI string `xml:"ZastKod"`
	// LateDelivery is true when the invoice was issued earlier without a
	// JIR and is sent again (NakDost).
	LateDelivery bool `xml:"NakDost"`
	// HandwrittenNumber is the number of an invoice from the bound paper
	// invoice book, re-entered after the till broke down (ParagonBrRac).
	HandwrittenNumber string `xml:"ParagonBrRac,omitempty"`
	// SpecialPurpose is reserved by the Tax Administration (SpecNamj).
	SpecialPurpose string `xml:"SpecNamj,omitempty"`
	// BuyerOIB is only for invoices to businesses paid in cash or by card
	// (OibPrimateljaRacuna).
	BuyerOIB string `xml:"OibPrimateljaRacuna,omitempty"`
}

// MarshalXML leaves out the Pdv, Pnp, OstaliPor and Naknade wrappers when
// their lists are empty, which "a>b,omitempty" tags don't do, and the schema
// rejects an empty wrapper.
func (r *Invoice) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	// The fields are those of Invoice, in the same order.
	out := struct {
		IssuerOIB         string        `xml:"Oib"`
		InVATSystem       bool          `xml:"USustPdv"`
		IssuedAt          string        `xml:"DatVrijeme"`
		SequenceScope     SequenceScope `xml:"OznSlijed"`
		Number            *Number       `xml:"BrRac"`
		VAT               *taxList      `xml:"Pdv,omitempty"`
		ConsumptionTax    *taxList      `xml:"Pnp,omitempty"`
		OtherTaxes        *otherTaxList `xml:"OstaliPor,omitempty"`
		VATExemptAmount   string        `xml:"IznosOslobPdv,omitempty"`
		MarginAmount      string        `xml:"IznosMarza,omitempty"`
		NotSubjectAmount  string        `xml:"IznosNePodlOpor,omitempty"`
		Fees              *feeList      `xml:"Naknade,omitempty"`
		Total             string        `xml:"IznosUkupno"`
		PaymentMethod     PaymentMethod `xml:"NacinPlac"`
		OperatorOIB       string        `xml:"OibOper"`
		ZKI               string        `xml:"ZastKod"`
		LateDelivery      bool          `xml:"NakDost"`
		HandwrittenNumber string        `xml:"ParagonBrRac,omitempty"`
		SpecialPurpose    string        `xml:"SpecNamj,omitempty"`
		BuyerOIB          string        `xml:"OibPrimateljaRacuna,omitempty"`
	}{
		IssuerOIB:         r.IssuerOIB,
		InVATSystem:       r.InVATSystem,
		IssuedAt:          r.IssuedAt,
		SequenceScope:     r.SequenceScope,
		Number:            r.Number,
		VATExemptAmount:   r.VATExemptAmount,
		MarginAmount:      r.MarginAmount,
		NotSubjectAmount:  r.NotSubjectAmount,
		Total:             r.Total,
		PaymentMethod:     r.PaymentMethod,
		OperatorOIB:       r.OperatorOIB,
		ZKI:               r.ZKI,
		LateDelivery:      r.LateDelivery,
		HandwrittenNumber: r.HandwrittenNumber,
		SpecialPurpose:    r.SpecialPurpose,
		BuyerOIB:          r.BuyerOIB,
	}
	if len(r.VAT) > 0 {
		out.VAT = &taxList{r.VAT}
	}
	if len(r.ConsumptionTax) > 0 {
		out.ConsumptionTax = &taxList{r.ConsumptionTax}
	}
	if len(r.OtherTaxes) > 0 {
		out.OtherTaxes = &otherTaxList{r.OtherTaxes}
	}
	if len(r.Fees) > 0 {
		out.Fees = &feeList{r.Fees}
	}
	return e.EncodeElement(out, start)
}

type taxList struct {
	Taxes []*Tax `xml:"Porez"`
}

type otherTaxList struct {
	Taxes []*OtherTax `xml:"Porez"`
}

type feeList struct {
	Fees []*Fee `xml:"Naknada"`
}

// Number identifies an invoice (BrRac). Printed, it reads
// "Number/Premises/Device".
type Number struct {
	// Sequential is the number in its sequence, digits only (BrOznRac).
	Sequential string `xml:"BrOznRac"`
	// Premises is the business premises label registered in ePorezna
	// (OznPosPr).
	Premises string `xml:"OznPosPr"`
	// Device is the device label, digits only (OznNapUr).
	Device string `xml:"OznNapUr"`
}

// Tax is the total of one tax rate (Porez).
type Tax struct {
	// Rate is a percentage with two decimals, such as "25.00".
	Rate   string `xml:"Stopa"`
	Base   string `xml:"Osnovica"`
	Amount string `xml:"Iznos"`
}

// OtherTax is the total of a tax other than VAT and consumption tax, such
// as luxury tax (Porez in OstaliPor).
type OtherTax struct {
	Name   string `xml:"Naziv"`
	Rate   string `xml:"Stopa"`
	Base   string `xml:"Osnovica"`
	Amount string `xml:"Iznos"`
}

// Fee is a charge that is not a tax (Naknada).
type Fee struct {
	Name   string `xml:"NazivN"`
	Amount string `xml:"IznosN"`
}

// InvoiceResponse is CIS's answer to an InvoiceRequest (RacunOdgovor). It
// holds a JIR or errors.
type InvoiceResponse struct {
	XMLName struct{}        `xml:"http://www.apis-it.hr/fin/2012/types/f73 RacunOdgovor"`
	ID      string          `xml:"Id,attr"`
	Header  *ResponseHeader `xml:"Zaglavlje"`
	// JIR is the unique invoice identifier CIS assigns.
	JIR    string           `xml:"Jir,omitempty"`
	Errors []*ResponseError `xml:"Greske>Greska,omitempty"`
}

// ResponseHeader heads every answer (Zaglavlje).
type ResponseHeader struct {
	// MessageID repeats the request's MessageID.
	MessageID string `xml:"IdPoruke"`
	SentAt    string `xml:"DatumVrijeme"`
}

// ResponseError is one error in an answer (Greska).
type ResponseError struct {
	// Code is the error code, such as "s005" (SifraGreske).
	Code    string `xml:"SifraGreske"`
	Message string `xml:"PorukaGreske"`
}
