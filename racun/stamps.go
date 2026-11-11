package racun

import (
	"fmt"
	"time"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/head"
	"github.com/invopop/gobl/num"

	"github.com/invopop/gobl.hr.eracun/addon/fiskal"
)

// qrBaseURL is where the QR code on a printed invoice points, for a buyer to
// check the invoice with the Tax Administration (§2.7).
const qrBaseURL = "https://porezna.gov.hr/rn"

// QR provides the URL the QR code on the printed invoice holds (§2.7): the
// JIR, or the ZKI while the invoice has no JIR, with the issue date and
// time to the minute and the total in cents.
func (r *Invoice) QR(jir string) (string, error) {
	issued, err := time.Parse(DateTimeFormat, r.IssuedAt)
	if err != nil {
		return "", fmt.Errorf("issue date and time %q %w", r.IssuedAt, ErrInvalid)
	}
	total, err := num.AmountFromString(r.Total)
	if err != nil {
		return "", fmt.Errorf("total %q %w", r.Total, ErrInvalid)
	}
	// The parameters are in the specification's order. None needs
	// escaping: a JIR is a UUID, a ZKI is hexadecimal, and the date and
	// amount are digits. The amount is in cents, with no separators or
	// leading zeros; a negative total keeps its minus sign.
	id := "jir=" + jir
	if jir == "" {
		id = "zki=" + r.ZKI
	}
	return fmt.Sprintf("%s?%s&datv=%s&izn=%d", qrBaseURL, id, issued.Format("20060102_1504"), total.Rescale(2).Value()), nil
}

// AddStamps adds the fiscalization stamps of r to the envelope: the ZKI
// and the QR code, and the JIR when CIS has given one. Call it when the
// invoice is issued, with no JIR, and again when CIS accepts it; the QR
// code then refers to the JIR.
func AddStamps(env *gobl.Envelope, r *Invoice, jir string) error {
	qr, err := r.QR(jir)
	if err != nil {
		return err
	}
	env.Head.AddStamp(&head.Stamp{Provider: fiskal.StampZKI, Value: r.ZKI})
	if jir != "" {
		env.Head.AddStamp(&head.Stamp{Provider: fiskal.StampJIR, Value: jir})
	}
	env.Head.AddStamp(&head.Stamp{Provider: fiskal.StampQR, Value: qr})
	return nil
}
