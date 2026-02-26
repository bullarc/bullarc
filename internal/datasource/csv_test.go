package datasource

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSVSource_Meta(t *testing.T) {
	src := NewCSVSource("any.csv")
	meta := src.Meta()
	assert.Equal(t, "csv", meta.Name)
	assert.NotEmpty(t, meta.Description)
}

func TestCSVSource_Fetch_AllBars(t *testing.T) {
	path := filepath.Join(testutil.TestdataDir(), "ohlcv_10.csv")
	src := NewCSVSource(path)

	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Len(t, bars, 10)

	// Verify first bar matches the CSV
	assert.Equal(t, time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), bars[0].Time)
	testutil.AssertFloatEqual(t, 149.95, bars[0].Open, 0.001)
	testutil.AssertFloatEqual(t, 150.48, bars[0].High, 0.001)
	testutil.AssertFloatEqual(t, 149.73, bars[0].Low, 0.001)
	testutil.AssertFloatEqual(t, 149.83, bars[0].Close, 0.001)
	testutil.AssertFloatEqual(t, 2753969, bars[0].Volume, 1)
}

func TestCSVSource_Fetch_TimeFilterStart(t *testing.T) {
	path := filepath.Join(testutil.TestdataDir(), "ohlcv_10.csv")
	src := NewCSVSource(path)

	start := time.Date(2024, 7, 5, 0, 0, 0, 0, time.UTC)
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{
		Symbol: "AAPL",
		Start:  start,
	})
	require.NoError(t, err)
	assert.Greater(t, len(bars), 0)
	for _, b := range bars {
		assert.False(t, b.Time.Before(start), "bar %v is before start %v", b.Time, start)
	}
}

func TestCSVSource_Fetch_TimeFilterRange(t *testing.T) {
	path := filepath.Join(testutil.TestdataDir(), "ohlcv_10.csv")
	src := NewCSVSource(path)

	start := time.Date(2024, 7, 3, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 7, 5, 0, 0, 0, 0, time.UTC)
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{
		Symbol: "AAPL",
		Start:  start,
		End:    end,
	})
	require.NoError(t, err)
	assert.Len(t, bars, 3)
	for _, b := range bars {
		assert.False(t, b.Time.Before(start))
		assert.False(t, b.Time.After(end))
	}
}

func TestCSVSource_Fetch_FileNotFound(t *testing.T) {
	src := NewCSVSource("/nonexistent/path/file.csv")

	_, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestCSVSource_Fetch_ContextCancelled(t *testing.T) {
	path := filepath.Join(testutil.TestdataDir(), "ohlcv_100.csv")
	src := NewCSVSource(path)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := src.Fetch(ctx, bullarc.DataQuery{Symbol: "AAPL"})
	require.Error(t, err)
	requireCode(t, err, "TIMEOUT")
}
