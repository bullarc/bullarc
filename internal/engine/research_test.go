package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/bullarc/bullarc/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- GetNewsSentiment tests ----

// TestGetNewsSentiment_NoNewsSource verifies that missing news source returns ErrNotConfigured.
func TestGetNewsSentiment_NoNewsSource(t *testing.T) {
	e := engine.New()
	_, err := e.GetNewsSentiment(context.Background(), "AAPL", 24)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_CONFIGURED")
}

// TestGetNewsSentiment_NoSentimentScorer verifies that missing scorer returns ErrNotConfigured.
func TestGetNewsSentiment_NoSentimentScorer(t *testing.T) {
	e := engine.New()
	e.RegisterNewsSource(&stubNewsSource{articles: []bullarc.NewsArticle{makeNewsArticle("a1")}})
	_, err := e.GetNewsSentiment(context.Background(), "AAPL", 24)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_CONFIGURED")
}

// TestGetNewsSentiment_EmptyArticlesReturnsNeutral verifies that no articles → neutral summary.
func TestGetNewsSentiment_EmptyArticlesReturnsNeutral(t *testing.T) {
	e := engine.New()
	e.RegisterNewsSource(&stubNewsSource{articles: nil})
	scorer := llm.NewSentimentScorer(&stubSentimentLLM{})
	e.RegisterSentimentScorer(scorer)

	summary, err := e.GetNewsSentiment(context.Background(), "AAPL", 24)
	require.NoError(t, err)
	assert.Equal(t, "AAPL", summary.Symbol)
	assert.Equal(t, "neutral", summary.AggregateSentiment)
	assert.Empty(t, summary.Headlines)
}

// TestGetNewsSentiment_BullishAggregate verifies that majority bullish articles
// produce a bullish aggregate with a non-zero score.
func TestGetNewsSentiment_BullishAggregate(t *testing.T) {
	articles := []bullarc.NewsArticle{
		makeNewsArticle("b1"),
		makeNewsArticle("b2"),
		makeNewsArticle("b3"),
	}
	provider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{
			sentimentJSON("bullish", 80),
			sentimentJSON("bullish", 90),
			sentimentJSON("bullish", 70),
		},
	}
	e := engine.New()
	e.RegisterNewsSource(&stubNewsSource{articles: articles})
	e.RegisterSentimentScorer(llm.NewSentimentScorer(provider))

	summary, err := e.GetNewsSentiment(context.Background(), "AAPL", 24)
	require.NoError(t, err)
	assert.Equal(t, "AAPL", summary.Symbol)
	assert.Equal(t, "bullish", summary.AggregateSentiment)
	assert.Greater(t, summary.AggregateScore, 0.0)
	assert.Len(t, summary.Headlines, 3)

	// Verify individual headline fields are populated.
	h := summary.Headlines[0]
	assert.NotEmpty(t, h.Headline)
	assert.Equal(t, "bullish", h.Sentiment)
	assert.Greater(t, h.Confidence, 0)
}

// TestGetNewsSentiment_BearishAggregate verifies that majority bearish articles
// produce a bearish aggregate.
func TestGetNewsSentiment_BearishAggregate(t *testing.T) {
	articles := []bullarc.NewsArticle{
		makeNewsArticle("c1"),
		makeNewsArticle("c2"),
	}
	provider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{
			sentimentJSON("bearish", 80),
			sentimentJSON("bearish", 70),
		},
	}
	e := engine.New()
	e.RegisterNewsSource(&stubNewsSource{articles: articles})
	e.RegisterSentimentScorer(llm.NewSentimentScorer(provider))

	summary, err := e.GetNewsSentiment(context.Background(), "AAPL", 24)
	require.NoError(t, err)
	assert.Equal(t, "bearish", summary.AggregateSentiment)
}

// TestGetNewsSentiment_CustomHours verifies that the hours parameter is
// forwarded as a time-window offset (the news source receives a time after
// now-hours).
func TestGetNewsSentiment_CustomHours(t *testing.T) {
	now := time.Now()
	var capturedSince time.Time
	ns := &trackingNewsSource{
		articles: []bullarc.NewsArticle{makeNewsArticle("h1")},
		captureSince: func(since time.Time) {
			capturedSince = since
		},
	}
	provider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{sentimentJSON("neutral", 50)},
	}
	e := engine.New()
	e.RegisterNewsSource(ns)
	e.RegisterSentimentScorer(llm.NewSentimentScorer(provider))

	_, err := e.GetNewsSentiment(context.Background(), "AAPL", 6)
	require.NoError(t, err)

	// The since time should be approximately now - 6 hours.
	expectedSince := now.Add(-6 * time.Hour)
	assert.WithinDuration(t, expectedSince, capturedSince, 2*time.Second)
}

// TestGetNewsSentiment_DefaultHours verifies that hours ≤ 0 defaults to 24.
func TestGetNewsSentiment_DefaultHours(t *testing.T) {
	now := time.Now()
	var capturedSince time.Time
	ns := &trackingNewsSource{
		articles: []bullarc.NewsArticle{makeNewsArticle("d1")},
		captureSince: func(since time.Time) {
			capturedSince = since
		},
	}
	provider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{sentimentJSON("neutral", 50)},
	}
	e := engine.New()
	e.RegisterNewsSource(ns)
	e.RegisterSentimentScorer(llm.NewSentimentScorer(provider))

	_, err := e.GetNewsSentiment(context.Background(), "AAPL", 0)
	require.NoError(t, err)

	expectedSince := now.Add(-24 * time.Hour)
	assert.WithinDuration(t, expectedSince, capturedSince, 2*time.Second)
}

// trackingNewsSource is a NewsSource that captures the `since` argument.
type trackingNewsSource struct {
	articles     []bullarc.NewsArticle
	captureSince func(since time.Time)
}

func (t *trackingNewsSource) FetchNews(_ context.Context, _ []string, since time.Time) ([]bullarc.NewsArticle, error) {
	if t.captureSince != nil {
		t.captureSince(since)
	}
	return t.articles, nil
}

// ---- FetchRiskMetrics tests ----

// TestFetchRiskMetrics_NoDataSource verifies that a missing data source returns an error.
func TestFetchRiskMetrics_NoDataSource(t *testing.T) {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	_, _, err := e.FetchRiskMetrics(context.Background(), "AAPL")
	require.Error(t, err)
}

// TestFetchRiskMetrics_NoBars verifies that empty bar data returns ErrInsufficientData.
func TestFetchRiskMetrics_NoBars(t *testing.T) {
	e := newEngineWithBars(nil)
	_, _, err := e.FetchRiskMetrics(context.Background(), "AAPL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "INSUFFICIENT_DATA")
}

// TestFetchRiskMetrics_ReturnsMetrics verifies that ATR-based risk metrics are returned
// for a properly initialised engine with sufficient bar data.
func TestFetchRiskMetrics_ReturnsMetrics(t *testing.T) {
	bars := trendingBars(100, 150, 0.3)
	e := newEngineWithBars(bars)

	risk, regime, err := e.FetchRiskMetrics(context.Background(), "AAPL")
	require.NoError(t, err)
	require.NotNil(t, risk, "risk metrics must be non-nil with sufficient data")
	assert.Greater(t, risk.ATR, 0.0)
	assert.Greater(t, risk.PositionSizePct, 0.0)
	assert.Greater(t, risk.TakeProfit, risk.StopLoss, "take-profit must be above stop-loss for BUY direction")
	assert.InDelta(t, 1.5, risk.RiskRewardRatio, 0.01)
	// Regime is empty when not configured.
	assert.Empty(t, regime)
}

// TestFetchRiskMetrics_IgnoresRiskConfigEnabled verifies that FetchRiskMetrics computes
// metrics even when the engine's RiskConfig.Enabled is false.
func TestFetchRiskMetrics_IgnoresRiskConfigEnabled(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetRiskConfig(engine.RiskConfig{Enabled: false})

	risk, _, err := e.FetchRiskMetrics(context.Background(), "AAPL")
	require.NoError(t, err)
	require.NotNil(t, risk, "FetchRiskMetrics must compute metrics even when Enabled=false")
}

// ---- AnalyzeDeep tests ----

// TestAnalyzeDeep_NoBars verifies that an engine with no bars returns an empty result.
func TestAnalyzeDeep_NoBars(t *testing.T) {
	e := newEngineWithBars(nil)
	result, err := e.AnalyzeDeep(context.Background(), "AAPL")
	require.NoError(t, err)
	assert.Empty(t, result.Signals)
}

// TestAnalyzeDeep_ProducesSignals verifies that AnalyzeDeep returns signals when
// sufficient bar data is available (LLM multi-step chain is skipped when no LLM provider
// is configured, but technical indicator signals still appear).
func TestAnalyzeDeep_ProducesSignals(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.AnalyzeDeep(context.Background(), "AAPL")
	require.NoError(t, err)
	assert.Equal(t, "AAPL", result.Symbol)
	// Without an LLM provider the multi-step chain is skipped but technical signals appear.
	assert.NotEmpty(t, result.Signals)
}

// TestAnalyzeDeep_UsesMultiStepChain verifies that with a multi-step-capable LLM provider
// the chain is executed and LLMAnalysis is populated.
func TestAnalyzeDeep_UsesMultiStepChain(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	// The LLM provider returns the expected JSON responses for each of the
	// three multi-step chain calls: technical thesis, (news thesis skipped),
	// and synthesis.
	techResponse := `{"signal":"BUY","confidence":75,"direction":"bullish","key_levels":"support 98","confluence":"SMA and RSI agree","thesis":"Strong bullish momentum."}`
	synthResponse := `{"signal":"BUY","confidence":72,"reasoning":"Technical signals confirm uptrend. No recent news available."}`
	provider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{
			{Text: techResponse},
			{Text: synthResponse},
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.AnalyzeDeep(context.Background(), "AAPL")
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)
	assert.NotEmpty(t, result.LLMAnalysis,
		"AnalyzeDeep must populate LLMAnalysis via the multi-step chain")
	assert.Contains(t, result.LLMAnalysis, "Technical signals confirm uptrend")
}
