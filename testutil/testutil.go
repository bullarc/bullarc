// Package testutil provides shared test helpers for the bullarc project.
package testutil

import (
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/bullarcdev/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestdataDir returns the absolute path to the testdata directory.
func TestdataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "testdata")
}

// LoadBarsFromCSV loads OHLCV bars from a CSV file in the testdata directory.
// The CSV must have columns: date,open,high,low,close,volume.
func LoadBarsFromCSV(t *testing.T, filename string) []bullarc.OHLCV {
	t.Helper()

	path := filepath.Join(TestdataDir(), filename)
	f, err := os.Open(path)
	require.NoError(t, err, "opening CSV file: %s", path)
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	require.NoError(t, err, "reading CSV file: %s", path)
	require.Greater(t, len(records), 1, "CSV must have header + data rows")

	var bars []bullarc.OHLCV
	for i, row := range records[1:] { // skip header
		require.Len(t, row, 6, "row %d must have 6 columns", i+1)

		ts, err := time.Parse("2006-01-02", row[0])
		require.NoError(t, err, "parsing date at row %d", i+1)

		open, err := strconv.ParseFloat(row[1], 64)
		require.NoError(t, err, "parsing open at row %d", i+1)

		high, err := strconv.ParseFloat(row[2], 64)
		require.NoError(t, err, "parsing high at row %d", i+1)

		low, err := strconv.ParseFloat(row[3], 64)
		require.NoError(t, err, "parsing low at row %d", i+1)

		close_, err := strconv.ParseFloat(row[4], 64)
		require.NoError(t, err, "parsing close at row %d", i+1)

		volume, err := strconv.ParseFloat(row[5], 64)
		require.NoError(t, err, "parsing volume at row %d", i+1)

		bars = append(bars, bullarc.OHLCV{
			Time:   ts,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close_,
			Volume: volume,
		})
	}

	return bars
}

// AssertFloatEqual asserts two floats are equal within a tolerance.
func AssertFloatEqual(t *testing.T, expected, actual, tolerance float64) {
	t.Helper()
	assert.InDelta(t, expected, actual, tolerance,
		"expected %.6f, got %.6f (tolerance %.6f)", expected, actual, tolerance)
}

// RequireFloatEqual requires two floats are equal within a tolerance.
func RequireFloatEqual(t *testing.T, expected, actual, tolerance float64) {
	t.Helper()
	require.InDelta(t, expected, actual, tolerance,
		"expected %.6f, got %.6f (tolerance %.6f)", expected, actual, tolerance)
}

// MakeBars creates synthetic OHLCV bars from close prices.
// Open, High, Low are derived from Close with small offsets.
// Volume is set to 1000 for all bars.
func MakeBars(closes ...float64) []bullarc.OHLCV {
	bars := make([]bullarc.OHLCV, len(closes))
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	for i, c := range closes {
		spread := math.Max(c*0.01, 0.01)
		bars[i] = bullarc.OHLCV{
			Time:   base.AddDate(0, 0, i),
			Open:   c - spread*0.3,
			High:   c + spread,
			Low:    c - spread,
			Close:  c,
			Volume: 1000,
		}
	}

	return bars
}
