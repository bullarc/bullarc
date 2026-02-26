package indicator

import (
	"fmt"

	"github.com/bullarc/bullarc"
)

// SuperTrend computes the SuperTrend indicator using ATR-based bands.
// Value holds the SuperTrend line. Extra contains "direction" (1 = uptrend, -1 = downtrend).
type SuperTrend struct {
	period     int
	multiplier float64
}

// NewSuperTrend creates a new SuperTrend indicator with the given ATR period and multiplier.
func NewSuperTrend(period int, multiplier float64) (*SuperTrend, error) {
	if period < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("SuperTrend period must be >= 1, got %d", period),
		)
	}
	if multiplier <= 0 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("SuperTrend multiplier must be > 0, got %f", multiplier),
		)
	}
	return &SuperTrend{period: period, multiplier: multiplier}, nil
}

// Meta returns metadata for the SuperTrend indicator.
func (s *SuperTrend) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:         fmt.Sprintf("SuperTrend_%d_%.1f", s.period, s.multiplier),
		Description:  "SuperTrend",
		Category:     "trend",
		Parameters:   map[string]any{"period": s.period, "multiplier": s.multiplier},
		WarmupPeriod: s.period,
	}
}

// Compute calculates SuperTrend values for the given bars.
func (s *SuperTrend) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	atrInd, _ := NewATR(s.period)
	atrVals, err := atrInd.Compute(bars)
	if err != nil {
		return nil, err
	}

	// atrVals[k] corresponds to bars[s.period + k].
	n := len(atrVals)
	values := make([]bullarc.IndicatorValue, n)

	var prevUpper, prevLower, prevSuperTrend float64

	for k := range n {
		bar := bars[s.period+k]
		atr := atrVals[k].Value

		hl2 := (bar.High + bar.Low) / 2
		basicUpper := hl2 + s.multiplier*atr
		basicLower := hl2 - s.multiplier*atr

		var finalUpper, finalLower, supertrend, direction float64

		if k == 0 {
			finalUpper = basicUpper
			finalLower = basicLower
		} else {
			prevBar := bars[s.period+k-1]
			if basicUpper < prevUpper || prevBar.Close > prevUpper {
				finalUpper = basicUpper
			} else {
				finalUpper = prevUpper
			}
			if basicLower > prevLower || prevBar.Close < prevLower {
				finalLower = basicLower
			} else {
				finalLower = prevLower
			}
		}

		if k == 0 {
			if bar.Close <= finalUpper {
				supertrend = finalUpper
				direction = -1
			} else {
				supertrend = finalLower
				direction = 1
			}
		} else if prevSuperTrend == prevUpper {
			if bar.Close > finalUpper {
				supertrend = finalLower
				direction = 1
			} else {
				supertrend = finalUpper
				direction = -1
			}
		} else {
			if bar.Close < finalLower {
				supertrend = finalUpper
				direction = -1
			} else {
				supertrend = finalLower
				direction = 1
			}
		}

		prevUpper = finalUpper
		prevLower = finalLower
		prevSuperTrend = supertrend

		values[k] = bullarc.IndicatorValue{
			Time:  bar.Time,
			Value: supertrend,
			Extra: map[string]float64{"direction": direction},
		}
	}

	return values, nil
}
