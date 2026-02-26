package indicator

import (
	"fmt"

	"github.com/bullarc/bullarc"
)

// EMA computes the Exponential Moving Average over a configurable period.
// The first value is seeded from the SMA of the initial period bars.
type EMA struct {
	period int
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

// barCloses extracts close prices from a slice of OHLCV bars.
func barCloses(bars []bullarc.OHLCV) []float64 {
	closes := make([]float64, len(bars))
	for i, b := range bars {
		closes[i] = b.Close
	}
	return closes
}
