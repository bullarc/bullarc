package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/bullarc/bullarc"
)

// getNewsSentimentTool builds the get_news_sentiment MCP tool.
// It fetches and LLM-scores recent headlines for a symbol over a configurable
// time window, then returns each headline's sentiment plus an aggregate score.
// Requires the backend to implement newsSentimentFetcher (news source + LLM
// sentiment scorer registered). When the capability is absent, returns an
// informative error so the client can reconfigure.
func getNewsSentimentTool(b Backend) Tool {
	return Tool{
		Name: "get_news_sentiment",
		Description: "Fetch and score recent news headlines for a symbol using an LLM sentiment " +
			"scorer. Returns each headline with its bullish/neutral/bearish classification " +
			"and confidence score, plus an aggregate sentiment and weighted-average confidence " +
			"for the requested time window. Requires a news source and LLM sentiment scorer " +
			"to be configured.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{
					"type":        "string",
					"description": "Ticker symbol to fetch news for (e.g. \"AAPL\").",
				},
				"hours": map[string]any{
					"type":        "number",
					"description": "Number of hours to look back for news (e.g. 24). Defaults to 24.",
				},
			},
			"required": []string{"symbol"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			symbol, _ := args["symbol"].(string)
			if symbol == "" {
				return "", fmt.Errorf("symbol is required")
			}

			hours := 24
			if v, ok := args["hours"].(float64); ok && v > 0 {
				hours = int(v)
			}

			nsp, ok := b.(newsSentimentFetcher)
			if !ok {
				return "", fmt.Errorf("news sentiment not available: backend does not implement " +
					"GetNewsSentiment; register a news source and LLM sentiment scorer")
			}

			summary, err := nsp.GetNewsSentiment(ctx, symbol, hours)
			if err != nil {
				return "", fmt.Errorf("get_news_sentiment failed: %w", err)
			}

			out := newsSentimentOutput{
				Symbol:             summary.Symbol,
				Hours:              hours,
				ArticleCount:       len(summary.Headlines),
				AggregateSentiment: summary.AggregateSentiment,
				AggregateScore:     summary.AggregateScore,
				Headlines:          make([]headlineItem, 0, len(summary.Headlines)),
			}
			for _, h := range summary.Headlines {
				out.Headlines = append(out.Headlines, headlineItem{
					Headline:    h.Headline,
					Source:      h.Source,
					PublishedAt: h.PublishedAt.Format(time.RFC3339),
					Sentiment:   h.Sentiment,
					Confidence:  h.Confidence,
					Reasoning:   h.Reasoning,
				})
			}

			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	}
}

// newsSentimentOutput is the JSON shape returned by the get_news_sentiment tool.
type newsSentimentOutput struct {
	Symbol             string        `json:"symbol"`
	Hours              int           `json:"hours"`
	ArticleCount       int           `json:"article_count"`
	AggregateSentiment string        `json:"aggregate_sentiment"`
	AggregateScore     float64       `json:"aggregate_score"`
	Headlines          []headlineItem `json:"headlines"`
}

// headlineItem is the per-headline JSON shape within get_news_sentiment output.
type headlineItem struct {
	Headline    string `json:"headline"`
	Source      string `json:"source"`
	PublishedAt string `json:"published_at"`
	Sentiment   string `json:"sentiment"`
	Confidence  int    `json:"confidence"`
	Reasoning   string `json:"reasoning,omitempty"`
}

// getRiskMetricsTool builds the get_risk_metrics MCP tool.
// It computes ATR-based position sizing, stop-loss, and take-profit for a symbol
// and optionally returns the LLM-detected market regime. The computation runs
// on-demand regardless of the engine's risk configuration.
func getRiskMetricsTool(b Backend) Tool {
	return Tool{
		Name: "get_risk_metrics",
		Description: "Compute ATR-based position sizing, stop-loss, and take-profit levels for a " +
			"symbol. Also returns the current market regime (e.g. low_vol_trending) when " +
			"regime detection is configured. Risk metrics are computed on-demand and do not " +
			"require the engine's risk configuration to be pre-enabled. Requires a data source " +
			"and ATR_14 indicator to be registered.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{
					"type":        "string",
					"description": "Ticker symbol to compute risk metrics for (e.g. \"AAPL\").",
				},
			},
			"required": []string{"symbol"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			symbol, _ := args["symbol"].(string)
			if symbol == "" {
				return "", fmt.Errorf("symbol is required")
			}

			rmf, ok := b.(riskMetricsFetcher)
			if !ok {
				return "", fmt.Errorf("risk metrics not available: backend does not implement " +
					"FetchRiskMetrics; register a data source and ATR_14 indicator")
			}

			risk, regime, err := rmf.FetchRiskMetrics(ctx, symbol)
			if err != nil {
				return "", fmt.Errorf("get_risk_metrics failed: %w", err)
			}

			out := riskMetricsOutput{
				Symbol:          symbol,
				ATR:             risk.ATR,
				PositionSizePct: risk.PositionSizePct,
				StopLoss:        risk.StopLoss,
				TakeProfit:      risk.TakeProfit,
				RiskRewardRatio: risk.RiskRewardRatio,
				Regime:          regime,
				Note:            "Stop-loss and take-profit computed for BUY-direction entry at current price.",
			}

			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	}
}

// riskMetricsOutput is the JSON shape returned by the get_risk_metrics tool.
type riskMetricsOutput struct {
	Symbol          string  `json:"symbol"`
	ATR             float64 `json:"atr"`
	PositionSizePct float64 `json:"position_size_pct"`
	StopLoss        float64 `json:"stop_loss"`
	TakeProfit      float64 `json:"take_profit"`
	RiskRewardRatio float64 `json:"risk_reward_ratio"`
	Regime          string  `json:"regime,omitempty"`
	Note            string  `json:"note,omitempty"`
}

// analyzeWithAITool builds the analyze_with_ai MCP tool.
// The depth parameter selects the analysis mode:
//   - "quick" (default): single-step LLM meta-signal analysis.
//   - "deep": three-step chain (technical thesis → news thesis → synthesis).
//
// The returned thesis includes technical signals, news sentiment, risk metrics,
// and plain-English reasoning. An LLM provider must be configured.
func analyzeWithAITool(b Backend) Tool {
	return Tool{
		Name: "analyze_with_ai",
		Description: "Run a full AI-powered analysis for a symbol and return a synthesized " +
			"trading thesis. The depth parameter controls the analysis chain: " +
			"\"quick\" runs a single-step LLM meta-signal (fast, one LLM call), while " +
			"\"deep\" runs a three-step chain (technical thesis → news thesis → synthesis) " +
			"for a more thorough assessment. The response includes the composite signal, " +
			"confidence, regime, risk metrics, and the full plain-English reasoning. " +
			"Requires an LLM provider to be configured.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{
					"type":        "string",
					"description": "Ticker symbol to analyze (e.g. \"AAPL\").",
				},
				"depth": map[string]any{
					"type":        "string",
					"enum":        []string{"quick", "deep"},
					"description": "Analysis depth: \"quick\" for single-step LLM, \"deep\" for multi-step chain. Defaults to \"quick\".",
				},
			},
			"required": []string{"symbol"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			symbol, _ := args["symbol"].(string)
			if symbol == "" {
				return "", fmt.Errorf("symbol is required")
			}

			depth, _ := args["depth"].(string)
			if depth == "" {
				depth = "quick"
			}

			if !b.HasLLMProvider() {
				return "", fmt.Errorf("LLM provider required for analyze_with_ai: configure ANTHROPIC_API_KEY or equivalent")
			}

			var result bullarc.AnalysisResult
			var err error

			if depth == "deep" {
				da, ok := b.(deepAnalyzer)
				if !ok {
					return "", fmt.Errorf("deep analysis not available: backend does not implement AnalyzeDeep")
				}
				result, err = da.AnalyzeDeep(ctx, symbol)
			} else {
				result, err = b.Analyze(ctx, bullarc.AnalysisRequest{Symbol: symbol, UseLLM: true})
			}

			if err != nil {
				return "", fmt.Errorf("analysis failed: %w", err)
			}

			if len(result.Signals) == 0 {
				return "", fmt.Errorf("no signals produced for %s (insufficient data or no data source)", symbol)
			}

			composite := result.Signals[0]
			out := analyzeWithAIOutput{
				Symbol:     result.Symbol,
				Depth:      depth,
				Signal:     string(composite.Type),
				Confidence: composite.Confidence,
				Regime:     result.Regime,
				Thesis:     result.LLMAnalysis,
				Timestamp:  result.Timestamp.Format(time.RFC3339),
			}
			if result.Risk != nil {
				out.Risk = &riskSummary{
					ATR:             result.Risk.ATR,
					PositionSizePct: result.Risk.PositionSizePct,
					StopLoss:        result.Risk.StopLoss,
					TakeProfit:      result.Risk.TakeProfit,
					RiskRewardRatio: result.Risk.RiskRewardRatio,
				}
			}

			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	}
}

// analyzeWithAIOutput is the JSON shape returned by the analyze_with_ai tool.
type analyzeWithAIOutput struct {
	Symbol     string       `json:"symbol"`
	Depth      string       `json:"depth"`
	Signal     string       `json:"signal"`
	Confidence float64      `json:"confidence"`
	Regime     string       `json:"regime,omitempty"`
	Thesis     string       `json:"thesis,omitempty"`
	Risk       *riskSummary `json:"risk,omitempty"`
	Timestamp  string       `json:"timestamp"`
}

// riskSummary is the embedded risk section within analyzeWithAIOutput.
type riskSummary struct {
	ATR             float64 `json:"atr"`
	PositionSizePct float64 `json:"position_size_pct"`
	StopLoss        float64 `json:"stop_loss"`
	TakeProfit      float64 `json:"take_profit"`
	RiskRewardRatio float64 `json:"risk_reward_ratio"`
}

// compareSymbolsTool builds the compare_symbols MCP tool.
// It runs a standard analysis (no LLM) on each symbol concurrently and returns
// the results ranked by composite confidence score descending. All symbols are
// analyzed and errors are reported per-symbol rather than failing the whole call.
func compareSymbolsTool(b Backend) Tool {
	return Tool{
		Name: "compare_symbols",
		Description: "Analyze multiple symbols and return a ranked comparison by composite " +
			"confidence score (highest first). Each entry shows the signal direction " +
			"(BUY/SELL/HOLD), confidence score, regime, and timestamp. Errors for " +
			"individual symbols are reported inline without failing the whole call.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbols": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Two or more ticker symbols to compare (e.g. [\"AAPL\", \"TSLA\", \"MSFT\"]).",
					"minItems":    2,
				},
			},
			"required": []string{"symbols"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			raw, ok := args["symbols"].([]any)
			if !ok || len(raw) < 2 {
				return "", fmt.Errorf("symbols must be an array of at least 2 ticker symbols")
			}

			symbols := make([]string, 0, len(raw))
			for _, v := range raw {
				s, ok := v.(string)
				if !ok || s == "" {
					return "", fmt.Errorf("each symbol must be a non-empty string")
				}
				symbols = append(symbols, s)
			}

			entries := make([]symbolCompareEntry, 0, len(symbols))
			for _, sym := range symbols {
				entry := symbolCompareEntry{Symbol: sym}
				result, err := b.Analyze(ctx, bullarc.AnalysisRequest{Symbol: sym})
				if err != nil {
					entry.Error = err.Error()
					entries = append(entries, entry)
					continue
				}
				if len(result.Signals) == 0 {
					entry.Error = fmt.Sprintf("no signals produced for %s", sym)
					entries = append(entries, entry)
					continue
				}
				composite := result.Signals[0]
				entry.Signal = string(composite.Type)
				entry.Confidence = composite.Confidence
				entry.Regime = result.Regime
				entry.Timestamp = result.Timestamp.Format(time.RFC3339)
				entries = append(entries, entry)
			}

			// Rank: symbols with errors sink to the bottom; among successful ones,
			// sort by confidence descending.
			sort.SliceStable(entries, func(i, j int) bool {
				if entries[i].Error != "" && entries[j].Error == "" {
					return false
				}
				if entries[i].Error == "" && entries[j].Error != "" {
					return true
				}
				return entries[i].Confidence > entries[j].Confidence
			})
			for i := range entries {
				entries[i].Rank = i + 1
			}

			data, err := json.MarshalIndent(entries, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal results: %w", err)
			}
			return string(data), nil
		},
	}
}

// symbolCompareEntry is the JSON shape for a single symbol in compare_symbols output.
type symbolCompareEntry struct {
	Rank       int     `json:"rank"`
	Symbol     string  `json:"symbol"`
	Signal     string  `json:"signal,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Regime     string  `json:"regime,omitempty"`
	Timestamp  string  `json:"timestamp,omitempty"`
	Error      string  `json:"error,omitempty"`
}
