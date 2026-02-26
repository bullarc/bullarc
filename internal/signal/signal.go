package signal

import (
	"strings"

	"github.com/bullarc/bullarc"
)

// Generator produces a trading signal from indicator values and the latest bar.
// Returns the signal and whether a signal was produced.
type Generator func(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool)

// ForIndicator returns the Generator for the given indicator name, or nil if
// no generator is registered for that indicator.
func ForIndicator(name string) Generator {
	switch {
	case strings.HasPrefix(name, "RSI"):
		return rsiGenerator
	case strings.HasPrefix(name, "MACD"):
		return macdGenerator
	case strings.HasPrefix(name, "BB"):
		return bbGenerator
	case strings.HasPrefix(name, "SMA"):
		return smaCrossGenerator
	case strings.HasPrefix(name, "EMA"):
		return emaCrossGenerator
	case strings.HasPrefix(name, "SuperTrend"):
		return supertrendGenerator
	case strings.HasPrefix(name, "Stoch"):
		return stochasticGenerator
	case name == "VWAP":
		return vwapGenerator
	case name == "OBV":
		return obvGenerator
	}
	return nil
}
