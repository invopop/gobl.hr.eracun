package racun

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequest(t *testing.T) {
	inv := testRequest().Invoice
	before := time.Now().Truncate(time.Second)
	req := NewRequest(inv)
	after := time.Now()

	assert.Equal(t, InvoiceRequestID, req.ID)
	assert.Same(t, inv, req.Invoice)
	id, err := uuid.Parse(req.Header.MessageID)
	require.NoError(t, err)
	assert.Equal(t, uuid.Version(4), id.Version())
	assert.NotEqual(t, req.Header.MessageID, NewRequest(inv).Header.MessageID, "every request gets a new message ID")

	// The send time is the Croatian clock's, whatever the host's zone.
	sentAt, err := time.ParseInLocation(DateTimeFormat, req.Header.SentAt, zagreb)
	require.NoError(t, err)
	assert.False(t, sentAt.Before(before) || sentAt.After(after), "sent at %s, not between %s and %s", sentAt, before, after)

	_, err = Sign(testCertificate(t), req)
	assert.NoError(t, err, "Sign takes the request as it is")
}

func TestInvoiceRequestXML(t *testing.T) {
	t.Run("writes the elements in schema order", func(t *testing.T) {
		out, err := xml.Marshal(testRequest())
		require.NoError(t, err)
		assert.Equal(t, `<RacunZahtjev xmlns="http://www.apis-it.hr/fin/2012/types/f73" Id="RacunZahtjev">`+
			`<Zaglavlje><IdPoruke>24d544c5-859a-47e2-85dc-0e1ce881b819</IdPoruke><DatumVrijeme>06.11.2026T13:58:10</DatumVrijeme></Zaglavlje>`+
			`<Racun><Oib>80432461483</Oib><USustPdv>true</USustPdv><DatVrijeme>06.11.2026T13:58:10</DatVrijeme><OznSlijed>P</OznSlijed>`+
			`<BrRac><BrOznRac>1</BrOznRac><OznPosPr>POS1</OznPosPr><OznNapUr>1</OznNapUr></BrRac>`+
			`<Pdv><Porez><Stopa>25.00</Stopa><Osnovica>10.00</Osnovica><Iznos>2.50</Iznos></Porez></Pdv>`+
			`<IznosUkupno>12.50</IznosUkupno><NacinPlac>G</NacinPlac><OibOper>80432461483</OibOper>`+
			`<ZastKod>8b55b3586e66d00b33266e44a774a408</ZastKod><NakDost>false</NakDost></Racun>`+
			`</RacunZahtjev>`, string(out))
	})

	t.Run("writes every optional element in schema order", func(t *testing.T) {
		req := fullRequest()

		out, err := xml.Marshal(req)
		require.NoError(t, err)
		assert.Contains(t, string(out), `<IznosUkupno>12.50</IznosUkupno>`)
		assertInOrder(t, string(out),
			"<Pdv>", "<Pnp>", "<OstaliPor><Porez><Naziv>", "<IznosOslobPdv>", "<IznosMarza>", "<IznosNePodlOpor>",
			"<Naknade><Naknada><NazivN>", "<IznosUkupno>", "<NacinPlac>", "<OibOper>", "<ZastKod>",
			"<NakDost>true", "<ParagonBrRac>", "<SpecNamj>", "<OibPrimateljaRacuna>",
		)

		back := new(InvoiceRequest)
		require.NoError(t, xml.Unmarshal(out, back))
		assert.Equal(t, req, back)
	})
}

func TestInvoiceResponseXML(t *testing.T) {
	t.Run("reads a JIR", func(t *testing.T) {
		in := `<tns:RacunOdgovor Id="G0x7f452a31e100-4D" xmlns:tns="http://www.apis-it.hr/fin/2012/types/f73">` +
			`<tns:Zaglavlje><tns:IdPoruke>24d544c5-859a-47e2-85dc-0e1ce881b819</tns:IdPoruke><tns:DatumVrijeme>06.11.2026T12:58:10</tns:DatumVrijeme></tns:Zaglavlje>` +
			`<tns:Jir>33663e03-9fb4-4469-8bbe-14118c9ddc1b</tns:Jir></tns:RacunOdgovor>`
		res := new(InvoiceResponse)
		require.NoError(t, xml.Unmarshal([]byte(in), res))
		assert.Equal(t, "33663e03-9fb4-4469-8bbe-14118c9ddc1b", res.JIR)
		assert.Equal(t, "24d544c5-859a-47e2-85dc-0e1ce881b819", res.Header.MessageID)
		assert.Empty(t, res.Errors)
	})

	t.Run("reads errors", func(t *testing.T) {
		in := `<tns:RacunOdgovor xmlns:tns="http://www.apis-it.hr/fin/2012/types/f73">` +
			`<tns:Zaglavlje><tns:IdPoruke>24d544c5-859a-47e2-85dc-0e1ce881b819</tns:IdPoruke><tns:DatumVrijeme>06.11.2026T12:58:10</tns:DatumVrijeme></tns:Zaglavlje>` +
			`<tns:Greske><tns:Greska><tns:SifraGreske>s005</tns:SifraGreske><tns:PorukaGreske>OIB iz poruke zahtjeva nije jednak OIB-u iz certifikata.</tns:PorukaGreske></tns:Greska></tns:Greske>` +
			`</tns:RacunOdgovor>`
		res := new(InvoiceResponse)
		require.NoError(t, xml.Unmarshal([]byte(in), res))
		assert.Empty(t, res.JIR)
		require.Len(t, res.Errors, 1)
		assert.Equal(t, "s005", res.Errors[0].Code)
	})
}

// assertInOrder checks that each part appears in s after the one before.
func assertInOrder(t *testing.T, s string, parts ...string) {
	t.Helper()
	at := 0
	for _, p := range parts {
		i := strings.Index(s[at:], p)
		if !assert.GreaterOrEqual(t, i, 0, "%q missing or out of order", p) {
			return
		}
		at += i + len(p)
	}
}
