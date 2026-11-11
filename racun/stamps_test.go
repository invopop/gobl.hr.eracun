package racun

import (
	"testing"

	"github.com/invopop/gobl.hr.eracun/addon/fiskal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testJIR = "33663e03-9fb4-4469-8bbe-14118c9ddc1b"

func TestQR(t *testing.T) {
	inv := &Invoice{IssuedAt: "06.11.2026T13:58:10", Total: "10.65", ZKI: "0123456789abcdef0123456789abcdef"}

	t.Run("with a JIR", func(t *testing.T) {
		qr, err := inv.QR(testJIR)
		require.NoError(t, err)
		assert.Equal(t, "https://porezna.gov.hr/rn?jir="+testJIR+"&datv=20261106_1358&izn=1065", qr)
	})
	t.Run("without a JIR", func(t *testing.T) {
		qr, err := inv.QR("")
		require.NoError(t, err)
		assert.Equal(t, "https://porezna.gov.hr/rn?zki=0123456789abcdef0123456789abcdef&datv=20261106_1358&izn=1065", qr)
	})
	t.Run("amounts in cents", func(t *testing.T) {
		for total, izn := range map[string]string{"1525.99": "152599", "0.50": "50", "-12.00": "-1200", "7": "700"} {
			qr, err := (&Invoice{IssuedAt: inv.IssuedAt, Total: total}).QR(testJIR)
			require.NoError(t, err)
			assert.Contains(t, qr, "&izn="+izn, total)
			assert.NotContains(t, qr, "&izn="+izn+"0", total)
		}
	})
	t.Run("a bad issue time", func(t *testing.T) {
		_, err := (&Invoice{IssuedAt: "2026-11-06", Total: "1.00"}).QR(testJIR)
		assert.ErrorIs(t, err, ErrInvalid)
	})
}

func TestAddStamps(t *testing.T) {
	env := loadExample(t, "cash-prices-include-vat.json")
	r, err := NewInvoice(env, testOptions(), testKey(t))
	require.NoError(t, err)

	require.NoError(t, AddStamps(env, r, ""))
	assert.Equal(t, r.ZKI, env.Head.Stamp(fiskal.StampZKI).Value)
	assert.Nil(t, env.Head.Stamp(fiskal.StampJIR))
	assert.Contains(t, env.Head.Stamp(fiskal.StampQR).Value, "zki="+r.ZKI)

	require.NoError(t, AddStamps(env, r, testJIR))
	assert.Equal(t, testJIR, env.Head.Stamp(fiskal.StampJIR).Value)
	assert.Contains(t, env.Head.Stamp(fiskal.StampQR).Value, "jir="+testJIR)
	assert.Len(t, env.Head.Stamps, 3)
}
