package signal

import (
	"fmt"

	"github.com/bullarcdev/bullarc"
)

func newSignal(t bullarc.SignalType, conf float64, name, symbol string, bar bullarc.OHLCV, explanation string) bullarc.Signal {
	return bullarc.Signal{
		Type:        t,
		Confidence:  conf,
		Indicator:   name,
		Symbol:      symbol,
		Timestamp:   bar.Time,
		Explanation: explanation,
	}
}

func lastVal(values []bullarc.IndicatorValue) (bullarc.IndicatorValue, bool) {
	if len(values) == 0 {
		return bullarc.IndicatorValue{}, false
	}
	return values[len(values)-1], true
}

// rsiGenerator emits BUY when RSI < 30, SELL when RSI > 70.
func rsiGenerator(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool) {
	v, ok := lastVal(values)
	if !ok {
		return bullarc.Signal{}, false
	}
	rsi := v.Value
	switch {
	case rsi < 20:
		return newSignal(bullarc.SignalBuy, 0.85, name, symbol, bar,
			fmt.Sprintf("RSI %.1f: deeply oversold", rsi)), true
	case rsi < 30:
		return newSignal(bullarc.SignalBuy, 0.65, name, symbol, bar,
			fmt.Sprintf("RSI %.1f: oversold", rsi)), true
	case rsi > 80:
		return newSignal(bullarc.SignalSell, 0.85, name, symbol, bar,
			fmt.Sprintf("RSI %.1f: deeply overbought", rsi)), true
	case rsi > 70:
		return newSignal(bullarc.SignalSell, 0.65, name, symbol, bar,
			fmt.Sprintf("RSI %.1f: overbought", rsi)), true
	default:
		return newSignal(bullarc.SignalHold, 0.5, name, symbol, bar,
			fmt.Sprintf("RSI %.1f: neutral", rsi)), true
	}
}

// macdGenerator emits BUY when the histogram is positive, SELL when negative.
func macdGenerator(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool) {
	v, ok := lastVal(values)
	if !ok {
		return bullarc.Signal{}, false
	}
	hist, has := v.Extra["histogram"]
	if !has {
		return bullarc.Signal{}, false
	}
	switch {
	case hist > 0:
		return newSignal(bullarc.SignalBuy, 0.65, name, symbol, bar,
			fmt.Sprintf("MACD histogram %.4f: bullish momentum", hist)), true
	case hist < 0:
		return newSignal(bullarc.SignalSell, 0.65, name, symbol, bar,
			fmt.Sprintf("MACD histogram %.4f: bearish momentum", hist)), true
	default:
		return newSignal(bullarc.SignalHold, 0.5, name, symbol, bar,
			"MACD histogram zero: neutral"), true
	}
}

// bbGenerator emits BUY when price is below the lower band, SELL when above upper.
func bbGenerator(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool) {
	v, ok := lastVal(values)
	if !ok {
		return bullarc.Signal{}, false
	}
	upper, hasUpper := v.Extra["upper"]
	lower, hasLower := v.Extra["lower"]
	if !hasUpper || !hasLower {
		return bullarc.Signal{}, false
	}
	price := bar.Close
	switch {
	case price < lower:
		return newSignal(bullarc.SignalBuy, 0.70, name, symbol, bar,
			fmt.Sprintf("price %.2f below lower band %.2f", price, lower)), true
	case price > upper:
		return newSignal(bullarc.SignalSell, 0.70, name, symbol, bar,
			fmt.Sprintf("price %.2f above upper band %.2f", price, upper)), true
	default:
		return newSignal(bullarc.SignalHold, 0.5, name, symbol, bar,
			"price within Bollinger Bands"), true
	}
}

// smaCrossGenerator emits BUY when price is >2% above the SMA, SELL when >2% below.
func smaCrossGenerator(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool) {
	v, ok := lastVal(values)
	if !ok {
		return bullarc.Signal{}, false
	}
	sma := v.Value
	if sma == 0 {
		return bullarc.Signal{}, false
	}
	price := bar.Close
	deviation := (price - sma) / sma
	switch {
	case deviation > 0.02:
		return newSignal(bullarc.SignalBuy, 0.55, name, symbol, bar,
			fmt.Sprintf("price %.2f above %s %.2f (+%.1f%%)", price, name, sma, deviation*100)), true
	case deviation < -0.02:
		return newSignal(bullarc.SignalSell, 0.55, name, symbol, bar,
			fmt.Sprintf("price %.2f below %s %.2f (%.1f%%)", price, name, sma, deviation*100)), true
	default:
		return newSignal(bullarc.SignalHold, 0.5, name, symbol, bar,
			fmt.Sprintf("price near %s (%.2f vs %.2f)", name, price, sma)), true
	}
}

// emaCrossGenerator applies the same price-cross logic as SMA.
func emaCrossGenerator(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool) {
	return smaCrossGenerator(name, symbol, bar, values)
}

// supertrendGenerator emits BUY when direction=1, SELL when direction=-1.
func supertrendGenerator(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool) {
	v, ok := lastVal(values)
	if !ok {
		return bullarc.Signal{}, false
	}
	dir, has := v.Extra["direction"]
	if !has {
		return bullarc.Signal{}, false
	}
	switch {
	case dir > 0:
		return newSignal(bullarc.SignalBuy, 0.75, name, symbol, bar,
			"SuperTrend: bullish trend"), true
	case dir < 0:
		return newSignal(bullarc.SignalSell, 0.75, name, symbol, bar,
			"SuperTrend: bearish trend"), true
	default:
		return newSignal(bullarc.SignalHold, 0.5, name, symbol, bar,
			"SuperTrend: no trend"), true
	}
}

// stochasticGenerator emits BUY when K < 20, SELL when K > 80.
func stochasticGenerator(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool) {
	v, ok := lastVal(values)
	if !ok {
		return bullarc.Signal{}, false
	}
	k := v.Value
	switch {
	case k < 20:
		return newSignal(bullarc.SignalBuy, 0.65, name, symbol, bar,
			fmt.Sprintf("Stochastic K=%.1f: oversold", k)), true
	case k > 80:
		return newSignal(bullarc.SignalSell, 0.65, name, symbol, bar,
			fmt.Sprintf("Stochastic K=%.1f: overbought", k)), true
	default:
		return newSignal(bullarc.SignalHold, 0.5, name, symbol, bar,
			fmt.Sprintf("Stochastic K=%.1f: neutral", k)), true
	}
}

// vwapGenerator emits BUY when price > VWAP, SELL when price < VWAP.
func vwapGenerator(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool) {
	v, ok := lastVal(values)
	if !ok {
		return bullarc.Signal{}, false
	}
	vwap := v.Value
	price := bar.Close
	switch {
	case price > vwap:
		return newSignal(bullarc.SignalBuy, 0.55, name, symbol, bar,
			fmt.Sprintf("price %.2f above VWAP %.2f", price, vwap)), true
	case price < vwap:
		return newSignal(bullarc.SignalSell, 0.55, name, symbol, bar,
			fmt.Sprintf("price %.2f below VWAP %.2f", price, vwap)), true
	default:
		return newSignal(bullarc.SignalHold, 0.5, name, symbol, bar,
			"price at VWAP"), true
	}
}

// obvGenerator emits BUY when OBV trended up over the last 5 bars, SELL when down.
// Requires at least 5 computed values.
func obvGenerator(name, symbol string, bar bullarc.OHLCV, values []bullarc.IndicatorValue) (bullarc.Signal, bool) {
	if len(values) < 5 {
		return bullarc.Signal{}, false
	}
	recent := values[len(values)-5:]
	first := recent[0].Value
	last := recent[len(recent)-1].Value
	switch {
	case last > first:
		return newSignal(bullarc.SignalBuy, 0.60, name, symbol, bar,
			fmt.Sprintf("OBV rising (%.0f → %.0f)", first, last)), true
	case last < first:
		return newSignal(bullarc.SignalSell, 0.60, name, symbol, bar,
			fmt.Sprintf("OBV falling (%.0f → %.0f)", first, last)), true
	default:
		return newSignal(bullarc.SignalHold, 0.5, name, symbol, bar,
			"OBV flat"), true
	}
}
