package indicator

import (
	"fmt"

	"github.com/bullarcdev/bullarc"
)

// OBV computes the On-Balance Volume.
// All bars produce a value; no warmup period is required.
type OBV struct{}

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
