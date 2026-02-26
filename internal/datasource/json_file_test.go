package datasource

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONSource_Meta(t *testing.T) {
	src := NewJSONSource("any.json")
	meta := src.Meta()
	assert.Equal(t, "json", meta.Name)
	assert.NotEmpty(t, meta.Description)
}

func TestJSONSource_Fetch_AllBars(t *testing.T) {
	path := filepath.Join(testutil.TestdataDir(), "ohlcv_100.json")
	src := NewJSONSource(path)

	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Len(t, bars, 100)

	// Verify first bar
	assert.Equal(t, time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), bars[0].Time)
	testutil.AssertFloatEqual(t, 149.95, bars[0].Open, 0.001)
	testutil.AssertFloatEqual(t, 150.48, bars[0].High, 0.001)
	testutil.AssertFloatEqual(t, 149.73, bars[0].Low, 0.001)
	testutil.AssertFloatEqual(t, 149.83, bars[0].Close, 0.001)
	testutil.AssertFloatEqual(t, 2753969, bars[0].Volume, 1)
}

func TestJSONSource_Fetch_TimeFilterRange(t *testing.T) {
	path := filepath.Join(testutil.TestdataDir(), "ohlcv_100.json")
	src := NewJSONSource(path)

	start := time.Date(2024, 7, 3, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 7, 5, 0, 0, 0, 0, time.UTC)
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{
		Symbol: "AAPL",
		Start:  start,
		End:    end,
	})
	require.NoError(t, err)
	assert.Greater(t, len(bars), 0)
	for _, b := range bars {
		assert.False(t, b.Time.Before(start))
		assert.False(t, b.Time.After(end))
	}
}

func TestJSONSource_Fetch_FileNotFound(t *testing.T) {
	src := NewJSONSource("/nonexistent/path/file.json")

	_, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestJSONSource_Fetch_ContextCancelled(t *testing.T) {
	path := filepath.Join(testutil.TestdataDir(), "ohlcv_100.json")
	src := NewJSONSource(path)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Fetch(ctx, bullarc.DataQuery{Symbol: "AAPL"})
	require.Error(t, err)
	requireCode(t, err, "TIMEOUT")
}
