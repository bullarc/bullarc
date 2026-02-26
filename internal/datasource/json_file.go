package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/bullarc/bullarc"
)

// jsonRecord is the on-disk representation of a single OHLCV bar in JSON files.
type jsonRecord struct {
	Date   string  `json:"date"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// JSONSource reads OHLCV bars from a local JSON file.
// The file must contain a JSON array of objects with fields:
// date (YYYY-MM-DD), open, high, low, close, volume.
type JSONSource struct {
	path string
}

// NewJSONSource creates a JSONSource that reads from path.
func NewJSONSource(path string) *JSONSource {
	return &JSONSource{path: path}
}

// Meta returns metadata for the JSON file data source.
func (s *JSONSource) Meta() bullarc.DataSourceMeta {
	return bullarc.DataSourceMeta{
		Name:        "json",
		Description: "Reads OHLCV bars from a local JSON file",
	}
}

// Fetch loads bars from the JSON file, optionally filtering by the query time range.
func (s *JSONSource) Fetch(ctx context.Context, query bullarc.DataQuery) ([]bullarc.OHLCV, error) {
	slog.Info("fetching bars from json", "path", s.path, "symbol", query.Symbol)

	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("read %s: %w", s.path, err))
	}

	var raw []jsonRecord
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("parse json %s: %w", s.path, err))
	}

	if len(raw) == 0 {
		return nil, bullarc.ErrInsufficientData.Wrap(fmt.Errorf("json %s contains no bars", s.path))
	}

	bars := make([]bullarc.OHLCV, 0, len(raw))
	for i, rec := range raw {
		if err := ctx.Err(); err != nil {
			return nil, bullarc.ErrTimeout.Wrap(err)
		}

		t, err := time.Parse("2006-01-02", rec.Date)
		if err != nil {
			return nil, bullarc.ErrDataSourceUnavailable.Wrap(
				fmt.Errorf("json %s record %d: parse date %q: %w", s.path, i, rec.Date, err))
		}

		if !query.Start.IsZero() && t.Before(query.Start) {
			continue
		}
		if !query.End.IsZero() && t.After(query.End) {
			continue
		}

		bars = append(bars, bullarc.OHLCV{
			Time:   t,
			Open:   rec.Open,
			High:   rec.High,
			Low:    rec.Low,
			Close:  rec.Close,
			Volume: rec.Volume,
		})
	}

	slog.Info("loaded bars from json", "path", s.path, "count", len(bars))
	return bars, nil
}
