// Converting a GOBL invoice into the Invoice CIS fiscalizes (Racun).
//
// The GOBL invoice must declare the hr-fiskal-b2c-v1 addon (package
// addon/fiskal) and be valid: the addon's rules check the fields the Invoice
// takes, so the conversion does not check their format again. The Invoice
// takes the issuer's OIB, the issue date and time, the number with the
// business premises and device labels, the numbering scope, the VAT system,
// the VAT breakdown, the total, the payment method, the operator's OIB, the
// paper invoice number and the buyer's OIB from the invoice. Only whether
// the Invoice is sent late comes from Options.

package racun

import (
	"crypto"
	"errors"
	"fmt"
	"strings"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/currency"
	"github.com/invopop/gobl/l10n"
	"github.com/invopop/gobl/num"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/pay"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/invopop/gobl/tax"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	"github.com/invopop/gobl.hr.eracun/addon/fiskal"
)

// Errors the conversion returns, wrapped with the field they are about.
var (
	// ErrNotInvoice is an envelope that holds no invoice.
	ErrNotInvoice = errors.New("envelope does not hold an invoice")
	// ErrMissing is an invoice without a field or addon fiscalization
	// requires.
	ErrMissing = errors.New("missing a field fiscalization requires")
	// ErrInvalid is a field whose value fiscalization can't take.
	ErrInvalid = errors.New("has a value fiscalization can't take")
	// ErrUnsupported is an invoice the conversion does not handle yet.
	ErrUnsupported = errors.New("not supported for fiscalization yet")
)

// Options are the Invoice values that are not in the GOBL invoice, as they
// depend on when it is sent.
type Options struct {
	// LateDelivery is true when the invoice was issued without a JIR and
	// is sent again (NakDost). An invoice with a paper invoice number is
	// always late.
	LateDelivery bool
}

// NewInvoice builds the Invoice of the GOBL invoice in the envelope, with its ZKI
// computed with key, the issuer's private key. The envelope must be
// calculated and valid.
func NewInvoice(env *gobl.Envelope, opts *Options, key crypto.Signer) (*Invoice, error) {
	if env == nil {
		return nil, ErrNotInvoice
	}
	inv, ok := env.Extract().(*bill.Invoice)
	if !ok {
		return nil, ErrNotInvoice
	}
	return FromInvoice(inv, opts, key)
}

// FromInvoice builds the Invoice of a calculated and valid GOBL invoice with
// the hr-fiskal-b2c-v1 addon, with its ZKI
// computed with key, the issuer's private key. The ZKI goes on the printed
// invoice and does not change when the Invoice is sent again.
func FromInvoice(inv *bill.Invoice, opts *Options, key crypto.Signer) (*Invoice, error) {
	if opts == nil {
		opts = new(Options)
	}
	r := new(Invoice)
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
	r.IssuedAt = formatDateTime(issuedAt, "T")
	ext := inv.Tax.Ext
	if r.Number, err = number(inv); err != nil {
		return nil, err
	}
	r.SequenceScope = SequencePremises
	if ext.Get(fiskal.ExtKeySequenceScope) == fiskal.SequenceDevice {
		r.SequenceScope = SequenceDevice
	}
	r.InVATSystem = ext.Get(fiskal.ExtKeyVATSystem) != fiskal.VATSystemOut
	if err := addTaxes(r, inv); err != nil {
		return nil, err
	}
	r.Total = amount(inv.Totals.Payable)
	r.PaymentMethod = paymentMethod(inv.Payment)
	if r.OperatorOIB, err = operatorOIB(inv); err != nil {
		return nil, err
	}
	r.HandwrittenNumber = ext.Get(fiskal.ExtKeyPaperNumber).String()
	r.LateDelivery = opts.LateDelivery || r.HandwrittenNumber != ""
	// CIS refuses a buyer's OIB on an invoice paid by bank transfer (error
	// 181, §2.1.5).
	if r.PaymentMethod != PaymentTransfer {
		r.BuyerOIB = buyerOIB(inv)
	}

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
	case !fiskal.V1.In(inv.GetAddons()...):
		return fmt.Errorf("addon %s %w", fiskal.V1, ErrMissing)
	case inv.Tax == nil:
		return fmt.Errorf("tax %w", ErrMissing)
	case inv.Totals == nil:
		return fmt.Errorf("totals %w: calculate the invoice first", ErrMissing)
	case inv.Type != bill.InvoiceTypeStandard:
		return fmt.Errorf("invoice type %q %w", inv.Type, ErrUnsupported)
	case inv.Currency != currency.EUR:
		return fmt.Errorf("currency %q %w: fiscalized invoices are in euros", inv.Currency, ErrInvalid)
	case inv.Totals.Payable.IsNegative():
		return fmt.Errorf("negative total %w", ErrUnsupported)
	case inv.Totals.Rounding != nil && !inv.Totals.Rounding.IsZero():
		// GOBL adds the rounding to Totals.Payable, so to the total and the
		// ZKI, but to no tax breakdown, and the request has no element for
		// it.
		return fmt.Errorf("rounding %s %w", inv.Totals.Rounding, ErrUnsupported)
	}
	// A charge or discount with VAT is in Totals.Taxes, so the VAT
	// breakdown includes it. One without taxes changes the total but no
	// breakdown, and is usually a fee (Naknade), which is not supported.
	for _, c := range inv.Charges {
		if len(c.Taxes) == 0 {
			return fmt.Errorf("charge %q without taxes %w: fees (Naknade) are not supported", c.Reason, ErrUnsupported)
		}
	}
	for _, d := range inv.Discounts {
		if len(d.Taxes) == 0 {
			return fmt.Errorf("discount %q without taxes %w", d.Reason, ErrUnsupported)
		}
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
	return inv.Supplier.TaxID.Code.String(), nil
}

// operatorOIB is the OIB of the supplier person with the operator key.
func operatorOIB(inv *bill.Invoice) (string, error) {
	op := eracun.Operator(inv.Supplier.People)
	if op == nil {
		return "", fmt.Errorf("operator %w: a supplier person with the %q key", ErrMissing, eracun.PersonKeyOperator)
	}
	id := org.IdentityForType(op.Identities, hr.IdentityTypeOIB)
	if id == nil {
		return "", fmt.Errorf("operator OIB %w", ErrMissing)
	}
	return id.Code.String(), nil
}

// issueTime is the issue date and time of the invoice. GOBL dates and times
// are local to the regime and have no zone, and so do the Invoice's, so they
// are copied as they are: a time.Time would move a time that does not exist
// in Croatia, such as 02:30 on the day clocks go forward. Fiscalization
// needs the time, so it must be set.
func issueTime(inv *bill.Invoice) (cal.DateTime, error) {
	if inv.IssueTime == nil || inv.IssueTime.IsZero() {
		return cal.DateTime{}, fmt.Errorf("issue time %w", ErrMissing)
	}
	d, t := inv.IssueDate, inv.IssueTime
	return cal.MakeDateTime(d.Year, d.Month, d.Day, t.Hour, t.Minute, t.Second), nil
}

// number is the invoice number: the GOBL invoice code, which the addon
// requires to be the bare sequential number, and the business premises and
// device labels.
func number(inv *bill.Invoice) (*Number, error) {
	n := &Number{
		Sequential: inv.Code.String(),
		Premises:   inv.Tax.Ext.Get(fiskal.ExtKeyPremises).String(),
		Device:     inv.Tax.Ext.Get(fiskal.ExtKeyDevice).String(),
	}
	switch {
	case n.Sequential == "":
		return nil, fmt.Errorf("invoice code %w", ErrMissing)
	case n.Premises == "":
		return nil, fmt.Errorf("premises label (%s) %w", fiskal.ExtKeyPremises, ErrMissing)
	case n.Device == "":
		return nil, fmt.Errorf("device label (%s) %w", fiskal.ExtKeyDevice, ErrMissing)
	}
	return n, nil
}

// addTaxes adds the VAT breakdown: one entry per rate with a percentage, and
// the exempt and outside-scope bases as totals.
func addTaxes(r *Invoice, inv *bill.Invoice) error {
	if inv.Totals.Taxes == nil {
		return nil
	}
	// The totals start with two decimals: Add keeps the decimals of its
	// receiver, so a zero Amount would round every base to whole euros.
	exempt, outside := num.MakeAmount(0, 2), num.MakeAmount(0, 2)
	var hasExempt, hasOutside bool
	for _, cat := range inv.Totals.Taxes.Categories {
		if cat.Code != tax.CategoryVAT {
			return fmt.Errorf("tax category %q %w", cat.Code, ErrUnsupported)
		}
		for _, rate := range cat.Rates {
			// The eRačun addon records a consumption tax (PP), a packaging
			// return fee (POVNAK), the motor vehicle tax (PPMV) or another
			// untaxed fee (N) as exempt VAT with its HR tax category. They
			// belong in Pnp, Naknade or OstaliPor, not IznosOslobPdv,
			// which is not supported yet.
			if c := rate.Ext.Get(eracun.ExtKeyTaxCategory); c.In(eracun.PassThroughTaxCategories...) {
				return fmt.Errorf("HR tax category %q %w", c, ErrUnsupported)
			}
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

// paymentMethod maps how the invoice was paid to NacinPlac. An invoice paid
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

// buyerOIB is the customer's OIB, for an invoice to a Croatian business. It
// is mandatory for one paid in cash or by card (§2.1.1.1).
func buyerOIB(inv *bill.Invoice) string {
	c := inv.Customer
	if c == nil || c.TaxID == nil || c.TaxID.Country != l10n.HR.Tax() {
		return ""
	}
	return c.TaxID.Code.String()
}
