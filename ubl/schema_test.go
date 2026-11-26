package ubl_test

import (
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The HR application response has no rule set in phorm, and its
// specification (Porezna uprava, AplikacijskiOdgovor v1.1) publishes no schema
// of its own: the message is a plain UBL 2.1 ApplicationResponse. With -xsd,
// it is checked against the OASIS UBL 2.1 schema in testdata/schema with
// xmllint, run in a container so that nothing has to be installed locally.
// Docker must be available.
const (
	xmllintImage = "alpine:3.20"
	arSchema     = "maindoc/UBL-ApplicationResponse-2.1.xsd"
)

// validateSchema fails the test if the document does not validate against
// the UBL 2.1 ApplicationResponse schema.
func validateSchema(t *testing.T, data []byte) {
	t.Helper()

	schemaDir, err := filepath.Abs(filepath.Join("testdata", "schema"))
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.xml"), data, 0o644))

	out, err := exec.Command("docker", "run", "--rm",
		"-v", schemaDir+":/xsd:ro",
		"-v", dir+":/w:ro",
		xmllintImage, "sh", "-c",
		"apk add -q libxml2-utils && xmllint --noout --schema /xsd/"+arSchema+" /w/doc.xml",
	).CombinedOutput()
	if err != nil {
		t.Errorf("%s: schema validation failed: %v\n%s", arSchema, err, out)
	}
}

// arDocument holds the parts of an application response whose values the
// HR specification fixes, read by element name.
type arDocument struct {
	XMLName         xml.Name
	CustomizationID string     `xml:"CustomizationID"`
	ProfileID       string     `xml:"ProfileID"`
	ID              string     `xml:"ID"`
	IssueDate       string     `xml:"IssueDate"`
	IssueTime       string     `xml:"IssueTime"`
	Sender          arCode     `xml:"SenderParty>EndpointID"`
	Receiver        arCode     `xml:"ReceiverParty>EndpointID"`
	Responses       []arAnswer `xml:"DocumentResponse"`
}

type arAnswer struct {
	Code          arCode   `xml:"Response>ResponseCode"`
	Description   string   `xml:"Response>Description"`
	Reasons       []arCode `xml:"Response>Status>StatusReasonCode"`
	StatusReasons []string `xml:"Response>Status>StatusReason"`
	DocID         string   `xml:"DocumentReference>ID"`
	DocIssueDate  string   `xml:"DocumentReference>IssueDate"`
}

type arCode struct {
	SchemeID string `xml:"schemeID,attr"`
	ListID   string `xml:"listID,attr"`
	Value    string `xml:",chardata"`
}

// checkResponseSpec checks an application response against the values that
// the HR application response specification fixes (section 2, AR-BT-1 to
// AR-BT-14). The UBL 2.1 schema says nothing about them.
func checkResponseSpec(t *testing.T, data []byte) {
	t.Helper()

	const specID = "urn:mfin.gov.hr:AplikacijskiOdgovor-2025:1.0"
	var doc arDocument
	require.NoError(t, xml.Unmarshal(data, &doc))

	assert.Equal(t, "urn:oasis:names:specification:ubl:schema:xsd:ApplicationResponse-2", doc.XMLName.Space)
	assert.Equal(t, "ApplicationResponse", doc.XMLName.Local)
	assert.Equal(t, specID, doc.CustomizationID, "AR-BT-1")
	assert.Equal(t, specID, doc.ProfileID, "AR-BT-2")
	assert.NotEmpty(t, doc.ID, "AR-BT-3")
	assert.NotEmpty(t, doc.IssueDate, "AR-BT-4")
	if doc.IssueTime != "" {
		assert.Regexp(t, regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`), doc.IssueTime, "AR-BT-5: HH:MM:SS without a time zone")
	}
	for _, p := range []arCode{doc.Sender, doc.Receiver} {
		assert.Equal(t, "9934", p.SchemeID, "AR-BT-6, AR-BT-8: AMS address")
		assert.NotEmpty(t, p.Value, "AR-BT-6, AR-BT-8")
	}

	require.Len(t, doc.Responses, 1)
	r := doc.Responses[0]
	assert.Equal(t, "UNCL4343OpSubset", r.Code.ListID, "AR-BT-10")
	assert.Contains(t, []string{"AB", "IP", "UQ", "CA", "RE", "AP", "PD"}, r.Code.Value, "AR-BT-10: Peppol invoice response codes")
	if r.Code.Value == "RE" {
		assert.NotEmpty(t, r.Description, "AR-BT-11 is required to reject")
		require.Len(t, r.Reasons, 1, "AR-BG-5 is required to reject")
		assert.Equal(t, "MFINVrstaRazlogaOdbijanja", r.Reasons[0].ListID, "AR-BT-12")
		assert.Contains(t, []string{"N", "U", "O"}, r.Reasons[0].Value, "AR-BT-12")
	} else {
		assert.LessOrEqual(t, len(r.Reasons), 1, "AR-BG-5: at most one status")
		for _, s := range r.Reasons {
			assert.Contains(t, []string{"OPStatusReason", "OPStatusAction"}, s.ListID, "AR-BG-5: a Peppol status reason or action by agreement")
		}
	}
	assert.Empty(t, r.StatusReasons, "AR-BG-5 has no status text")
	assert.NotEmpty(t, r.DocID, "AR-BT-13")
	assert.NotEmpty(t, r.DocIssueDate, "AR-BT-14")
}
