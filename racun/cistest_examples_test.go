package racun

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/invopop/gobl/bill"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/cbc"
	"github.com/stretchr/testify/require"
)

// TestCISTest converts each example with a new number and the current time,
// and fiscalizes it on CIS test with the demo certificate. It records
// nothing. It runs with -cistest only.
func TestCISTestExamples(t *testing.T) {
	if !*cisTest {
		t.Skip("run with -cistest to send to CIS test (see README)")
	}
	path, password := os.Getenv("CIS_TEST_CERT"), os.Getenv("CIS_TEST_PASSWORD")
	if path == "" || password == "" {
		t.Fatal("-cistest needs CIS_TEST_CERT (path of the PKCS#12 demo certificate) and CIS_TEST_PASSWORD (its password)")
	}
	cert, err := LoadCertificate(path, password)
	require.NoError(t, err)
	bundle, err := os.ReadFile(finaCABundle)
	require.NoError(t, err)
	client, err := New(&Config{Environment: EnvironmentSandbox, Certificate: cert, CABundle: bundle})
	require.NoError(t, err)

	for i, name := range examples(t) {
		t.Run(name, func(t *testing.T) {
			inv := loadExample(t, name).Extract().(*bill.Invoice)
			now := time.Now().In(location)
			inv.Code = cbc.Code(strconv.FormatInt(now.Unix()%1_000_000*10+int64(i), 10))
			inv.IssueDate = cal.DateOf(now)
			inv.IssueTime = cal.NewTime(now.Hour(), now.Minute(), now.Second())

			r, err := FromInvoice(inv, testOptions(), cert.Key)
			require.NoError(t, err)
			req := &ReceiptRequest{
				ID:      ReceiptRequestID,
				Header:  &Header{MessageID: uuid.NewString(), SentAt: now.Format(DateTimeFormat)},
				Receipt: r,
			}
			res, err := client.Fiscalize(context.Background(), req)
			require.NoError(t, err)
			t.Logf("receipt %s/%s/%s, ZKI %s, JIR %s", r.Number.Sequential, r.Number.Premises, r.Number.Device, r.ZKI, res.JIR)
		})
	}
}
