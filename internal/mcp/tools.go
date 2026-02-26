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
	BacktestCSV(ctx context.Context, csvPath, symbol string, indicators []string) (bullarc.BacktestResult, error)
	ListIndicators() []bullarc.IndicatorMeta
}

// RegisterTools adds the backtest_strategy and list_indicators tools to srv.
func RegisterTools(srv *Server, b Backend) {
	srv.AddTool(backTestStrategyTool(b))
	srv.AddTool(listIndicatorsTool(b))
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
