package engine_test

import (
	"context"
	"testing"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/internal/engine"
	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDataSource is an in-memory data source used for engine tests.
type stubDataSource struct {
	bars []bullarc.OHLCV
}

func (s *stubDataSource) Meta() bullarc.DataSourceMeta {
	return bullarc.DataSourceMeta{Name: "stub", Description: "in-memory test data source"}
}

func (s *stubDataSource) Fetch(_ context.Context, _ bullarc.DataQuery) ([]bullarc.OHLCV, error) {
	return s.bars, nil
}

// newEngineWithBars builds an Engine with all default indicators and a stub data source.
func newEngineWithBars(bars []bullarc.OHLCV) *engine.Engine {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	return e
}

// trendingBars returns a synthetic uptrending bar series long enough to warm up all indicators.
func trendingBars(n int, startPrice, step float64) []bullarc.OHLCV {
	closes := make([]float64, n)
	for i := range closes {
		closes[i] = startPrice + float64(i)*step
	}
	return testutil.MakeBars(closes...)
}

// TestAnalyze_NoDataSource verifies that an engine with no data source returns no signals.
func TestAnalyze_NoDataSource(t *testing.T) {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Empty(t, result.Signals)
}

// TestAnalyze_NoBars verifies that empty bars from the data source produce no signals.
func TestAnalyze_NoBars(t *testing.T) {
	e := newEngineWithBars(nil)
	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Empty(t, result.Signals)
}

// TestAnalyze_CompositeSignalIsFirst verifies the first signal is always the composite.
func TestAnalyze_CompositeSignalIsFirst(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals, "expected signals with 100 bars")

	composite := result.Signals[0]
	assert.Equal(t, "composite", composite.Indicator)
	assert.Equal(t, "AAPL", composite.Symbol)
}

// TestAnalyze_CompositeTypeIsValid verifies the composite signal type is one of BUY, SELL, or HOLD.
func TestAnalyze_CompositeTypeIsValid(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	valid := composite.Type == bullarc.SignalBuy ||
		composite.Type == bullarc.SignalSell ||
		composite.Type == bullarc.SignalHold
	assert.True(t, valid, "composite type %q must be BUY, SELL, or HOLD", composite.Type)
}

// TestAnalyze_ConfidenceInRange verifies all signal confidence values are within [0, 100].
func TestAnalyze_ConfidenceInRange(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	for _, sig := range result.Signals {
		assert.GreaterOrEqual(t, sig.Confidence, 0.0, "signal %q confidence must be >= 0", sig.Indicator)
		assert.LessOrEqual(t, sig.Confidence, 100.0, "signal %q confidence must be <= 100", sig.Indicator)
	}
}

// TestAnalyze_IndicatorValuesPopulated verifies that indicator values are stored in the result.
func TestAnalyze_IndicatorValuesPopulated(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	assert.NotEmpty(t, result.IndicatorValues)
	assert.Contains(t, result.IndicatorValues, "RSI_14")
	assert.Contains(t, result.IndicatorValues, "MACD_12_26_9")
	assert.Contains(t, result.IndicatorValues, "BB_20_2.0")
}

// TestAnalyze_SelectiveIndicators verifies that requesting specific indicators limits the result.
func TestAnalyze_SelectiveIndicators(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol:     "AAPL",
		Indicators: []string{"RSI_14"},
	})
	require.NoError(t, err)

	assert.Contains(t, result.IndicatorValues, "RSI_14")
	assert.NotContains(t, result.IndicatorValues, "MACD_12_26_9")

	// Signals: composite + at most one individual (RSI_14).
	assert.LessOrEqual(t, len(result.Signals), 2)
}

// TestAnalyze_BullishCompositeBuy verifies a sustained uptrend produces a BUY composite.
// Prices rise strongly so that OBV, SMA cross, EMA cross, and VWAP all emit BUY.
func TestAnalyze_BullishCompositeBuy(t *testing.T) {
	// 100 bars with a steep uptrend: price rises from 100 to 200.
	bars := trendingBars(100, 100, 1.0)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.Equal(t, bullarc.SignalBuy, composite.Type,
		"steep uptrend should yield a BUY composite (got %s, explanation: %s)",
		composite.Type, composite.Explanation)
}

// TestAnalyze_BearishCompositeSell verifies a sustained downtrend produces a SELL composite.
func TestAnalyze_BearishCompositeSell(t *testing.T) {
	// 100 bars with a steep downtrend: price falls from 200 to 100.
	bars := trendingBars(100, 200, -1.0)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.Equal(t, bullarc.SignalSell, composite.Type,
		"steep downtrend should yield a SELL composite (got %s, explanation: %s)",
		composite.Type, composite.Explanation)
}

// TestAnalyze_SymbolPropagated verifies the symbol flows through to all signals.
func TestAnalyze_SymbolPropagated(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "TSLA"})
	require.NoError(t, err)

	for _, sig := range result.Signals {
		assert.Equal(t, "TSLA", sig.Symbol, "signal from %q should carry the request symbol", sig.Indicator)
	}
}

// TestFilteredIndicators_EmptyReturnsAll verifies that an empty enabled list returns all defaults.
func TestFilteredIndicators_EmptyReturnsAll(t *testing.T) {
	all := engine.DefaultIndicators()
	filtered := engine.FilteredIndicators(nil)
	assert.Equal(t, len(all), len(filtered))
}

// TestFilteredIndicators_Subset verifies that a non-empty enabled list filters correctly.
func TestFilteredIndicators_Subset(t *testing.T) {
	enabled := []string{"RSI_14", "MACD_12_26_9"}
	filtered := engine.FilteredIndicators(enabled)
	require.Len(t, filtered, 2)
	names := make([]string, len(filtered))
	for i, ind := range filtered {
		names[i] = ind.Meta().Name
	}
	assert.ElementsMatch(t, enabled, names)
}

// TestFilteredIndicators_UnknownNamesIgnored verifies that unknown names produce no entry.
func TestFilteredIndicators_UnknownNamesIgnored(t *testing.T) {
	filtered := engine.FilteredIndicators([]string{"RSI_14", "NONEXISTENT"})
	require.Len(t, filtered, 1)
	assert.Equal(t, "RSI_14", filtered[0].Meta().Name)
}

// TestFilteredIndicators_AllUnknownReturnsEmpty verifies all-unknown names yield empty slice.
func TestFilteredIndicators_AllUnknownReturnsEmpty(t *testing.T) {
	filtered := engine.FilteredIndicators([]string{"NONEXISTENT"})
	assert.Empty(t, filtered)
}

// TestSmoke_FullPipelineWithCSV is an end-to-end smoke test using the reference CSV dataset.
func TestSmoke_FullPipelineWithCSV(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals, "expected signals with 100 real bars")

	composite := result.Signals[0]
	assert.Equal(t, "composite", composite.Indicator)
	assert.Equal(t, "AAPL", composite.Symbol)
	assert.NotEmpty(t, composite.Explanation)

	// All default indicators should have produced values.
	assert.NotEmpty(t, result.IndicatorValues)
}
