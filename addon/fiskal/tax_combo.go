package fiskal

import (
	"github.com/invopop/gobl/addons/eu/en16931"
	"github.com/invopop/gobl/tax"
)

// normalizeTaxCombo sets the EN 16931 tax category (untdid-tax-category) of
// each tax combo from its key, as gobl.fr.ctc's flow10 addon does. GOBL
// groups the tax totals by percentage and extensions, not by key, so without
// it an exempt and an outside-scope line, which have no percentage, would
// add up to one total. CIS needs them apart: the exempt amount
// (IznosOslobPdv) and the amount not subject to tax (IznosNePodlOpor).
func normalizeTaxCombo(tc *tax.Combo) {
	if tc == nil {
		return
	}
	en16931.NormalizeTaxCombo(tc)
}
