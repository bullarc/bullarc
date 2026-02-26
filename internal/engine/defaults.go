package engine

import (
	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/internal/indicator"
)

// DefaultIndicators returns the built-in set of indicators used by the engine.
// Each indicator is constructed with standard default parameters.
func DefaultIndicators() []bullarc.Indicator {
	sma14, _ := indicator.NewSMA(14)
	sma50, _ := indicator.NewSMA(50)
	ema14, _ := indicator.NewEMA(14)
	rsi14, _ := indicator.NewRSI(14)
	macd, _ := indicator.NewMACD(12, 26, 9)
	bb, _ := indicator.NewBollingerBands(20, 2.0)
	atr14, _ := indicator.NewATR(14)
	supertrend, _ := indicator.NewSuperTrend(7, 3.0)
	stoch, _ := indicator.NewStochastic(14, 3, 3)

	return []bullarc.Indicator{
		sma14,
		sma50,
		ema14,
		rsi14,
		macd,
		bb,
		atr14,
		supertrend,
		stoch,
		indicator.NewVWAP(),
		indicator.NewOBV(),
	}
}

// FilteredIndicators returns the subset of default indicators whose names appear
// in enabled. If enabled is empty, all default indicators are returned.
func FilteredIndicators(enabled []string) []bullarc.Indicator {
	all := DefaultIndicators()
	if len(enabled) == 0 {
		return all
	}
	set := make(map[string]struct{}, len(enabled))
	for _, name := range enabled {
		set[name] = struct{}{}
	}
	var out []bullarc.Indicator
	for _, ind := range all {
		if _, ok := set[ind.Meta().Name]; ok {
			out = append(out, ind)
		}
	}
	return out
}
