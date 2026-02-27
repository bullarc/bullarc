package indicator

import (
	"fmt"

	"github.com/bullarc/bullarc"
)

// OBV computes the On-Balance Volume.
// All bars produce a value; no warmup period is required.
type OBV struct {
	// Incremental state for Update.
	prevClose float64 // previous bar close
	obv       float64 // current OBV value
	count     int     // total bars received
}

// NewOBV creates a new OBV indicator.
func NewOBV() *OBV {
	return &OBV{}
}

// Meta returns metadata for the OBV indicator.
func (o *OBV) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{
		Name:         "OBV",
		Description:  "On-Balance Volume",
		Category:     "volume",
		Parameters:   map[string]any{},
		WarmupPeriod: 0,
	}
}

// Compute calculates OBV values for the given bars.
func (o *OBV) Compute(bars []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	if len(bars) == 0 {
		return nil, bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("OBV needs at least 1 bar, got 0"),
		)
	}

	values := make([]bullarc.IndicatorValue, len(bars))
	var obv float64

	values[0] = bullarc.IndicatorValue{
		Time:  bars[0].Time,
		Value: obv,
	}

	for i := 1; i < len(bars); i++ {
		switch {
		case bars[i].Close > bars[i-1].Close:
			obv += bars[i].Volume
		case bars[i].Close < bars[i-1].Close:
			obv -= bars[i].Volume
		}
		values[i] = bullarc.IndicatorValue{
			Time:  bars[i].Time,
			Value: obv,
		}
	}

	return values, nil
}

// Update processes a single new bar incrementally and returns the new OBV value.
// Always returns a value (no warmup period).
func (o *OBV) Update(bar bullarc.OHLCV) *bullarc.IndicatorValue {
	o.count++

	if o.count == 1 {
		o.prevClose = bar.Close
		return &bullarc.IndicatorValue{
			Time:  bar.Time,
			Value: 0,
		}
	}

	switch {
	case bar.Close > o.prevClose:
		o.obv += bar.Volume
	case bar.Close < o.prevClose:
		o.obv -= bar.Volume
	}
	o.prevClose = bar.Close

	return &bullarc.IndicatorValue{
		Time:  bar.Time,
		Value: o.obv,
	}
}
