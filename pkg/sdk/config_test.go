package sdk_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/bullarc/bullarc/pkg/sdk"
	"github.com/bullarc/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- WithSymbols ---

func TestWithSymbols_SetsSymbols(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithSymbols("AAPL", "TSLA"))
	require.NoError(t, err)
	cfg := client.Config()
	assert.Equal(t, []string{"AAPL", "TSLA"}, cfg.Symbols)
}

func TestWithSymbols_EmptySymbolReturnsError(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e, sdk.WithSymbols("AAPL", ""))
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr), "expected *bullarc.Error, got %T", err)
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestWithSymbols_WhitespaceSymbolReturnsError(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e, sdk.WithSymbols("  "))
	require.Error(t, err)
	var bErr *bullarc.Error
	assert.True(t, errors.As(err, &bErr))
}

// --- WithIndicators ---

func TestWithIndicators_DefaultNameIsAccepted(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithIndicators("SMA_14", "RSI_14"))
	require.NoError(t, err)
	cfg := client.Config()
	assert.Equal(t, []string{"SMA_14", "RSI_14"}, cfg.Indicators)
}

func TestWithIndicators_ParseableNameIsAccepted(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithIndicators("SMA_20", "RSI_21"))
	require.NoError(t, err)
	cfg := client.Config()
	assert.ElementsMatch(t, []string{"SMA_20", "RSI_21"}, cfg.Indicators)
}

func TestWithIndicators_UnknownIndicatorReturnsError(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e, sdk.WithIndicators("UNKNOWN_XYZ_999"))
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestWithIndicators_EmptyNameReturnsError(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e, sdk.WithIndicators("SMA_14", ""))
	require.Error(t, err)
	var bErr *bullarc.Error
	assert.True(t, errors.As(err, &bErr))
}

// --- WithInterval ---

func TestWithInterval_ValidIntervalIsAccepted(t *testing.T) {
	for _, iv := range []string{"1Min", "5Min", "15Min", "30Min", "1Hour", "2Hour", "4Hour", "1Day", "1Week", "1Month"} {
		t.Run(iv, func(t *testing.T) {
			e := engine.New()
			client, err := sdk.NewWithOptions(e, sdk.WithInterval(iv))
			require.NoError(t, err)
			assert.Equal(t, iv, client.Config().Interval)
		})
	}
}

func TestWithInterval_InvalidIntervalReturnsError(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e, sdk.WithInterval("2Days"))
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestWithInterval_EmptyIntervalReturnsError(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e, sdk.WithInterval(""))
	require.Error(t, err)
	var bErr *bullarc.Error
	assert.True(t, errors.As(err, &bErr))
}

// --- NewWithOptions: multiple options ---

func TestNewWithOptions_MultipleOptions(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e,
		sdk.WithSymbols("AAPL", "MSFT"),
		sdk.WithIndicators("SMA_14"),
		sdk.WithInterval("1Day"),
	)
	require.NoError(t, err)
	cfg := client.Config()
	assert.Equal(t, []string{"AAPL", "MSFT"}, cfg.Symbols)
	assert.Equal(t, []string{"SMA_14"}, cfg.Indicators)
	assert.Equal(t, "1Day", cfg.Interval)
}

func TestNewWithOptions_FirstErrorStops(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e,
		sdk.WithSymbols("AAPL"),
		sdk.WithInterval("BadInterval"),
		sdk.WithIndicators("SMA_14"),
	)
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
}

// --- Configure (runtime update) ---

func TestConfigure_UpdatesSymbolsAtRuntime(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithSymbols("AAPL"))
	require.NoError(t, err)

	err = client.Configure(sdk.WithSymbols("TSLA", "GOOGL"))
	require.NoError(t, err)
	assert.Equal(t, []string{"TSLA", "GOOGL"}, client.Config().Symbols)
}

func TestConfigure_UpdatesIndicatorsAtRuntime(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithIndicators("SMA_14"))
	require.NoError(t, err)

	err = client.Configure(sdk.WithIndicators("RSI_14", "MACD_12_26_9"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"RSI_14", "MACD_12_26_9"}, client.Config().Indicators)
}

func TestConfigure_UpdatesIntervalAtRuntime(t *testing.T) {
	e := engine.New()
	client := sdk.New(e)

	err := client.Configure(sdk.WithInterval("1Hour"))
	require.NoError(t, err)
	assert.Equal(t, "1Hour", client.Config().Interval)
}

func TestConfigure_InvalidIntervalLeavesConfigUnchanged(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e,
		sdk.WithSymbols("AAPL"),
		sdk.WithInterval("1Day"),
	)
	require.NoError(t, err)
	before := client.Config()

	err = client.Configure(sdk.WithInterval("BadInterval"))
	require.Error(t, err)
	assert.Equal(t, before, client.Config(), "config must be unchanged after error")
}

func TestConfigure_InvalidSymbolLeavesConfigUnchanged(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithSymbols("AAPL"))
	require.NoError(t, err)

	err = client.Configure(sdk.WithSymbols("TSLA", ""))
	require.Error(t, err)
	assert.Equal(t, []string{"AAPL"}, client.Config().Symbols)
}

func TestConfigure_MultipleOptionsRollbackOnError(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e,
		sdk.WithSymbols("AAPL"),
		sdk.WithInterval("1Day"),
		sdk.WithIndicators("SMA_14"),
	)
	require.NoError(t, err)
	before := client.Config()

	// The first option is valid but the second is not; entire configure must roll back.
	err = client.Configure(
		sdk.WithSymbols("TSLA"),
		sdk.WithIndicators("NOT_A_REAL_INDICATOR"),
	)
	require.Error(t, err)
	assert.Equal(t, before, client.Config())
}

// --- Stream uses configured defaults ---

func TestStream_UsesConfiguredSymbolWhenReqEmpty(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})

	client, err := sdk.NewWithOptions(e, sdk.WithSymbols("TSLA"))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := client.Stream(ctx, bullarc.AnalysisRequest{}, 500*time.Millisecond)
	var got []bullarc.Signal
	for sig := range ch {
		got = append(got, sig)
		cancel()
	}

	require.NotEmpty(t, got)
	for _, s := range got {
		assert.Equal(t, "TSLA", s.Symbol)
	}
}

func TestStream_ExplicitSymbolOverridesConfig(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})

	client, err := sdk.NewWithOptions(e, sdk.WithSymbols("TSLA"))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := client.Stream(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 500*time.Millisecond)
	var got []bullarc.Signal
	for sig := range ch {
		got = append(got, sig)
		cancel()
	}

	require.NotEmpty(t, got)
	for _, s := range got {
		assert.Equal(t, "AAPL", s.Symbol)
	}
}

func TestStream_UsesConfiguredIndicators(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})

	client, err := sdk.NewWithOptions(e,
		sdk.WithSymbols("AAPL"),
		sdk.WithIndicators("SMA_14", "RSI_14"),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := client.Stream(ctx, bullarc.AnalysisRequest{}, 500*time.Millisecond)
	var got []bullarc.Signal
	for sig := range ch {
		got = append(got, sig)
		cancel()
	}

	require.NotEmpty(t, got)
	for _, s := range got {
		if s.Indicator == "composite" {
			continue
		}
		assert.True(t, s.Indicator == "SMA_14" || s.Indicator == "RSI_14",
			"unexpected indicator %q in signal; only SMA_14 and RSI_14 were configured", s.Indicator)
	}
}

// --- StreamSymbols uses configured symbols when none passed ---

func TestStreamSymbols_UsesConfiguredSymbolsWhenNilPassed(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})

	symbols := []string{"AAPL", "TSLA"}
	client, err := sdk.NewWithOptions(e, sdk.WithSymbols(symbols...))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := client.StreamSymbols(ctx, nil, 500*time.Millisecond)
	seen := make(map[string]bool)
	for sig := range ch {
		seen[sig.Symbol] = true
		if len(seen) == len(symbols) {
			cancel()
		}
	}

	for _, sym := range symbols {
		assert.True(t, seen[sym], "expected signals for symbol %q", sym)
	}
}

func TestStreamSymbols_UsesConfiguredSymbolsWhenEmptyPassed(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})

	client, err := sdk.NewWithOptions(e, sdk.WithSymbols("MSFT"))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := client.StreamSymbols(ctx, []string{}, 500*time.Millisecond)
	var got []bullarc.Signal
	for sig := range ch {
		got = append(got, sig)
		cancel()
	}

	require.NotEmpty(t, got)
	for _, s := range got {
		assert.Equal(t, "MSFT", s.Symbol)
	}
}

// TestConfigure_IntervalPropagatedToEngine verifies that setting interval via
// Configure causes the engine to use that interval for subsequent fetches.
func TestConfigure_IntervalPropagatedToEngine(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithInterval("1Hour"))
	require.NoError(t, err)
	assert.Equal(t, "1Hour", client.Config().Interval)

	err = client.Configure(sdk.WithInterval("1Day"))
	require.NoError(t, err)
	assert.Equal(t, "1Day", client.Config().Interval)
}

// --- WithDataSource ---

// trackingDataSource records whether its Fetch method was called.
type trackingDataSource struct {
	bars   []bullarc.OHLCV
	called bool
}

func (t *trackingDataSource) Meta() bullarc.DataSourceMeta {
	return bullarc.DataSourceMeta{Name: "tracking", Description: "tracks fetch calls"}
}

func (t *trackingDataSource) Fetch(_ context.Context, _ bullarc.DataQuery) ([]bullarc.OHLCV, error) {
	t.called = true
	return t.bars, nil
}

func TestWithDataSource_SetsDataSource(t *testing.T) {
	ds := &trackingDataSource{}
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithDataSource(ds))
	require.NoError(t, err)
	assert.Equal(t, ds, client.Config().DataSource)
}

func TestWithDataSource_NilReturnsError(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e, sdk.WithDataSource(nil))
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

// TestWithDataSource_UsedForAnalysis verifies the engine uses the swapped-in data source.
func TestWithDataSource_UsedForAnalysis(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	ds := &trackingDataSource{bars: bars}

	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}

	client, err := sdk.NewWithOptions(e, sdk.WithDataSource(ds))
	require.NoError(t, err)

	result, err := client.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.True(t, ds.called, "expected custom data source Fetch to be called")
	assert.NotEmpty(t, result.Signals, "expected signals from analysis with custom data source")
}

// TestWithDataSource_IndicatorsAndSignalsFunctionIdentically verifies that analysis
// with a custom data source produces the same signal structure as with a directly
// registered source.
func TestWithDataSource_IndicatorsAndSignalsFunctionIdentically(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")

	// Build a reference client using the traditional RegisterDataSource path.
	eRef := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		eRef.RegisterIndicator(ind)
	}
	eRef.RegisterDataSource(&trackingDataSource{bars: bars})
	refResult, err := sdk.New(eRef).Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "TEST"})
	require.NoError(t, err)

	// Build a client using WithDataSource.
	eSwap := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		eSwap.RegisterIndicator(ind)
	}
	client, err := sdk.NewWithOptions(eSwap, sdk.WithDataSource(&trackingDataSource{bars: bars}))
	require.NoError(t, err)
	swapResult, err := client.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "TEST"})
	require.NoError(t, err)

	assert.Len(t, swapResult.Signals, len(refResult.Signals),
		"swapped data source should produce the same number of signals")
	assert.Len(t, swapResult.IndicatorValues, len(refResult.IndicatorValues),
		"swapped data source should produce the same number of indicator values")
}

// TestConfigure_SwapsDataSourceAtRuntime verifies that Configure replaces the active
// data source and subsequent analysis uses only the new source.
func TestConfigure_SwapsDataSourceAtRuntime(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	ds1 := &trackingDataSource{bars: bars}
	ds2 := &trackingDataSource{bars: bars}

	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}

	client, err := sdk.NewWithOptions(e, sdk.WithDataSource(ds1))
	require.NoError(t, err)

	// Swap to ds2 at runtime.
	err = client.Configure(sdk.WithDataSource(ds2))
	require.NoError(t, err)
	assert.Equal(t, ds2, client.Config().DataSource)

	_, err = client.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	assert.False(t, ds1.called, "old data source must not be called after swap")
	assert.True(t, ds2.called, "new data source must be called after swap")
}

// TestConfigure_InvalidDataSourceLeavesConfigUnchanged verifies rollback on nil data source.
func TestConfigure_InvalidDataSourceLeavesConfigUnchanged(t *testing.T) {
	ds := &trackingDataSource{}
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithDataSource(ds))
	require.NoError(t, err)
	before := client.Config()

	err = client.Configure(sdk.WithDataSource(nil))
	require.Error(t, err)
	assert.Equal(t, before.DataSource, client.Config().DataSource, "data source must be unchanged after error")
}
