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
