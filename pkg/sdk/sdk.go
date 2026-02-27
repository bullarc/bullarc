// Package sdk provides a high-level client for the bullarc engine.
package sdk

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bullarc/bullarc"
)

// intervalEngine is satisfied by engines that expose a runtime interval setter.
type intervalEngine interface {
	SetInterval(string)
}

// dataSourceSetter is satisfied by engines that support replacing their primary data source.
type dataSourceSetter interface {
	SetDataSource(ds bullarc.DataSource)
}

// Client is a high-level SDK client wrapping the bullarc engine.
type Client struct {
	engine bullarc.Engine
	mu     sync.RWMutex
	cfg    ClientConfig
}

// New creates a new SDK Client with the given engine and default configuration.
func New(engine bullarc.Engine) *Client {
	return &Client{engine: engine}
}

// NewWithOptions creates a new SDK Client and applies the given options at
// construction time. Returns a typed *bullarc.Error if any option is invalid.
func NewWithOptions(eng bullarc.Engine, opts ...Option) (*Client, error) {
	c := &Client{engine: eng}
	for _, opt := range opts {
		if err := opt(&c.cfg); err != nil {
			return nil, err
		}
	}
	c.propagateConfig()
	return c, nil
}

// Configure applies options to the client at runtime, updating the active
// configuration. On error the configuration is left unchanged.
func (c *Client) Configure(opts ...Option) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Apply to a draft copy so we can roll back on error.
	draft := ClientConfig{
		Symbols:    cloneStrings(c.cfg.Symbols),
		Indicators: cloneStrings(c.cfg.Indicators),
		Interval:   c.cfg.Interval,
		DataSource: c.cfg.DataSource,
	}
	for _, opt := range opts {
		if err := opt(&draft); err != nil {
			return err
		}
	}
	c.cfg = draft
	c.propagateConfig()
	return nil
}

// Config returns a snapshot of the current client configuration.
func (c *Client) Config() ClientConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ClientConfig{
		Symbols:    cloneStrings(c.cfg.Symbols),
		Indicators: cloneStrings(c.cfg.Indicators),
		Interval:   c.cfg.Interval,
		DataSource: c.cfg.DataSource,
	}
}

// Analyze runs analysis for the given request through the underlying engine,
// applying configured defaults for missing symbol and indicators.
func (c *Client) Analyze(ctx context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
	return c.engine.Analyze(ctx, c.applyConfigToReq(req))
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
// If req.Symbol is empty and the client has configured symbols, the first
// configured symbol is used. If req.Indicators is empty and the client has
// configured indicators, those are used.
//
// Signals within a single AnalysisResult are delivered in order; results from
// successive polls are appended in arrival order. Each signal is delivered
// exactly once.
func (c *Client) Stream(ctx context.Context, req bullarc.AnalysisRequest, pollInterval time.Duration) <-chan bullarc.Signal {
	req = c.applyConfigToReq(req)
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
// all resulting Signals to a single merged channel. If symbols is nil or empty
// and the client has configured symbols, those are used. Signals from different
// symbols are interleaved in arrival order. The channel is closed once all
// per-symbol goroutines have exited (ctx cancelled).
func (c *Client) StreamSymbols(ctx context.Context, symbols []string, pollInterval time.Duration) <-chan bullarc.Signal {
	if len(symbols) == 0 {
		c.mu.RLock()
		symbols = cloneStrings(c.cfg.Symbols)
		c.mu.RUnlock()
	}
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

// applyConfigToReq returns req enriched with client-level defaults.
// Symbol and Indicators from cfg are applied only when the request's own
// fields are zero/empty.
func (c *Client) applyConfigToReq(req bullarc.AnalysisRequest) bullarc.AnalysisRequest {
	c.mu.RLock()
	cfg := ClientConfig{
		Symbols:    cloneStrings(c.cfg.Symbols),
		Indicators: cloneStrings(c.cfg.Indicators),
		Interval:   c.cfg.Interval,
	}
	c.mu.RUnlock()

	if req.Symbol == "" && len(cfg.Symbols) > 0 {
		req.Symbol = cfg.Symbols[0]
	}
	if len(req.Indicators) == 0 && len(cfg.Indicators) > 0 {
		req.Indicators = cfg.Indicators
	}
	return req
}

// propagateConfig pushes configuration to the underlying engine where possible.
// Must be called while c.mu is held for writing (or during construction).
func (c *Client) propagateConfig() {
	if c.cfg.Interval != "" {
		if ie, ok := c.engine.(intervalEngine); ok {
			ie.SetInterval(c.cfg.Interval)
		}
	}
	if c.cfg.DataSource != nil {
		if ds, ok := c.engine.(dataSourceSetter); ok {
			ds.SetDataSource(c.cfg.DataSource)
		}
	}
}

func cloneStrings(s []string) []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s))
	copy(out, s)
	return out
}
