package indicator

import (
	"testing"

	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEMA_InvalidPeriod(t *testing.T) {
	_, err := NewEMA(0)
	require.Error(t, err)
	requireCode(t, err, "INVALID_PARAMETER")
}

func TestEMA_InsufficientData(t *testing.T) {
	ema, err := NewEMA(14)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3)
	_, err = ema.Compute(bars)
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestEMA_Meta(t *testing.T) {
	ema, err := NewEMA(14)
	require.NoError(t, err)

	meta := ema.Meta()
	assert.Equal(t, "EMA_14", meta.Name)
	assert.Equal(t, "trend", meta.Category)
	assert.Equal(t, 13, meta.WarmupPeriod)
}

func TestEMA_SeedsFromSMA(t *testing.T) {
	// The first EMA value must equal the SMA of the first period values.
	ema, err := NewEMA(5)
	require.NoError(t, err)

	bars := testutil.MakeBars(10, 20, 30, 40, 50, 60)
	vals, err := ema.Compute(bars)
	require.NoError(t, err)

	// First EMA = SMA(10,20,30,40,50) = 30
	testutil.AssertFloatEqual(t, 30.0, vals[0].Value, 0.0001)
}

func TestEMA_ReferenceValues(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	ema, err := NewEMA(14)
	require.NoError(t, err)

	vals, err := ema.Compute(bars)
	require.NoError(t, err)

	// values[0] is at row 13 — seeds from SMA, must equal SMA_14 at row 13
	testutil.RequireFloatEqual(t, 156.017857, vals[0].Value, 0.001)
	// values[36] is at row 49
	testutil.RequireFloatEqual(t, 179.229788, vals[36].Value, 0.001)
	// values[86] is at row 99
	testutil.RequireFloatEqual(t, 186.861824, vals[86].Value, 0.001)
}

func TestEMA_ExponentialWeighting(t *testing.T) {
	// EMA should react faster than SMA when price rises sharply.
	ema, err := NewEMA(3)
	require.NoError(t, err)

	bars := testutil.MakeBars(10, 10, 10, 100)
	vals, err := ema.Compute(bars)
	require.NoError(t, err)

	// k = 2/(3+1) = 0.5
	// EMA[0] = SMA(10,10,10) = 10
	// EMA[1] = 100*0.5 + 10*0.5 = 55
	testutil.AssertFloatEqual(t, 10.0, vals[0].Value, 0.0001)
	testutil.AssertFloatEqual(t, 55.0, vals[1].Value, 0.0001)
}
