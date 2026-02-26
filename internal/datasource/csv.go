package datasource

import (
	"context"
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/bullarcdev/bullarc"
)

// CSVSource reads OHLCV bars from a local CSV file.
// The file must have a header row followed by columns: date,open,high,low,close,volume.
// Dates must be in "2006-01-02" format.
type CSVSource struct {
	path string
}

// NewCSVSource creates a CSVSource that reads from path.
func NewCSVSource(path string) *CSVSource {
	return &CSVSource{path: path}
}

// Meta returns metadata for the CSV data source.
func (s *CSVSource) Meta() bullarc.DataSourceMeta {
	return bullarc.DataSourceMeta{
		Name:        "csv",
		Description: "Reads OHLCV bars from a local CSV file",
	}
}

// Fetch loads bars from the CSV file, optionally filtering by the query time range.
// Symbol filtering is not applied; CSV files are assumed to be single-symbol by convention.
func (s *CSVSource) Fetch(ctx context.Context, query bullarc.DataQuery) ([]bullarc.OHLCV, error) {
	slog.Info("fetching bars from csv", "path", s.path, "symbol", query.Symbol)

	f, err := os.Open(s.path)
	if err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("open %s: %w", s.path, err))
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("parse csv %s: %w", s.path, err))
	}

	if len(records) < 2 {
		return nil, bullarc.ErrInsufficientData.Wrap(fmt.Errorf("csv %s has no data rows", s.path))
	}

	rows := records[1:] // skip header
	bars := make([]bullarc.OHLCV, 0, len(rows))

	for i, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, bullarc.ErrTimeout.Wrap(err)
		}

		if len(row) < 6 {
			return nil, bullarc.ErrDataSourceUnavailable.Wrap(
				fmt.Errorf("csv %s row %d: expected 6 columns, got %d", s.path, i+2, len(row)))
		}

		t, err := time.Parse("2006-01-02", row[0])
		if err != nil {
			return nil, bullarc.ErrDataSourceUnavailable.Wrap(
				fmt.Errorf("csv %s row %d: parse date %q: %w", s.path, i+2, row[0], err))
		}

		if !query.Start.IsZero() && t.Before(query.Start) {
			continue
		}
		if !query.End.IsZero() && t.After(query.End) {
			continue
		}

		o, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("csv row %d open: %w", i+2, err))
		}
		h, err := strconv.ParseFloat(row[2], 64)
		if err != nil {
			return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("csv row %d high: %w", i+2, err))
		}
		lo, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("csv row %d low: %w", i+2, err))
		}
		c, err := strconv.ParseFloat(row[4], 64)
		if err != nil {
			return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("csv row %d close: %w", i+2, err))
		}
		v, err := strconv.ParseFloat(row[5], 64)
		if err != nil {
			return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("csv row %d volume: %w", i+2, err))
		}

		bars = append(bars, bullarc.OHLCV{
			Time:   t,
			Open:   o,
			High:   h,
			Low:    lo,
			Close:  c,
			Volume: v,
		})
	}

	slog.Info("loaded bars from csv", "path", s.path, "count", len(bars))
	return bars, nil
}
