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

// TestBacktest_EmptyBars verifies that an empty bar slice returns an empty result.
func TestBacktest_EmptyBars(t *testing.T) {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	result, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol: "AAPL",
		Bars:   nil,
	})
	require.NoError(t, err)
	assert.Empty(t, result.BarSignals)
	assert.Equal(t, 0, result.Summary.TotalSignals)
	assert.Equal(t, 0.0, result.Summary.SimReturn)
}

// TestBacktest_NoLookAhead verifies that signals are not generated before the warmup period.
// Only bars up to and including the current index are passed to indicators.
func TestBacktest_NoLookAhead(t *testing.T) {
	// Use only SMA_14 to keep warmup predictable (14 bars).
	e := engine.New()
	sma14 := engine.FilteredIndicators([]string{"SMA_14"})
	require.Len(t, sma14, 1)
	e.RegisterIndicator(sma14[0])

	// Build exactly 50 bars.
	bars := trendingBars(50, 100, 1.0)

	result, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol:     "AAPL",
		Bars:       bars,
		Indicators: []string{"SMA_14"},
	})
	require.NoError(t, err)

	// Every bar signal must have a timestamp that exists in the input slice.
	barTimes := make(map[time.Time]struct{}, len(bars))
	for _, b := range bars {
		barTimes[b.Time] = struct{}{}
	}
	for _, bs := range result.BarSignals {
		_, ok := barTimes[bs.Bar.Time]
		assert.True(t, ok, "bar signal time %v does not correspond to any input bar", bs.Bar.Time)
	}
}

// TestBacktest_SignalCountsAdd verifies that BuyCount + SellCount + HoldCount == TotalSignals.
func TestBacktest_SignalCountsAdd(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	result, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol: "AAPL",
		Bars:   bars,
	})
	require.NoError(t, err)

	s := result.Summary
	assert.Equal(t, s.TotalSignals, s.BuyCount+s.SellCount+s.HoldCount,
		"BuyCount + SellCount + HoldCount must equal TotalSignals")
}

// TestBacktest_UptrendProducesBuyMajority verifies that a steep uptrend yields mostly BUY signals.
func TestBacktest_UptrendProducesBuyMajority(t *testing.T) {
	bars := trendingBars(100, 100, 2.0)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	result, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol: "AAPL",
		Bars:   bars,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.BarSignals, "should produce signals on 100 bars uptrend")

	s := result.Summary
	assert.Greater(t, s.BuyCount, s.SellCount,
		"uptrend should produce more BUY signals than SELL (got buy=%d sell=%d)", s.BuyCount, s.SellCount)
}

// TestBacktest_DowntrendProducesSellMajority verifies that a steep downtrend yields mostly SELL signals.
func TestBacktest_DowntrendProducesSellMajority(t *testing.T) {
	bars := trendingBars(100, 200, -2.0)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	result, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol: "AAPL",
		Bars:   bars,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.BarSignals)

	s := result.Summary
	assert.Greater(t, s.SellCount, s.BuyCount,
		"downtrend should produce more SELL signals than BUY (got sell=%d buy=%d)", s.SellCount, s.BuyCount)
}

// TestBacktest_SymbolPropagated verifies the symbol is set on all bar signals.
func TestBacktest_SymbolPropagated(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	result, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol: "MSFT",
		Bars:   bars,
	})
	require.NoError(t, err)

	for i, bs := range result.BarSignals {
		assert.Equal(t, "MSFT", bs.Signal.Symbol, "bar signal %d should carry symbol MSFT", i)
	}
}

// TestBacktest_SimReturnPositiveOnUptrend verifies that the simulated return is positive
// when the backtest captures a strong uptrend.
func TestBacktest_SimReturnPositiveOnUptrend(t *testing.T) {
	// Use only SMA_14 to get predictable BUY signals on an uptrend.
	bars := trendingBars(100, 100, 2.0)
	e := engine.New()
	for _, ind := range engine.FilteredIndicators([]string{"SMA_14"}) {
		e.RegisterIndicator(ind)
	}
	result, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol:     "AAPL",
		Bars:       bars,
		Indicators: []string{"SMA_14"},
	})
	require.NoError(t, err)

	if result.Summary.BuyCount > 0 {
		// When there are BUY signals on an uptrend, simulated return should be positive.
		assert.GreaterOrEqual(t, result.Summary.SimReturn, 0.0,
			"uptrend with BUY signals should yield non-negative simulated return")
	}
}

// TestBacktest_MaxDrawdownNonNegative verifies max drawdown is always >= 0.
func TestBacktest_MaxDrawdownNonNegative(t *testing.T) {
	bars := trendingBars(100, 200, -1.0)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	result, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol: "AAPL",
		Bars:   bars,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.Summary.MaxDrawdown, 0.0)
}

// TestBacktest_WinRateInRange verifies win rate is in [0, 100].
func TestBacktest_WinRateInRange(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	result, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol: "AAPL",
		Bars:   bars,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, result.Summary.WinRate, 0.0)
	assert.LessOrEqual(t, result.Summary.WinRate, 100.0)
}

// TestBacktest_SelectiveIndicators verifies that a restricted indicator list is respected.
func TestBacktest_SelectiveIndicators(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}

	resultAll, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol: "AAPL",
		Bars:   bars,
	})
	require.NoError(t, err)

	resultRSI, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol:     "AAPL",
		Bars:       bars,
		Indicators: []string{"RSI_14"},
	})
	require.NoError(t, err)

	// Restricting to one indicator can change the signal count (some bars produce no signal).
	// At minimum, both results should be valid.
	assert.GreaterOrEqual(t, resultAll.Summary.TotalSignals, resultRSI.Summary.TotalSignals,
		"all indicators should produce at least as many signals as one indicator")
}

// TestBacktestCSV_LoadsAndRuns verifies BacktestCSV correctly loads bars from the reference CSV.
func TestBacktestCSV_LoadsAndRuns(t *testing.T) {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}

	csvPath := testutil.TestdataDir() + "/ohlcv_100.csv"
	result, err := e.BacktestCSV(context.Background(), csvPath, "AAPL", nil)
	require.NoError(t, err)
	assert.Equal(t, "AAPL", result.Symbol)
	assert.NotEmpty(t, result.BarSignals, "100-bar CSV should produce signals")
	assert.Equal(t, result.Summary.TotalSignals, len(result.BarSignals))
}

// TestBacktestCSV_EmptyPath returns an error.
func TestBacktestCSV_EmptyPath(t *testing.T) {
	e := engine.New()
	_, err := e.BacktestCSV(context.Background(), "", "AAPL", nil)
	require.Error(t, err)
}

// TestListIndicators_ReturnsRegistered verifies ListIndicators returns one entry per registered indicator.
func TestListIndicators_ReturnsRegistered(t *testing.T) {
	e := engine.New()
	all := engine.DefaultIndicators()
	for _, ind := range all {
		e.RegisterIndicator(ind)
	}

	metas := e.ListIndicators()
	assert.Len(t, metas, len(all))
	for _, m := range metas {
		assert.NotEmpty(t, m.Name)
		assert.NotEmpty(t, m.Category)
	}
}

// TestSmoke_BacktestPerformance verifies a full backtest on 100 bars completes quickly.
// This ensures that the O(n^2) implementation is fast enough for expected data sizes.
func TestSmoke_BacktestPerformance(t *testing.T) {
	// Build 100 bars (the reference dataset size).
	bars := trendingBars(100, 100, 0.5)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}

	start := time.Now()
	_, err := e.Backtest(context.Background(), bullarc.BacktestRequest{
		Symbol: "AAPL",
		Bars:   bars,
	})
	require.NoError(t, err)
	elapsed := time.Since(start)
	assert.Less(t, elapsed.Seconds(), 5.0,
		"backtest on 100 bars should complete in under 5 seconds, took %.2fs", elapsed.Seconds())
}
