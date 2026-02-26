package indicator

import (
	"fmt"
	"math"

	"github.com/bullarcdev/bullarc"
)

// ATR computes the Average True Range over a configurable period
// using Wilder's smoothing method.
type ATR struct {
	period int
}

// NewATR creates a new ATR indicator with the given period.
func NewATR(period int) (*ATR, error) {
	if period < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("ATR period must be >= 1, got %d", period),
		)
	}
	return &ATR{period: period}, nil
}

// Meta returns metadata for the ATR indicator.
func (a *ATR) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:         fmt.Sprintf("ATR_%d", a.period),
		Description:  "Average True Range",
		Category:     "volatility",
		Parameters:   map[string]any{"period": a.period},
		WarmupPeriod: a.period,
	}
}

// Compute calculates ATR values for the given bars using Wilder's smoothing.
func (a *ATR) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	if len(bars) < a.period+1 {
		return nil, bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("ATR(%d) needs %d bars, got %d", a.period, a.period+1, len(bars)),
		)
	}

	// Seed ATR with the simple average of the first period true ranges.
	var sumTR float64
	for i := 1; i <= a.period; i++ {
		sumTR += trueRange(bars[i], bars[i-1])
	}
	atr := sumTR / float64(a.period)

	n := len(bars) - a.period
	values := make([]bullarc.IndicatorValue, n)
	values[0] = bullarc.IndicatorValue{
		Time:  bars[a.period].Time,
		Value: atr,
	}

	for i := 1; i < n; i++ {
		tr := trueRange(bars[a.period+i], bars[a.period+i-1])
		atr = (atr*float64(a.period-1) + tr) / float64(a.period)
		values[i] = bullarc.IndicatorValue{
			Time:  bars[a.period+i].Time,
			Value: atr,
		}
	}

	return values, nil
}

// trueRange computes the true range for a bar given the previous bar.
func trueRange(bar, prev bullarc.OHLCV) float64 {
	hl := bar.High - bar.Low
	hc := math.Abs(bar.High - prev.Close)
	lc := math.Abs(bar.Low - prev.Close)
	return math.Max(hl, math.Max(hc, lc))
}
