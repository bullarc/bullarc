package indicator

import (
	"testing"

	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSMA_InvalidPeriod(t *testing.T) {
	_, err := NewSMA(0)
	require.Error(t, err)
	requireCode(t, err, "INVALID_PARAMETER")
}

func TestSMA_InsufficientData(t *testing.T) {
	sma, err := NewSMA(14)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3)
	_, err = sma.Compute(bars)
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestSMA_Meta(t *testing.T) {
	sma, err := NewSMA(14)
	require.NoError(t, err)

	meta := sma.Meta()
	assert.Equal(t, "SMA_14", meta.Name)
	assert.Equal(t, "trend", meta.Category)
	assert.Equal(t, 13, meta.WarmupPeriod)
}

func TestSMA_OutputLength(t *testing.T) {
	sma, err := NewSMA(14)
	require.NoError(t, err)

	bars := testutil.MakeBars(make([]float64, 20)...)
	vals, err := sma.Compute(bars)
	require.NoError(t, err)
	assert.Len(t, vals, 7) // 20 - 14 + 1
}

func TestSMA_ReferenceValues(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	t.Run("SMA_14", func(t *testing.T) {
		sma, err := NewSMA(14)
		require.NoError(t, err)

		vals, err := sma.Compute(bars)
		require.NoError(t, err)

		// values[0] is at row 13 (0-indexed)
		testutil.RequireFloatEqual(t, 156.017857, vals[0].Value, 0.001)
		// values[36] is at row 49
		testutil.RequireFloatEqual(t, 178.466429, vals[36].Value, 0.001)
		// values[86] is at row 99
		testutil.RequireFloatEqual(t, 185.887143, vals[86].Value, 0.001)
	})

	t.Run("SMA_50", func(t *testing.T) {
		sma, err := NewSMA(50)
		require.NoError(t, err)

		vals, err := sma.Compute(bars)
		require.NoError(t, err)

		// values[0] is at row 49
		testutil.RequireFloatEqual(t, 169.3158, vals[0].Value, 0.001)
		// values[50] is at row 99
		testutil.RequireFloatEqual(t, 186.1164, vals[50].Value, 0.001)
	})
}

func TestSMA_SingleBar(t *testing.T) {
	sma, err := NewSMA(1)
	require.NoError(t, err)

	bars := testutil.MakeBars(42.5)
	vals, err := sma.Compute(bars)
	require.NoError(t, err)
	require.Len(t, vals, 1)
	testutil.AssertFloatEqual(t, 42.5, vals[0].Value, 0.0001)
}
