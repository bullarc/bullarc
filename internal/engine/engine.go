// Package engine implements the core analysis engine.
package engine

import (
	"context"
	"log/slog"
	"time"

	"github.com/bullarcdev/bullarc"
)

// Engine orchestrates analysis by coordinating indicators, data sources,
// and LLM providers.
type Engine struct {
	indicators  map[string]bullarc.Indicator
	dataSources []bullarc.DataSource
	llmProvider bullarc.LLMProvider
}

// New creates a new Engine with default configuration.
func New() *Engine {
	return &Engine{
		indicators: make(map[string]bullarc.Indicator),
	}
}

// RegisterIndicator adds an indicator to the engine.
func (e *Engine) RegisterIndicator(ind bullarc.Indicator) {
	e.indicators[ind.Meta().Name] = ind
}

// RegisterDataSource adds a data source to the engine.
func (e *Engine) RegisterDataSource(ds bullarc.DataSource) {
	e.dataSources = append(e.dataSources, ds)
}

// RegisterLLMProvider sets the LLM provider for the engine.
func (e *Engine) RegisterLLMProvider(llm bullarc.LLMProvider) {
	e.llmProvider = llm
}

// Analyze performs analysis for the given request.
func (e *Engine) Analyze(ctx context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
	slog.Info("analysis started",
		"symbol", req.Symbol,
		"indicators", req.Indicators,
		"use_llm", req.UseLLM)

	result := bullarc.AnalysisResult{
		Symbol:    req.Symbol,
		Timestamp: time.Now(),
	}

	slog.Info("analysis complete",
		"symbol", req.Symbol,
		"signals", len(result.Signals))

	return result, nil
}
