package indicator

import (
	"fmt"

	"github.com/bullarc/bullarc"
)

// EMA computes the Exponential Moving Average over a configurable period.
// The first value is seeded from the SMA of the initial period bars.
type EMA struct {
	period int

	// Incremental state for Update.
	seedSum float64 // running sum for SMA seed
	prevEMA float64 // previous EMA value
	count   int     // total bars received
	seeded  bool    // true once the initial SMA seed has been computed
}

// NewEMA creates a new EMA indicator with the given period.
func NewEMA(period int) (*EMA, error) {
	if period < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("EMA period must be >= 1, got %d", period),
		)
	}
	return &EMA{period: period}, nil
}

// Meta returns metadata for the EMA indicator.
func (e *EMA) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:         fmt.Sprintf("EMA_%d", e.period),
		Description:  "Exponential Moving Average",
		Category:     "trend",
		Parameters:   map[string]any{"period": e.period},
		WarmupPeriod: e.period - 1,
	}
}

// Compute calculates EMA values for the given bars.
func (e *EMA) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	if len(bars) < e.period {
		return nil, bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("EMA(%d) needs %d bars, got %d", e.period, e.period, len(bars)),
		)
	}

	closes := barCloses(bars)
	emaVals := computeEMAOverFloats(closes, e.period)

	values := make([]bullarc.IndicatorValue, len(emaVals))
	for i, v := range emaVals {
		values[i] = bullarc.IndicatorValue{
			Time:  bars[e.period-1+i].Time,
			Value: v,
		}
	}
	return values, nil
}

// Update processes a single new bar incrementally and returns the new EMA value.
// Returns nil during the warmup period (fewer than period bars received).
func (e *EMA) Update(bar bullarc.OHLCV) *bullarc.IndicatorValue {
	e.count++
	k := 2.0 / float64(e.period+1)

	if !e.seeded {
		e.seedSum += bar.Close
		if e.count < e.period {
			return nil
		}
		e.prevEMA = e.seedSum / float64(e.period)
		e.seeded = true
		return &bullarc.IndicatorValue{
			Time:  bar.Time,
			Value: e.prevEMA,
		}
	}

	e.prevEMA = bar.Close*k + e.prevEMA*(1-k)
	return &bullarc.IndicatorValue{
		Time:  bar.Time,
		Value: e.prevEMA,
	}
}

// barCloses extracts close prices from a slice of OHLCV bars.
func barCloses(bars []bullarc.OHLCV) []float64 {
	closes := make([]float64, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
	}
	return closes
}
