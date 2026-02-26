package indicator

import (
	"testing"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestATR_InvalidPeriod(t *testing.T) {
	_, err := NewATR(0)
	require.Error(t, err)
	requireCode(t, err, "INVALID_PARAMETER")
}

func TestATR_InsufficientData(t *testing.T) {
	atr, err := NewATR(14)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3)
	_, err = atr.Compute(bars)
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestATR_Meta(t *testing.T) {
	atr, err := NewATR(14)
	require.NoError(t, err)

	meta := atr.Meta()
	assert.Equal(t, "ATR_14", meta.Name)
	assert.Equal(t, "volatility", meta.Category)
	assert.Equal(t, 14, meta.WarmupPeriod)
}

func TestATR_OutputLength(t *testing.T) {
	atr, err := NewATR(14)
	require.NoError(t, err)

	bars := testutil.MakeBars(make([]float64, 20)...)
	vals, err := atr.Compute(bars)
	require.NoError(t, err)
	// len = N - period = 20 - 14 = 6
	assert.Len(t, vals, 6)
}

func TestATR_NonNegative(t *testing.T) {
	atr, err := NewATR(5)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := atr.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		assert.GreaterOrEqual(t, v.Value, 0.0, "ATR must be non-negative")
	}
}

func TestATR_WilderSmoothing(t *testing.T) {
	// Verify Wilder smoothing: ATR decays toward new TRs.
	atr, err := NewATR(3)
	require.NoError(t, err)

	// Create bars where TR is always 1.0 (high-low spread = 1).
	bars := make([]bullarc.OHLCV, 10)
	for i := range bars {
		bars[i] = bullarc.OHLCV{
			Open:  10,
			High:  10.5,
			Low:   9.5,
			Close: 10,
		}
	}

	vals, err := atr.Compute(bars)
	require.NoError(t, err)

	// All TRs = 1.0, so ATR should stabilize at 1.0.
	for _, v := range vals {
		testutil.AssertFloatEqual(t, 1.0, v.Value, 0.0001)
	}
}
