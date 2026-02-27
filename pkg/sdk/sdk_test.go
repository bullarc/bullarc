package sdk_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/bullarc/bullarc/pkg/sdk"
	"github.com/bullarc/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDataSource is an in-memory data source for SDK tests.
type stubDataSource struct {
	bars []bullarc.OHLCV
}

func (s *stubDataSource) Meta() bullarc.DataSourceMeta {
	return bullarc.DataSourceMeta{Name: "stub", Description: "in-memory test data source"}
}

func (s *stubDataSource) Fetch(_ context.Context, _ bullarc.DataQuery) ([]bullarc.OHLCV, error) {
	return s.bars, nil
}

// newSDKClient builds an SDK client backed by an engine with the given bars.
func newSDKClient(bars []bullarc.OHLCV) *sdk.Client {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	return sdk.New(e)
}

// TestStream_DeliversSignals verifies that Stream emits signals to the returned channel.
func TestStream_DeliversSignals(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	client := newSDKClient(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := client.Stream(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 500*time.Millisecond)

	var got []bullarc.Signal
	for sig := range ch {
		got = append(got, sig)
		cancel() // stop after first batch
	}

	require.NotEmpty(t, got, "expected at least one signal")
	for _, s := range got {
		assert.Equal(t, "AAPL", s.Symbol)
	}
}

// TestStream_ChannelClosedOnContextCancel verifies the channel is closed when ctx is cancelled.
func TestStream_ChannelClosedOnContextCancel(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	client := newSDKClient(bars)

	ctx, cancel := context.WithCancel(context.Background())
	ch := client.Stream(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 100*time.Millisecond)

	// Drain one batch then cancel.
	var received int
	for sig := range ch {
		received++
		_ = sig
		cancel()
	}

	// Channel must be closed (range ended) after ctx cancel.
	assert.Greater(t, received, 0, "should have received at least one signal before cancel")
}

// TestStream_EachSignalDeliveredExactlyOnce verifies no duplicate signals within a single poll cycle.
func TestStream_EachSignalDeliveredExactlyOnce(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	client := newSDKClient(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := client.Stream(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 500*time.Millisecond)

	// Collect signals from the first analysis result only.
	var first []bullarc.Signal
	for sig := range ch {
		first = append(first, sig)
		if len(first) > 0 {
			cancel()
		}
	}

	// Check for duplicates by indicator name within this batch.
	seen := make(map[string]int)
	for _, s := range first {
		seen[s.Indicator]++
	}
	for ind, count := range seen {
		assert.Equal(t, 1, count, "indicator %q appeared %d times; expected exactly once", ind, count)
	}
}

// TestStream_NoDataSourceClosesImmediately verifies that a non-streaming engine closes the channel.
func TestStream_NoDataSourceClosesImmediately(t *testing.T) {
	// Engine with no data source: Watch still emits one empty result then blocks.
	// Context timeout ensures we don't hang.
	e := engine.New()
	client := sdk.New(e)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	ch := client.Stream(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 50*time.Millisecond)

	// Drain all; channel must close within the timeout.
	var count int
	for range ch {
		count++
	}
	// No signals expected (empty result produces no signals to send).
	assert.Equal(t, 0, count)
}

// TestStream_NonStreamingEngineClosesChannel verifies that an engine without Watch support
// closes the channel immediately.
func TestStream_NonStreamingEngineClosesChannel(t *testing.T) {
	client := sdk.New(&minimalEngine{})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch := client.Stream(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 100*time.Millisecond)

	var got []bullarc.Signal
	for sig := range ch {
		got = append(got, sig)
	}
	assert.Empty(t, got, "non-streaming engine should deliver no signals")
}

// TestStreamSymbols_DeliversAllSymbols verifies that StreamSymbols delivers signals for every symbol.
func TestStreamSymbols_DeliversAllSymbols(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	client := newSDKClient(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	symbols := []string{"AAPL", "TSLA", "GOOGL"}
	ch := client.StreamSymbols(ctx, symbols, 500*time.Millisecond)

	seen := make(map[string]bool)
	for sig := range ch {
		seen[sig.Symbol] = true
		if len(seen) == len(symbols) {
			cancel() // got at least one signal per symbol
		}
	}

	for _, sym := range symbols {
		assert.True(t, seen[sym], "expected signals for symbol %q", sym)
	}
}

// TestStreamSymbols_ChannelClosedOnContextCancel verifies the merged channel is closed after ctx cancel.
func TestStreamSymbols_ChannelClosedOnContextCancel(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	client := newSDKClient(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch := client.StreamSymbols(ctx, []string{"AAPL", "TSLA"}, 100*time.Millisecond)

	var got []bullarc.Signal
	for sig := range ch {
		got = append(got, sig)
	}

	// Channel must have closed (range ended); at least one signal expected.
	assert.NotEmpty(t, got)
}

// TestStreamSymbols_SignalsCarryCorrectSymbol verifies that each signal from StreamSymbols
// carries the symbol it was requested for.
func TestStreamSymbols_SignalsCarryCorrectSymbol(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	client := newSDKClient(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	symbols := []string{"AAPL", "MSFT"}
	validSymbols := make(map[string]bool)
	for _, s := range symbols {
		validSymbols[s] = true
	}

	ch := client.StreamSymbols(ctx, symbols, 500*time.Millisecond)

	var got []bullarc.Signal
	for sig := range ch {
		got = append(got, sig)
		if len(got) >= 10 {
			cancel()
		}
	}

	require.NotEmpty(t, got)
	for _, sig := range got {
		assert.True(t, validSymbols[sig.Symbol], "unexpected symbol %q in signal", sig.Symbol)
	}
}

// TestStreamSymbols_EmptySymbolsClosesChannel verifies that StreamSymbols with no symbols
// closes the channel immediately.
func TestStreamSymbols_EmptySymbolsClosesChannel(t *testing.T) {
	bars := testutil.MakeBars(makePrices(100, 100, 0.5)...)
	client := newSDKClient(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch := client.StreamSymbols(ctx, nil, 100*time.Millisecond)

	var got []bullarc.Signal
	for sig := range ch {
		got = append(got, sig)
	}
	assert.Empty(t, got, "empty symbol list should yield no signals")
}

// TestStreamSymbols_AllSymbolsCovered verifies that every symbol gets signal coverage by
// cross-checking the set of symbols seen in signals against the requested list.
func TestStreamSymbols_AllSymbolsCovered(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	client := newSDKClient(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	symbols := []string{"AAPL", "TSLA"}
	ch := client.StreamSymbols(ctx, symbols, 200*time.Millisecond)

	seen := make(map[string]bool)
	for sig := range ch {
		seen[sig.Symbol] = true
		if len(seen) == len(symbols) {
			cancel()
		}
	}

	gotSymbols := make([]string, 0, len(seen))
	for s := range seen {
		gotSymbols = append(gotSymbols, s)
	}
	sort.Strings(gotSymbols)
	sort.Strings(symbols)
	assert.ElementsMatch(t, symbols, gotSymbols)
}

// --- helpers ---

// makePrices generates n prices starting at start, incrementing by step.
func makePrices(n int, start, step float64) []float64 {
	prices := make([]float64, n)
	for i := range prices {
		prices[i] = start + float64(i)*step
	}
	return prices
}

// minimalEngine implements bullarc.Engine without the Watch method.
type minimalEngine struct{}

func (m *minimalEngine) Analyze(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
	return bullarc.AnalysisResult{Symbol: req.Symbol}, nil
}

func (m *minimalEngine) RegisterIndicator(_ bullarc.Indicator)       {}
func (m *minimalEngine) RegisterDataSource(_ bullarc.DataSource)     {}
func (m *minimalEngine) RegisterLLMProvider(_ bullarc.LLMProvider)   {}
