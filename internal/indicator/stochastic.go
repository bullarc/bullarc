package indicator

import (
	"fmt"

	"github.com/bullarcdev/bullarc"
)

// Stochastic computes the Slow Stochastic oscillator.
// Value holds the smoothed %K line. Extra contains "d" (%D line).
type Stochastic struct {
	period  int
	smoothK int
	smoothD int
}

// NewStochastic creates a new Stochastic indicator with the given parameters.
// period is the lookback for high/low, smoothK smooths the raw %K, smoothD smooths %K to get %D.
func NewStochastic(period, smoothK, smoothD int) (*Stochastic, error) {
	if period < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("Stochastic period must be >= 1, got %d", period),
		)
	}
	if smoothK < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("Stochastic smoothK must be >= 1, got %d", smoothK),
		)
	}
	if smoothD < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("Stochastic smoothD must be >= 1, got %d", smoothD),
		)
	}
	return &Stochastic{period: period, smoothK: smoothK, smoothD: smoothD}, nil
}

// Meta returns metadata for the Stochastic indicator.
func (s *Stochastic) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:         fmt.Sprintf("Stoch_%d_%d_%d", s.period, s.smoothK, s.smoothD),
		Description:  "Slow Stochastic Oscillator",
		Category:     "momentum",
		Parameters:   map[string]any{"period": s.period, "smooth_k": s.smoothK, "smooth_d": s.smoothD},
		WarmupPeriod: s.period + s.smoothK + s.smoothD - 3,
	}
}

// Compute calculates Stochastic values for the given bars.
func (s *Stochastic) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	needed := s.period + s.smoothK + s.smoothD - 2
	if len(bars) < needed {
		return nil, bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("Stochastic(%d,%d,%d) needs %d bars, got %d",
				s.period, s.smoothK, s.smoothD, needed, len(bars)),
		)
	}

	// Compute raw %K for each window of s.period bars.
	rawKLen := len(bars) - s.period + 1
	rawK := make([]float64, rawKLen)
	for i := range rawKLen {
		loLow := bars[i].Low
		hiHigh := bars[i].High
		for _, b := range bars[i+1 : i+s.period] {
			if b.Low < loLow {
				loLow = b.Low
			}
			if b.High > hiHigh {
				hiHigh = b.High
			}
		}
		if hiHigh == loLow {
			rawK[i] = 0
		} else {
			rawK[i] = (bars[i+s.period-1].Close - loLow) / (hiHigh - loLow) * 100
		}
	}

	// Slow %K = SMA(rawK, smoothK).
	slowK := smaSlice(rawK, s.smoothK)

	// %D = SMA(slowK, smoothD).
	slowD := smaSlice(slowK, s.smoothD)

	// Output is aligned to %D. At output[k], both %K and %D are reported.
	// %D[k] corresponds to bars[period+smoothK+smoothD-3+k].
	// %K at same bar = slowK[k+smoothD-1].
	startBar := s.period + s.smoothK + s.smoothD - 3
	n := len(slowD)
	values := make([]bullarc.IndicatorValue, n)
	for k := range n {
		values[k] = bullarc.IndicatorValue{
			Time:  bars[startBar+k].Time,
			Value: slowK[k+s.smoothD-1],
			Extra: map[string]float64{"d": slowD[k]},
		}
	}

	return values, nil
}
