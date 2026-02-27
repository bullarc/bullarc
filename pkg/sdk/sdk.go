// Package sdk provides a high-level client for the bullarc engine.
package sdk

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bullarc/bullarc"
)

// Client is a high-level SDK client wrapping the bullarc engine.
type Client struct {
	engine bullarc.Engine
}

// New creates a new SDK Client with the given engine.
func New(engine bullarc.Engine) *Client {
	return &Client{engine: engine}
}

// Analyze runs analysis for the given request through the underlying engine.
func (c *Client) Analyze(ctx context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
	return c.engine.Analyze(ctx, req)
}

// backtestEngine is satisfied by *engine.Engine when it has Backtest support.
type backtestEngine interface {
	Backtest(ctx context.Context, req bullarc.BacktestRequest) (bullarc.BacktestResult, error)
}

// Backtest runs a chronological backtest on the provided bars. The underlying engine
// must support backtesting; if it does not, an error is returned.
func (c *Client) Backtest(ctx context.Context, req bullarc.BacktestRequest) (bullarc.BacktestResult, error) {
	bt, ok := c.engine.(backtestEngine)
	if !ok {
		return bullarc.BacktestResult{}, fmt.Errorf("engine does not support backtesting")
	}
	return bt.Backtest(ctx, req)
}

// streamEngine is satisfied by *engine.Engine when it has Watch support.
type streamEngine interface {
	Watch(ctx context.Context, req bullarc.AnalysisRequest, pollInterval time.Duration, onResult func(bullarc.AnalysisResult)) error
}

// Stream polls the engine for new analysis results at pollInterval and delivers
// each Signal from every result to the returned channel. The channel is closed
// when ctx is cancelled. If the underlying engine does not support streaming,
// the channel is closed immediately.
//
// Signals within a single AnalysisResult are delivered in order; results from
// successive polls are appended in arrival order. Each signal is delivered
// exactly once.
func (c *Client) Stream(ctx context.Context, req bullarc.AnalysisRequest, pollInterval time.Duration) <-chan bullarc.Signal {
	ch := make(chan bullarc.Signal, 64)
	go func() {
		defer close(ch)
		we, ok := c.engine.(streamEngine)
		if !ok {
			return
		}
		_ = we.Watch(ctx, req, pollInterval, func(result bullarc.AnalysisResult) {
			for _, sig := range result.Signals {
				select {
				case ch <- sig:
				case <-ctx.Done():
					return
				}
			}
		})
	}()
	return ch
}

// StreamSymbols polls the engine for each symbol at pollInterval and delivers
// all resulting Signals to a single merged channel. Signals from different
// symbols are interleaved in arrival order. The channel is closed once all
// per-symbol goroutines have exited (ctx cancelled).
func (c *Client) StreamSymbols(ctx context.Context, symbols []string, pollInterval time.Duration) <-chan bullarc.Signal {
	ch := make(chan bullarc.Signal, 64)
	var wg sync.WaitGroup
	for _, sym := range symbols {
		wg.Add(1)
		go func(symbol string) {
			defer wg.Done()
			sub := c.Stream(ctx, bullarc.AnalysisRequest{Symbol: symbol}, pollInterval)
			for sig := range sub {
				select {
				case ch <- sig:
				case <-ctx.Done():
					return
				}
			}
		}(sym)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}
