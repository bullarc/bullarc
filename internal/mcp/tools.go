package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bullarc/bullarc"
)

// streamingBackend extends Backend with push-based signal subscription.
// The concrete *engine.Engine satisfies this interface.
type streamingBackend interface {
	Subscribe(ctx context.Context, symbol string) <-chan bullarc.Signal
}

// Backend provides the capabilities exposed through MCP tools.
// The concrete *engine.Engine satisfies this interface.
type Backend interface {
	Analyze(ctx context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error)
	BacktestCSV(ctx context.Context, csvPath, symbol string, indicators []string) (bullarc.BacktestResult, error)
	ExplainBacktestCSV(ctx context.Context, csvPath, symbol string, indicators []string) (bullarc.BacktestResult, error)
	ListIndicators() []bullarc.IndicatorMeta
	HasLLMProvider() bool
}

// RegisterTools adds the get_signals, backtest_strategy, list_indicators,
// explain_signal, stream_signals, and explain_backtest tools to srv.
func RegisterTools(srv *Server, b Backend) {
	srv.AddTool(getSignalsTool(b))
	srv.AddTool(backTestStrategyTool(b))
	srv.AddTool(listIndicatorsTool(b))
	srv.AddTool(explainSignalTool(b))
	srv.AddTool(streamSignalsTool(b))
	srv.AddTool(explainBacktestTool(b))
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

// streamSignalsTool builds the stream_signals MCP tool.
// It subscribes to the engine's signal bus for the given symbol, triggers a
// single analysis, and waits up to timeout_seconds for signals to arrive.
// This gives MCP clients push-based signal delivery: the client calls the tool
// once and receives all signals computed during the wait window.
func streamSignalsTool(b Backend) Tool {
	return Tool{
		Name: "stream_signals",
		Description: "Subscribe to signal updates for a symbol and return all signals computed " +
			"within a wait window. Unlike get_signals, this tool waits for the engine to push " +
			"new signals rather than polling, making it suitable for detecting rapid market changes. " +
			"Requires a streaming-capable backend.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{
					"type":        "string",
					"description": "Ticker symbol to subscribe to (e.g. \"AAPL\").",
				},
				"timeout_seconds": map[string]any{
					"type":        "number",
					"description": "Maximum seconds to wait for signals. Defaults to 5. Maximum 30.",
				},
			},
			"required": []string{"symbol"},
		},
		Handler: func(ctx context.Context, args map[string]any) (string, error) {
			symbol, _ := args["symbol"].(string)
			if symbol == "" {
				return "", fmt.Errorf("symbol is required")
			}

			timeoutSec := 5.0
			if v, ok := args["timeout_seconds"].(float64); ok && v > 0 {
				timeoutSec = v
				if timeoutSec > 30 {
					timeoutSec = 30
				}
			}

			sb, ok := b.(streamingBackend)
			if !ok {
				return "", fmt.Errorf("streaming not supported by this backend")
			}

			waitCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec*float64(time.Second)))
			defer cancel()

			ch := sb.Subscribe(waitCtx, symbol)

			// Trigger analysis to produce signals that will be pushed to the channel.
			go func() {
				_, _ = b.Analyze(waitCtx, bullarc.AnalysisRequest{Symbol: symbol})
			}()

			var signals []streamSignalOutput
			for {
				select {
				case sig, open := <-ch:
					if !open {
						goto done
					}
					signals = append(signals, streamSignalOutput{
						Symbol:      sig.Symbol,
						Signal:      string(sig.Type),
						Confidence:  sig.Confidence,
						Indicator:   sig.Indicator,
						Timestamp:   sig.Timestamp.Format(time.RFC3339),
						Explanation: sig.Explanation,
					})
				case <-waitCtx.Done():
					goto done
				}
			}
		done:
			if len(signals) == 0 {
				return "", fmt.Errorf("no signals received for %s within %.0fs", symbol, timeoutSec)
			}
			data, err := json.MarshalIndent(signals, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal signals: %w", err)
			}
			return string(data), nil
		},
	}
}

// streamSignalOutput is the JSON shape for a single signal in stream_signals output.
type streamSignalOutput struct {
	Symbol      string  `json:"symbol"`
	Signal      string  `json:"signal"`
	Confidence  float64 `json:"confidence"`
	Indicator   string  `json:"indicator"`
	Timestamp   string  `json:"timestamp"`
	Explanation string  `json:"explanation,omitempty"`
}

// explainBacktestTool builds the explain_backtest MCP tool.
// It runs a backtest on a CSV file and returns both the raw performance statistics
// and an AI-generated plain English explanation. When no LLM provider is configured,
// the raw stats are returned together with a note directing the user to configure one.
func explainBacktestTool(b Backend) Tool {
	return Tool{
		Name: "explain_backtest",
		Description: "Run a backtest on historical OHLCV data from a CSV file and return " +
			"performance statistics together with an AI-generated plain English explanation " +
			"covering overall strategy performance, notable winning and losing periods, and " +
			"which signals contributed most to the results. When no LLM provider is configured, " +
			"the raw statistics are returned with guidance on enabling AI explanations.",
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

			ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			result, err := b.ExplainBacktestCSV(ctx, csvPath, symbol, indicators)
			if err != nil {
				return "", fmt.Errorf("backtest failed: %w", err)
			}

			output := explainedBacktestOutput{
				Symbol:       result.Symbol,
				Timestamp:    result.Timestamp.Format(time.RFC3339),
				TotalSignals: result.Summary.TotalSignals,
				BuyCount:     result.Summary.BuyCount,
				SellCount:    result.Summary.SellCount,
				HoldCount:    result.Summary.HoldCount,
				SimReturn:    result.Summary.SimReturn,
				MaxDrawdown:  result.Summary.MaxDrawdown,
				WinRate:      result.Summary.WinRate,
				LLMAnalysis:  result.LLMAnalysis,
			}
			if !b.HasLLMProvider() {
				output.Note = "Configure an LLM provider (e.g. ANTHROPIC_API_KEY) to receive an AI-generated explanation."
			}

			data, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	}
}

// explainedBacktestOutput is the JSON shape returned by the explain_backtest tool.
type explainedBacktestOutput struct {
	Symbol       string  `json:"symbol"`
	Timestamp    string  `json:"timestamp"`
	TotalSignals int     `json:"total_signals"`
	BuyCount     int     `json:"buy_count"`
	SellCount    int     `json:"sell_count"`
	HoldCount    int     `json:"hold_count"`
	SimReturn    float64 `json:"sim_return_pct"`
	MaxDrawdown  float64 `json:"max_drawdown_pct"`
	WinRate      float64 `json:"win_rate_pct"`
	LLMAnalysis  string  `json:"llm_explanation,omitempty"`
	Note         string  `json:"note,omitempty"`
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
