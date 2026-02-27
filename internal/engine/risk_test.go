package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/bullarc/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeATRBars returns bars with predictable volatility so ATR is computable.
// Each bar has a range of `spread` around the close price.
func makeATRBars(closes []float64, spread float64) []bullarc.OHLCV {
	bars := make([]bullarc.OHLCV, len(closes))
	for i, c := range closes {
		bars[i] = bullarc.OHLCV{
			Time:   time.Now().AddDate(0, 0, -len(closes)+i),
			Open:   c,
			High:   c + spread,
			Low:    c - spread,
			Close:  c,
			Volume: 1_000_000,
		}
	}
	return bars
}

// newRiskEngine builds an engine with all default indicators (including ATR_14)
// and a stub data source.
func newRiskEngine(bars []bullarc.OHLCV) *engine.Engine {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	return e
}

// TestRiskMetrics_DisabledByDefault verifies that risk metrics are not populated
// when the engine uses its default configuration (disabled).
func TestRiskMetrics_DisabledByDefault(t *testing.T) {
	bars := trendingBars(100, 100, 1.0) // bullish
	e := newRiskEngine(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Nil(t, result.Risk, "risk metrics should be nil when disabled")
}

// TestRiskMetrics_EnabledBuySignal verifies that risk metrics are populated for a BUY signal.
func TestRiskMetrics_EnabledBuySignal(t *testing.T) {
	bars := trendingBars(100, 100, 1.0) // steep uptrend → BUY
	e := newRiskEngine(bars)
	e.SetRiskConfig(engine.RiskConfig{
		Enabled:              true,
		MaxPositionSizePct:   5.0,
		StopLossMultiplier:   2.0,
		TakeProfitMultiplier: 3.0,
		ATRIndicatorName:     "ATR_14",
	})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	if composite.Type == bullarc.SignalHold {
		t.Skip("composite is HOLD, risk metrics expected absent")
	}

	require.NotNil(t, result.Risk, "risk metrics should be set for a non-HOLD signal")
	risk := result.Risk

	assert.Greater(t, risk.ATR, 0.0, "ATR must be positive")
	assert.Greater(t, risk.PositionSizePct, 0.0, "position size must be positive")
	assert.LessOrEqual(t, risk.PositionSizePct, 5.0, "position size must not exceed max")
	assert.InDelta(t, 1.5, risk.RiskRewardRatio, 0.001, "risk/reward ratio must be 3/2 = 1.5")

	latestClose := bars[len(bars)-1].Close
	if composite.Type == bullarc.SignalBuy {
		assert.Less(t, risk.StopLoss, latestClose, "BUY stop-loss must be below entry")
		assert.Greater(t, risk.TakeProfit, latestClose, "BUY take-profit must be above entry")
	}
}

// TestRiskMetrics_EnabledSellSignal verifies stop/take-profit directions for a SELL signal.
func TestRiskMetrics_EnabledSellSignal(t *testing.T) {
	bars := trendingBars(100, 200, -1.0) // steep downtrend → SELL
	e := newRiskEngine(bars)
	e.SetRiskConfig(engine.RiskConfig{
		Enabled:              true,
		MaxPositionSizePct:   5.0,
		StopLossMultiplier:   2.0,
		TakeProfitMultiplier: 3.0,
		ATRIndicatorName:     "ATR_14",
	})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	if composite.Type != bullarc.SignalSell {
		t.Skipf("composite is %s, need SELL to test sell risk metrics", composite.Type)
	}

	require.NotNil(t, result.Risk)
	risk := result.Risk

	latestClose := bars[len(bars)-1].Close
	assert.Greater(t, risk.StopLoss, latestClose, "SELL stop-loss must be above entry")
	assert.Less(t, risk.TakeProfit, latestClose, "SELL take-profit must be below entry")
}

// TestRiskMetrics_HoldSignalOmitted verifies that risk metrics are nil for a HOLD signal.
func TestRiskMetrics_HoldSignalOmitted(t *testing.T) {
	// Flat bars produce a HOLD composite.
	closes := testutil.MakeBars(func() []float64 {
		v := make([]float64, 100)
		for i := range v {
			v[i] = 100.0
		}
		return v
	}()...)

	e := newRiskEngine(closes)
	e.SetRiskConfig(engine.RiskConfig{
		Enabled: true,
	})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	if len(result.Signals) == 0 {
		return
	}
	composite := result.Signals[0]
	if composite.Type == bullarc.SignalHold {
		assert.Nil(t, result.Risk, "risk metrics must be nil for HOLD signal")
	}
}

// TestRiskMetrics_MaxPositionSizeCap verifies position size never exceeds the configured max.
func TestRiskMetrics_MaxPositionSizeCap(t *testing.T) {
	// Use very tight spread bars so ATR is small → large uncapped position size.
	// The cap should always enforce 2.0% max.
	closes := make([]float64, 100)
	for i := range closes {
		closes[i] = 100.0 + float64(i)*0.1
	}
	bars := makeATRBars(closes, 0.01) // tiny spread → tiny ATR → large uncapped position

	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	e.SetRiskConfig(engine.RiskConfig{
		Enabled:              true,
		MaxPositionSizePct:   2.0,
		StopLossMultiplier:   2.0,
		TakeProfitMultiplier: 3.0,
		ATRIndicatorName:     "ATR_14",
	})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	if result.Risk == nil {
		// Composite may be HOLD; skip.
		return
	}
	assert.LessOrEqual(t, result.Risk.PositionSizePct, 2.0,
		"position size must never exceed MaxPositionSizePct")
}

// TestRiskMetrics_InverseProportion verifies higher ATR yields smaller position size.
func TestRiskMetrics_InverseProportion(t *testing.T) {
	// Low volatility: spread = 1
	lowVolCloses := make([]float64, 100)
	for i := range lowVolCloses {
		lowVolCloses[i] = 100.0 + float64(i)
	}
	lowVolBars := makeATRBars(lowVolCloses, 1.0)

	// High volatility: same closes but spread = 10 (5x more volatile)
	highVolBars := makeATRBars(lowVolCloses, 10.0)

	cfg := engine.RiskConfig{
		Enabled:              true,
		MaxPositionSizePct:   100.0, // disable cap so we can observe the inverse relationship
		StopLossMultiplier:   2.0,
		TakeProfitMultiplier: 3.0,
		ATRIndicatorName:     "ATR_14",
	}

	runAnalysis := func(bars []bullarc.OHLCV) *bullarc.RiskMetrics {
		e := engine.New()
		for _, ind := range engine.DefaultIndicators() {
			e.RegisterIndicator(ind)
		}
		e.RegisterDataSource(&stubDataSource{bars: bars})
		e.SetRiskConfig(cfg)
		result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "TEST"})
		if err != nil || result.Risk == nil {
			return nil
		}
		return result.Risk
	}

	lowRisk := runAnalysis(lowVolBars)
	highRisk := runAnalysis(highVolBars)

	if lowRisk == nil || highRisk == nil {
		t.Skip("composite was HOLD for one of the bar sets, skipping inverse proportion check")
	}

	assert.Greater(t, lowRisk.ATR, 0.0)
	assert.Greater(t, highRisk.ATR, 0.0)
	// Higher ATR → smaller position.
	assert.Greater(t, highRisk.ATR, lowRisk.ATR,
		"high-vol bars must have larger ATR")
	assert.Less(t, highRisk.PositionSizePct, lowRisk.PositionSizePct,
		"higher ATR must yield a smaller position size")
}

// TestRiskMetrics_InsufficientATRData verifies that risk metrics are nil when ATR
// is not present in indicator values (e.g., not enough bars for ATR warmup).
func TestRiskMetrics_InsufficientATRData(t *testing.T) {
	// Only 5 bars — not enough to warm up ATR_14 (needs 15).
	bars := trendingBars(5, 100, 1.0)

	e := newRiskEngine(bars)
	e.SetRiskConfig(engine.RiskConfig{
		Enabled:              true,
		MaxPositionSizePct:   5.0,
		StopLossMultiplier:   2.0,
		TakeProfitMultiplier: 3.0,
		ATRIndicatorName:     "ATR_14",
	})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	// No bars produced (skipped), so no risk metrics.
	assert.Nil(t, result.Risk, "risk metrics must be nil when ATR is unavailable")
}

// TestRiskMetrics_DefaultsApplied verifies that zero-valued RiskConfig fields fall back
// to the package defaults (5%, 2x, 3x).
func TestRiskMetrics_DefaultsApplied(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	e := newRiskEngine(bars)
	// Set enabled but leave multipliers/max at zero → defaults should be applied.
	e.SetRiskConfig(engine.RiskConfig{
		Enabled:          true,
		ATRIndicatorName: "ATR_14",
	})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	if composite.Type == bullarc.SignalHold || result.Risk == nil {
		return
	}

	assert.LessOrEqual(t, result.Risk.PositionSizePct, engine.DefaultMaxPositionSizePct+1e-9)
	assert.InDelta(t, engine.DefaultTakeProfitMultiplier/engine.DefaultStopLossMultiplier,
		result.Risk.RiskRewardRatio, 0.001)
}
