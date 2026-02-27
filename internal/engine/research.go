package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/llm"
)

// GetNewsSentiment fetches and LLM-scores recent news headlines for symbol over
// the past hours hours. When hours ≤ 0 it defaults to 24. Returns an empty
// summary (no headlines) when no news source or sentiment scorer is registered,
// or when no articles are found in the window.
func (e *Engine) GetNewsSentiment(ctx context.Context, symbol string, hours int) (bullarc.NewsSentimentSummary, error) {
	snap := e.snapshot(nil)

	if snap.newsSource == nil {
		return bullarc.NewsSentimentSummary{}, bullarc.ErrNotConfigured.Wrap(
			fmt.Errorf("no news source registered"))
	}
	if snap.sentimentScorer == nil {
		return bullarc.NewsSentimentSummary{}, bullarc.ErrNotConfigured.Wrap(
			fmt.Errorf("no sentiment scorer configured; register an LLM provider and sentiment scorer"))
	}

	if hours <= 0 {
		hours = 24
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	articles, err := snap.newsSource.FetchNews(ctx, []string{symbol}, since)
	if err != nil {
		return bullarc.NewsSentimentSummary{}, fmt.Errorf("news fetch failed: %w", err)
	}
	if len(articles) == 0 {
		slog.Info("no recent news for symbol", "symbol", symbol, "hours", hours)
		return bullarc.NewsSentimentSummary{
			Symbol:             symbol,
			AggregateSentiment: "neutral",
			AggregateScore:     0,
		}, nil
	}

	results, err := snap.sentimentScorer.ScoreArticles(ctx, articles)
	if err != nil {
		return bullarc.NewsSentimentSummary{}, fmt.Errorf("sentiment scoring failed: %w", err)
	}

	articleByID := make(map[string]bullarc.NewsArticle, len(articles))
	for _, a := range articles {
		articleByID[a.ID] = a
	}

	headlines := make([]bullarc.ScoredNewsHeadline, 0, len(results))
	var bullishSum, bearishSum, neutralSum float64
	for _, r := range results {
		a, ok := articleByID[r.ArticleID]
		if !ok {
			continue
		}
		headlines = append(headlines, bullarc.ScoredNewsHeadline{
			ArticleID:   r.ArticleID,
			Headline:    a.Headline,
			Source:      a.Source,
			PublishedAt: a.PublishedAt,
			Sentiment:   string(r.Sentiment),
			Confidence:  r.Confidence,
			Reasoning:   r.Reasoning,
		})
		switch r.Sentiment {
		case llm.SentimentBullish:
			bullishSum += float64(r.Confidence)
		case llm.SentimentBearish:
			bearishSum += float64(r.Confidence)
		default:
			neutralSum += float64(r.Confidence)
		}
	}

	n := float64(len(headlines))
	aggregate := "neutral"
	aggregateScore := 0.0
	if n > 0 {
		switch {
		case bullishSum >= bearishSum && bullishSum >= neutralSum:
			aggregate = "bullish"
			aggregateScore = bullishSum / n
		case bearishSum > bullishSum && bearishSum >= neutralSum:
			aggregate = "bearish"
			aggregateScore = bearishSum / n
		default:
			aggregateScore = neutralSum / n
		}
	}

	slog.Info("news sentiment computed",
		"symbol", symbol,
		"hours", hours,
		"articles", len(headlines),
		"aggregate", aggregate,
		"score", aggregateScore)

	return bullarc.NewsSentimentSummary{
		Symbol:             symbol,
		Headlines:          headlines,
		AggregateSentiment: aggregate,
		AggregateScore:     aggregateScore,
	}, nil
}

// FetchRiskMetrics computes ATR-based position sizing and stop-loss/take-profit
// prices for symbol using the engine's indicator suite. The computation always
// runs regardless of the engine's RiskConfig.Enabled flag. The second return
// value is the LLM-detected market regime (empty when regime detection is off
// or unavailable). Returns bullarc.ErrInsufficientData when bars or ATR values
// are unavailable.
func (e *Engine) FetchRiskMetrics(ctx context.Context, symbol string) (*bullarc.RiskMetrics, string, error) {
	snap := e.snapshot(nil)

	bars, err := e.fetchBarsWithSnapshot(ctx, symbol, snap)
	if err != nil {
		return nil, "", fmt.Errorf("data fetch failed: %w", err)
	}
	if len(bars) == 0 {
		return nil, "", bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("no bars available for %s", symbol))
	}

	// Compute all registered indicators so ATR values are available.
	indicatorValues := make(map[string][]bullarc.IndicatorValue)
	for _, ind := range snap.indicators {
		values, indErr := ind.Compute(bars)
		if indErr != nil {
			slog.Warn("indicator compute failed in FetchRiskMetrics",
				"indicator", ind.Meta().Name, "symbol", symbol, "err", indErr)
			continue
		}
		indicatorValues[ind.Meta().Name] = values
	}

	latestBar := bars[len(bars)-1]

	// Force risk computation on-demand with defaults, independent of the
	// engine's persisted riskConfig.Enabled flag.
	riskCfg := snap.riskConfig
	riskCfg.Enabled = true
	if riskCfg.ATRIndicatorName == "" {
		riskCfg.ATRIndicatorName = "ATR_14"
	}
	if riskCfg.MaxPositionSizePct <= 0 {
		riskCfg.MaxPositionSizePct = DefaultMaxPositionSizePct
	}
	if riskCfg.StopLossMultiplier <= 0 {
		riskCfg.StopLossMultiplier = DefaultStopLossMultiplier
	}
	if riskCfg.TakeProfitMultiplier <= 0 {
		riskCfg.TakeProfitMultiplier = DefaultTakeProfitMultiplier
	}

	// Use a BUY-direction synthetic signal: stop below entry, take-profit above.
	syntheticSignal := bullarc.Signal{Type: bullarc.SignalBuy, Symbol: symbol}
	risk, ok := computeRiskMetrics(syntheticSignal, latestBar.Close, indicatorValues, riskCfg)
	if !ok {
		return nil, "", bullarc.ErrInsufficientData.Wrap(
			fmt.Errorf("ATR not available for %s; ensure ATR_14 indicator is registered", symbol))
	}

	// Detect regime when enabled and an LLM provider is present.
	regime := ""
	if snap.regimeConfig.Enabled && snap.llmProvider != nil {
		regime = e.detectRegime(ctx, symbol, bars, indicatorValues, snap)
	}

	slog.Info("risk metrics fetched",
		"symbol", symbol,
		"atr", risk.ATR,
		"position_size_pct", risk.PositionSizePct,
		"stop_loss", risk.StopLoss,
		"take_profit", risk.TakeProfit,
		"regime", regime)

	return risk, regime, nil
}
