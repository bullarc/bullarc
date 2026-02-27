package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/config"
	"github.com/bullarc/bullarc/internal/datasource"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/bullarc/bullarc/internal/llm"
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
	analyzeLLM    bool
)

func init() {
	analyzeCmd.Flags().StringVarP(&analyzeSymbol, "symbol", "s", "", "symbol to analyze (required)")
	_ = analyzeCmd.MarkFlagRequired("symbol")
	analyzeCmd.Flags().StringVarP(&analyzeConfig, "config", "c", "", "path to config file")
	analyzeCmd.Flags().StringVar(&analyzeCSV, "csv", "", "path to CSV file for local data")
	analyzeCmd.Flags().BoolVar(&analyzeLLM, "llm", false, "generate plain English explanation via LLM")
}

func runAnalyze(cmd *cobra.Command, _ []string) error {
	e, err := buildEngine(analyzeConfig, analyzeCSV)
	if err != nil {
		return err
	}
	result, err := e.Analyze(cmd.Context(), bullarc.AnalysisRequest{Symbol: analyzeSymbol, UseLLM: analyzeLLM})
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
		if cfg.DataSources.Massive.Enabled {
			var opts []datasource.MassiveOption
			if cfg.DataSources.Massive.BaseURL != "" {
				opts = append(opts, datasource.WithMassiveBaseURL(cfg.DataSources.Massive.BaseURL))
			}
			e.RegisterDataSource(datasource.NewMassiveSource(
				cfg.DataSources.Massive.APIKey,
				opts...,
			))
		}
		if cfg.LLM.APIKey != "" {
			e.RegisterLLMProvider(llm.NewAnthropicProvider(cfg.LLM.APIKey, cfg.LLM.Model))
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
	if result.LLMAnalysis != "" {
		fmt.Printf("\nexplanation:\n%s\n", result.LLMAnalysis)
	}
}
