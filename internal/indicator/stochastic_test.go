package indicator

import (
	"testing"

	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStochastic_InvalidParams(t *testing.T) {
	t.Run("zero period", func(t *testing.T) {
		_, err := NewStochastic(0, 3, 3)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
	t.Run("zero smoothK", func(t *testing.T) {
		_, err := NewStochastic(14, 0, 3)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
	t.Run("zero smoothD", func(t *testing.T) {
		_, err := NewStochastic(14, 3, 0)
		require.Error(t, err)
		requireCode(t, err, "INVALID_PARAMETER")
	})
}

func TestStochastic_InsufficientData(t *testing.T) {
	s, err := NewStochastic(14, 3, 3)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3)
	_, err = s.Compute(bars)
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestStochastic_Meta(t *testing.T) {
	s, err := NewStochastic(14, 3, 3)
	require.NoError(t, err)

	meta := s.Meta()
	assert.Equal(t, "Stoch_14_3_3", meta.Name)
	assert.Equal(t, "momentum", meta.Category)
	assert.Equal(t, 17, meta.WarmupPeriod) // 14+3+3-3
}

func TestStochastic_Range(t *testing.T) {
	s, err := NewStochastic(14, 3, 3)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := s.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		assert.GreaterOrEqual(t, v.Value, 0.0)
		assert.LessOrEqual(t, v.Value, 100.0)
		assert.GreaterOrEqual(t, v.Extra["d"], 0.0)
		assert.LessOrEqual(t, v.Extra["d"], 100.0)
	}
}

func TestStochastic_ExtremeValues(t *testing.T) {
	// Closing at top of range should yield high %K.
	s, err := NewStochastic(5, 1, 1)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3, 4, 5)
	vals, err := s.Compute(bars)
	require.NoError(t, err)
	require.NotEmpty(t, vals)

	// Close at 5, low at ~1, high at ~5 — %K near 100.
	assert.Greater(t, vals[0].Value, 50.0)
}

func TestStochastic_OutputLength(t *testing.T) {
	s, err := NewStochastic(14, 3, 3)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := s.Compute(bars)
	require.NoError(t, err)

	// len = 100 - (14+3+3-3) = 100 - 17 = 83
	assert.Len(t, vals, 83)
}
