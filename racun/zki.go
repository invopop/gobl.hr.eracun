package racun

import (
	"crypto"
	"crypto/md5" //nolint:gosec // the specification defines the ZKI with MD5
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/invopop/gobl/cal"
)

// ZKIInput is the invoice data the ZKI protects.
type ZKIInput struct {
	IssuerOIB string
	// IssuedAt is the issue date and time as printed on the invoice:
	// Croatian local time, with no zone. Convert a moment, such as the
	// till's clock, to Croatian local time before setting it.
	IssuedAt cal.DateTime
	Number   *Number
	// Total is the invoice total as written in the request, such as
	// "1245.56".
	Total string
}

// ZKI computes the issuer's protection code (zaštitni kod izdavatelja,
// §12): the MD5 of the RSA-SHA256 signature of the issuer OIB, the issue
// time, the invoice number, premises and device, and the total, joined with
// no separator. The result is 32 lowercase hex characters.
//
// Only the holder of the private key can compute it, and it needs no
// connection to CIS, so an invoice can carry it when CIS can't be reached.
// The key must be RSA: an RSA signature is the same every time, so the ZKI
// of an invoice is too, while an ECDSA one changes with each call.
func ZKI(key crypto.Signer, in *ZKIInput) (string, error) {
	if key == nil {
		return "", errors.New("no key")
	}
	if _, ok := key.Public().(*rsa.PublicKey); !ok {
		return "", errors.New("the key is not RSA")
	}
	if in == nil || in.Number == nil {
		return "", errors.New("no invoice number")
	}
	data := zkiData(in)
	digest := sha256.Sum256([]byte(data))
	sig, err := key.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return "", fmt.Errorf("signing ZKI data: %w", err)
	}
	sum := md5.Sum(sig) //nolint:gosec // see import
	return hex.EncodeToString(sum[:]), nil
}

func zkiData(in *ZKIInput) string {
	return in.IssuerOIB +
		formatDateTime(in.IssuedAt, " ") +
		in.Number.Sequential +
		in.Number.Premises +
		in.Number.Device +
		in.Total
}
