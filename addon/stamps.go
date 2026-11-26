package eracun

import "github.com/invopop/gobl/cbc"

// Stamps added to an invoice once the Croatian fiscalization system (CIS)
// has accepted its record. CIS gives no fiscalization code for an eRačun:
// its answer carries only its own identifier for the request and the time
// it answered (T1 v1.15, §5.7.1).
const (
	// StampCISID is idZahtjeva, the UUID CIS gave the request that
	// fiscalized the invoice.
	StampCISID cbc.Key = "eracun-cis-id"
	// StampCISDate is when CIS answered that request, in RFC 3339 with
	// the offset.
	StampCISDate cbc.Key = "eracun-cis-date"
)
