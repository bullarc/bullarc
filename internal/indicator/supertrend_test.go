package indicator

import (
	"testing"

	"github.com/bullarc/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuperTrend_InvalidParams(t *testing.T) {
	t.Run("zero period", func(t *testing.T) {
		_, err := NewSuperTrend(0, 3.0)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
	t.Run("zero multiplier", func(t *testing.T) {
		_, err := NewSuperTrend(7, 0)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
}

func TestSuperTrend_InsufficientData(t *testing.T) {
	st, err := NewSuperTrend(7, 3.0)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3)
	_, err = st.Compute(bars)
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestSuperTrend_Meta(t *testing.T) {
	st, err := NewSuperTrend(7, 3.0)
	require.NoError(t, err)

	meta := st.Meta()
	assert.Equal(t, "SuperTrend_7_3.0", meta.Name)
	assert.Equal(t, "trend", meta.Category)
	assert.Equal(t, 7, meta.WarmupPeriod)
}

func TestSuperTrend_DirectionValues(t *testing.T) {
	st, err := NewSuperTrend(7, 3.0)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := st.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		dir := v.Extra["direction"]
		assert.True(t, dir == 1 || dir == -1, "direction must be 1 or -1, got %f", dir)
	}
}

func TestSuperTrend_OutputLength(t *testing.T) {
	st, err := NewSuperTrend(7, 3.0)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := st.Compute(bars)
	require.NoError(t, err)

	// len = N - period = 100 - 7 = 93
	assert.Len(t, vals, 93)
}

func TestSuperTrend_Positive(t *testing.T) {
	st, err := NewSuperTrend(3, 2.0)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := st.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		assert.Greater(t, v.Value, 0.0, "SuperTrend value should be positive for real prices")
	}
}
