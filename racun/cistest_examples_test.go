package racun

import (
	"strings"
	"testing"
	"time"

	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/stretchr/testify/require"
)

// TestCISTestExamples converts each example and fiscalizes it through the
// whole client, with a number and an issue time taken from when it is sent.
// It replays the cassettes, or records them with -record.
func TestCISTestExamples(t *testing.T) {
	for i, name := range examples(t) {
		t.Run(name, func(t *testing.T) {
			cassette := "example-" + strings.TrimSuffix(name, ".json")
			fiscalizeCassette(t, cassette, true, func(t *testing.T, cert *Certificate, sentAt time.Time, messageID string) *InvoiceRequest {
				inv := loadExample(t, name).Extract().(*bill.Invoice)
				now := sentAt.In(location)
				inv.Code = cbc.Code(invoiceNumber(sentAt, i))
				inv.IssueDate = cal.DateOf(now)
				inv.IssueTime = cal.NewTime(now.Hour(), now.Minute(), now.Second())
				r, err := FromInvoice(inv, testOptions(), cert.Key)
				require.NoError(t, err)
				return &InvoiceRequest{
					ID:      InvoiceRequestID,
					Header:  &Header{MessageID: messageID, SentAt: now.Format(DateTimeFormat)},
					Invoice: r,
				}
			})
		})
	}
}
