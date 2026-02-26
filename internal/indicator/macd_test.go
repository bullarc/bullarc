package indicator

import (
	"testing"

	"github.com/bullarc/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMACD_InvalidParams(t *testing.T) {
	t.Run("zero fast", func(t *testing.T) {
		_, err := NewMACD(0, 26, 9)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
	t.Run("slow not greater than fast", func(t *testing.T) {
		_, err := NewMACD(12, 12, 9)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
	t.Run("zero signal", func(t *testing.T) {
		_, err := NewMACD(12, 26, 0)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
}

func TestMACD_InsufficientData(t *testing.T) {
	macd, err := NewMACD(12, 26, 9)
	require.NoError(t, err)

	bars := testutil.MakeBars(make([]float64, 10)...)
	_, err = macd.Compute(bars)
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestMACD_Meta(t *testing.T) {
	macd, err := NewMACD(12, 26, 9)
	require.NoError(t, err)

	meta := macd.Meta()
	assert.Equal(t, "MACD_12_26_9", meta.Name)
	assert.Equal(t, "momentum", meta.Category)
	assert.Equal(t, 33, meta.WarmupPeriod) // 26 + 9 - 2
}

func TestMACD_OutputLength(t *testing.T) {
	macd, err := NewMACD(12, 26, 9)
	require.NoError(t, err)

	bars := testutil.MakeBars(make([]float64, 100)...)
	vals, err := macd.Compute(bars)
	require.NoError(t, err)
	// len = 100 - (26 + 9 - 2) = 100 - 33 = 67
	assert.Len(t, vals, 67)
}

func TestMACD_ExtraFields(t *testing.T) {
	macd, err := NewMACD(12, 26, 9)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := macd.Compute(bars)
	require.NoError(t, err)
	require.NotEmpty(t, vals)

	for _, v := range vals {
		_, hasSignal := v.Extra["signal"]
		_, hasHist := v.Extra["histogram"]
		assert.True(t, hasSignal, "missing signal in extra")
		assert.True(t, hasHist, "missing histogram in extra")
		testutil.AssertFloatEqual(t, v.Value-v.Extra["signal"], v.Extra["histogram"], 0.0001)
	}
}

func TestMACD_ReferenceValues(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	macd, err := NewMACD(12, 26, 9)
	require.NoError(t, err)

	vals, err := macd.Compute(bars)
	require.NoError(t, err)

	// values[0] is at row 33
	testutil.RequireFloatEqual(t, 6.841839, vals[0].Value, 0.001)
	testutil.RequireFloatEqual(t, 6.841882, vals[0].Extra["signal"], 0.001)
	testutil.RequireFloatEqual(t, -0.000042, vals[0].Extra["histogram"], 0.0001)

	// values[66] is at row 99
	testutil.RequireFloatEqual(t, 1.646379, vals[66].Value, 0.001)
	testutil.RequireFloatEqual(t, 0.982558, vals[66].Extra["signal"], 0.001)
	testutil.RequireFloatEqual(t, 0.663821, vals[66].Extra["histogram"], 0.001)
}
