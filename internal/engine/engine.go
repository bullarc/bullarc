// Package engine implements the core analysis engine.
package engine

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/config"
	"github.com/bullarc/bullarc/internal/llm"
	"github.com/bullarc/bullarc/internal/signal"
	"github.com/bullarc/bullarc/internal/webhook"
)

// Engine orchestrates analysis by coordinating indicators, data sources,
// and LLM providers. All exported methods are safe for concurrent use.
type Engine struct {
	mu                sync.RWMutex
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
	e.mu.Lock()
	defer e.mu.Unlock()
	e.indicators[ind.Meta().Name] = ind
}

// RegisterDataSource adds a data source to the engine.
func (e *Engine) RegisterDataSource(ds bullarc.DataSource) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dataSources = append(e.dataSources, ds)
}

// HasDataSource reports whether at least one data source is registered.
func (e *Engine) HasDataSource() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.dataSources) > 0
}

// RegisterLLMProvider sets the LLM provider for the engine.
func (e *Engine) RegisterLLMProvider(llm bullarc.LLMProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llmProvider = llm
}

// SetInterval updates the data interval used when fetching bars.
func (e *Engine) SetInterval(interval string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.interval = interval
}

// SetDataSource replaces the primary data source used by the engine.
// If no data sources are registered yet, the given source becomes the first.
func (e *Engine) SetDataSource(ds bullarc.DataSource) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.dataSources) == 0 {
		e.dataSources = []bullarc.DataSource{ds}
		return
	}
	e.dataSources[0] = ds
}

// SetLLMProvider replaces the LLM provider used by the engine for generating
// explanations. Passing nil disables LLM explanations.
func (e *Engine) SetLLMProvider(llm bullarc.LLMProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llmProvider = llm
}

// RegisterWebhookDispatcher attaches a webhook dispatcher that receives each
// AnalysisResult immediately after Analyze completes. Dispatch errors are
// logged but do not affect the returned result.
func (e *Engine) RegisterWebhookDispatcher(d *webhook.Dispatcher) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.webhookDispatcher = d
}

// engineSnapshot holds a point-in-time copy of the engine's mutable fields.
// It is used by Analyze to avoid holding the lock during I/O operations.
type engineSnapshot struct {
	indicators        []bullarc.Indicator
	primaryDS         bullarc.DataSource
	llmProvider       bullarc.LLMProvider
	webhookDispatcher *webhook.Dispatcher
	lookback          int
	interval          string
}

// snapshot takes a brief read lock and captures the current engine state.
func (e *Engine) snapshot(indicatorNames []string) engineSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	snap := engineSnapshot{
		indicators: e.selectIndicatorsLocked(indicatorNames),
		llmProvider:       e.llmProvider,
		webhookDispatcher: e.webhookDispatcher,
		lookback:          e.lookback,
		interval:          e.interval,
	}
	if len(e.dataSources) > 0 {
		snap.primaryDS = e.dataSources[0]
	}
	return snap
}

// Analyze fetches market data, computes indicators, generates per-indicator signals,
// and aggregates them into a composite BUY/SELL/HOLD signal.
// Analyze is safe for concurrent use by multiple goroutines.
func (e *Engine) Analyze(ctx context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
	slog.Info("analysis started",
		"symbol", req.Symbol,
		"indicators", req.Indicators,
		"use_llm", req.UseLLM)

	// Take a brief snapshot of engine state so that long-running I/O (data
	// fetching, LLM calls) is performed without holding the lock.
	snap := e.snapshot(req.Indicators)

	result := bullarc.AnalysisResult{
		Symbol:          req.Symbol,
		Timestamp:       time.Now(),
		IndicatorValues: make(map[string][]bullarc.IndicatorValue),
	}

	bars, err := e.fetchBarsWithSnapshot(ctx, req.Symbol, snap)
	if err != nil {
		return result, err
	}
	if len(bars) == 0 {
		slog.Warn("no bars available, skipping analysis", "symbol", req.Symbol)
		return result, nil
	}

	latestBar := bars[len(bars)-1]
	var indSignals []bullarc.Signal

	for _, ind := range snap.indicators {
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

	if req.UseLLM && snap.llmProvider != nil {
		explanation, llmErr := e.explainResultWithProvider(ctx, result, snap.llmProvider)
		if llmErr != nil {
			slog.Warn("llm explanation failed", "symbol", req.Symbol, "err", llmErr)
		} else {
			result.LLMAnalysis = explanation
		}
	}

	if snap.webhookDispatcher != nil {
		if err := snap.webhookDispatcher.Dispatch(ctx, result); err != nil {
			slog.Warn("webhook dispatch failed", "symbol", req.Symbol, "err", err)
		}
	}

	return result, nil
}

// explainResultWithProvider calls the given LLM provider to generate a plain
// English explanation of the analysis result.
func (e *Engine) explainResultWithProvider(ctx context.Context, result bullarc.AnalysisResult, provider bullarc.LLMProvider) (string, error) {
	prompt := llm.AnalysisPrompt(result)
	resp, err := provider.Complete(ctx, bullarc.LLMRequest{Prompt: prompt, MaxTokens: 512})
	if err != nil {
		return "", err
	}
	slog.Info("llm explanation generated", "symbol", result.Symbol, "tokens", resp.TokensUsed, "model", resp.Model)
	return resp.Text, nil
}

func (e *Engine) fetchBarsWithSnapshot(ctx context.Context, symbol string, snap engineSnapshot) ([]bullarc.OHLCV, error) {
	if snap.primaryDS == nil {
		return nil, nil
	}
	end := time.Now()
	start := end.AddDate(0, 0, -snap.lookback)
	q := bullarc.DataQuery{
		Symbol:   symbol,
		Start:    start,
		End:      end,
		Interval: snap.interval,
	}
	bars, err := snap.primaryDS.Fetch(ctx, q)
	if err != nil {
		return nil, err
	}
	slog.Info("fetched bars", "symbol", symbol, "count", len(bars))
	return bars, nil
}

// selectIndicatorsLocked returns the indicators matching names from the current
// engine state. Must be called with e.mu held for reading.
func (e *Engine) selectIndicatorsLocked(names []string) []bullarc.Indicator {
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
