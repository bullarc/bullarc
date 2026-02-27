package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Continuously analyze a symbol and emit results on new bars",
	RunE:  runWatch,
}

var (
	watchSymbol       string
	watchConfig       string
	watchCSV          string
	watchInterval     time.Duration
	watchLLMKey       string
	watchAlpacaKey    string
	watchAlpacaSecret string
)

func init() {
	watchCmd.Flags().StringVarP(&watchSymbol, "symbol", "s", "", "symbol to watch (required)")
	_ = watchCmd.MarkFlagRequired("symbol")
	watchCmd.Flags().StringVarP(&watchConfig, "config", "c", "", "path to config file")
	watchCmd.Flags().StringVar(&watchCSV, "csv", "", "path to CSV file for local data")
	watchCmd.Flags().DurationVarP(&watchInterval, "interval", "i", time.Minute, "poll interval")
	watchCmd.Flags().StringVar(&watchLLMKey, "llm-key", "", "Anthropic API key (overrides ANTHROPIC_API_KEY env var and config)")
	watchCmd.Flags().StringVar(&watchAlpacaKey, "alpaca-key", "", "Alpaca API key ID (overrides ALPACA_API_KEY env var and config)")
	watchCmd.Flags().StringVar(&watchAlpacaSecret, "alpaca-secret", "", "Alpaca secret key (overrides ALPACA_SECRET_KEY env var and config)")
}

func runWatch(cmd *cobra.Command, _ []string) error {
	e, err := buildEngine(watchConfig, watchCSV, watchLLMKey, watchAlpacaKey, watchAlpacaSecret)
	if err != nil {
		return err
	}
	if !e.HasDataSource() {
		return errNoDataSource()
	}
	fmt.Printf("watching %s every %s (ctrl-c to stop)\n", watchSymbol, watchInterval)
	return e.Watch(cmd.Context(), bullarc.AnalysisRequest{Symbol: watchSymbol}, watchInterval, func(result bullarc.AnalysisResult) {
		PrintResult(os.Stdout, result)
	})
}
