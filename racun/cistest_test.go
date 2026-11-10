package racun

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// invopopOIB is Invopop's own OIB, the one the demo certificate carries.
const invopopOIB = "80432461483"

// cisTestScenarios are the invoices sent to CIS test. CIS test does not
// check that the premises and device exist.
var cisTestScenarios = []struct {
	name string
	// operator is the operator's OIB. A wrong check digit gets s013.
	operator string
	accepted bool
}{
	{"accepted", invopopOIB, true},
	{"refused", "12345678901", false},
}

// TestCISTest fiscalizes one invoice per scenario through the whole client:
// CIS test accepts one and refuses the other. It replays the cassettes, or
// records them with -record.
func TestCISTest(t *testing.T) {
	for i, s := range cisTestScenarios {
		t.Run(s.name, func(t *testing.T) {
			fiscalizeCassette(t, s.name, s.accepted, func(t *testing.T, cert *Certificate, sentAt time.Time, messageID string) *InvoiceRequest {
				req, err := cisTestInvoice(cert, invoiceNumber(sentAt, i), s.operator, sentAt, messageID)
				require.NoError(t, err)
				return req
			})
		})
	}
}

// invoiceNumber is a sequential number for an invoice sent at sentAt, so
// that a recording never repeats one and a replay rebuilds the same one.
func invoiceNumber(sentAt time.Time, i int) string {
	return strconv.FormatInt(sentAt.Unix()%1_000_000*10+int64(i), 10)
}

// cisTestInvoice is a €12.50 card invoice with 25 % VAT, issued at sentAt
// by Invopop at premises POS1, device 1.
func cisTestInvoice(cert *Certificate, number, operator string, sentAt time.Time, messageID string) (*InvoiceRequest, error) {
	now := sentAt.In(location)
	num := &Number{Sequential: number, Premises: testPremises, Device: "1"}
	zki, err := ZKI(cert.Key, &ZKIInput{IssuerOIB: invopopOIB, IssuedAt: now, Number: num, Total: testTotal})
	if err != nil {
		return nil, err
	}
	return &InvoiceRequest{
		ID:     InvoiceRequestID,
		Header: &Header{MessageID: messageID, SentAt: now.Format(DateTimeFormat)},
		Invoice: &Invoice{
			IssuerOIB:     invopopOIB,
			InVATSystem:   true,
			IssuedAt:      now.Format(DateTimeFormat),
			SequenceScope: SequencePremises,
			Number:        num,
			VAT:           []*Tax{{Rate: "25.00", Base: "10.00", Amount: "2.50"}},
			Total:         testTotal,
			PaymentMethod: PaymentCard,
			OperatorOIB:   operator,
			ZKI:           zki,
		},
	}, nil
}
