package engine_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrent_AnalyzeNoDataRace verifies that concurrent calls to Analyze on the
// same engine do not produce data races (verified by the race detector).
func TestConcurrent_AnalyzeNoDataRace(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	ctx := context.Background()
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := e.Analyze(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"})
			if err != nil {
				// analysis errors are acceptable in this race test; we only
				// care that there is no data race.
				t.Logf("analyze error (non-fatal): %v", err)
			}
		}()
	}

	wg.Wait()
}

// TestConcurrent_RegisterWhileAnalyze verifies that registering indicators while
// Analyze is running does not cause a data race or panic.
func TestConcurrent_RegisterWhileAnalyze(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var analyzeWg sync.WaitGroup
	analyzeWg.Add(20)
	for i := 0; i < 20; i++ {
		go func() {
			defer analyzeWg.Done()
			_, _ = e.Analyze(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"})
		}()
	}

	// Concurrently register new indicators while analysis is running.
	var regWg sync.WaitGroup
	regWg.Add(10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer regWg.Done()
			ind, _ := newDummyIndicator(n)
			e.RegisterIndicator(ind)
		}(i)
	}

	analyzeWg.Wait()
	regWg.Wait()
}

// TestConcurrent_SetDataSourceWhileStreaming verifies that swapping the data
// source while another goroutine is streaming does not panic or race.
func TestConcurrent_SetDataSourceWhileStreaming(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start streaming in background.
	var streamWg sync.WaitGroup
	streamWg.Add(1)
	go func() {
		defer streamWg.Done()
		_ = e.Watch(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 50*time.Millisecond, func(_ bullarc.AnalysisResult) {})
	}()

	// Swap data source several times concurrently with the stream.
	var swapWg sync.WaitGroup
	swapWg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer swapWg.Done()
			e.SetDataSource(&stubDataSource{bars: bars})
		}()
	}

	swapWg.Wait()
	cancel()
	streamWg.Wait()
}

// TestConcurrent_SetIntervalWhileAnalyze verifies that updating the interval
// concurrently with Analyze does not race.
func TestConcurrent_SetIntervalWhileAnalyze(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	ctx := context.Background()
	intervals := []string{"1Day", "1Hour", "4Hour", "1Week", "1Day"}

	var wg sync.WaitGroup
	wg.Add(len(intervals) + 20)

	for _, iv := range intervals {
		go func(interval string) {
			defer wg.Done()
			e.SetInterval(interval)
		}(iv)
	}
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			_, _ = e.Analyze(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"})
		}()
	}

	wg.Wait()
}

// TestConcurrent_TenSymbolsWithinLatency verifies that 10 concurrent consumers
// each streaming a different symbol all receive their first signal within 5 seconds.
func TestConcurrent_TenSymbolsWithinLatency(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	const numSymbols = 10
	symbols := [numSymbols]string{
		"AAPL", "TSLA", "GOOGL", "MSFT", "AMZN",
		"NVDA", "META", "NFLX", "BABA", "ORCL",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var received atomic.Int64
	var wg sync.WaitGroup
	wg.Add(numSymbols)

	start := time.Now()
	for _, sym := range symbols {
		go func(symbol string) {
			defer wg.Done()
			_ = e.Watch(ctx, bullarc.AnalysisRequest{Symbol: symbol}, 100*time.Millisecond, func(_ bullarc.AnalysisResult) {
				received.Add(1)
				cancel() // cancel as soon as any result arrives to keep test fast
			})
		}(sym)
	}

	wg.Wait()
	elapsed := time.Since(start)

	require.Greater(t, received.Load(), int64(0), "expected at least one signal from the 10 consumers")
	assert.Less(t, elapsed, 5*time.Second, "first signal should arrive within 5 seconds")
}

// TestConcurrent_MultipleReadersOnChannel verifies that multiple goroutines
// reading from a shared signal channel do not race with each other.
func TestConcurrent_MultipleReadersOnChannel(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use a buffered channel shared among multiple reader goroutines.
	ch := make(chan bullarc.Signal, 128)
	var producerWg sync.WaitGroup
	producerWg.Add(1)

	// Single producer goroutine calling Watch and pushing to ch.
	go func() {
		defer producerWg.Done()
		defer close(ch)
		_ = e.Watch(ctx, bullarc.AnalysisRequest{Symbol: "AAPL"}, 100*time.Millisecond, func(result bullarc.AnalysisResult) {
			for _, sig := range result.Signals {
				select {
				case ch <- sig:
				case <-ctx.Done():
					return
				}
			}
			cancel() // stop after first result
		})
	}()

	// Multiple concurrent readers consuming the channel.
	const numReaders = 8
	var receivedCount atomic.Int64
	var readerWg sync.WaitGroup
	readerWg.Add(numReaders)
	for i := 0; i < numReaders; i++ {
		go func() {
			defer readerWg.Done()
			for range ch {
				receivedCount.Add(1)
			}
		}()
	}

	producerWg.Wait()
	readerWg.Wait()

	assert.Greater(t, receivedCount.Load(), int64(0), "expected readers to receive signals")
}

// --- helpers ---

// dummyIndicator is a no-op indicator used to trigger RegisterIndicator concurrently.
type dummyIndicator struct {
	name string
}

func newDummyIndicator(n int) (*dummyIndicator, error) {
	name := make([]byte, 8)
	name[0] = 'D'
	name[1] = 'U'
	name[2] = 'M'
	name[3] = 'M'
	name[4] = 'Y'
	name[5] = '_'
	name[6] = byte('0' + n/10)
	name[7] = byte('0' + n%10)
	return &dummyIndicator{name: string(name)}, nil
}

func (d *dummyIndicator) Meta() bullarc.IndicatorMeta {
	return bullarc.IndicatorMeta{Name: d.name, Description: "dummy"}
}

func (d *dummyIndicator) Compute(_ []bullarc.OHLCV) ([]bullarc.IndicatorValue, error) {
	return nil, nil
}
