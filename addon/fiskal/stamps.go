package fiskal

import "github.com/invopop/gobl/cbc"

// Stamps added to a fiscalized invoice. The ZKI and the QR code go on every
// printed invoice, even one CIS has not answered yet; the JIR is added when
// CIS accepts the invoice, and the QR code then refers to it.
//
// The keys name the system, as tbai-qr and verifactu-qr do, and as this
// repo's eRačun stamps (eracun-cis-*) do.
//
// TODO: list them as stamp definitions in the AddonDef (Stamps), with names
// such as "Croatia Fiskal: ZKI" and a Usage, once the hr regime branch is
// rebased on a GOBL release with the stamp registry.
const (
	// StampZKI is the ZKI (zaštitni kod izdavatelja), the issuer's
	// protection code: 32 hexadecimal characters computed from the invoice
	// and signed with the issuer's certificate.
	StampZKI cbc.Key = "fiskal-zki"
	// StampJIR is the JIR (jedinstveni identifikator računa), the unique
	// invoice identifier CIS gives an accepted invoice: a UUID.
	StampJIR cbc.Key = "fiskal-jir"
	// StampQR is the URL the QR code on the printed invoice holds, for a
	// buyer to check the invoice with the Tax Administration. It has the JIR,
	// or the ZKI while the invoice has no JIR.
	StampQR cbc.Key = "fiskal-qr"
)
