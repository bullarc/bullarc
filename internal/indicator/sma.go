package indicator

import (
	"fmt"

	"github.com/bullarcdev/bullarc"
)

// SMA computes the Simple Moving Average over a configurable period.
type SMA struct {
	period int
}

// NewSMA creates a new SMA indicator with the given period.
func NewSMA(period int) (*SMA, error) {
	if period < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("SMA period must be >= 1, got %d", period),
		)
	}
	return &SMA{period: period}, nil
}

// Meta returns metadata for the SMA indicator.
func (s *SMA) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:         fmt.Sprintf("SMA_%d", s.period),
		Description:  "Simple Moving Average",
		Category:     "trend",
		Parameters:   map[string]any{"period": s.period},
		WarmupPeriod: s.period - 1,
	}
}

// Compute calculates SMA values for the given bars.
func (s *SMA) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	if len(bars) < s.period {
		return nil, bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("SMA(%d) needs %d bars, got %d", s.period, s.period, len(bars)),
		)
	}

	var sum float64
	for i := 0; i < s.period; i++ {
		sum += bars[i].Close
	}

	n := len(bars) - s.period + 1
	values := make([]bullarc.IndicatorValue, n)
	values[0] = bullarc.IndicatorValue{
		Time:  bars[s.period-1].Time,
		Value: sum / float64(s.period),
	}

	for i := 1; i < n; i++ {
		sum += bars[s.period+i-1].Close - bars[i-1].Close
		values[i] = bullarc.IndicatorValue{
			Time:  bars[s.period+i-1].Time,
			Value: sum / float64(s.period),
		}
	}

	return values, nil
}
