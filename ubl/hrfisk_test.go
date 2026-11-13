package ubl_test

import (
	"os"
	"testing"

	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHRFISK20DataDecodes(t *testing.T) {
	src, err := os.ReadFile("testdata/hr/eRacun-Oslobodjeno PDV.xml")
	require.NoError(t, err)
	doc, err := goblubl.Decode(src)
	require.NoError(t, err)
	data, ok := goblubl.FindExtensionData[*hrubl.HRFISK20Data](doc.(*goblubl.Invoice).Extensions)
	require.True(t, ok)
	assert.Equal(t, hrubl.NamespaceHRExtension, data.Namespace)
	st := data.HRTaxTotal.HRTaxSubtotals[0]
	assert.Equal(t, "E", st.HRTaxCategory.ID)
	assert.Equal(t, "100.00", st.TaxableAmount.Value)
}
