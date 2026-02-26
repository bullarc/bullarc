package indicator

import (
	"testing"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOBV_EmptyBars(t *testing.T) {
	o := NewOBV()
	_, err := o.Compute([]bullarc.OHLCV{})
	require.Error(t, err)
	requireCode(t, err, "INSUFFICIENT_DATA")
}

func TestOBV_Meta(t *testing.T) {
	o := NewOBV()
	meta := o.Meta()
	assert.Equal(t, "OBV", meta.Name)
	assert.Equal(t, "volume", meta.Category)
	assert.Equal(t, 0, meta.WarmupPeriod)
}

func TestOBV_OutputLength(t *testing.T) {
	o := NewOBV()
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	vals, err := o.Compute(bars)
	require.NoError(t, err)
	assert.Len(t, vals, 100)
}

func TestOBV_InitialValueZero(t *testing.T) {
	o := NewOBV()
	bars := testutil.MakeBars(10, 11, 12)
	vals, err := o.Compute(bars)
	require.NoError(t, err)

	// First OBV is always 0.
	testutil.AssertFloatEqual(t, 0.0, vals[0].Value, 0.0001)
}

func TestOBV_RisingPrices(t *testing.T) {
	o := NewOBV()
	bars := testutil.MakeBars(10, 11, 12, 13)
	vals, err := o.Compute(bars)
	require.NoError(t, err)

	// Each up close adds volume (1000 each from MakeBars).
	assert.Equal(t, 0.0, vals[0].Value)
	assert.Equal(t, 1000.0, vals[1].Value)
	assert.Equal(t, 2000.0, vals[2].Value)
	assert.Equal(t, 3000.0, vals[3].Value)
}

func TestOBV_FallingPrices(t *testing.T) {
	o := NewOBV()
	bars := testutil.MakeBars(13, 12, 11, 10)
	vals, err := o.Compute(bars)
	require.NoError(t, err)

	assert.Equal(t, 0.0, vals[0].Value)
	assert.Equal(t, -1000.0, vals[1].Value)
	assert.Equal(t, -2000.0, vals[2].Value)
	assert.Equal(t, -3000.0, vals[3].Value)
}

func TestOBV_FlatPrices(t *testing.T) {
	o := NewOBV()
	bars := testutil.MakeBars(10, 10, 10)
	vals, err := o.Compute(bars)
	require.NoError(t, err)

	for _, v := range vals {
		testutil.AssertFloatEqual(t, 0.0, v.Value, 0.0001)
	}
}
