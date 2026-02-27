package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- optional capability stubs ----

// newsSentimentStub wraps stubBackend and implements the newsSentimentFetcher interface.
type newsSentimentStub struct {
	stubBackend
	fetchFn func(ctx context.Context, symbol string, hours int) (bullarc.NewsSentimentSummary, error)
}

func (s *newsSentimentStub) GetNewsSentiment(ctx context.Context, symbol string, hours int) (bullarc.NewsSentimentSummary, error) {
	if s.fetchFn != nil {
		return s.fetchFn(ctx, symbol, hours)
	}
	return bullarc.NewsSentimentSummary{Symbol: symbol, AggregateSentiment: "neutral"}, nil
}

// riskMetricsStub wraps stubBackend and implements the riskMetricsFetcher interface.
type riskMetricsStub struct {
	stubBackend
	fetchFn func(ctx context.Context, symbol string) (*bullarc.RiskMetrics, string, error)
}

func (s *riskMetricsStub) FetchRiskMetrics(ctx context.Context, symbol string) (*bullarc.RiskMetrics, string, error) {
	if s.fetchFn != nil {
		return s.fetchFn(ctx, symbol)
	}
	return &bullarc.RiskMetrics{ATR: 2.0, PositionSizePct: 5.0, StopLoss: 148.0, TakeProfit: 156.0, RiskRewardRatio: 1.5}, "", nil
}

// deepAnalyzerStub wraps stubBackend and implements the deepAnalyzer interface.
type deepAnalyzerStub struct {
	stubBackend
	deepFn func(ctx context.Context, symbol string) (bullarc.AnalysisResult, error)
}

func (s *deepAnalyzerStub) AnalyzeDeep(ctx context.Context, symbol string) (bullarc.AnalysisResult, error) {
	if s.deepFn != nil {
		return s.deepFn(ctx, symbol)
	}
	return bullarc.AnalysisResult{Symbol: symbol, Timestamp: time.Now()}, nil
}

// ---- helpers ----

func callResearchTool(t *testing.T, b mcp.Backend, toolName string, args map[string]any) (string, bool) {
	t.Helper()
	srv := mcp.New("test", "0.0.0")
	mcp.RegisterTools(srv, b)
	return invokeToolViaServer(t, srv, toolName, args)
}

// ---- get_news_sentiment tests ----

// TestGetNewsSentiment_MissingSymbol verifies that omitting symbol returns an error.
func TestGetNewsSentiment_MissingSymbol(t *testing.T) {
	b := &newsSentimentStub{}
	text, isError := callResearchTool(t, b, "get_news_sentiment", map[string]any{})
	assert.True(t, isError)
	assert.Contains(t, text, "symbol is required")
}

// TestGetNewsSentiment_BackendLacksCapability verifies that a non-supporting backend
// returns an informative error.
func TestGetNewsSentiment_BackendLacksCapability(t *testing.T) {
	b := &stubBackend{}
	text, isError := callResearchTool(t, b, "get_news_sentiment", map[string]any{"symbol": "AAPL"})
	assert.True(t, isError)
	assert.Contains(t, text, "GetNewsSentiment")
}

// TestGetNewsSentiment_NoArticles verifies that an empty article list returns a
// neutral aggregate with zero headlines.
func TestGetNewsSentiment_NoArticles(t *testing.T) {
	b := &newsSentimentStub{
		fetchFn: func(_ context.Context, symbol string, _ int) (bullarc.NewsSentimentSummary, error) {
			return bullarc.NewsSentimentSummary{
				Symbol:             symbol,
				AggregateSentiment: "neutral",
				AggregateScore:     0,
			}, nil
		},
	}

	text, isError := callResearchTool(t, b, "get_news_sentiment", map[string]any{"symbol": "AAPL"})
	require.False(t, isError, "expected no error for empty news, got: %s", text)

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	assert.Equal(t, "AAPL", out["symbol"])
	assert.Equal(t, "neutral", out["aggregate_sentiment"])
	assert.Equal(t, float64(0), out["article_count"])
}

// TestGetNewsSentiment_WithHeadlines verifies that headlines and aggregate are returned.
func TestGetNewsSentiment_WithHeadlines(t *testing.T) {
	now := time.Date(2026, 2, 27, 8, 0, 0, 0, time.UTC)
	b := &newsSentimentStub{
		fetchFn: func(_ context.Context, symbol string, hours int) (bullarc.NewsSentimentSummary, error) {
			return bullarc.NewsSentimentSummary{
				Symbol: symbol,
				Headlines: []bullarc.ScoredNewsHeadline{
					{
						ArticleID:   "a1",
						Headline:    "Apple Reports Record Earnings",
						Source:      "Reuters",
						PublishedAt: now,
						Sentiment:   "bullish",
						Confidence:  90,
						Reasoning:   "Strong beat",
					},
					{
						ArticleID:   "a2",
						Headline:    "iPhone Sales Miss Estimates",
						Source:      "Bloomberg",
						PublishedAt: now.Add(-2 * time.Hour),
						Sentiment:   "bearish",
						Confidence:  70,
						Reasoning:   "Miss is concerning",
					},
				},
				AggregateSentiment: "bullish",
				AggregateScore:     80,
			}, nil
		},
	}

	text, isError := callResearchTool(t, b, "get_news_sentiment", map[string]any{
		"symbol": "AAPL",
		"hours":  float64(48),
	})
	require.False(t, isError, "expected no error, got: %s", text)

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	assert.Equal(t, "AAPL", out["symbol"])
	assert.Equal(t, float64(48), out["hours"])
	assert.Equal(t, float64(2), out["article_count"])
	assert.Equal(t, "bullish", out["aggregate_sentiment"])
	assert.InDelta(t, 80.0, out["aggregate_score"], 0.01)

	headlines, _ := out["headlines"].([]any)
	require.Len(t, headlines, 2)
	first := headlines[0].(map[string]any)
	assert.Equal(t, "Apple Reports Record Earnings", first["headline"])
	assert.Equal(t, "bullish", first["sentiment"])
	assert.Equal(t, float64(90), first["confidence"])
}

// TestGetNewsSentiment_BackendError verifies that a backend error surfaces as a tool error.
func TestGetNewsSentiment_BackendError(t *testing.T) {
	b := &newsSentimentStub{
		fetchFn: func(_ context.Context, _ string, _ int) (bullarc.NewsSentimentSummary, error) {
			return bullarc.NewsSentimentSummary{}, fmt.Errorf("news source unavailable")
		},
	}

	text, isError := callResearchTool(t, b, "get_news_sentiment", map[string]any{"symbol": "AAPL"})
	assert.True(t, isError)
	assert.Contains(t, text, "news source unavailable")
}

// TestGetNewsSentiment_DefaultHours verifies that omitting hours defaults to 24.
func TestGetNewsSentiment_DefaultHours(t *testing.T) {
	var capturedHours int
	b := &newsSentimentStub{
		fetchFn: func(_ context.Context, _ string, hours int) (bullarc.NewsSentimentSummary, error) {
			capturedHours = hours
			return bullarc.NewsSentimentSummary{AggregateSentiment: "neutral"}, nil
		},
	}

	_, isError := callResearchTool(t, b, "get_news_sentiment", map[string]any{"symbol": "AAPL"})
	require.False(t, isError)
	assert.Equal(t, 24, capturedHours)
}

// ---- get_risk_metrics tests ----

// TestGetRiskMetrics_MissingSymbol verifies that omitting symbol returns an error.
func TestGetRiskMetrics_MissingSymbol(t *testing.T) {
	b := &riskMetricsStub{}
	text, isError := callResearchTool(t, b, "get_risk_metrics", map[string]any{})
	assert.True(t, isError)
	assert.Contains(t, text, "symbol is required")
}

// TestGetRiskMetrics_BackendLacksCapability verifies that a non-supporting backend
// returns an informative error.
func TestGetRiskMetrics_BackendLacksCapability(t *testing.T) {
	b := &stubBackend{}
	text, isError := callResearchTool(t, b, "get_risk_metrics", map[string]any{"symbol": "AAPL"})
	assert.True(t, isError)
	assert.Contains(t, text, "FetchRiskMetrics")
}

// TestGetRiskMetrics_ReturnsMetrics verifies that risk metrics and regime are returned.
func TestGetRiskMetrics_ReturnsMetrics(t *testing.T) {
	b := &riskMetricsStub{
		fetchFn: func(_ context.Context, symbol string) (*bullarc.RiskMetrics, string, error) {
			return &bullarc.RiskMetrics{
				ATR:             2.45,
				PositionSizePct: 4.08,
				StopLoss:        187.10,
				TakeProfit:      194.45,
				RiskRewardRatio: 1.5,
			}, "low_vol_trending", nil
		},
	}

	text, isError := callResearchTool(t, b, "get_risk_metrics", map[string]any{"symbol": "AAPL"})
	require.False(t, isError, "expected no error, got: %s", text)

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	assert.Equal(t, "AAPL", out["symbol"])
	assert.InDelta(t, 2.45, out["atr"], 0.01)
	assert.InDelta(t, 4.08, out["position_size_pct"], 0.01)
	assert.InDelta(t, 187.10, out["stop_loss"], 0.01)
	assert.InDelta(t, 194.45, out["take_profit"], 0.01)
	assert.InDelta(t, 1.5, out["risk_reward_ratio"], 0.01)
	assert.Equal(t, "low_vol_trending", out["regime"])
	assert.NotEmpty(t, out["note"])
}

// TestGetRiskMetrics_NoRegime verifies that regime field is omitted when empty.
func TestGetRiskMetrics_NoRegime(t *testing.T) {
	b := &riskMetricsStub{
		fetchFn: func(_ context.Context, _ string) (*bullarc.RiskMetrics, string, error) {
			return &bullarc.RiskMetrics{ATR: 1.0, PositionSizePct: 5.0, StopLoss: 98.0, TakeProfit: 103.0, RiskRewardRatio: 1.5}, "", nil
		},
	}

	text, isError := callResearchTool(t, b, "get_risk_metrics", map[string]any{"symbol": "TSLA"})
	require.False(t, isError, "expected no error, got: %s", text)

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	assert.Nil(t, out["regime"], "regime must be absent when empty")
}

// TestGetRiskMetrics_BackendError verifies that a backend error surfaces as a tool error.
func TestGetRiskMetrics_BackendError(t *testing.T) {
	b := &riskMetricsStub{
		fetchFn: func(_ context.Context, _ string) (*bullarc.RiskMetrics, string, error) {
			return nil, "", fmt.Errorf("insufficient data for ATR")
		},
	}

	text, isError := callResearchTool(t, b, "get_risk_metrics", map[string]any{"symbol": "AAPL"})
	assert.True(t, isError)
	assert.Contains(t, text, "insufficient data for ATR")
}

// ---- analyze_with_ai tests ----

// TestAnalyzeWithAI_MissingSymbol verifies that omitting symbol returns an error.
func TestAnalyzeWithAI_MissingSymbol(t *testing.T) {
	b := &stubBackend{hasLLMProvider: true}
	text, isError := callResearchTool(t, b, "analyze_with_ai", map[string]any{})
	assert.True(t, isError)
	assert.Contains(t, text, "symbol is required")
}

// TestAnalyzeWithAI_NoLLMProvider verifies that a missing LLM provider returns an error.
func TestAnalyzeWithAI_NoLLMProvider(t *testing.T) {
	b := &stubBackend{hasLLMProvider: false}
	text, isError := callResearchTool(t, b, "analyze_with_ai", map[string]any{"symbol": "AAPL"})
	assert.True(t, isError)
	assert.Contains(t, text, "LLM provider required")
}

// TestAnalyzeWithAI_QuickDepth verifies that quick depth uses the standard Analyze path.
func TestAnalyzeWithAI_QuickDepth(t *testing.T) {
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)
	var capturedReq bullarc.AnalysisRequest
	b := &stubBackend{
		hasLLMProvider: true,
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			capturedReq = req
			return bullarc.AnalysisResult{
				Symbol:      req.Symbol,
				Timestamp:   now,
				LLMAnalysis: "Bullish trend confirmed by RSI and SMA.",
				Regime:      "low_vol_trending",
				Signals: []bullarc.Signal{
					{Type: bullarc.SignalBuy, Confidence: 72.0, Indicator: "composite", Symbol: req.Symbol},
				},
				Risk: &bullarc.RiskMetrics{ATR: 2.1, PositionSizePct: 4.76, StopLoss: 146.0, TakeProfit: 153.1, RiskRewardRatio: 1.5},
			}, nil
		},
	}

	text, isError := callResearchTool(t, b, "analyze_with_ai", map[string]any{
		"symbol": "AAPL",
		"depth":  "quick",
	})
	require.False(t, isError, "expected no error, got: %s", text)
	assert.True(t, capturedReq.UseLLM, "UseLLM must be true for analyze_with_ai")
	assert.Equal(t, "AAPL", capturedReq.Symbol)

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	assert.Equal(t, "AAPL", out["symbol"])
	assert.Equal(t, "quick", out["depth"])
	assert.Equal(t, "BUY", out["signal"])
	assert.InDelta(t, 72.0, out["confidence"], 0.01)
	assert.Equal(t, "low_vol_trending", out["regime"])
	assert.Equal(t, "Bullish trend confirmed by RSI and SMA.", out["thesis"])
	assert.NotNil(t, out["risk"])
}

// TestAnalyzeWithAI_DeepDepth verifies that deep depth calls AnalyzeDeep.
func TestAnalyzeWithAI_DeepDepth(t *testing.T) {
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)
	var deepCalled bool
	b := &deepAnalyzerStub{
		stubBackend: stubBackend{hasLLMProvider: true},
		deepFn: func(_ context.Context, symbol string) (bullarc.AnalysisResult, error) {
			deepCalled = true
			return bullarc.AnalysisResult{
				Symbol:      symbol,
				Timestamp:   now,
				LLMAnalysis: "Multi-step synthesis: bullish momentum with positive news catalysts.",
				Regime:      "low_vol_trending",
				Signals: []bullarc.Signal{
					{Type: bullarc.SignalBuy, Confidence: 81.0, Indicator: "LLMMultiStep", Symbol: symbol},
				},
			}, nil
		},
	}

	text, isError := callResearchTool(t, b, "analyze_with_ai", map[string]any{
		"symbol": "AAPL",
		"depth":  "deep",
	})
	require.False(t, isError, "expected no error for deep analysis, got: %s", text)
	assert.True(t, deepCalled, "AnalyzeDeep must be called for depth=deep")

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	assert.Equal(t, "deep", out["depth"])
	assert.Equal(t, "BUY", out["signal"])
	assert.InDelta(t, 81.0, out["confidence"], 0.01)
	assert.Contains(t, out["thesis"].(string), "Multi-step synthesis")
}

// TestAnalyzeWithAI_DeepDepthNoCapability verifies that deep depth without AnalyzeDeep returns an error.
func TestAnalyzeWithAI_DeepDepthNoCapability(t *testing.T) {
	b := &stubBackend{hasLLMProvider: true}
	text, isError := callResearchTool(t, b, "analyze_with_ai", map[string]any{
		"symbol": "AAPL",
		"depth":  "deep",
	})
	assert.True(t, isError)
	assert.Contains(t, text, "AnalyzeDeep")
}

// TestAnalyzeWithAI_DefaultDepthIsQuick verifies that omitting depth defaults to quick.
func TestAnalyzeWithAI_DefaultDepthIsQuick(t *testing.T) {
	now := time.Now()
	b := &stubBackend{
		hasLLMProvider: true,
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			return bullarc.AnalysisResult{
				Symbol:    req.Symbol,
				Timestamp: now,
				Signals: []bullarc.Signal{
					{Type: bullarc.SignalHold, Confidence: 50.0, Indicator: "composite", Symbol: req.Symbol},
				},
			}, nil
		},
	}

	text, isError := callResearchTool(t, b, "analyze_with_ai", map[string]any{"symbol": "AAPL"})
	require.False(t, isError, "expected no error, got: %s", text)

	var out map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &out))
	assert.Equal(t, "quick", out["depth"])
}

// TestAnalyzeWithAI_AnalysisError verifies that a backend error surfaces as a tool error.
func TestAnalyzeWithAI_AnalysisError(t *testing.T) {
	b := &stubBackend{
		hasLLMProvider: true,
		analyzeFunc: func(_ context.Context, _ bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			return bullarc.AnalysisResult{}, fmt.Errorf("data source unavailable")
		},
	}

	text, isError := callResearchTool(t, b, "analyze_with_ai", map[string]any{"symbol": "AAPL"})
	assert.True(t, isError)
	assert.Contains(t, text, "analysis failed")
}

// TestAnalyzeWithAI_NoSignals verifies that an empty signals list returns an informative error.
func TestAnalyzeWithAI_NoSignals(t *testing.T) {
	b := &stubBackend{
		hasLLMProvider: true,
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			return bullarc.AnalysisResult{Symbol: req.Symbol, Timestamp: time.Now()}, nil
		},
	}

	text, isError := callResearchTool(t, b, "analyze_with_ai", map[string]any{"symbol": "EMPTY"})
	assert.True(t, isError)
	assert.Contains(t, text, "no signals produced")
}

// ---- compare_symbols tests ----

// TestCompareSymbols_TooFewSymbols verifies that fewer than 2 symbols returns an error.
func TestCompareSymbols_TooFewSymbols(t *testing.T) {
	b := &stubBackend{}
	text, isError := callResearchTool(t, b, "compare_symbols", map[string]any{
		"symbols": []any{"AAPL"},
	})
	assert.True(t, isError)
	assert.Contains(t, text, "at least 2")
}

// TestCompareSymbols_MissingSymbols verifies that omitting symbols returns an error.
func TestCompareSymbols_MissingSymbols(t *testing.T) {
	b := &stubBackend{}
	text, isError := callResearchTool(t, b, "compare_symbols", map[string]any{})
	assert.True(t, isError)
	assert.Contains(t, text, "at least 2")
}

// TestCompareSymbols_RankedByConfidence verifies that symbols are ranked highest-confidence first.
func TestCompareSymbols_RankedByConfidence(t *testing.T) {
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)
	confidences := map[string]float64{"AAPL": 55.0, "TSLA": 80.0, "META": 65.0}
	b := &stubBackend{
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			return bullarc.AnalysisResult{
				Symbol:    req.Symbol,
				Timestamp: now,
				Signals: []bullarc.Signal{
					{Type: bullarc.SignalBuy, Confidence: confidences[req.Symbol], Indicator: "composite", Symbol: req.Symbol},
				},
			}, nil
		},
	}

	text, isError := callResearchTool(t, b, "compare_symbols", map[string]any{
		"symbols": []any{"AAPL", "TSLA", "META"},
	})
	require.False(t, isError, "expected no error, got: %s", text)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &results))
	require.Len(t, results, 3)

	// Ranked by confidence desc: TSLA(80) > META(65) > AAPL(55).
	assert.Equal(t, "TSLA", results[0]["symbol"])
	assert.Equal(t, float64(1), results[0]["rank"])
	assert.Equal(t, "META", results[1]["symbol"])
	assert.Equal(t, float64(2), results[1]["rank"])
	assert.Equal(t, "AAPL", results[2]["symbol"])
	assert.Equal(t, float64(3), results[2]["rank"])
}

// TestCompareSymbols_ErrorSymbolsSinkToBottom verifies that symbols with errors
// sink below successful ones in the ranking.
func TestCompareSymbols_ErrorSymbolsSinkToBottom(t *testing.T) {
	now := time.Now()
	b := &stubBackend{
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			if req.Symbol == "FAIL" {
				return bullarc.AnalysisResult{}, fmt.Errorf("data unavailable")
			}
			return bullarc.AnalysisResult{
				Symbol:    req.Symbol,
				Timestamp: now,
				Signals: []bullarc.Signal{
					{Type: bullarc.SignalHold, Confidence: 50.0, Indicator: "composite", Symbol: req.Symbol},
				},
			}, nil
		},
	}

	text, isError := callResearchTool(t, b, "compare_symbols", map[string]any{
		"symbols": []any{"AAPL", "FAIL"},
	})
	require.False(t, isError, "errors must not fail the whole call, got: %s", text)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &results))
	require.Len(t, results, 2)

	// AAPL succeeds, FAIL has error → AAPL ranked first.
	assert.Equal(t, "AAPL", results[0]["symbol"])
	assert.Nil(t, results[0]["error"])
	assert.Equal(t, "FAIL", results[1]["symbol"])
	assert.NotEmpty(t, results[1]["error"])
}

// TestCompareSymbols_IncludesRegime verifies that regime is forwarded from AnalysisResult.
func TestCompareSymbols_IncludesRegime(t *testing.T) {
	now := time.Now()
	b := &stubBackend{
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			return bullarc.AnalysisResult{
				Symbol:    req.Symbol,
				Timestamp: now,
				Regime:    "high_vol_trending",
				Signals: []bullarc.Signal{
					{Type: bullarc.SignalSell, Confidence: 60.0, Indicator: "composite", Symbol: req.Symbol},
				},
			}, nil
		},
	}

	text, isError := callResearchTool(t, b, "compare_symbols", map[string]any{
		"symbols": []any{"AAPL", "TSLA"},
	})
	require.False(t, isError, "expected no error, got: %s", text)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &results))
	require.Len(t, results, 2)
	assert.Equal(t, "high_vol_trending", results[0]["regime"])
}
