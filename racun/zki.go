package racun

import (
	"crypto"
	"crypto/md5" //nolint:gosec // the specification defines the ZKI with MD5
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// ZKIInput is the receipt data the ZKI protects.
type ZKIInput struct {
	IssuerOIB string
	// IssuedAt is the issue time in Croatian local time, as printed on the
	// receipt.
	IssuedAt time.Time
	Number   *Number
	// Total is the receipt total as written in the request, such as
	// "1245.56".
	Total string
}

// ZKI computes the issuer's protection code (zaštitni kod izdavatelja,
// §12): the MD5 of the RSA-SHA256 signature of the issuer OIB, the issue
// time, the receipt number, premises and device, and the total, joined with
// no separator. The result is 32 lowercase hex characters.
//
// Only the holder of the private key can compute it, and it needs no
// connection to CIS, so a receipt can carry it when CIS can't be reached.
func ZKI(key crypto.Signer, in *ZKIInput) (string, error) {
	if key == nil {
		return "", errors.New("no key")
	}
	if in == nil || in.Number == nil {
		return "", errors.New("no receipt number")
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
		in.IssuedAt.Format(zkiDateTimeFormat) +
		in.Number.Sequential +
		in.Number.Premises +
		in.Number.Device +
		in.Total
}
