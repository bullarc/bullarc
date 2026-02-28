package main

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc/internal/engine"
)

//go:embed embed_ohlcv_100.csv
var embeddedCSV []byte

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Run a zero-config demo with embedded sample data",
	Long: `Run backtests against embedded OHLCV sample data (100 bars, AAPL Jul-Aug 2024).
No API keys, no config files, no data files required.`,
	RunE: runDemo,
}

type demoScenario struct {
	title      string
	indicators []string
}

func runDemo(cmd *cobra.Command, _ []string) error {
	// Suppress engine log noise for a clean demo output.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	tmpDir, err := os.MkdirTemp("", "bullarc-demo-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	csvPath := filepath.Join(tmpDir, "ohlcv_100.csv")
	if err := os.WriteFile(csvPath, embeddedCSV, 0o600); err != nil {
		return fmt.Errorf("write temp CSV: %w", err)
	}

	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}

	scenarios := []demoScenario{
		{
			title:      "Backtest: SMA + RSI (trend + momentum)",
			indicators: []string{"SMA_14", "RSI_14"},
		},
		{
			title:      "Backtest: RSI + MACD + SuperTrend (advanced combo)",
			indicators: []string{"RSI_14", "MACD_12_26_9", "SuperTrend_7_3.0"},
		},
		{
			title:      "Backtest: all 11 indicators (full suite)",
			indicators: nil,
		},
	}

	fmt.Println("bullarc demo — zero config required")
	fmt.Println("====================================")
	fmt.Println("Data: 100 AAPL bars, Jul-Aug 2024 (embedded)")
	fmt.Println()

	for i, sc := range scenarios {
		fmt.Printf("--- %d. %s ---\n\n", i+1, sc.title)

		result, err := e.BacktestCSV(cmd.Context(), csvPath, "AAPL", sc.indicators)
		if err != nil {
			return fmt.Errorf("backtest %q: %w", sc.title, err)
		}

		PrintBacktestResult(os.Stdout, result)
		fmt.Println()
	}

	fmt.Println("====================================")
	fmt.Println("Next steps:")
	fmt.Println("  bullarc analyze --csv data.csv --llm    AI-powered analysis")
	fmt.Println("  bullarc backtest --csv data.csv          backtest your own data")
	fmt.Println("  bullarc mcp                              use with Claude Desktop")
	fmt.Println("  bullarc configure --llm-key KEY          store credentials")

	return nil
}
