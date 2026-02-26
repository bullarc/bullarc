package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/internal/engine"
	"github.com/bullarcdev/bullarc/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWatch_EmitsInitialResult verifies Watch calls onResult immediately with the first analysis.
func TestWatch_EmitsInitialResult(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var got []bullarc.AnalysisResult
	done := make(chan struct{})

	go func() {
		defer close(done)
		_ = e.Watch(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 500*time.Millisecond, func(r bullarc.AnalysisResult) {
			got = append(got, r)
			cancel() // cancel after first result
		})
	}()

	<-done
	require.NotEmpty(t, got, "expected at least one result")
	assert.Equal(t, "AAPL", got[0].Symbol)
	assert.NotEmpty(t, got[0].Signals)
}

// TestWatch_StopsOnContextCancel verifies Watch returns ctx.Err() when context is cancelled.
func TestWatch_StopsOnContextCancel(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := e.Watch(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 100*time.Millisecond, func(_ bullarc.AnalysisResult) {})
	assert.ErrorIs(t, err, context.Canceled)
}

// TestWatch_DeduplicatesUnchangedData verifies Watch does not re-emit when bars do not change.
func TestWatch_DeduplicatesUnchangedData(t *testing.T) {
	bars := testutil.LoadBarsFromCSV(t, "ohlcv_100.csv")
	e := newEngineWithBars(bars) // static bars – same result on every poll

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	var count int
	_ = e.Watch(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 50*time.Millisecond, func(_ bullarc.AnalysisResult) {
		count++
	})

	// Only the initial emission should occur because bars never change.
	assert.Equal(t, 1, count, "static data source should emit exactly once")
}

// TestWatch_NoDataSourceEmitsOnce verifies Watch still calls onResult (empty result) if no data source.
func TestWatch_NoDataSourceEmitsOnce(t *testing.T) {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var count int
	_ = e.Watch(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 50*time.Millisecond, func(_ bullarc.AnalysisResult) {
		count++
	})

	// No data source → empty result emitted exactly once on initial run; subsequent polls skip.
	assert.Equal(t, 1, count)
}
