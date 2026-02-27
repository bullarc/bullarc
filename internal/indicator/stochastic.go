package indicator

import (
	"fmt"

	"github.com/bullarc/bullarc"
)

// Stochastic computes the Slow Stochastic oscillator.
// Value holds the smoothed %K line. Extra contains "d" (%D line).
type Stochastic struct {
	period  int
	smoothK int
	smoothD int

	// Incremental state for Update.
	bars     []bullarc.OHLCV // sliding window of last period bars (for high/low/close)
	rawKBuf  []float64       // sliding window of raw %K values for smoothK SMA
	slowKBuf []float64       // sliding window of slow %K values for smoothD SMA
	count    int             // total bars received
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

// Update processes a single new bar incrementally and returns the new Stochastic value.
// Returns nil during the warmup period.
func (s *Stochastic) Update(bar bullarc.OHLCV) *bullarc.IndicatorValue {
	s.count++

	// Maintain the sliding window of bars for period lookback.
	if len(s.bars) < s.period {
		s.bars = append(s.bars, bar)
	} else {
		idx := (s.count - 1) % s.period
		s.bars[idx] = bar
	}

	// Need at least period bars to compute raw %K.
	if s.count < s.period {
		return nil
	}

	// Compute raw %K from the current window.
	loLow := s.bars[0].Low
	hiHigh := s.bars[0].High
	for i := 1; i < len(s.bars); i++ {
		if s.bars[i].Low < loLow {
			loLow = s.bars[i].Low
		}
		if s.bars[i].High > hiHigh {
			hiHigh = s.bars[i].High
		}
	}
	rawK := 0.0
	if hiHigh != loLow {
		rawK = (bar.Close - loLow) / (hiHigh - loLow) * 100
	}

	// Add rawK to the smoothK sliding window.
	if len(s.rawKBuf) < s.smoothK {
		s.rawKBuf = append(s.rawKBuf, rawK)
	} else {
		rawKIdx := (s.count - s.period) % s.smoothK
		s.rawKBuf[rawKIdx] = rawK
	}

	// Need smoothK raw %K values to compute slow %K.
	rawKCount := s.count - s.period + 1
	if rawKCount < s.smoothK {
		return nil
	}

	// Slow %K = SMA of rawK.
	var sumK float64
	for _, v := range s.rawKBuf {
		sumK += v
	}
	slowK := sumK / float64(s.smoothK)

	// Add slowK to the smoothD sliding window.
	if len(s.slowKBuf) < s.smoothD {
		s.slowKBuf = append(s.slowKBuf, slowK)
	} else {
		slowKCount := rawKCount - s.smoothK + 1
		slowKIdx := (slowKCount - 1) % s.smoothD
		s.slowKBuf[slowKIdx] = slowK
	}

	// Need smoothD slow %K values to compute %D.
	slowKCount := rawKCount - s.smoothK + 1
	if slowKCount < s.smoothD {
		return nil
	}

	// %D = SMA of slow %K.
	var sumD float64
	for _, v := range s.slowKBuf {
		sumD += v
	}
	slowD := sumD / float64(s.smoothD)

	return &bullarc.IndicatorValue{
		Time:  bar.Time,
		Value: slowK,
		Extra: map[string]float64{"d": slowD},
	}
}
