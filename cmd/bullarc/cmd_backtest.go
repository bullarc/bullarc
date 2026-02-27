package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc/internal/engine"
)

var backtestCmd = &cobra.Command{
	Use:   "backtest",
	Short: "Run a backtest against historical data from a CSV file",
	Long: `Run technical indicator signals against historical OHLCV data loaded from a CSV file.
The CSV must have a header row followed by columns: date,open,high,low,close,volume.
Dates must be in "2006-01-02" format.`,
	RunE: runBacktest,
}

var (
	backtestCSV        string
	backtestSymbol     string
	backtestIndicators string
	backtestConfig     string
)

func init() {
	backtestCmd.Flags().StringVar(&backtestCSV, "csv", "", "path to CSV file with historical OHLCV data (required)")
	backtestCmd.Flags().StringVarP(&backtestSymbol, "symbol", "s", "UNKNOWN", "symbol label for the backtest result")
	backtestCmd.Flags().StringVar(&backtestIndicators, "indicators", "", "comma-separated list of indicator names to use (default: all)")
	backtestCmd.Flags().StringVarP(&backtestConfig, "config", "c", "", "path to config file")
	_ = backtestCmd.MarkFlagRequired("csv")
}

func runBacktest(cmd *cobra.Command, _ []string) error {
	if backtestCSV == "" {
		return fmt.Errorf("--csv is required")
	}

	var e *engine.Engine
	if backtestConfig != "" {
		var err error
		e, err = buildEngine(backtestConfig, "", "", "", "")
		if err != nil {
			return err
		}
	} else {
		e = engine.New()
		for _, ind := range engine.DefaultIndicators() {
			e.RegisterIndicator(ind)
		}
	}

	indicators := parseIndicatorList(backtestIndicators)
	result, err := e.BacktestCSV(cmd.Context(), backtestCSV, backtestSymbol, indicators)
	if err != nil {
		return fmt.Errorf("backtest failed: %w", err)
	}

	PrintBacktestResult(os.Stdout, result)
	return nil
}

// parseIndicatorList splits a comma-separated indicator list into a slice.
// Returns nil when the input is empty.
func parseIndicatorList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, name := range strings.Split(s, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}
