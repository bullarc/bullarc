// Package sdk provides a high-level client for the bullarc engine.
package sdk

import (
	"context"

	"github.com/bullarcdev/bullarc"
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
