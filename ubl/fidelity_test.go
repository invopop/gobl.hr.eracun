package ubl_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	hrubl "github.com/invopop/gobl.hr.eracun/ubl"
	goblubl "github.com/invopop/gobl.ubl"
	"github.com/invopop/gobl/num"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pendingDifferences are losses that a change elsewhere fixes. The test
// fails once one of them no longer happens, so it can be taken off the list.
var pendingDifferences = []struct {
	path   string
	lost   bool
	reason string
}{
	{"/cac:AccountingSupplierParty/cac:Party/cac:PartyLegalEntity/cbc:CompanyLegalForm", true,
		"BT-33, which gobl.ubl doesn't map to GOBL yet"},
}

// TestExamplesKeepContent checks that exporting an imported HR CIUS invoice
// gives back what the original said: every element value and attribute of
// the original must be in the export, and the export may add nothing, apart
// from the differences in acceptedDifference and pendingDifferences. Element
// order is not compared. Exporting the export again must give the same XML.
func TestExamplesKeepContent(t *testing.T) {
	examples, err := filepath.Glob(filepath.Join("testdata", "hr", "*.xml"))
	require.NoError(t, err)
	require.NotEmpty(t, examples)
	pending := make([]int, len(pendingDifferences))

	for _, example := range examples {
		name := strings.TrimSuffix(filepath.Base(example), ".xml")
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(example)
			require.NoError(t, err)
			exported := roundTrip(t, data)
			assert.Equal(t, string(exported), string(roundTrip(t, exported)), "a second round trip should change nothing")

			in, err := xmlLeaves(data)
			require.NoError(t, err)
			ex, err := xmlLeaves(exported)
			require.NoError(t, err)
			lost, added := leafDiff(in, ex)
			var diffs []string
			for _, l := range lost {
				if !acceptedDifference(l, true, in, ex) && !pendingDifference(l, true, pending) {
					diffs = append(diffs, "lost:  "+l.String())
				}
			}
			for _, a := range added {
				if !acceptedDifference(a, false, in, ex) && !pendingDifference(a, false, pending) {
					diffs = append(diffs, "added: "+a.String())
				}
			}
			assert.Empty(t, diffs, "the export should say what the original said")
		})
	}
	for i, n := range pending {
		assert.NotZero(t, n, "%s no longer differs: take it off pendingDifferences", pendingDifferences[i].reason)
	}
}

// roundTrip imports the UBL document into GOBL and exports it back to the
// HR CIUS format.
func roundTrip(t *testing.T, data []byte) []byte {
	t.Helper()
	doc, err := goblubl.Decode(data)
	require.NoError(t, err)
	env, err := goblubl.Import(doc)
	require.NoError(t, err)
	require.NoError(t, env.Calculate())
	out, err := goblubl.Export(env, goblubl.WithFormat(hrubl.FormatHRCIUS))
	require.NoError(t, err)
	exported, err := goblubl.Encode(out)
	require.NoError(t, err)
	return exported
}

func pendingDifference(l leaf, lost bool, pending []int) bool {
	for i, p := range pendingDifferences {
		if p.lost == lost && strings.HasSuffix(l.path, p.path) {
			pending[i]++
			return true
		}
	}
	return false
}

// leaf is the value of an element without child elements, or of an
// attribute, with the path of element names that leads to it.
type leaf struct {
	path  string
	value string
}

func (l leaf) String() string {
	return fmt.Sprintf("%s = %q", l.path, l.value)
}

// leafPrefixes names the namespaces of the leaf paths.
var leafPrefixes = map[string]string{
	goblubl.NamespaceCBC:           "cbc",
	goblubl.NamespaceCAC:           "cac",
	goblubl.NamespaceEXT:           "ext",
	hrubl.NamespaceHRExtension:     "hrextac",
	goblubl.NamespaceUBLInvoice:    "",
	goblubl.NamespaceUBLCreditNote: "",
	"urn:oasis:names:specification:ubl:schema:xsd:CommonSignatureComponents-2":    "sig",
	"urn:oasis:names:specification:ubl:schema:xsd:SignatureAggregateComponents-2": "sac",
}

func leafName(n xml.Name) string {
	p, ok := leafPrefixes[n.Space]
	if !ok {
		p = n.Space
	}
	if p == "" {
		return n.Local
	}
	return p + ":" + n.Local
}

// leafGroups are the repeated elements of a document whose leaves are told
// apart by the element's position, so that a value moving from one to
// another shows. Charges and allowances are counted apart, as gobl.ubl
// writes the charges first.
var leafGroups = []string{"cac:AllowanceCharge", "cac:InvoiceLine", "cac:CreditNoteLine", "cac:PaymentMeans"}

// xmlLeaves lists the leaves of the document, without namespace
// declarations and schema locations.
func xmlLeaves(data []byte) ([]leaf, error) {
	type frame struct {
		path     string
		text     strings.Builder
		children bool
		group    bool
		start    int
	}
	var stack []*frame
	var leaves []leaf
	counts := make(map[string]int)
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return leaves, nil
			}
			return nil, err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			path := "/" + leafName(tk.Name)
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = true
				path = parent.path + path
			}
			for _, a := range tk.Attr {
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" || a.Name.Space == goblubl.NamespaceXSI {
					continue
				}
				leaves = append(leaves, leaf{path + "/@" + leafName(a.Name), strings.TrimSpace(a.Value)})
			}
			group := len(stack) == 1 && slices.Contains(leafGroups, leafName(tk.Name))
			stack = append(stack, &frame{path: path, group: group, start: len(leaves)})
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(tk)
			}
		case xml.EndElement:
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !f.children {
				leaves = append(leaves, leaf{f.path, strings.Join(strings.Fields(f.text.String()), " ")})
			}
			if f.group {
				key := f.path
				for _, l := range leaves[f.start:] {
					if l.path == f.path+"/cbc:ChargeIndicator" {
						key += "/" + l.value
					}
				}
				counts[key]++
				for i := f.start; i < len(leaves); i++ {
					leaves[i].path = fmt.Sprintf("%s[%s%d]%s", f.path, strings.TrimPrefix(key, f.path), counts[key], strings.TrimPrefix(leaves[i].path, f.path))
				}
			}
		}
	}
}

// leafDiff provides the leaves only in a and those only in b, counting
// repeated leaves.
func leafDiff(a, b []leaf) (onlyA, onlyB []leaf) {
	count := make(map[leaf]int)
	for _, l := range a {
		count[l]++
	}
	for _, l := range b {
		count[l]--
	}
	for l, n := range count {
		for ; n > 0; n-- {
			onlyA = append(onlyA, l)
		}
		for ; n < 0; n++ {
			onlyB = append(onlyB, l)
		}
	}
	sort.Slice(onlyA, func(i, j int) bool { return onlyA[i].String() < onlyA[j].String() })
	sort.Slice(onlyB, func(i, j int) bool { return onlyB[i].String() < onlyB[j].String() })
	return onlyA, onlyB
}

// acceptedDifference reports the differences that carry no information:
//   - the empty signature placeholder, which HR invoices don't need as the
//     UBL is not signed (HR-BR-33 only tolerates an empty one);
//   - a price base quantity of one, the EN 16931 default (BT-149), and its
//     unit;
//   - a number written with other trailing zeros, such as 0.00 for 0;
//   - a trading name (BT-28) that repeats a party's legal name (BT-27),
//     which gobl.ubl writes for every format.
func acceptedDifference(l leaf, lost bool, original, exported []leaf) bool {
	other := original
	if lost {
		other = exported
	}
	switch {
	case strings.HasSuffix(l.path, "/sig:UBLDocumentSignatures/sac:SignatureInformation"):
		return lost && l.value == ""
	case strings.HasSuffix(l.path, "/cac:Price/cbc:BaseQuantity"):
		return lost && isOne(l.value)
	case strings.HasSuffix(l.path, "/cac:Price/cbc:BaseQuantity/@unitCode"):
		return lost && allOne(original, strings.TrimSuffix(l.path, "/@unitCode"))
	case strings.HasSuffix(l.path, "/hrextac:HRObracunPDVPoNaplati"):
		// Only the flag's presence carries meaning, and the export writes the
		// VAT Act wording whatever the original said.
		return slices.ContainsFunc(other, func(o leaf) bool { return o.path == l.path })
	case strings.HasSuffix(l.path, "/cac:PartyName/cbc:Name") && !lost:
		legal := strings.TrimSuffix(l.path, "/cac:PartyName/cbc:Name") + "/cac:PartyLegalEntity/cbc:RegistrationName"
		if hasLeaf(original, leaf{legal, l.value}) {
			return true
		}
	}
	return sameNumberAt(l, other)
}

func isOne(v string) bool {
	a, err := num.AmountFromString(v)
	return err == nil && a.Equals(num.MakeAmount(1, 0))
}

func allOne(leaves []leaf, path string) bool {
	for _, l := range leaves {
		if l.path == path && !isOne(l.value) {
			return false
		}
	}
	return true
}

func hasLeaf(leaves []leaf, want leaf) bool {
	for _, l := range leaves {
		if l == want {
			return true
		}
	}
	return false
}

// sameNumberAt reports a number that the other document writes at the same
// path with different trailing zeros.
func sameNumberAt(l leaf, other []leaf) bool {
	a, err := num.AmountFromString(l.value)
	if err != nil {
		return false
	}
	for _, o := range other {
		if o.path != l.path || o.value == l.value {
			continue
		}
		if b, err := num.AmountFromString(o.value); err == nil && a.Equals(b) {
			return true
		}
	}
	return false
}

// TestLeafDiffSeesMoves checks that a value moving to another charge counts
// as a difference.
func TestLeafDiffSeesMoves(t *testing.T) {
	doc := func(a, b string) []leaf {
		data := `<Invoice xmlns="` + goblubl.NamespaceUBLInvoice + `" xmlns:cac="` + goblubl.NamespaceCAC + `" xmlns:cbc="` + goblubl.NamespaceCBC + `">` +
			`<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator><cbc:Amount>1</cbc:Amount><cbc:AllowanceChargeReason>` + a + `</cbc:AllowanceChargeReason></cac:AllowanceCharge>` +
			`<cac:AllowanceCharge><cbc:ChargeIndicator>true</cbc:ChargeIndicator><cbc:Amount>2</cbc:Amount><cbc:AllowanceChargeReason>` + b + `</cbc:AllowanceChargeReason></cac:AllowanceCharge>` +
			`</Invoice>`
		leaves, err := xmlLeaves([]byte(data))
		require.NoError(t, err)
		return leaves
	}
	lost, added := leafDiff(doc("X", "Y"), doc("X", "Y"))
	assert.Empty(t, lost)
	assert.Empty(t, added)
	lost, added = leafDiff(doc("X", "Y"), doc("Y", "X"))
	assert.Len(t, lost, 2)
	assert.Len(t, added, 2)
}
