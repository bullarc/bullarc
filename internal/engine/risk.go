package engine

import (
	"log/slog"
	"math"

	"github.com/bullarc/bullarc"
)

const (
	// DefaultMaxPositionSizePct is the maximum suggested position size as a percentage of capital.
	DefaultMaxPositionSizePct = 5.0
	// DefaultStopLossMultiplier is the default ATR multiplier for stop-loss distance.
	DefaultStopLossMultiplier = 2.0
	// DefaultTakeProfitMultiplier is the default ATR multiplier for take-profit distance.
	DefaultTakeProfitMultiplier = 3.0

	// riskPerTradePct is the internal risk budget per trade (1% of capital).
	// Used to derive position size from stop-loss distance.
	riskPerTradePct = 1.0
)

// RiskConfig controls ATR-based position sizing and stop-loss computation.
type RiskConfig struct {
	Enabled              bool    `json:"enabled" yaml:"enabled"`
	MaxPositionSizePct   float64 `json:"max_position_size_pct" yaml:"max_position_size_pct"`
	StopLossMultiplier   float64 `json:"stop_loss_multiplier" yaml:"stop_loss_multiplier"`
	TakeProfitMultiplier float64 `json:"take_profit_multiplier" yaml:"take_profit_multiplier"`
	// ATRIndicatorName is the name of the ATR indicator whose values are used.
	// Defaults to "ATR_14" when empty.
	ATRIndicatorName string `json:"atr_indicator_name" yaml:"atr_indicator_name"`
}

// defaultRiskConfig returns a RiskConfig with standard default values (disabled).
func defaultRiskConfig() RiskConfig {
	return RiskConfig{
		Enabled:              false,
		MaxPositionSizePct:   DefaultMaxPositionSizePct,
		StopLossMultiplier:   DefaultStopLossMultiplier,
		TakeProfitMultiplier: DefaultTakeProfitMultiplier,
		ATRIndicatorName:     "ATR_14",
	}
}

// computeRiskMetrics derives ATR-based position sizing and stop/take-profit prices.
//
// Risk metrics are computed only when:
//   - cfg.Enabled is true
//   - the composite signal is BUY or SELL (not HOLD)
//   - ATR indicator values are present and the latest value is positive
//
// Position size is inversely proportional to ATR: higher volatility yields a
// smaller suggested position. The formula risks riskPerTradePct (1%) of capital
// per trade, sizing the position so that a stop triggered at stopLossMultiplier×ATR
// away from entry loses exactly that budget.
//
// Returns nil and false when any precondition is not met.
func computeRiskMetrics(
	composite bullarc.Signal,
	entryPrice float64,
	indicatorValues map[string][]bullarc.IndicatorValue,
	cfg RiskConfig,
) (*bullarc.RiskMetrics, bool) {
	if !cfg.Enabled {
		return nil, false
	}

	if composite.Type == bullarc.SignalHold {
		slog.Info("risk metrics omitted: HOLD signal", "symbol", composite.Symbol)
		return nil, false
	}

	atrName := cfg.ATRIndicatorName
	if atrName == "" {
		atrName = "ATR_14"
	}

	atrValues, ok := indicatorValues[atrName]
	if !ok || len(atrValues) == 0 {
		slog.Warn("risk metrics omitted: ATR indicator not available",
			"symbol", composite.Symbol,
			"atr_indicator", atrName)
		return nil, false
	}

	atr := atrValues[len(atrValues)-1].Value
	if atr <= 0 || math.IsNaN(atr) || math.IsInf(atr, 0) {
		slog.Warn("risk metrics omitted: ATR value is invalid",
			"symbol", composite.Symbol,
			"atr", atr)
		return nil, false
	}

	if entryPrice <= 0 {
		slog.Warn("risk metrics omitted: entry price is non-positive",
			"symbol", composite.Symbol,
			"entry_price", entryPrice)
		return nil, false
	}

	stopMult := cfg.StopLossMultiplier
	if stopMult <= 0 {
		stopMult = DefaultStopLossMultiplier
	}
	tpMult := cfg.TakeProfitMultiplier
	if tpMult <= 0 {
		tpMult = DefaultTakeProfitMultiplier
	}
	maxSize := cfg.MaxPositionSizePct
	if maxSize <= 0 {
		maxSize = DefaultMaxPositionSizePct
	}

	// Position size: risk riskPerTradePct% of capital per trade.
	// Stop-loss distance as fraction of price = stopMult * ATR / entryPrice.
	// positionSizePct = riskPerTradePct / (stopMult * ATR / entryPrice)
	atrPct := atr / entryPrice
	positionSizePct := riskPerTradePct / (stopMult * atrPct)
	if positionSizePct > maxSize {
		positionSizePct = maxSize
	}

	var stopLoss, takeProfit float64
	if composite.Type == bullarc.SignalBuy {
		stopLoss = entryPrice - stopMult*atr
		takeProfit = entryPrice + tpMult*atr
	} else {
		stopLoss = entryPrice + stopMult*atr
		takeProfit = entryPrice - tpMult*atr
	}

	riskReward := tpMult / stopMult

	return &bullarc.RiskMetrics{
		PositionSizePct: positionSizePct,
		StopLoss:        stopLoss,
		TakeProfit:      takeProfit,
		RiskRewardRatio: riskReward,
		ATR:             atr,
	}, true
}
