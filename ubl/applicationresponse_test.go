package ubl_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplicationResponse round-trips the rejection example of the HR
// application response specification, with placeholder OIBs.
func TestApplicationResponse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "hr-ar", "odbijanje.xml"))
	require.NoError(t, err)
	doc, err := goblubl.Decode(data)
	require.NoError(t, err)
	env, err := goblubl.Import(doc)
	require.NoError(t, err)
	env.Head.UUID = staticUUID
	st, ok := env.Extract().(*bill.Status)
	require.True(t, ok, "document is not a status")
	st.UUID = staticUUID
	require.NoError(t, env.Calculate())
	require.NoError(t, env.Validate())
	assert.Contains(t, st.GetAddons(), eracun.V1)

	js, err := json.MarshalIndent(st, "", "\t")
	require.NoError(t, err)
	jsonPath := filepath.Join("testdata", "hr-ar", "out", "odbijanje.json")
	if *updateOut {
		require.NoError(t, os.WriteFile(jsonPath, js, 0644))
	}
	expected, err := os.ReadFile(jsonPath)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(js), "Status should match the expected JSON. Update with --update flag.")

	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	xmlData, err := goblubl.Encode(out)
	require.NoError(t, err)
	xmlPath := filepath.Join("testdata", "hr-ar", "out", "odbijanje.xml")
	if *updateOut {
		require.NoError(t, os.WriteFile(xmlPath, xmlData, 0644))
	}
	expected, err = os.ReadFile(xmlPath)
	require.NoError(t, err)
	assert.Equal(t, string(expected), string(xmlData), "Output should match the expected XML. Update with --update flag.")
}
