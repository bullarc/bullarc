// Package engine implements the core analysis engine.
package engine

import (
	"context"
	"log/slog"
	"time"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/internal/config"
	"github.com/bullarcdev/bullarc/internal/signal"
	"github.com/bullarcdev/bullarc/internal/webhook"
)

// Engine orchestrates analysis by coordinating indicators, data sources,
// and LLM providers.
type Engine struct {
	indicators        map[string]bullarc.Indicator
	dataSources       []bullarc.DataSource
	llmProvider       bullarc.LLMProvider
	webhookDispatcher *webhook.Dispatcher
	// lookback is the number of calendar days of history to request per analysis.
	lookback int
	// interval is the default bar interval passed to the data source.
	interval string
}

// New creates a new Engine with default configuration.
func New() *Engine {
	return &Engine{
		indicators: make(map[string]bullarc.Indicator),
		lookback:   200,
		interval:   "1Day",
	}
}

// NewWithConfig creates an Engine pre-configured from cfg.
// Lookback and interval are set from EngineConfig if non-zero/non-empty.
// Indicators are built from IndicatorsConfig (empty Enabled = all defaults).
// Data sources and LLM providers must still be registered separately.
func NewWithConfig(cfg *config.Config) *Engine {
	e := New()
	if cfg.Engine.MaxBars > 0 {
		e.lookback = cfg.Engine.MaxBars
	}
	if cfg.Engine.DefaultInterval != "" {
		e.interval = cfg.Engine.DefaultInterval
	}
	for _, ind := range IndicatorsFromConfig(cfg.Indicators) {
		e.RegisterIndicator(ind)
	}
	if cfg.Webhooks.Enabled && len(cfg.Webhooks.URLs) > 0 {
		e.RegisterWebhookDispatcher(webhook.New(cfg.Webhooks.URLs, cfg.Webhooks.Timeout))
	}
	return e
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

// RegisterWebhookDispatcher attaches a webhook dispatcher that receives each
// AnalysisResult immediately after Analyze completes. Dispatch errors are
// logged but do not affect the returned result.
func (e *Engine) RegisterWebhookDispatcher(d *webhook.Dispatcher) {
	e.webhookDispatcher = d
}

// Analyze fetches market data, computes indicators, generates per-indicator signals,
// and aggregates them into a composite BUY/SELL/HOLD signal.
func (e *Engine) Analyze(ctx context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
	slog.Info("analysis started",
		"symbol", req.Symbol,
		"indicators", req.Indicators,
		"use_llm", req.UseLLM)

	result := bullarc.AnalysisResult{
		Symbol:          req.Symbol,
		Timestamp:       time.Now(),
		IndicatorValues: make(map[string][]bullarc.IndicatorValue),
	}

	bars, err := e.fetchBars(ctx, req.Symbol)
	if err != nil {
		return result, err
	}
	if len(bars) == 0 {
		slog.Warn("no bars available, skipping analysis", "symbol", req.Symbol)
		return result, nil
	}

	indicators := e.selectIndicators(req.Indicators)
	latestBar := bars[len(bars)-1]
	var indSignals []bullarc.Signal

	for _, ind := range indicators {
		name := ind.Meta().Name
		values, err := ind.Compute(bars)
		if err != nil {
			slog.Warn("indicator compute failed", "indicator", name, "err", err)
			continue
		}
		result.IndicatorValues[name] = values

		gen := signal.ForIndicator(name)
		if gen == nil {
			continue
		}
		sig, ok := gen(name, req.Symbol, latestBar, values)
		if ok {
			indSignals = append(indSignals, sig)
		}
	}

	composite := signal.Aggregate(req.Symbol, indSignals)
	result.Signals = append([]bullarc.Signal{composite}, indSignals...)

	slog.Info("analysis complete",
		"symbol", req.Symbol,
		"composite", composite.Type,
		"confidence", composite.Confidence,
		"signals", len(indSignals))

	if e.webhookDispatcher != nil {
		if err := e.webhookDispatcher.Dispatch(ctx, result); err != nil {
			slog.Warn("webhook dispatch failed", "symbol", req.Symbol, "err", err)
		}
	}

	return result, nil
}

func (e *Engine) fetchBars(ctx context.Context, symbol string) ([]bullarc.OHLCV, error) {
	if len(e.dataSources) == 0 {
		return nil, nil
	}
	end := time.Now()
	start := end.AddDate(0, 0, -e.lookback)
	q := bullarc.DataQuery{
		Symbol:   symbol,
		Start:    start,
		End:      end,
		Interval: e.interval,
	}
	bars, err := e.dataSources[0].Fetch(ctx, q)
	if err != nil {
		return nil, err
	}
	slog.Info("fetched bars", "symbol", symbol, "count", len(bars))
	return bars, nil
}

func (e *Engine) selectIndicators(names []string) []bullarc.Indicator {
	if len(names) == 0 {
		inds := make([]bullarc.Indicator, 0, len(e.indicators))
		for _, ind := range e.indicators {
			inds = append(inds, ind)
		}
		return inds
	}
	var selected []bullarc.Indicator
	for _, name := range names {
		if ind, ok := e.indicators[name]; ok {
			selected = append(selected, ind)
		}
	}
	return selected
}
