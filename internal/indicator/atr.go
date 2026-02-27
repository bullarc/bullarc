package indicator

import (
	"fmt"
	"math"

	"github.com/bullarc/bullarc"
)

// ATR computes the Average True Range over a configurable period
// using Wilder's smoothing method.
type ATR struct {
	period int

	// Incremental state for Update.
	prevBar bullarc.OHLCV // previous bar for true range calculation
	prevATR float64       // previous ATR value (Wilder smoothed)
	sumTR   float64       // running sum for initial seed
	count   int           // total bars received
	seeded  bool          // true once the initial ATR seed has been computed
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

// Update processes a single new bar incrementally and returns the new ATR value.
// Returns nil during the warmup period (fewer than period+1 bars received).
func (a *ATR) Update(bar bullarc.OHLCV) *bullarc.IndicatorValue {
	a.count++

	if a.count == 1 {
		a.prevBar = bar
		return nil
	}

	tr := trueRange(bar, a.prevBar)
	a.prevBar = bar

	if !a.seeded {
		a.sumTR += tr
		if a.count <= a.period {
			return nil
		}
		// count == period+1: seed ATR from the mean of the first period true ranges
		a.prevATR = a.sumTR / float64(a.period)
		a.seeded = true
		return &bullarc.IndicatorValue{
			Time:  bar.Time,
			Value: a.prevATR,
		}
	}

	a.prevATR = (a.prevATR*float64(a.period-1) + tr) / float64(a.period)
	return &bullarc.IndicatorValue{
		Time:  bar.Time,
		Value: a.prevATR,
	}
}

// trueRange computes the true range for a bar given the previous bar.
func trueRange(bar, prev bullarc.OHLCV) float64 {
	hl := bar.High - bar.Low
	hc := math.Abs(bar.High - prev.Close)
	lc := math.Abs(bar.Low - prev.Close)
	return math.Max(hl, math.Max(hc, lc))
}
