package ubl_test

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	eracun "github.com/invopop/gobl.hr.eracun/addon"
	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cbc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplicationResponse round-trips the application responses in
// testdata/hr-ar, built from the rejection example of the HR application
// response specification, and checks each export against the values the
// specification fixes. With -xsd the export must also pass the UBL 2.1 schema.
func TestApplicationResponse(t *testing.T) {
	examples, err := filepath.Glob(filepath.Join("testdata", "hr-ar", "*.xml"))
	require.NoError(t, err)
	require.NotEmpty(t, examples)

	for _, example := range examples {
		name := strings.TrimSuffix(filepath.Base(example), ".xml")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(example)
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
			jsonPath := filepath.Join("testdata", "hr-ar", "out", name+".json")
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
			xmlPath := filepath.Join("testdata", "hr-ar", "out", name+".xml")
			if *updateOut {
				require.NoError(t, os.WriteFile(xmlPath, xmlData, 0644))
			}
			checkResponseSpec(t, xmlData)
			if *validateXSD {
				validateSchema(t, xmlData)
			}
			expected, err = os.ReadFile(xmlPath)
			require.NoError(t, err)
			assert.Equal(t, string(expected), string(xmlData), "Output should match the expected XML. Update with --update flag.")
		})
	}
}

// TestApplicationResponseCodes takes each response code of the Peppol invoice
// response list, which the HR specification allows by agreement besides the
// rejection, through import and export.
func TestApplicationResponseCodes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "hr-ar", "odbijanje.xml"))
	require.NoError(t, err)
	var base arDocument
	require.NoError(t, xml.Unmarshal(data, &base))
	require.Len(t, base.Responses, 1)
	description := base.Responses[0].Description
	reason := base.Responses[0].Reasons[0].Value

	tests := []struct {
		code string
		key  cbc.Key
		out  string
	}{
		{"AB", bill.StatusLineAcknowledged, "AB"},
		{"IP", bill.StatusLineProcessing, "IP"},
		{"UQ", bill.StatusLineQuerying, "UQ"},
		{"AP", bill.StatusLineAccepted, "AP"},
		{"CA", bill.StatusLineAccepted, "AP"},
		{"PD", bill.StatusLinePaid, "PD"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			// Without the rejection's description and reason, which only a
			// rejection requires.
			x := string(data)
			x = strings.Replace(x, ">RE<", ">"+tt.code+"<", 1)
			x = strings.Replace(x, "<cbc:Description>"+description+"</cbc:Description>", "", 1)
			start := strings.Index(x, "<cac:Status>")
			end := strings.Index(x, "</cac:Status>")
			require.True(t, start > 0 && end > start, "rejection reason not found")
			require.Contains(t, x[start:end], reason)
			x = x[:start] + x[end+len("</cac:Status>"):]

			doc, err := goblubl.Decode([]byte(x))
			require.NoError(t, err)
			env, err := goblubl.Import(doc)
			require.NoError(t, err)
			st, ok := env.Extract().(*bill.Status)
			require.True(t, ok, "document is not a status")
			require.Len(t, st.Lines, 1)
			assert.Equal(t, tt.key, st.Lines[0].Key)
			assert.Empty(t, st.Lines[0].Reasons)
			require.NoError(t, env.Validate())

			out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
			require.NoError(t, err)
			xmlData, err := goblubl.Encode(out)
			require.NoError(t, err)
			checkResponseSpec(t, xmlData)
			var res arDocument
			require.NoError(t, xml.Unmarshal(xmlData, &res))
			assert.Equal(t, tt.out, res.Responses[0].Code.Value)
			if *validateXSD {
				validateSchema(t, xmlData)
			}
		})
	}

	// Responses other than a rejection carry the Peppol status reasons and
	// actions agreed between the parties.
	t.Run("query with a reason and an action", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join("testdata", "hr-ar", "upit.xml"))
		require.NoError(t, err)
		doc, err := goblubl.Decode(data)
		require.NoError(t, err)
		env, err := goblubl.Import(doc)
		require.NoError(t, err)
		require.NoError(t, env.Validate())
		line := env.Extract().(*bill.Status).Lines[0]
		assert.Equal(t, bill.StatusLineQuerying, line.Key)
		require.Len(t, line.Reasons, 1)
		assert.Equal(t, bill.ReasonKeyReferences, line.Reasons[0].Key)
		assert.Equal(t, "Nedostaje broj narudžbenice.", line.Reasons[0].Description)
		require.Len(t, line.Actions, 1)
		assert.Equal(t, bill.ActionKeyProvide, line.Actions[0].Key)

		out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
		require.NoError(t, err)
		xmlData, err := goblubl.Encode(out)
		require.NoError(t, err)
		var res arDocument
		require.NoError(t, xml.Unmarshal(xmlData, &res))
		assert.Equal(t, []arCode{
			{ListID: "OPStatusReason", Value: "REF"},
			{ListID: "OPStatusAction", Value: "PIN"},
		}, res.Responses[0].Reasons)
		assert.Equal(t, []string{"Nedostaje broj narudžbenice."}, res.Responses[0].StatusReasons)
	})

	// The text of a reason is not repeated as the description, which only a
	// rejection requires.
	t.Run("query without a description", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join("testdata", "hr-ar", "upit.xml"))
		require.NoError(t, err)
		x := strings.Replace(string(data), "<cbc:Description>Upit o računu</cbc:Description>", "", 1)
		doc, err := goblubl.Decode([]byte(x))
		require.NoError(t, err)
		env, err := goblubl.Import(doc)
		require.NoError(t, err)
		require.NoError(t, env.Validate())
		out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
		require.NoError(t, err)
		xmlData, err := goblubl.Encode(out)
		require.NoError(t, err)
		var res arDocument
		require.NoError(t, xml.Unmarshal(xmlData, &res))
		assert.Empty(t, res.Responses[0].Description)
		assert.Equal(t, []string{"Nedostaje broj narudžbenice."}, res.Responses[0].StatusReasons)
	})

	t.Run("acceptance with a rejection reason", func(t *testing.T) {
		doc, err := goblubl.Decode([]byte(strings.Replace(string(data), ">RE<", ">AP<", 1)))
		require.NoError(t, err)
		env, err := goblubl.Import(doc)
		require.NoError(t, err)
		err = env.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GOBL-HR-ERACUN-BILL-STATUS-10")
	})
}
