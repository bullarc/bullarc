package indicator

import (
	"fmt"
	"math"

	"github.com/bullarc/bullarc"
)

// RSI computes the Relative Strength Index over a configurable period
// using Wilder's smoothing method.
type RSI struct {
	period int

	// Incremental state for Update.
	prevClose float64 // previous bar close for delta computation
	avgGain   float64 // Wilder-smoothed average gain
	avgLoss   float64 // Wilder-smoothed average loss
	sumGain   float64 // running sum for initial seed
	sumLoss   float64 // running sum for initial seed
	count     int     // total bars received
	seeded    bool    // true once the initial seed has been computed
}

// NewRSI creates a new RSI indicator with the given period.
func NewRSI(period int) (*RSI, error) {
	if period < 1 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("RSI period must be >= 1, got %d", period),
		)
	}
	return &RSI{period: period}, nil
}

// Meta returns metadata for the RSI indicator.
func (r *RSI) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:         fmt.Sprintf("RSI_%d", r.period),
		Description:  "Relative Strength Index",
		Category:     "momentum",
		Parameters:   map[string]any{"period": r.period},
		WarmupPeriod: r.period,
	}
}

// Compute calculates RSI values for the given bars.
func (r *RSI) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	if len(bars) < r.period+1 {
		return nil, bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("RSI(%d) needs %d bars, got %d", r.period, r.period+1, len(bars)),
		)
	}

	// Seed average gain/loss from first period price changes.
	var avgGain, avgLoss float64
	for i := 1; i <= r.period; i++ {
		delta := bars[i].Close - bars[i-1].Close
		avgGain += math.Max(delta, 0)
		avgLoss += math.Max(-delta, 0)
	}
	avgGain /= float64(r.period)
	avgLoss /= float64(r.period)

	n := len(bars) - r.period
	values := make([]bullarc.IndicatorValue, n)
	values[0] = bullarc.IndicatorValue{
		Time:  bars[r.period].Time,
		Value: rsiValue(avgGain, avgLoss),
	}

	for i := 1; i < n; i++ {
		delta := bars[r.period+i].Close - bars[r.period+i-1].Close
		gain := math.Max(delta, 0)
		loss := math.Max(-delta, 0)
		avgGain = (avgGain*float64(r.period-1) + gain) / float64(r.period)
		avgLoss = (avgLoss*float64(r.period-1) + loss) / float64(r.period)
		values[i] = bullarc.IndicatorValue{
			Time:  bars[r.period+i].Time,
			Value: rsiValue(avgGain, avgLoss),
		}
	}

	return values, nil
}

// Update processes a single new bar incrementally and returns the new RSI value.
// Returns nil during the warmup period (fewer than period+1 bars received).
func (r *RSI) Update(bar bullarc.OHLCV) *bullarc.IndicatorValue {
	r.count++

	if r.count == 1 {
		r.prevClose = bar.Close
		return nil
	}

	delta := bar.Close - r.prevClose
	gain := math.Max(delta, 0)
	loss := math.Max(-delta, 0)
	r.prevClose = bar.Close

	if !r.seeded {
		r.sumGain += gain
		r.sumLoss += loss
		if r.count <= r.period {
			return nil
		}
		// count == period+1: seed average gain/loss
		r.avgGain = r.sumGain / float64(r.period)
		r.avgLoss = r.sumLoss / float64(r.period)
		r.seeded = true
		return &bullarc.IndicatorValue{
			Time:  bar.Time,
			Value: rsiValue(r.avgGain, r.avgLoss),
		}
	}

	r.avgGain = (r.avgGain*float64(r.period-1) + gain) / float64(r.period)
	r.avgLoss = (r.avgLoss*float64(r.period-1) + loss) / float64(r.period)
	return &bullarc.IndicatorValue{
		Time:  bar.Time,
		Value: rsiValue(r.avgGain, r.avgLoss),
	}
}

// rsiValue computes a single RSI value from smoothed average gain and loss.
func rsiValue(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}
