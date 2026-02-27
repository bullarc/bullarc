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
	watchCmd.Flags().StringVarP(&watchSymbol, "symbol", "s", "", "symbol to watch (defaults to first symbol in saved watchlist)")
	watchCmd.Flags().StringVarP(&watchConfig, "config", "c", "", "path to config file")
	watchCmd.Flags().StringVar(&watchCSV, "csv", "", "path to CSV file for local data")
	watchCmd.Flags().DurationVarP(&watchInterval, "interval", "i", time.Minute, "poll interval")
	watchCmd.Flags().StringVar(&watchLLMKey, "llm-key", "", "Anthropic API key (overrides ANTHROPIC_API_KEY env var and config)")
	watchCmd.Flags().StringVar(&watchAlpacaKey, "alpaca-key", "", "Alpaca API key ID (overrides ALPACA_API_KEY env var and config)")
	watchCmd.Flags().StringVar(&watchAlpacaSecret, "alpaca-secret", "", "Alpaca secret key (overrides ALPACA_SECRET_KEY env var and config)")
}

func runWatch(cmd *cobra.Command, _ []string) error {
	sym := watchSymbol
	if sym == "" {
		syms := loadWatchlistFromKeystore()
		if len(syms) == 0 {
			return fmt.Errorf("provide --symbol or configure a default watchlist with `bullarc configure --watchlist AAPL,MSFT`")
		}
		sym = syms[0]
	}

	e, err := buildEngine(watchConfig, watchCSV, watchLLMKey, watchAlpacaKey, watchAlpacaSecret)
	if err != nil {
		return err
	}
	if !e.HasDataSource() {
		return errNoDataSource()
	}
	fmt.Printf("watching %s every %s (ctrl-c to stop)\n", sym, watchInterval)
	return e.Watch(cmd.Context(), bullarc.AnalysisRequest{Symbol: sym}, watchInterval, func(result bullarc.AnalysisResult) {
		PrintResult(os.Stdout, result)
	})
}
