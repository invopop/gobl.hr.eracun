package racun

import (
	"crypto/x509"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var validateXSD = flag.Bool("xsd", false, "validate signed requests and the expected invoices against the schema with xmllint in Docker")

func TestSign(t *testing.T) {
	cert := testCertificate(t)

	signed, err := Sign(cert, testRequest())
	require.NoError(t, err)
	doc := etree.NewDocument()
	require.NoError(t, doc.ReadFromBytes(signed))
	root := doc.Root()

	t.Run("adds the signature as the root's last child", func(t *testing.T) {
		children := root.ChildElements()
		require.Len(t, children, 3)
		assert.Equal(t, "Zaglavlje", children[0].Tag)
		assert.Equal(t, "Racun", children[1].Tag)
		assert.Equal(t, "Signature", children[2].Tag)
		assert.Equal(t, NamespaceDSig, children[2].NamespaceURI())
	})

	t.Run("uses the specification's algorithms", func(t *testing.T) {
		si := root.FindElement("./Signature/SignedInfo")
		require.NotNil(t, si)
		assert.Equal(t, "http://www.w3.org/2001/10/xml-exc-c14n#", si.FindElement("./CanonicalizationMethod").SelectAttrValue("Algorithm", ""))
		assert.Equal(t, "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256", si.FindElement("./SignatureMethod").SelectAttrValue("Algorithm", ""))
		ref := si.FindElement("./Reference")
		assert.Equal(t, "#RacunZahtjev", ref.SelectAttrValue("URI", ""))
		assert.Equal(t, "http://www.w3.org/2001/04/xmlenc#sha256", ref.FindElement("./DigestMethod").SelectAttrValue("Algorithm", ""))
		var transforms []string
		for _, tr := range ref.FindElements("./Transforms/Transform") {
			transforms = append(transforms, tr.SelectAttrValue("Algorithm", ""))
		}
		assert.Equal(t, []string{
			"http://www.w3.org/2000/09/xmldsig#enveloped-signature",
			"http://www.w3.org/2001/10/xml-exc-c14n#",
		}, transforms)
		assert.NotNil(t, root.FindElement("./Signature/KeyInfo/X509Data/X509Certificate"))
	})

	t.Run("verifies with the certificate", func(t *testing.T) {
		_, err := validator(cert).Validate(root.Copy())
		assert.NoError(t, err)
	})

	t.Run("fails to verify after a change", func(t *testing.T) {
		changed := root.Copy()
		changed.FindElement("./Racun/IznosUkupno").SetText("1.00")
		_, err := validator(cert).Validate(changed)
		assert.Error(t, err)
	})

	t.Run("needs a certificate and an Id", func(t *testing.T) {
		_, err := Sign(nil, testRequest())
		assert.EqualError(t, err, "no certificate")
		req := testRequest()
		req.ID = ""
		_, err = Sign(cert, req)
		assert.EqualError(t, err, "the request has no Id to reference")
	})

	t.Run("needs a header with a message ID and a time", func(t *testing.T) {
		const msg = "the request needs a header with a message ID and the time it is sent"
		for name, change := range map[string]func(*InvoiceRequest){
			"no header":     func(r *InvoiceRequest) { r.Header = nil },
			"no message ID": func(r *InvoiceRequest) { r.Header.MessageID = "" },
			"no time":       func(r *InvoiceRequest) { r.Header.SentAt = "" },
		} {
			t.Run(name, func(t *testing.T) {
				req := testRequest()
				change(req)
				_, err := Sign(cert, req)
				assert.EqualError(t, err, msg)
			})
		}
	})
}

func validator(cert *Certificate) *dsig.ValidationContext {
	ctx := dsig.NewDefaultValidationContext(&dsig.MemoryX509CertificateStore{Roots: []*x509.Certificate{cert.Cert}})
	ctx.IdAttribute = "Id"
	return ctx
}

// TestSignedRequestAgainstSchema checks a signed request with xmllint
// against FiskalizacijaSchema.xsd. It needs Docker and runs with -xsd only.
func TestSignedRequestAgainstSchema(t *testing.T) {
	if !*validateXSD {
		t.Skip("run with -xsd to validate against the schema (needs Docker)")
	}
	dir := t.TempDir()
	for _, name := range []string{"FiskalizacijaSchema.xsd", "xmldsig-core-schema.xsd"} {
		data, err := os.ReadFile(filepath.Join("testdata", "schema", name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o644))
	}
	signed, err := Sign(testCertificate(t), fullRequest())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "request.xml"), signed, 0o644))

	script := "apk add -q libxml2-utils && xmllint --noout --nonet --schema /w/FiskalizacijaSchema.xsd /w/request.xml"
	out, err := exec.Command("docker", "run", "--rm", "-v", dir+":/w:ro", "alpine:3.20", "sh", "-c", script).CombinedOutput()
	t.Logf("xmllint:\n%s", out)
	require.NoError(t, err)
}
