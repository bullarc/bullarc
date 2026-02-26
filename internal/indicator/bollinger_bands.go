package indicator

import (
	"fmt"
	"math"

	"github.com/bullarcdev/bullarc"
)

// BollingerBands computes the Bollinger Bands over a configurable period
// and standard deviation multiplier.
type BollingerBands struct {
	period     int
	multiplier float64
}

// NewBollingerBands creates a new BollingerBands indicator.
func NewBollingerBands(period int, multiplier float64) (*BollingerBands, error) {
	if period < 2 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("BollingerBands period must be >= 2, got %d", period),
		)
	}
	if multiplier <= 0 {
		return nil, bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("BollingerBands multiplier must be > 0, got %f", multiplier),
		)
	}
	return &BollingerBands{period: period, multiplier: multiplier}, nil
}

// Meta returns metadata for the BollingerBands indicator.
func (b *BollingerBands) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:         fmt.Sprintf("BB_%d_%.1f", b.period, b.multiplier),
		Description:  "Bollinger Bands",
		Category:     "volatility",
		Parameters:   map[string]any{"period": b.period, "multiplier": b.multiplier},
		WarmupPeriod: b.period - 1,
	}
}

// Compute calculates Bollinger Bands for the given bars.
// Value holds the middle band (SMA). Extra contains "upper", "lower",
// "bandwidth", and "percent_b".
func (b *BollingerBands) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	if len(bars) < b.period {
		return nil, bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("BollingerBands(%d) needs %d bars, got %d", b.period, b.period, len(bars)),
		)
	}

	n := len(bars) - b.period + 1
	values := make([]bullarc.IndicatorValue, n)

	for i := range n {
		window := bars[i : i+b.period]
		middle := windowSMA(window)
		stddev := populationStdDev(window, middle)

		upper := middle + b.multiplier*stddev
		lower := middle - b.multiplier*stddev
		close := bars[i+b.period-1].Close

		bandwidth := 0.0
		if middle != 0 {
			bandwidth = (upper - lower) / middle
		}

		percentB := 0.0
		if upper != lower {
			percentB = (close - lower) / (upper - lower)
		}

		values[i] = bullarc.IndicatorValue{
			Time:  bars[i+b.period-1].Time,
			Value: middle,
			Extra: map[string]float64{
				"upper":     upper,
				"lower":     lower,
				"bandwidth": bandwidth,
				"percent_b": percentB,
			},
		}
	}

	return values, nil
}

// windowSMA computes the mean of the close prices in a window.
func windowSMA(bars []bullarc.OHLCV) float64 {
	var sum float64
	for _, b := range bars {
		sum += b.Close
	}
	return sum / float64(len(bars))
}

// populationStdDev computes the population standard deviation of close prices.
func populationStdDev(bars []bullarc.OHLCV, mean float64) float64 {
	var sumSq float64
	for _, b := range bars {
		diff := b.Close - mean
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(bars)))
}
