package ubl_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/uuid"
	"github.com/invopop/phorm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const staticUUID uuid.UUID = "0195ce71-dc9c-72c8-bf2c-9890a4a9f0a2"

var updateOut = flag.Bool("update", false, "Update the expected output files in testdata")

// validate is a flag that enables schematron validation against phorm
var validate = flag.Bool("validate", false, "Run phorm schematron validation on generated XML")

// TestExamples round-trips the official HR CIUS examples in testdata/hr: each
// one is imported into GOBL, which must pass the addon's rules, and exported
// back to UBL. Both results are compared with the files in testdata/hr/out,
// and with -validate the UBL must also pass the Tax Administration's rules.
func TestExamples(t *testing.T) {
	var pc *phorm.Client
	if *validate {
		pc = phormClient(t)
	}
	examples, err := filepath.Glob(filepath.Join("testdata", "hr", "*.xml"))
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
			inv, ok := env.Extract().(*bill.Invoice)
			require.True(t, ok, "document is not an invoice")
			inv.UUID = staticUUID
			require.NoError(t, env.Calculate())
			require.NoError(t, env.Validate())

			js, err := json.MarshalIndent(env.Extract(), "", "\t")
			require.NoError(t, err)
			jsonPath := filepath.Join("testdata", "hr", "out", name+".json")
			if *updateOut {
				require.NoError(t, os.WriteFile(jsonPath, js, 0644))
			}
			expected, err := os.ReadFile(jsonPath)
			require.NoError(t, err)
			assert.JSONEq(t, string(expected), string(js), "Invoice should match the expected JSON. Update with --update flag.")

			out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
			require.NoError(t, err)
			xmlData, err := goblubl.Encode(out)
			require.NoError(t, err)
			xmlPath := filepath.Join("testdata", "hr", "out", name+".xml")
			if *updateOut {
				require.NoError(t, os.WriteFile(xmlPath, xmlData, 0644))
			}
			if *validate {
				validateXML(t, pc, hrubl.FormatHRCIUS.VESID(inv), xmlData)
			}
			expected, err = os.ReadFile(xmlPath)
			require.NoError(t, err)
			assert.Equal(t, string(expected), string(xmlData), "Output should match the expected XML. Update with --update flag.")
		})
	}
}
