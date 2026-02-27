package sdk

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/engine"
)

// validIntervals is the set of supported data intervals.
var validIntervals = map[string]struct{}{
	"1Min":   {},
	"5Min":   {},
	"15Min":  {},
	"30Min":  {},
	"1Hour":  {},
	"2Hour":  {},
	"4Hour":  {},
	"1Day":   {},
	"1Week":  {},
	"1Month": {},
}

// ClientConfig holds the programmatic configuration for a Client.
type ClientConfig struct {
	// Symbols is the default list of symbols for streaming analysis.
	// Used by Stream and StreamSymbols when no explicit symbol is provided.
	Symbols []string

	// Indicators is the default set of indicator names used per analysis.
	// An empty slice means all default indicators are used.
	Indicators []string

	// Interval is the data bar interval (e.g. "1Day", "1Hour").
	// Empty means the engine's own default is used.
	Interval string

	// DataSource is the custom data source to use instead of the engine default.
	// Nil means the engine's own registered data sources are used.
	DataSource bullarc.DataSource
}

// Option configures a Client at construction time or at runtime via Configure.
type Option func(*ClientConfig) error

// WithSymbols sets the default symbols. Each symbol must be a non-empty,
// non-whitespace-only string.
func WithSymbols(symbols ...string) Option {
	return func(cfg *ClientConfig) error {
		for i, s := range symbols {
			if strings.TrimSpace(s) == "" {
				return bullarc.ErrInvalidParameter.Wrap(
					fmt.Errorf("symbol at index %d must not be empty", i),
				)
			}
		}
		cfg.Symbols = make([]string, len(symbols))
		copy(cfg.Symbols, symbols)
		return nil
	}
}

// WithIndicators sets the default indicators by name. Each name must match a
// default indicator (e.g. "SMA_14") or a parseable pattern (e.g. "SMA_20",
// "MACD_10_22_9", "RSI_21").
func WithIndicators(indicators ...string) Option {
	return func(cfg *ClientConfig) error {
		for _, name := range indicators {
			if name == "" {
				return bullarc.ErrInvalidParameter.Wrap(
					fmt.Errorf("indicator name must not be empty"),
				)
			}
			if !isKnownIndicator(name) {
				return bullarc.ErrInvalidParameter.Wrap(
					fmt.Errorf("unknown indicator %q", name),
				)
			}
		}
		cfg.Indicators = make([]string, len(indicators))
		copy(cfg.Indicators, indicators)
		return nil
	}
}

// WithDataSource sets a custom data source adapter. The engine will use this
// source instead of any previously registered data source. ds must not be nil.
func WithDataSource(ds bullarc.DataSource) Option {
	return func(cfg *ClientConfig) error {
		if ds == nil {
			return bullarc.ErrInvalidParameter.Wrap(
				fmt.Errorf("data source must not be nil"),
			)
		}
		cfg.DataSource = ds
		return nil
	}
}

// WithInterval sets the default data interval. Must be one of the supported
// values: 1Min, 5Min, 15Min, 30Min, 1Hour, 2Hour, 4Hour, 1Day, 1Week, 1Month.
func WithInterval(interval string) Option {
	return func(cfg *ClientConfig) error {
		if _, ok := validIntervals[interval]; !ok {
			return bullarc.ErrInvalidParameter.Wrap(
				fmt.Errorf("unsupported interval %q: must be one of %s", interval, validIntervalList()),
			)
		}
		cfg.Interval = interval
		return nil
	}
}

// isKnownIndicator reports whether name is a registered default indicator or
// a parseable name pattern supported by the engine.
func isKnownIndicator(name string) bool {
	for _, ind := range engine.DefaultIndicators() {
		if ind.Meta().Name == name {
			return true
		}
	}
	return engine.BuildIndicatorFromName(name) != nil
}

// validIntervalList returns a sorted, comma-joined list of supported intervals.
func validIntervalList() string {
	parts := make([]string, 0, len(validIntervals))
	for k := range validIntervals {
		parts = append(parts, k)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}
