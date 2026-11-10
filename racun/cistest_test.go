package racun

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var (
	cisTest = flag.Bool("cistest", false, "send receipts to CIS test and check the answers; needs "+
		envCertificate+" and "+envPassword+" (see README)")
	record = flag.Bool("record", false, "with -cistest, save CIS's answers as the fixtures in "+cisTestDir)
)

// The environment the -cistest call reads its certificate from. Neither the
// certificate nor its password is ever in the repository.
const (
	// envCertificate is the path of the PKCS#12 demo certificate.
	envCertificate = "CIS_TEST_CERT"
	// envPassword is the certificate's password.
	envPassword = "CIS_TEST_PASSWORD"
)

// invopopOIB is Invopop's own OIB, the one the demo certificate carries.
const invopopOIB = "80432461483"

// Where the -cistest call finds the FINA CA bundle, and where -record saves
// CIS's answers. The offline tests read those answers: they are the only
// answers signed by CIS that the tests have.
const (
	finaCABundle = "testdata/fina-ca.pem"
	cisTestDir   = "testdata/cistest"
)

// cisTestScenarios are the receipts -cistest sends. CIS test does not check
// that the premises and device exist.
var cisTestScenarios = []struct {
	name string
	// operator is the operator's OIB. A wrong check digit gets s013.
	operator string
	accepted bool
}{
	{"accepted", invopopOIB, true},
	{"refused", "12345678901", false},
}

// TestCISTest sends one receipt per scenario to CIS test and checks the
// answer. It runs with -cistest only. With -record, it also saves each answer
// in testdata/cistest and logs the values client_test.go expects of them.
func TestCISTest(t *testing.T) {
	if !*cisTest {
		t.Skip("run with -cistest to send to CIS test (see README)")
	}
	path, password := os.Getenv(envCertificate), os.Getenv(envPassword)
	if path == "" || password == "" {
		t.Fatalf("-cistest needs %s (path of the PKCS#12 demo certificate) and %s (its password)", envCertificate, envPassword)
	}
	cert, err := LoadCertificate(path, password)
	require.NoError(t, err)
	bundle, err := os.ReadFile(finaCABundle)
	require.NoError(t, err)
	client, err := New(&Config{Environment: EnvironmentSandbox, Certificate: cert, CABundle: bundle})
	require.NoError(t, err)

	for i, s := range cisTestScenarios {
		t.Run(s.name, func(t *testing.T) {
			// A new number every run, so a run never repeats a receipt.
			number := strconv.FormatInt(time.Now().Unix()%1_000_000*10+int64(i), 10)
			req, err := cisTestReceipt(cert, number, s.operator)
			require.NoError(t, err)
			envelope, err := client.SignReceipt(req)
			require.NoError(t, err)

			sentAt := time.Now().UTC()
			status, data, err := client.post(context.Background(), soapActionReceipt, envelope)
			require.NoError(t, err)
			res, err := client.readReceiptAnswer(status, data)
			t.Logf("receipt %s, message %s, HTTP %d", number, req.Header.MessageID, status)
			if *record {
				require.NoError(t, saveCISTest(s.name+"-answer.xml", data))
			}
			switch {
			case err == nil && s.accepted:
				t.Logf("accepted: JIR %s", res.JIR)
				if *record {
					t.Logf("update client_test.go:\n\tacceptedJIR       = %q\n\tacceptedMessageID = %q\n\trecordedAt = time.Date(%d, %d, %d, %d, %d, 0, 0, time.UTC)",
						res.JIR, res.Header.MessageID, sentAt.Year(), sentAt.Month(), sentAt.Day(), sentAt.Hour(), sentAt.Minute())
				}
			case errors.Is(err, ErrCIS) && !s.accepted:
				t.Logf("refused as expected: %v", err)
			default:
				t.Errorf("unexpected outcome: %v", err)
			}
		})
	}
}

// cisTestReceipt is a €12.50 card receipt with 25 % VAT, issued now by
// Invopop at premises POS1, device 1.
func cisTestReceipt(cert *Certificate, number, operator string) (*ReceiptRequest, error) {
	loc, err := time.LoadLocation("Europe/Zagreb")
	if err != nil {
		return nil, err
	}
	now := time.Now().In(loc)
	num := &Number{Sequential: number, Premises: testPremises, Device: "1"}
	zki, err := ZKI(cert.Key, &ZKIInput{IssuerOIB: invopopOIB, IssuedAt: now, Number: num, Total: testTotal})
	if err != nil {
		return nil, err
	}
	return &ReceiptRequest{
		ID:     ReceiptRequestID,
		Header: &Header{MessageID: uuid.NewString(), SentAt: now.Format(DateTimeFormat)},
		Receipt: &Receipt{
			IssuerOIB:     invopopOIB,
			InVATSystem:   true,
			IssuedAt:      now.Format(DateTimeFormat),
			SequenceScope: SequencePremises,
			Number:        num,
			VAT:           []*Tax{{Rate: "25.00", Base: "10.00", Amount: "2.50"}},
			Total:         testTotal,
			PaymentMethod: PaymentCard,
			OperatorOIB:   operator,
			ZKI:           zki,
		},
	}, nil
}

func saveCISTest(name string, data []byte) error {
	if err := os.MkdirAll(cisTestDir, 0o755); err != nil { //nolint:gosec
		return err
	}
	path := filepath.Join(cisTestDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("saving %s: %w", path, err)
	}
	return nil
}
