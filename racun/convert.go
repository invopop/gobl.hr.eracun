// Converting a GOBL invoice into the receipt CIS fiscalizes (Racun).
//
// Most of the receipt comes from the invoice: the issuer's OIB, the issue
// date and time, the sequential number, the VAT breakdown, the total, the
// payment method and the buyer's OIB. The values a GOBL invoice can't carry
// yet come from Options: the business premises and device labels, the
// operator's OIB, the numbering scope, and whether the receipt is sent late.
// A B2C addon is meant to carry them as extensions.

package racun

import (
	"crypto"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/currency"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/pay"
	"github.com/invopop/gobl/tax"
)

// Errors the conversion returns, wrapped with the field they are about.
var (
	// ErrNotInvoice is an envelope that holds no invoice.
	ErrNotInvoice = errors.New("envelope does not hold an invoice")
	// ErrMissing is an invoice or options without a field the receipt
	// requires.
	ErrMissing = errors.New("missing a field the receipt requires")
	// ErrInvalid is a field whose value the receipt can't take.
	ErrInvalid = errors.New("has a value the receipt can't take")
	// ErrUnsupported is an invoice the conversion does not handle yet.
	ErrUnsupported = errors.New("not supported for receipts yet")
)

// location is Croatian local time. GOBL dates and times are local to the
// regime, and receipts are written in Croatian local time with no offset.
var location = mustLocation("Europe/Zagreb")

var (
	// sequentialPattern is a receipt number: digits with no leading zero
	// (BrOznRac).
	sequentialPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
	// premisesPattern is a business premises label (OznPosPr).
	premisesPattern = regexp.MustCompile(`^[0-9a-zA-Z]{1,20}$`)
	// devicePattern is a device label (OznNapUr).
	devicePattern = regexp.MustCompile(`^[0-9]{1,20}$`)
	// oibPattern is an OIB.
	oibPattern = regexp.MustCompile(`^[0-9]{11}$`)
)

// Options are the receipt values a GOBL invoice doesn't carry yet.
type Options struct {
	// Premises is the business premises label registered in ePorezna
	// (OznPosPr): letters and digits.
	Premises string
	// Device is the label of the till that issued the receipt (OznNapUr):
	// digits.
	Device string
	// SequenceScope says whether receipt numbers count per premises or per
	// device. The default is per premises.
	SequenceScope SequenceScope
	// OperatorOIB is the OIB of the person at the till, or the issuer's OIB
	// for a self-service device.
	OperatorOIB string
	// NotInVATSystem is true for an issuer outside the VAT system.
	NotInVATSystem bool
	// LateDelivery is true when the receipt was issued without a JIR and
	// is sent again (NakDost).
	LateDelivery bool
	// HandwrittenNumber is the number of a paper receipt entered after the
	// till broke down (ParagonBrRac).
	HandwrittenNumber string
}

// NewReceipt builds the receipt of the invoice in the envelope, with its ZKI
// computed with key, the issuer's private key. The envelope must be
// calculated.
func NewReceipt(env *gobl.Envelope, opts *Options, key crypto.Signer) (*Receipt, error) {
	if env == nil {
		return nil, ErrNotInvoice
	}
	inv, ok := env.Extract().(*bill.Invoice)
	if !ok {
		return nil, ErrNotInvoice
	}
	return FromInvoice(inv, opts, key)
}

// FromInvoice builds the receipt of a calculated invoice, with its ZKI
// computed with key, the issuer's private key. The ZKI goes on the printed
// receipt and does not change when the receipt is sent again.
func FromInvoice(inv *bill.Invoice, opts *Options, key crypto.Signer) (*Receipt, error) {
	if opts == nil {
		opts = new(Options)
	}
	r := new(Receipt)
	if err := checkInvoice(inv); err != nil {
		return nil, err
	}

	var err error
	if r.IssuerOIB, err = issuerOIB(inv); err != nil {
		return nil, err
	}
	issuedAt, err := issueTime(inv)
	if err != nil {
		return nil, err
	}
	r.IssuedAt = issuedAt.Format(DateTimeFormat)
	if r.Number, err = number(inv, opts); err != nil {
		return nil, err
	}
	r.SequenceScope = opts.SequenceScope
	if r.SequenceScope == "" {
		r.SequenceScope = SequencePremises
	}
	if r.SequenceScope != SequencePremises && r.SequenceScope != SequenceDevice {
		return nil, fmt.Errorf("sequence scope %q %w", r.SequenceScope, ErrInvalid)
	}
	r.InVATSystem = !opts.NotInVATSystem
	if err := addTaxes(r, inv); err != nil {
		return nil, err
	}
	r.Total = amount(inv.Totals.Payable)
	r.PaymentMethod = paymentMethod(inv.Payment)
	if r.OperatorOIB, err = oib("operator OIB", opts.OperatorOIB); err != nil {
		return nil, err
	}
	r.LateDelivery = opts.LateDelivery
	r.HandwrittenNumber = opts.HandwrittenNumber
	r.BuyerOIB = buyerOIB(inv)

	r.ZKI, err = ZKI(key, &ZKIInput{
		IssuerOIB: r.IssuerOIB,
		IssuedAt:  issuedAt,
		Number:    r.Number,
		Total:     r.Total,
	})
	if err != nil {
		return nil, fmt.Errorf("computing the ZKI: %w", err)
	}
	return r, nil
}

// checkInvoice refuses invoices the conversion does not handle.
func checkInvoice(inv *bill.Invoice) error {
	switch {
	case inv == nil:
		return ErrNotInvoice
	case inv.Totals == nil:
		return fmt.Errorf("totals %w: calculate the invoice first", ErrMissing)
	case inv.Type != bill.InvoiceTypeStandard:
		return fmt.Errorf("invoice type %q %w", inv.Type, ErrUnsupported)
	case inv.Currency != currency.EUR:
		return fmt.Errorf("currency %q %w: receipts are in euros", inv.Currency, ErrInvalid)
	case inv.Totals.Payable.IsNegative():
		return fmt.Errorf("negative total %w", ErrUnsupported)
	}
	return nil
}

func issuerOIB(inv *bill.Invoice) (string, error) {
	if inv.Supplier == nil || inv.Supplier.TaxID == nil || inv.Supplier.TaxID.Code == "" {
		return "", fmt.Errorf("supplier tax ID %w", ErrMissing)
	}
	if inv.Supplier.TaxID.Country != l10n.HR.Tax() {
		return "", fmt.Errorf("supplier tax ID country %q %w: the issuer must be Croatian", inv.Supplier.TaxID.Country, ErrInvalid)
	}
	return oib("supplier tax ID", inv.Supplier.TaxID.Code.String())
}

func oib(field, code string) (string, error) {
	if code == "" {
		return "", fmt.Errorf("%s %w", field, ErrMissing)
	}
	if !oibPattern.MatchString(code) {
		return "", fmt.Errorf("%s %q %w: an OIB is 11 digits", field, code, ErrInvalid)
	}
	return code, nil
}

// issueTime is the issue date and time of the invoice in Croatian local
// time. Receipts need the time, so it must be set.
func issueTime(inv *bill.Invoice) (time.Time, error) {
	if inv.IssueTime == nil || inv.IssueTime.IsZero() {
		return time.Time{}, fmt.Errorf("issue time %w", ErrMissing)
	}
	d, t := inv.IssueDate, inv.IssueTime
	return time.Date(d.Year, d.Month, d.Day, t.Hour, t.Minute, t.Second, 0, location), nil
}

// number is the receipt number: the invoice code, which must be the bare
// sequential number, and the premises and device labels from opts.
func number(inv *bill.Invoice, opts *Options) (*Number, error) {
	code := inv.Code.String()
	switch {
	case code == "":
		return nil, fmt.Errorf("invoice code %w", ErrMissing)
	case !sequentialPattern.MatchString(code):
		return nil, fmt.Errorf("invoice code %q %w: digits with no leading zero", code, ErrInvalid)
	case opts.Premises == "":
		return nil, fmt.Errorf("premises label %w", ErrMissing)
	case !premisesPattern.MatchString(opts.Premises):
		return nil, fmt.Errorf("premises label %q %w: up to 20 letters and digits", opts.Premises, ErrInvalid)
	case opts.Device == "":
		return nil, fmt.Errorf("device label %w", ErrMissing)
	case !devicePattern.MatchString(opts.Device):
		return nil, fmt.Errorf("device label %q %w: up to 20 digits", opts.Device, ErrInvalid)
	}
	return &Number{Sequential: code, Premises: opts.Premises, Device: opts.Device}, nil
}

// addTaxes adds the VAT breakdown: one entry per rate with a percentage, and
// the exempt and outside-scope bases as totals.
func addTaxes(r *Receipt, inv *bill.Invoice) error {
	if inv.Totals.Taxes == nil {
		return nil
	}
	var exempt, outside num.Amount
	var hasExempt, hasOutside bool
	for _, cat := range inv.Totals.Taxes.Categories {
		if cat.Code != tax.CategoryVAT {
			return fmt.Errorf("tax category %q %w", cat.Code, ErrUnsupported)
		}
		for _, rate := range cat.Rates {
			switch {
			case rate.Key.In(tax.KeyExempt, tax.KeyExport):
				exempt = exempt.Add(rate.Base)
				hasExempt = true
			case rate.Key == tax.KeyOutsideScope:
				outside = outside.Add(rate.Base)
				hasOutside = true
			case rate.Key.In(tax.KeyReverseCharge, tax.KeyIntraCommunity):
				return fmt.Errorf("VAT key %q %w", rate.Key, ErrUnsupported)
			case rate.Percent == nil:
				return fmt.Errorf("VAT rate without a percentage (key %q) %w", rate.Key, ErrMissing)
			default:
				r.VAT = append(r.VAT, &Tax{
					Rate:   percent(*rate.Percent),
					Base:   amount(rate.Base),
					Amount: amount(rate.Amount),
				})
			}
		}
	}
	if hasExempt {
		r.VATExemptAmount = amount(exempt)
	}
	if hasOutside {
		r.NotSubjectAmount = amount(outside)
	}
	return nil
}

// amount writes an amount with two decimals, as IznosType asks.
func amount(a num.Amount) string {
	return a.Rescale(2).String()
}

// percent writes a percentage as a number with two decimals, such as
// "25.00" for 25 %, as StopaType asks.
func percent(p num.Percentage) string {
	return p.Amount().Rescale(2).String()
}

// paymentMethod maps how the invoice was paid to NacinPlac. A receipt paid
// at the till records its payment as advances; an invoice without them
// falls back to the payment instructions. Several methods, or one without
// a code of its own, are O (other), as the schema says.
func paymentMethod(p *bill.PaymentDetails) PaymentMethod {
	var keys []cbc.Key
	if p != nil {
		for _, a := range p.Advances {
			keys = append(keys, a.Key)
		}
		if len(keys) == 0 && p.Instructions != nil {
			keys = append(keys, p.Instructions.Key)
		}
	}
	method := PaymentOther
	for i, k := range keys {
		m := meansMethod(k)
		if i > 0 && m != method {
			return PaymentOther
		}
		method = m
	}
	return method
}

func meansMethod(k cbc.Key) PaymentMethod {
	base := cbc.Key(strings.SplitN(k.String(), "+", 2)[0])
	switch base {
	case pay.MeansKeyCash:
		return PaymentCash
	case pay.MeansKeyCard:
		return PaymentCard
	case pay.MeansKeyCreditTransfer, pay.MeansKeyDebitTransfer, pay.MeansKeyDirectDebit:
		return PaymentTransfer
	}
	return PaymentOther
}

// buyerOIB is the customer's OIB, for a receipt to a Croatian business.
func buyerOIB(inv *bill.Invoice) string {
	c := inv.Customer
	if c == nil || c.TaxID == nil || c.TaxID.Country != l10n.HR.Tax() {
		return ""
	}
	return c.TaxID.Code.String()
}

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}
