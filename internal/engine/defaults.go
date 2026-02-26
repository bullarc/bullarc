package engine

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/internal/config"
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

// IndicatorsFromConfig builds the active indicator set from configuration.
// If cfg.Enabled is empty all defaults are used. Otherwise each name is resolved
// first against the default set, then by parsing the name format
// (e.g. "RSI_21", "SMA_20", "MACD_10_22_9") to construct a custom instance.
// Unrecognised names are logged and skipped.
func IndicatorsFromConfig(cfg config.IndicatorsConfig) []bullarc.Indicator {
	all := DefaultIndicators()
	if len(cfg.Enabled) == 0 {
		return all
	}
	byName := make(map[string]bullarc.Indicator, len(all))
	for _, ind := range all {
		byName[ind.Meta().Name] = ind
	}
	var out []bullarc.Indicator
	for _, name := range cfg.Enabled {
		if ind, ok := byName[name]; ok {
			out = append(out, ind)
			continue
		}
		ind := buildIndicatorFromName(name)
		if ind != nil {
			out = append(out, ind)
		} else {
			slog.Warn("unknown indicator name in config, skipping", "name", name)
		}
	}
	return out
}

// buildIndicatorFromName parses a name like "SMA_20" or "MACD_10_22_9" and
// constructs the corresponding indicator. Returns nil for unrecognised patterns.
func buildIndicatorFromName(name string) bullarc.Indicator {
	switch {
	case name == "VWAP":
		return indicator.NewVWAP()
	case name == "OBV":
		return indicator.NewOBV()
	case strings.HasPrefix(name, "SMA_"):
		p, err := parseSingleInt(name, "SMA_")
		if err != nil {
			return nil
		}
		ind, _ := indicator.NewSMA(p)
		return ind
	case strings.HasPrefix(name, "EMA_"):
		p, err := parseSingleInt(name, "EMA_")
		if err != nil {
			return nil
		}
		ind, _ := indicator.NewEMA(p)
		return ind
	case strings.HasPrefix(name, "RSI_"):
		p, err := parseSingleInt(name, "RSI_")
		if err != nil {
			return nil
		}
		ind, _ := indicator.NewRSI(p)
		return ind
	case strings.HasPrefix(name, "ATR_"):
		p, err := parseSingleInt(name, "ATR_")
		if err != nil {
			return nil
		}
		ind, _ := indicator.NewATR(p)
		return ind
	case strings.HasPrefix(name, "MACD_"):
		parts := strings.Split(strings.TrimPrefix(name, "MACD_"), "_")
		if len(parts) != 3 {
			return nil
		}
		fast, err1 := strconv.Atoi(parts[0])
		slow, err2 := strconv.Atoi(parts[1])
		sig, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return nil
		}
		ind, _ := indicator.NewMACD(fast, slow, sig)
		return ind
	case strings.HasPrefix(name, "BB_"):
		p, m, err := parseIntFloat(name, "BB_")
		if err != nil {
			return nil
		}
		ind, _ := indicator.NewBollingerBands(p, m)
		return ind
	case strings.HasPrefix(name, "SuperTrend_"):
		p, m, err := parseIntFloat(name, "SuperTrend_")
		if err != nil {
			return nil
		}
		ind, _ := indicator.NewSuperTrend(p, m)
		return ind
	case strings.HasPrefix(name, "Stoch_"):
		parts := strings.Split(strings.TrimPrefix(name, "Stoch_"), "_")
		if len(parts) != 3 {
			return nil
		}
		period, err1 := strconv.Atoi(parts[0])
		smoothK, err2 := strconv.Atoi(parts[1])
		smoothD, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return nil
		}
		ind, _ := indicator.NewStochastic(period, smoothK, smoothD)
		return ind
	}
	return nil
}

func parseSingleInt(name, prefix string) (int, error) {
	return strconv.Atoi(strings.TrimPrefix(name, prefix))
}

func parseIntFloat(name, prefix string) (int, float64, error) {
	rest := strings.TrimPrefix(name, prefix)
	idx := strings.Index(rest, "_")
	if idx < 0 {
		return 0, 0, strconv.ErrSyntax
	}
	p, err := strconv.Atoi(rest[:idx])
	if err != nil {
		return 0, 0, err
	}
	m, err := strconv.ParseFloat(rest[idx+1:], 64)
	if err != nil {
		return 0, 0, err
	}
	return p, m, nil
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
