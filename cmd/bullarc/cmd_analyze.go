package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/config"
	"github.com/bullarc/bullarc/internal/datasource"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/bullarc/bullarc/internal/llm"
)

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Run a one-shot technical analysis on one or more symbols",
	RunE:  runAnalyze,
}

var (
	analyzeSymbol  string
	analyzeSymbols string
	analyzeConfig  string
	analyzeCSV     string
	analyzeLLM     bool
	analyzeLLMKey  string
)

func init() {
	analyzeCmd.Flags().StringVarP(&analyzeSymbol, "symbol", "s", "", "symbol to analyze")
	analyzeCmd.Flags().StringVar(&analyzeSymbols, "symbols", "", "comma-separated list of symbols (table output)")
	analyzeCmd.Flags().StringVarP(&analyzeConfig, "config", "c", "", "path to config file")
	analyzeCmd.Flags().StringVar(&analyzeCSV, "csv", "", "path to CSV file for local data")
	analyzeCmd.Flags().BoolVar(&analyzeLLM, "llm", false, "generate plain English explanation via LLM")
	analyzeCmd.Flags().StringVar(&analyzeLLMKey, "llm-key", "", "Anthropic API key (overrides config and ANTHROPIC_API_KEY env var)")
}

func runAnalyze(cmd *cobra.Command, _ []string) error {
	symbols := resolveSymbols(analyzeSymbol, analyzeSymbols)
	if len(symbols) == 0 {
		return fmt.Errorf("provide --symbol or --symbols")
	}

	e, err := buildEngine(analyzeConfig, analyzeCSV, analyzeLLMKey)
	if err != nil {
		return err
	}

	if len(symbols) == 1 {
		result, err := e.Analyze(cmd.Context(), bullarc.AnalysisRequest{Symbol: symbols[0], UseLLM: analyzeLLM})
		if err != nil {
			return fmt.Errorf("analyze %s: %w", symbols[0], err)
		}
		PrintResult(os.Stdout, result)
		return nil
	}

	results := make([]bullarc.AnalysisResult, 0, len(symbols))
	for _, sym := range symbols {
		result, err := e.Analyze(cmd.Context(), bullarc.AnalysisRequest{Symbol: sym, UseLLM: analyzeLLM})
		if err != nil {
			return fmt.Errorf("analyze %s: %w", sym, err)
		}
		results = append(results, result)
	}
	PrintTable(os.Stdout, results)
	return nil
}

// resolveSymbols combines the single --symbol flag and the comma-separated --symbols flag.
func resolveSymbols(single, multi string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	add(single)
	for _, s := range strings.Split(multi, ",") {
		add(s)
	}
	return out
}

// buildEngine constructs an Engine from an optional config file, optional CSV data source,
// and an optional LLM API key override. LLM key resolution order:
// llmKey param > config file > ANTHROPIC_API_KEY env var.
func buildEngine(cfgPath, csvPath, llmKey string) (*engine.Engine, error) {
	var (
		e         *engine.Engine
		llmModel  string
		cfgLLMKey string
	)
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
		cfgLLMKey = cfg.LLM.APIKey
		llmModel = cfg.LLM.Model
	} else {
		e = engine.New()
		for _, ind := range engine.DefaultIndicators() {
			e.RegisterIndicator(ind)
		}
	}
	if csvPath != "" {
		e.RegisterDataSource(datasource.NewCSVSource(csvPath))
	}
	effectiveKey := llmKey
	if effectiveKey == "" {
		effectiveKey = cfgLLMKey
	}
	if effectiveKey == "" {
		effectiveKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if effectiveKey != "" {
		e.RegisterLLMProvider(llm.NewAnthropicProvider(effectiveKey, llmModel))
	}
	return e, nil
}

