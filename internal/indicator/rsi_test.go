package indicator

import (
	"testing"

	"github.com/bullarc/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRSI_InvalidPeriod(t *testing.T) {
	_, err := NewRSI(0)
	require.Error(t, err)
	requireCode(t, err, "INVALID_PARAMETER")
}

func TestRSI_InsufficientData(t *testing.T) {
	rsi, err := NewRSI(14)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3)
	_, err = rsi.Compute(bars)
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestRSI_Meta(t *testing.T) {
	rsi, err := NewRSI(14)
	require.NoError(t, err)

	meta := rsi.Meta()
	assert.Equal(t, "RSI_14", meta.Name)
	assert.Equal(t, "momentum", meta.Category)
	assert.Equal(t, 14, meta.WarmupPeriod)
}

func TestRSI_Range(t *testing.T) {
	rsi, err := NewRSI(14)
	require.NoError(t, err)

	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := rsi.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		assert.GreaterOrEqual(t, v.Value, 0.0)
		assert.LessOrEqual(t, v.Value, 100.0)
	}
}

func TestRSI_AllGains(t *testing.T) {
	// Constantly rising prices → RSI = 100.
	rsi, err := NewRSI(3)
	require.NoError(t, err)

	bars := testutil.MakeBars(1, 2, 3, 4, 5, 6, 7)
	vals, err := rsi.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		testutil.AssertFloatEqual(t, 100.0, v.Value, 0.0001)
	}
}

func TestRSI_AllLosses(t *testing.T) {
	// Constantly falling prices → RSI = 0.
	rsi, err := NewRSI(3)
	require.NoError(t, err)

	bars := testutil.MakeBars(7, 6, 5, 4, 3, 2, 1)
	vals, err := rsi.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		testutil.AssertFloatEqual(t, 0.0, v.Value, 0.0001)
	}
}

func TestRSI_ReferenceValues(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	rsi, err := NewRSI(14)
	require.NoError(t, err)

	vals, err := rsi.Compute(bars)
	require.NoError(t, err)

	// values[0] is at row 14
	testutil.RequireFloatEqual(t, 76.302798, vals[0].Value, 0.001)
	// values[35] is at row 49
	testutil.RequireFloatEqual(t, 60.153744, vals[35].Value, 0.001)
	// values[85] is at row 99
	testutil.RequireFloatEqual(t, 52.042801, vals[85].Value, 0.001)
}
