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

// signalBus is the subset of signal.Bus used by the engine.
// It is an interface so tests can substitute a no-op bus if needed.
type signalBus interface {
	Publish(signals []bullarc.Signal)
	Subscribe(ctx context.Context, filter func(bullarc.Signal) bool) <-chan bullarc.Signal
}

// Engine orchestrates analysis by coordinating indicators, data sources,
// and LLM providers. All exported methods are safe for concurrent use.
type Engine struct {
	mu                   sync.RWMutex
	indicators           map[string]bullarc.Indicator
	dataSources          []bullarc.DataSource
	llmProvider          bullarc.LLMProvider
	newsSource           bullarc.NewsSource
	sentimentScorer      *llm.SentimentScorer
	newsSentimentWeight  float64
	llmMetaSignalWeight  float64
	multiStepMode        bool
	webhookDispatcher    *webhook.Dispatcher
	bus                  signalBus
	// lookback is the number of calendar days of history to request per analysis.
	lookback int
	// interval is the default bar interval passed to the data source.
	interval string
}

// New creates a new Engine with default configuration.
func New() *Engine {
	return &Engine{
		indicators:          make(map[string]bullarc.Indicator),
		bus:                 signal.NewBus(),
		lookback:            200,
		interval:            "1Day",
		newsSentimentWeight: 1.0,
		llmMetaSignalWeight: 2.0,
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

// HasLLMProvider reports whether an LLM provider is registered.
func (e *Engine) HasLLMProvider() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.llmProvider != nil
}

// RegisterLLMProvider sets the LLM provider for the engine.
func (e *Engine) RegisterLLMProvider(llm bullarc.LLMProvider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llmProvider = llm
}

// RegisterNewsSource sets the news data source used for sentiment signals.
func (e *Engine) RegisterNewsSource(ns bullarc.NewsSource) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.newsSource = ns
}

// RegisterSentimentScorer sets the LLM-backed scorer used to classify news
// article headlines for the news sentiment signal.
func (e *Engine) RegisterSentimentScorer(ss *llm.SentimentScorer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.sentimentScorer = ss
}

// SetNewsSentimentWeight sets the weight multiplier applied to the confidence
// of the news sentiment signal before it participates in aggregation.
// A value of 1.0 (the default) gives the news signal equal weight to one
// technical indicator vote. Values > 1.0 amplify it; < 1.0 reduce it.
func (e *Engine) SetNewsSentimentWeight(w float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.newsSentimentWeight = w
}

// SetLLMMetaSignalWeight sets the weight multiplier applied to the confidence
// of the LLM meta-signal before it participates in aggregation.
// The default is 2.0, giving the LLM meta-signal twice the voting power of
// a single technical indicator to reflect its synthesized nature.
// Values < 1.0 reduce its influence; setting it to 0 effectively disables it.
func (e *Engine) SetLLMMetaSignalWeight(w float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.llmMetaSignalWeight = w
}

// SetMultiStepMode enables or disables multi-step LLM analysis. When enabled and
// UseLLM is true, the engine runs a three-step chain (technical thesis → news thesis
// → synthesis) instead of the single-call LLM meta-signal. Only one mode runs per
// analysis. The synthesis reasoning is stored in AnalysisResult.LLMAnalysis.
func (e *Engine) SetMultiStepMode(enabled bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.multiStepMode = enabled
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

// Subscribe returns a channel that receives every signal produced by future
// Analyze calls for the given symbol. If symbol is empty, all signals for all
// symbols are delivered. The channel is closed when ctx is cancelled, at which
// point the subscription is removed and its resources are reclaimed.
//
// Consumers do not poll — signals are pushed as each Analyze completes.
func (e *Engine) Subscribe(ctx context.Context, symbol string) <-chan bullarc.Signal {
	var filter func(bullarc.Signal) bool
	if symbol != "" {
		filter = func(s bullarc.Signal) bool { return s.Symbol == symbol }
	}
	return e.bus.Subscribe(ctx, filter)
}

// engineSnapshot holds a point-in-time copy of the engine's mutable fields.
// It is used by Analyze to avoid holding the lock during I/O operations.
type engineSnapshot struct {
	indicators          []bullarc.Indicator
	primaryDS           bullarc.DataSource
	llmProvider         bullarc.LLMProvider
	newsSource          bullarc.NewsSource
	sentimentScorer     *llm.SentimentScorer
	newsSentimentWeight float64
	llmMetaSignalWeight float64
	multiStepMode       bool
	webhookDispatcher   *webhook.Dispatcher
	lookback            int
	interval            string
}

// snapshot takes a brief read lock and captures the current engine state.
func (e *Engine) snapshot(indicatorNames []string) engineSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	snap := engineSnapshot{
		indicators:          e.selectIndicatorsLocked(indicatorNames),
		llmProvider:         e.llmProvider,
		newsSource:          e.newsSource,
		sentimentScorer:     e.sentimentScorer,
		newsSentimentWeight: e.newsSentimentWeight,
		llmMetaSignalWeight: e.llmMetaSignalWeight,
		multiStepMode:       e.multiStepMode,
		webhookDispatcher:   e.webhookDispatcher,
		lookback:            e.lookback,
		interval:            e.interval,
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

	// Collect scored headlines when multi-step mode is active so they can be
	// passed to the news thesis step of the chain. In non-multi-step mode the
	// standard news sentiment signal path is used instead.
	var multiStepHeadlines []llm.ScoredHeadline
	if snap.newsSource != nil && snap.sentimentScorer != nil {
		if snap.multiStepMode && req.UseLLM && snap.llmProvider != nil {
			newsSig, headlines, ok := e.generateNewsSentimentWithHeadlines(ctx, req.Symbol, snap)
			if ok {
				indSignals = append(indSignals, newsSig)
			}
			multiStepHeadlines = headlines
		} else {
			if newsSig, ok := e.generateNewsSentimentSignal(ctx, req.Symbol, snap); ok {
				indSignals = append(indSignals, newsSig)
			}
		}
	}

	// Multi-step analysis chain and single-call meta-signal are mutually exclusive.
	// Only one runs per analysis invocation.
	if req.UseLLM && snap.llmProvider != nil {
		prelimComposite := signal.Aggregate(req.Symbol, indSignals)
		if snap.multiStepMode {
			if chainSig, reasoning, ok := llm.RunMultiStepChain(
				ctx, req.Symbol, result.IndicatorValues, prelimComposite,
				latestBar.Close, multiStepHeadlines, snap.llmProvider,
			); ok {
				chainSig = applySignalWeight(chainSig, snap.llmMetaSignalWeight)
				indSignals = append(indSignals, chainSig)
				result.LLMAnalysis = reasoning
				slog.Info("multi-step chain complete",
					"symbol", req.Symbol,
					"signal", chainSig.Type,
					"confidence", chainSig.Confidence)
			}
		} else {
			if llmSig, ok := e.generateLLMMetaSignal(ctx, req.Symbol, result.IndicatorValues, prelimComposite, latestBar.Close, snap); ok {
				indSignals = append(indSignals, llmSig)
			}
		}
	}

	composite := signal.Aggregate(req.Symbol, indSignals)
	result.Signals = append([]bullarc.Signal{composite}, indSignals...)

	slog.Info("analysis complete",
		"symbol", req.Symbol,
		"composite", composite.Type,
		"confidence", composite.Confidence,
		"signals", len(indSignals))

	// In multi-step mode the synthesis reasoning already populates LLMAnalysis.
	// If the chain failed entirely, LLMAnalysis is left empty (acceptance criteria:
	// "if step 1 fails, omit LLM analysis entirely"). The single-call explainer
	// only runs in non-multi-step mode.
	if req.UseLLM && snap.llmProvider != nil && !snap.multiStepMode {
		explanation, llmErr := e.explainResultWithProvider(ctx, result, snap.llmProvider)
		if llmErr != nil {
			slog.Warn("llm explanation failed", "symbol", req.Symbol, "err", llmErr)
		} else {
			result.LLMAnalysis = explanation
		}
	}

	if req.UseLLM && snap.llmProvider != nil {
		result.Anomalies = e.detectAnomalies(ctx, req.Symbol, result.IndicatorValues, bars, snap)
	}

	if snap.webhookDispatcher != nil {
		if err := snap.webhookDispatcher.Dispatch(ctx, result); err != nil {
			slog.Warn("webhook dispatch failed", "symbol", req.Symbol, "err", err)
		}
	}

	// Push signals to all active bus subscribers so consumers receive results
	// without polling.
	if len(result.Signals) > 0 {
		e.bus.Publish(result.Signals)
	}

	return result, nil
}

// generateNewsSentimentSignal fetches recent news for the symbol, scores each
// article's headline via the sentiment scorer, and converts the aggregate
// result into a single trading signal. Returns (zero, false) when no recent
// news is available so the caller can omit it from aggregation.
func (e *Engine) generateNewsSentimentSignal(ctx context.Context, symbol string, snap engineSnapshot) (bullarc.Signal, bool) {
	since := time.Now().Add(-24 * time.Hour)
	articles, err := snap.newsSource.FetchNews(ctx, []string{symbol}, since)
	if err != nil {
		slog.Warn("news fetch failed, skipping news sentiment", "symbol", symbol, "err", err)
		return bullarc.Signal{}, false
	}
	if len(articles) == 0 {
		slog.Info("no recent news for symbol, skipping news sentiment", "symbol", symbol)
		return bullarc.Signal{}, false
	}

	results, err := snap.sentimentScorer.ScoreArticles(ctx, articles)
	if err != nil {
		slog.Warn("sentiment scoring failed, skipping news sentiment", "symbol", symbol, "err", err)
		return bullarc.Signal{}, false
	}

	// Build a lookup so we can correlate results to article metadata.
	articleByID := make(map[string]struct{}, len(articles))
	for _, a := range articles {
		articleByID[a.ID] = struct{}{}
	}

	scored := make([]signal.ScoredArticle, 0, len(results))
	for _, r := range results {
		if _, ok := articleByID[r.ArticleID]; !ok {
			continue
		}
		scored = append(scored, signal.ScoredArticle{
			Sentiment:  string(r.Sentiment),
			Confidence: r.Confidence,
		})
	}

	sig, ok := signal.NewsSentimentSignal(symbol, scored)
	if !ok {
		return bullarc.Signal{}, false
	}

	// Apply the weight multiplier to the confidence so that operators can
	// tune how much the news sentiment vote influences the composite signal.
	if snap.newsSentimentWeight != 1.0 {
		sig.Confidence = sig.Confidence * snap.newsSentimentWeight
		if sig.Confidence > 100 {
			sig.Confidence = 100
		}
		if sig.Confidence < 0 {
			sig.Confidence = 0
		}
	}

	slog.Info("news sentiment signal generated",
		"symbol", symbol,
		"type", sig.Type,
		"confidence", sig.Confidence,
		"articles", len(scored))
	return sig, true
}

// generateLLMMetaSignal calls the LLM to synthesize all indicator values and the
// preliminary composite signal into a structured BUY/SELL/HOLD meta-signal.
// The signal confidence is scaled by snap.llmMetaSignalWeight before being stored so
// that the meta-signal participates in aggregation with the configured influence.
// Returns (zero, false) when the LLM call fails or returns an invalid response,
// in which case the analysis continues without the meta-signal.
func (e *Engine) generateLLMMetaSignal(
	ctx context.Context,
	symbol string,
	indicatorValues map[string][]bullarc.IndicatorValue,
	prelimComposite bullarc.Signal,
	currentPrice float64,
	snap engineSnapshot,
) (bullarc.Signal, bool) {
	sig, ok := llm.GenerateMetaSignal(ctx, symbol, indicatorValues, prelimComposite, currentPrice, snap.llmProvider)
	if !ok {
		slog.Warn("LLM meta-signal omitted due to error or invalid response", "symbol", symbol)
		return bullarc.Signal{}, false
	}

	if snap.llmMetaSignalWeight != 1.0 {
		sig.Confidence = sig.Confidence * snap.llmMetaSignalWeight
		if sig.Confidence > 100 {
			sig.Confidence = 100
		}
		if sig.Confidence < 0 {
			sig.Confidence = 0
		}
	}

	slog.Info("LLM meta-signal generated",
		"symbol", symbol,
		"type", sig.Type,
		"confidence", sig.Confidence,
		"weight", snap.llmMetaSignalWeight)
	return sig, true
}

// generateNewsSentimentWithHeadlines fetches and scores news articles, returning
// both the aggregated sentiment signal and the individual ScoredHeadlines for
// use in the multi-step analysis chain. The signal may be absent (ok=false) if
// no articles are available or scoring fails, but headlines may still be
// non-empty if scoring succeeded but the signal could not be derived.
func (e *Engine) generateNewsSentimentWithHeadlines(
	ctx context.Context,
	symbol string,
	snap engineSnapshot,
) (sig bullarc.Signal, headlines []llm.ScoredHeadline, sigOK bool) {
	since := time.Now().Add(-24 * time.Hour)
	articles, err := snap.newsSource.FetchNews(ctx, []string{symbol}, since)
	if err != nil {
		slog.Warn("news fetch failed, skipping news sentiment", "symbol", symbol, "err", err)
		return bullarc.Signal{}, nil, false
	}
	if len(articles) == 0 {
		slog.Info("no recent news for symbol, skipping news sentiment", "symbol", symbol)
		return bullarc.Signal{}, nil, false
	}

	results, err := snap.sentimentScorer.ScoreArticles(ctx, articles)
	if err != nil {
		slog.Warn("sentiment scoring failed, skipping news sentiment", "symbol", symbol, "err", err)
		return bullarc.Signal{}, nil, false
	}

	// Build scored headlines for the multi-step chain and a lookup for the signal builder.
	articleByID := make(map[string]bullarc.NewsArticle, len(articles))
	for _, a := range articles {
		articleByID[a.ID] = a
	}

	scored := make([]signal.ScoredArticle, 0, len(results))
	headlines = make([]llm.ScoredHeadline, 0, len(results))
	for _, r := range results {
		a, ok := articleByID[r.ArticleID]
		if !ok {
			continue
		}
		scored = append(scored, signal.ScoredArticle{
			Sentiment:  string(r.Sentiment),
			Confidence: r.Confidence,
		})
		headlines = append(headlines, llm.ScoredHeadline{
			Headline:   a.Headline,
			Sentiment:  string(r.Sentiment),
			Confidence: r.Confidence,
		})
	}

	newsSig, ok := signal.NewsSentimentSignal(symbol, scored)
	if !ok {
		return bullarc.Signal{}, headlines, false
	}

	if snap.newsSentimentWeight != 1.0 {
		newsSig.Confidence = newsSig.Confidence * snap.newsSentimentWeight
		if newsSig.Confidence > 100 {
			newsSig.Confidence = 100
		}
		if newsSig.Confidence < 0 {
			newsSig.Confidence = 0
		}
	}

	slog.Info("news sentiment signal generated",
		"symbol", symbol,
		"type", newsSig.Type,
		"confidence", newsSig.Confidence,
		"articles", len(scored))

	return newsSig, headlines, true
}

// applySignalWeight scales a signal's confidence by the given weight multiplier
// and clamps the result to [0, 100].
func applySignalWeight(sig bullarc.Signal, weight float64) bullarc.Signal {
	if weight == 1.0 {
		return sig
	}
	sig.Confidence = sig.Confidence * weight
	if sig.Confidence > 100 {
		sig.Confidence = 100
	}
	if sig.Confidence < 0 {
		sig.Confidence = 0
	}
	return sig
}

// detectAnomalies calls the LLM to identify divergences and anomalies in the
// historical indicator values. If fewer than 10 bars of data are available,
// detection is skipped and an empty slice is returned. Errors are logged and
// treated as non-fatal: the caller receives an empty slice rather than an error.
func (e *Engine) detectAnomalies(
	ctx context.Context,
	symbol string,
	indicatorValues map[string][]bullarc.IndicatorValue,
	bars []bullarc.OHLCV,
	snap engineSnapshot,
) []bullarc.Anomaly {
	if len(bars) < 10 {
		slog.Info("skipping anomaly detection: insufficient data",
			"symbol", symbol, "bars", len(bars))
		return []bullarc.Anomaly{}
	}

	anomalies, ok := llm.DetectAnomalies(ctx, symbol, indicatorValues, snap.llmProvider)
	if !ok {
		slog.Warn("anomaly detection failed, returning empty list", "symbol", symbol)
		return []bullarc.Anomaly{}
	}

	slog.Info("anomaly detection complete",
		"symbol", symbol,
		"anomalies", len(anomalies))
	return anomalies
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
