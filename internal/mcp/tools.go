package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bullarc/bullarc"
)

// Backend provides the capabilities exposed through MCP tools.
// The concrete *engine.Engine satisfies this interface.
type Backend interface {
	Analyze(ctx context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error)
	BacktestCSV(ctx context.Context, csvPath, symbol string, indicators []string) (bullarc.BacktestResult, error)
	ListIndicators() []bullarc.IndicatorMeta
	HasLLMProvider() bool
}

// RegisterTools adds the get_signals, backtest_strategy, list_indicators, and
// explain_signal tools to srv.
func RegisterTools(srv *Server, b Backend) {
	srv.AddTool(getSignalsTool(b))
	srv.AddTool(backTestStrategyTool(b))
	srv.AddTool(listIndicatorsTool(b))
	srv.AddTool(explainSignalTool(b))
}

// backTestStrategyTool builds the backtest_strategy MCP tool.
// It loads historical data from a CSV file and returns backtest summary statistics.
func backTestStrategyTool(b Backend) Tool {
	return Tool{
		Name: "backtest_strategy",
		Description: "Run a backtest on historical OHLCV data from a CSV file and return " +
			"performance statistics including signal counts, simulated return, maximum " +
			"drawdown, and win rate.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"csv_path": map[string]any{
					"type":        "string",
					"description": "Absolute path to a CSV file with OHLCV data (date,open,high,low,close,volume).",
				},
				"symbol": map[string]any{
					"type":        "string",
					"description": "Ticker symbol label for the backtest result. Defaults to UNKNOWN.",
				},
				"indicators": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Indicator names to use (e.g. SMA_14, RSI_14). Empty means all registered indicators.",
				},
			},
			"required": []string{"csv_path"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			csvPath, _ := args["csv_path"].(string)
			if csvPath == "" {
				return "", fmt.Errorf("csv_path is required")
			}

			symbol, _ := args["symbol"].(string)
			if symbol == "" {
				symbol = "UNKNOWN"
			}

			var indicators []string
			if raw, ok := args["indicators"].([]any); ok {
				for _, v := range raw {
					if s, ok := v.(string); ok && s != "" {
						indicators = append(indicators, s)
					}
				}
			}

			// Use a minimum 60-second timeout for the backtest as required.
			ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			result, err := b.BacktestCSV(ctx, csvPath, symbol, indicators)
			if err != nil {
				return "", fmt.Errorf("backtest failed: %w", err)
			}

			output := backtestOutput{
				Symbol:       result.Symbol,
				Timestamp:    result.Timestamp.Format(time.RFC3339),
				TotalSignals: result.Summary.TotalSignals,
				BuyCount:     result.Summary.BuyCount,
				SellCount:    result.Summary.SellCount,
				HoldCount:    result.Summary.HoldCount,
				SimReturn:    result.Summary.SimReturn,
				MaxDrawdown:  result.Summary.MaxDrawdown,
				WinRate:      result.Summary.WinRate,
			}
			data, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	}
}

// backtestOutput is the JSON shape returned by the backtest_strategy tool.
type backtestOutput struct {
	Symbol       string  `json:"symbol"`
	Timestamp    string  `json:"timestamp"`
	TotalSignals int     `json:"total_signals"`
	BuyCount     int     `json:"buy_count"`
	SellCount    int     `json:"sell_count"`
	HoldCount    int     `json:"hold_count"`
	SimReturn    float64 `json:"sim_return_pct"`
	MaxDrawdown  float64 `json:"max_drawdown_pct"`
	WinRate      float64 `json:"win_rate_pct"`
}

// getSignalsTool builds the get_signals MCP tool.
// It runs a live analysis for each requested symbol and returns the composite
// trading signal, confidence score, and timestamp.
func getSignalsTool(b Backend) Tool {
	return Tool{
		Name: "get_signals",
		Description: "Analyze one or more ticker symbols using registered indicators and return " +
			"the current composite trading signal (BUY, SELL, or HOLD), confidence score, " +
			"and timestamp for each symbol.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbols": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "One or more ticker symbols to analyze (e.g. [\"AAPL\", \"TSLA\"]).",
					"minItems":    1,
				},
			},
			"required": []string{"symbols"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			raw, ok := args["symbols"].([]any)
			if !ok || len(raw) == 0 {
				return "", fmt.Errorf("symbols is required and must be a non-empty array")
			}

			var symbols []string
			for _, v := range raw {
				s, ok := v.(string)
				if !ok || s == "" {
					return "", fmt.Errorf("each symbol must be a non-empty string")
				}
				symbols = append(symbols, s)
			}

			results := make([]signalOutput, 0, len(symbols))
			for _, sym := range symbols {
				out := signalOutput{Symbol: sym}

				result, err := b.Analyze(ctx, bullarc.AnalysisRequest{Symbol: sym})
				if err != nil {
					out.Error = err.Error()
					results = append(results, out)
					continue
				}

				if len(result.Signals) == 0 {
					out.Error = fmt.Sprintf("no signals produced for %s (insufficient data or no data source)", sym)
					results = append(results, out)
					continue
				}

				composite := result.Signals[0]
				out.Signal = string(composite.Type)
				out.Confidence = composite.Confidence
				out.Timestamp = result.Timestamp.Format(time.RFC3339)
				out.Explanation = composite.Explanation
				results = append(results, out)
			}

			data, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal results: %w", err)
			}
			return string(data), nil
		},
	}
}

// signalOutput is the JSON shape for a single symbol result returned by get_signals.
type signalOutput struct {
	Symbol      string  `json:"symbol"`
	Signal      string  `json:"signal,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
	Timestamp   string  `json:"timestamp,omitempty"`
	Explanation string  `json:"explanation,omitempty"`
	Error       string  `json:"error,omitempty"`
}

// explainSignalTool builds the explain_signal MCP tool.
// It runs analysis with LLM enabled and returns the plain-English explanation.
func explainSignalTool(b Backend) Tool {
	return Tool{
		Name: "explain_signal",
		Description: "Generate a plain-English explanation of the current trading signal for a " +
			"symbol using an LLM. Returns an explanation of what the technical indicators mean " +
			"for a retail investor. Requires an LLM provider to be configured.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{
					"type":        "string",
					"description": "The ticker symbol to explain (e.g. \"AAPL\").",
				},
			},
			"required": []string{"symbol"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			symbol, _ := args["symbol"].(string)
			if symbol == "" {
				return "", fmt.Errorf("symbol is required")
			}

			if !b.HasLLMProvider() {
				return "", fmt.Errorf("LLM key is required for signal explanations: configure an LLM provider via ANTHROPIC_API_KEY or the config file")
			}

			result, err := b.Analyze(ctx, bullarc.AnalysisRequest{Symbol: symbol, UseLLM: true})
			if err != nil {
				return "", fmt.Errorf("analysis failed: %w", err)
			}

			if result.LLMAnalysis == "" {
				return "", fmt.Errorf("no explanation produced for %s", symbol)
			}

			return result.LLMAnalysis, nil
		},
	}
}

// listIndicatorsTool builds the list_indicators MCP tool.
// It returns metadata for all indicators registered with the engine.
func listIndicatorsTool(b Backend) Tool {
	return Tool{
		Name:        "list_indicators",
		Description: "List all technical indicators registered with the engine, including their names, categories, and parameters.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(_ context.Context, _ map[string]any) (string, error) {
			metas := b.ListIndicators()
			data, err := json.MarshalIndent(metas, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal indicators: %w", err)
			}
			return string(data), nil
		},
	}
}
