package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/internal/config"
	"github.com/bullarcdev/bullarc/internal/datasource"
	"github.com/bullarcdev/bullarc/internal/engine"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Run a one-shot technical analysis on a symbol",
	RunE:  runAnalyze,
}

var (
	analyzeSymbol string
	analyzeConfig string
	analyzeCSV    string
)

func init() {
	analyzeCmd.Flags().StringVarP(&analyzeSymbol, "symbol", "s", "", "symbol to analyze (required)")
	_ = analyzeCmd.MarkFlagRequired("symbol")
	analyzeCmd.Flags().StringVarP(&analyzeConfig, "config", "c", "", "path to config file")
	analyzeCmd.Flags().StringVar(&analyzeCSV, "csv", "", "path to CSV file for local data")
}

func runAnalyze(cmd *cobra.Command, _ []string) error {
	e, err := buildEngine(analyzeConfig, analyzeCSV)
	if err != nil {
		return err
	}
	result, err := e.Analyze(cmd.Context(), bullarc.AnalysisRequest{Symbol: analyzeSymbol})
	if err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	printResult(result)
	return nil
}

// buildEngine constructs an Engine from an optional config file and optional CSV data source.
func buildEngine(cfgPath, csvPath string) (*engine.Engine, error) {
	var e *engine.Engine
	if cfgPath != "" {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		e = engine.NewWithConfig(cfg)
		if cfg.DataSources.Alpaca.Enabled {
			e.RegisterDataSource(datasource.NewAlpacaSource(
				cfg.DataSources.Alpaca.KeyID,
				cfg.DataSources.Alpaca.SecretKey,
				datasource.WithBaseURL(cfg.DataSources.Alpaca.BaseURL),
			))
		}
	} else {
		e = engine.New()
		for _, ind := range engine.DefaultIndicators() {
			e.RegisterIndicator(ind)
		}
	}
	if csvPath != "" {
		e.RegisterDataSource(datasource.NewCSVSource(csvPath))
	}
	return e, nil
}

// printResult writes a human-readable analysis summary to stdout.
func printResult(result bullarc.AnalysisResult) {
	fmt.Printf("symbol:    %s\n", result.Symbol)
	fmt.Printf("timestamp: %s\n", result.Timestamp.Format(time.RFC3339))
	if len(result.Signals) == 0 {
		fmt.Println("no signals (insufficient data)")
		return
	}
	composite := result.Signals[0]
	fmt.Printf("signal:    %s (confidence=%.0f%%)\n", composite.Type, composite.Confidence)
	fmt.Printf("summary:   %s\n", composite.Explanation)
}
