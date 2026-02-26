// Package sdk provides a high-level client for the bullarc engine.
package sdk

import (
	"context"
	"fmt"

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
