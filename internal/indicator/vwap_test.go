package indicator

import (
	"testing"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVWAP_EmptyBars(t *testing.T) {
	v := NewVWAP()
	_, err := v.Compute([]bullarc.OHLCV{})
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestVWAP_Meta(t *testing.T) {
	v := NewVWAP()
	meta := v.Meta()
	assert.Equal(t, "VWAP", meta.Name)
	assert.Equal(t, "volume", meta.Category)
	assert.Equal(t, 0, meta.WarmupPeriod)
}

func TestVWAP_OutputLength(t *testing.T) {
	v := NewVWAP()
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := v.Compute(bars)
	require.NoError(t, err)
	assert.Len(t, vals, 100)
}

func TestVWAP_SingleBar(t *testing.T) {
	v := NewVWAP()
	bars := []bullarc.OHLCV{
		{High: 12, Low: 8, Close: 10, Volume: 100},
	}
	vals, err := v.Compute(bars)
	require.NoError(t, err)
	require.Len(t, vals, 1)

	// typical_price = (12 + 8 + 10) / 3 = 10
	testutil.AssertFloatEqual(t, 10.0, vals[0].Value, 0.0001)
}

func TestVWAP_Cumulative(t *testing.T) {
	v := NewVWAP()

	// Two equal bars: VWAP should be stable.
	bars := []bullarc.OHLCV{
		{High: 12, Low: 8, Close: 10, Volume: 100},
		{High: 12, Low: 8, Close: 10, Volume: 100},
	}
	vals, err := v.Compute(bars)
	require.NoError(t, err)
	require.Len(t, vals, 2)
	testutil.AssertFloatEqual(t, 10.0, vals[0].Value, 0.0001)
	testutil.AssertFloatEqual(t, 10.0, vals[1].Value, 0.0001)
}

func TestVWAP_Positive(t *testing.T) {
	v := NewVWAP()
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := v.Compute(bars)
	require.NoError(t, err)

	for _, val := range vals {
		assert.Greater(t, val.Value, 0.0)
	}
}
