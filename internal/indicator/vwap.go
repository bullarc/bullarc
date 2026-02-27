package indicator

import (
	"fmt"

	"github.com/bullarc/bullarc"
)

// VWAP computes the cumulative Volume Weighted Average Price.
// All bars are valid; no warmup period is required.
type VWAP struct {
	// Incremental state for Update.
	cumTPV float64 // cumulative typical price * volume
	cumVol float64 // cumulative volume
}

// NewVWAP creates a new VWAP indicator.
func NewVWAP() *VWAP {
	return &VWAP{}
}

// Meta returns metadata for the VWAP indicator.
func (v *VWAP) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:         "VWAP",
		Description:  "Volume Weighted Average Price",
		Category:     "volume",
		Parameters:   map[string]any{},
		WarmupPeriod: 0,
	}
}

// Compute calculates cumulative VWAP for the given bars.
func (v *VWAP) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	if len(bars) == 0 {
		return nil, bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("VWAP needs at least 1 bar, got 0"),
		)
	}

	values := make([]bullarc.IndicatorValue, len(bars))
	var cumTPV, cumVol float64

	for i, b := range bars {
		tp := (b.High + b.Low + b.Close) / 3
		cumTPV += tp * b.Volume
		cumVol += b.Volume

		vwap := 0.0
		if cumVol > 0 {
			vwap = cumTPV / cumVol
		}
		values[i] = bullarc.IndicatorValue{
			Time:  b.Time,
			Value: vwap,
		}
	}

	return values, nil
}

// Update processes a single new bar incrementally and returns the new VWAP value.
// Always returns a value (no warmup period).
func (v *VWAP) Update(bar bullarc.OHLCV) *bullarc.IndicatorValue {
	tp := (bar.High + bar.Low + bar.Close) / 3
	v.cumTPV += tp * bar.Volume
	v.cumVol += bar.Volume

	vwap := 0.0
	if v.cumVol > 0 {
		vwap = v.cumTPV / v.cumVol
	}

	return &bullarc.IndicatorValue{
		Time:  bar.Time,
		Value: vwap,
	}
}
