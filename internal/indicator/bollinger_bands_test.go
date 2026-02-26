package indicator

import (
	"testing"

	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBollingerBands_InvalidParams(t *testing.T) {
	t.Run("period too small", func(t *testing.T) {
		_, err := NewBollingerBands(1, 2.0)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
	t.Run("non-positive multiplier", func(t *testing.T) {
		_, err := NewBollingerBands(20, 0)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
}

func TestBollingerBands_InsufficientData(t *testing.T) {
	bb, err := NewBollingerBands(20, 2.0)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3)
	_, err = bb.Compute(bars)
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestBollingerBands_Meta(t *testing.T) {
	bb, err := NewBollingerBands(20, 2.0)
	require.NoError(t, err)

	meta := bb.Meta()
	assert.Equal(t, "BB_20_2.0", meta.Name)
	assert.Equal(t, "volatility", meta.Category)
	assert.Equal(t, 19, meta.WarmupPeriod)
}

func TestBollingerBands_BandOrdering(t *testing.T) {
	bb, err := NewBollingerBands(20, 2.0)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := bb.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		upper := v.Extra["upper"]
		lower := v.Extra["lower"]
		assert.GreaterOrEqual(t, upper, v.Value, "upper >= middle")
		assert.LessOrEqual(t, lower, v.Value, "lower <= middle")
		assert.GreaterOrEqual(t, upper, lower, "upper >= lower")
	}
}

func TestBollingerBands_ConstantPrices(t *testing.T) {
	// Constant prices → zero std dev → upper = lower = middle.
	bb, err := NewBollingerBands(5, 2.0)
	require.NoError(t, err)

	bars := testutil.MakeBars(100, 100, 100, 100, 100)
	vals, err := bb.Compute(bars)
	require.NoError(t, err)
	require.Len(t, vals, 1)

	testutil.AssertFloatEqual(t, 100.0, vals[0].Value, 0.0001)
	testutil.AssertFloatEqual(t, 100.0, vals[0].Extra["upper"], 0.0001)
	testutil.AssertFloatEqual(t, 100.0, vals[0].Extra["lower"], 0.0001)
}

func TestBollingerBands_ExtraFields(t *testing.T) {
	bb, err := NewBollingerBands(20, 2.0)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := bb.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		_, hasUpper := v.Extra["upper"]
		_, hasLower := v.Extra["lower"]
		_, hasBW := v.Extra["bandwidth"]
		_, hasPB := v.Extra["percent_b"]
		assert.True(t, hasUpper)
		assert.True(t, hasLower)
		assert.True(t, hasBW)
		assert.True(t, hasPB)
	}
}
