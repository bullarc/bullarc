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
	analyzeSymbol       string
	analyzeSymbols      string
	analyzeConfig       string
	analyzeCSV          string
	analyzeLLM          bool
	analyzeLLMKey       string
	analyzeAlpacaKey    string
	analyzeAlpacaSecret string
)

func init() {
	analyzeCmd.Flags().StringVarP(&analyzeSymbol, "symbol", "s", "", "symbol to analyze")
	analyzeCmd.Flags().StringVar(&analyzeSymbols, "symbols", "", "comma-separated list of symbols (table output)")
	analyzeCmd.Flags().StringVarP(&analyzeConfig, "config", "c", "", "path to config file")
	analyzeCmd.Flags().StringVar(&analyzeCSV, "csv", "", "path to CSV file for local data")
	analyzeCmd.Flags().BoolVar(&analyzeLLM, "llm", false, "generate plain English explanation via LLM")
	analyzeCmd.Flags().StringVar(&analyzeLLMKey, "llm-key", "", "Anthropic API key (overrides ANTHROPIC_API_KEY env var and config)")
	analyzeCmd.Flags().StringVar(&analyzeAlpacaKey, "alpaca-key", "", "Alpaca API key ID (overrides ALPACA_API_KEY env var and config)")
	analyzeCmd.Flags().StringVar(&analyzeAlpacaSecret, "alpaca-secret", "", "Alpaca secret key (overrides ALPACA_SECRET_KEY env var and config)")
}

func runAnalyze(cmd *cobra.Command, _ []string) error {
	symbols := resolveSymbols(analyzeSymbol, analyzeSymbols)
	if len(symbols) == 0 {
		symbols = loadWatchlistFromKeystore()
	}
	if len(symbols) == 0 {
		return fmt.Errorf("provide --symbol or --symbols, or configure a default watchlist with `bullarc configure --watchlist AAPL,MSFT`")
	}

	e, err := buildEngine(analyzeConfig, analyzeCSV, analyzeLLMKey, analyzeAlpacaKey, analyzeAlpacaSecret)
	if err != nil {
		return err
	}
	if !e.HasDataSource() {
		return errNoDataSource()
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

// errNoDataSource returns a descriptive error explaining how to configure a data source.
func errNoDataSource() error {
	return fmt.Errorf("no data source configured\n\n" +
		"Set Alpaca API credentials via environment variables:\n" +
		"  ALPACA_API_KEY=<key-id>\n" +
		"  ALPACA_SECRET_KEY=<secret>\n\n" +
		"Or pass them as flags:\n" +
		"  --alpaca-key <key-id> --alpaca-secret <secret>\n\n" +
		"Alternatively, use a local CSV file with --csv <path>")
}

// buildEngine constructs an Engine from an optional config file, optional CSV data source,
// and optional API key overrides. Key resolution order:
//   - Alpaca: alpacaKeyID flag > ALPACA_API_KEY env var > keystore > config file (when Enabled)
//   - LLM: llmKey flag > ANTHROPIC_API_KEY env var > keystore > config file
func buildEngine(cfgPath, csvPath, llmKey, alpacaKeyID, alpacaSecretKey string) (*engine.Engine, error) {
	var (
		e         *engine.Engine
		llmModel  string
		cfgLLMKey string
		cfgAlpaca config.AlpacaDataSourceConfig
	)

	if cfgPath != "" {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		e = engine.NewWithConfig(cfg)
		cfgLLMKey = cfg.LLM.APIKey
		llmModel = cfg.LLM.Model
		cfgAlpaca = cfg.DataSources.Alpaca
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
	} else {
		e = engine.New()
		for _, ind := range engine.DefaultIndicators() {
			e.RegisterIndicator(ind)
		}
	}

	if csvPath != "" {
		e.RegisterDataSource(datasource.NewCSVSource(csvPath))
	}

	// Load persistent credentials (best-effort; errors are silently ignored).
	var keystoreCreds config.Credentials
	if ksPath, err := config.DefaultKeystorePath(); err == nil {
		keystoreCreds, _ = config.LoadCredentials(ksPath)
	}

	// Resolve Alpaca credentials: flag > env var > keystore > config file (when Enabled).
	effectiveAlpacaKeyID := alpacaKeyID
	if effectiveAlpacaKeyID == "" {
		effectiveAlpacaKeyID = os.Getenv("ALPACA_API_KEY")
	}
	effectiveAlpacaSecretKey := alpacaSecretKey
	if effectiveAlpacaSecretKey == "" {
		effectiveAlpacaSecretKey = os.Getenv("ALPACA_SECRET_KEY")
	}
	if effectiveAlpacaKeyID == "" && keystoreCreds.AlpacaKeyID != "" {
		effectiveAlpacaKeyID = keystoreCreds.AlpacaKeyID
		if effectiveAlpacaSecretKey == "" {
			effectiveAlpacaSecretKey = keystoreCreds.AlpacaSecretKey
		}
	}
	var alpacaBaseURL string
	if effectiveAlpacaKeyID == "" && cfgAlpaca.Enabled {
		effectiveAlpacaKeyID = cfgAlpaca.KeyID
		effectiveAlpacaSecretKey = cfgAlpaca.SecretKey
		alpacaBaseURL = cfgAlpaca.BaseURL
	} else if cfgAlpaca.Enabled {
		alpacaBaseURL = cfgAlpaca.BaseURL
	}

	if effectiveAlpacaKeyID != "" {
		var opts []datasource.AlpacaOption
		if alpacaBaseURL != "" {
			opts = append(opts, datasource.WithBaseURL(alpacaBaseURL))
		}
		e.RegisterDataSource(datasource.NewAlpacaSource(effectiveAlpacaKeyID, effectiveAlpacaSecretKey, opts...))
	}

	// Resolve LLM key: flag > ANTHROPIC_API_KEY env var > keystore > config file.
	effectiveKey := llmKey
	if effectiveKey == "" {
		effectiveKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if effectiveKey == "" {
		effectiveKey = keystoreCreds.LLMAPIKey
	}
	if effectiveKey == "" {
		effectiveKey = cfgLLMKey
	}
	if effectiveKey != "" {
		e.RegisterLLMProvider(llm.NewAnthropicProvider(effectiveKey, llmModel))
	}

	return e, nil
}
