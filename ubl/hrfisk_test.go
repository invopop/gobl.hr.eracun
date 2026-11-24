package ubl_test

import (
	"os"
	"strings"
	"testing"

	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/regimes/hr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHRFISK20DataDecodes(t *testing.T) {
	src, err := os.ReadFile("testdata/hr/eRacun-Oslobodjeno PDV.xml")
	require.NoError(t, err)
	doc, err := goblubl.Decode(src)
	require.NoError(t, err)
	data, err := hrubl.FindHRFISK20Data(doc.(*goblubl.Invoice))
	require.NoError(t, err)
	require.NotNil(t, data)
	assert.Equal(t, hrubl.NamespaceHRExtension, data.XMLName.Space)
	st := data.HRTaxTotal.HRTaxSubtotals[0]
	assert.Equal(t, "E", st.HRTaxCategory.ID)
	assert.Equal(t, "100.00", st.TaxableAmount.Value)
}

// swapExtensions puts the second UBL extension of the document before the
// first.
func swapExtensions(t *testing.T, src []byte) []byte {
	t.Helper()
	const start, end = "<ext:UBLExtension>", "</ext:UBLExtension>"
	s := string(src)
	i := strings.Index(s, start)
	j := strings.Index(s, end) + len(end)
	k := strings.Index(s[j:], start) + j
	l := strings.Index(s[k:], end) + k + len(end)
	require.True(t, i >= 0 && i < j && j <= k && k < l, "two extensions")
	return []byte(s[:i] + s[k:l] + s[j:k] + s[i:j] + s[l:])
}

func TestFindHRFISK20Data(t *testing.T) {
	cashBasis, err := os.ReadFile("testdata/hr/eRacun-PDV25-Obracun PDV po naplati.xml")
	require.NoError(t, err)

	find := func(t *testing.T, src []byte) *hrubl.HRFISK20Data {
		t.Helper()
		doc, err := goblubl.Decode(src)
		require.NoError(t, err)
		data, err := hrubl.FindHRFISK20Data(doc.(*goblubl.Invoice))
		require.NoError(t, err)
		return data
	}

	t.Run("data first", func(t *testing.T) {
		data := find(t, cashBasis)
		require.NotNil(t, data)
		assert.NotEmpty(t, data.HRObracunPDVPoNaplati)
	})

	t.Run("signature placeholder first", func(t *testing.T) {
		swapped := swapExtensions(t, cashBasis)
		require.Less(t, strings.Index(string(swapped), "UBLDocumentSignatures"), strings.Index(string(swapped), "HRFISK20Data"))
		data := find(t, swapped)
		require.NotNil(t, data)
		assert.NotEmpty(t, data.HRObracunPDVPoNaplati)

		doc, err := goblubl.Decode(swapped)
		require.NoError(t, err)
		env, err := goblubl.Import(doc)
		require.NoError(t, err)
		assert.True(t, env.Extract().(*bill.Invoice).HasTags(hr.TagCashBasis))
	})

	t.Run("signature placeholder only", func(t *testing.T) {
		src, err := os.ReadFile("testdata/hr/eRacun-PDV25.xml")
		require.NoError(t, err)
		require.Contains(t, string(src), "UBLDocumentSignatures")
		assert.Nil(t, find(t, src))
	})

	t.Run("malformed content is an error", func(t *testing.T) {
		in := new(goblubl.Invoice)
		in.AddExtension(&goblubl.Extension{
			ExtensionContent: &goblubl.ExtensionContent{Inner: []byte("<hrextac:HRFISK20Data>")},
		})
		_, err := hrubl.FindHRFISK20Data(in)
		assert.ErrorContains(t, err, "decoding extension content")
	})

	t.Run("no extensions", func(t *testing.T) {
		data, err := hrubl.FindHRFISK20Data(new(goblubl.Invoice))
		require.NoError(t, err)
		assert.Nil(t, data)
	})
}
