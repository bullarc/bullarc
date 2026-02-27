package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc/internal/config"
)

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Persist API keys for use across sessions",
	Long: `Configure stores API keys in ~/.config/bullarc/credentials with
restricted file permissions (0600) so they are available in every
subsequent session without re-entry.

Stored keys are lower priority than environment variables. Setting
ANTHROPIC_API_KEY or ALPACA_API_KEY always overrides the stored value.`,
	RunE: runConfigure,
}

var (
	configureLLMKey       string
	configureAlpacaKey    string
	configureAlpacaSecret string
	configureWatchlist    string
)

func init() {
	configureCmd.Flags().StringVar(&configureLLMKey, "llm-key", "", "Anthropic API key to store persistently")
	configureCmd.Flags().StringVar(&configureAlpacaKey, "alpaca-key", "", "Alpaca API key ID to store persistently")
	configureCmd.Flags().StringVar(&configureAlpacaSecret, "alpaca-secret", "", "Alpaca secret key to store persistently")
	configureCmd.Flags().StringVar(&configureWatchlist, "watchlist", "", "comma-separated default symbol watchlist (e.g. AAPL,MSFT,BTC/USD)")
}

func runConfigure(_ *cobra.Command, _ []string) error {
	if configureLLMKey == "" && configureAlpacaKey == "" && configureAlpacaSecret == "" && configureWatchlist == "" {
		return fmt.Errorf("provide at least one of --llm-key, --alpaca-key, --alpaca-secret, or --watchlist")
	}

	ksPath, err := config.DefaultKeystorePath()
	if err != nil {
		return fmt.Errorf("configure: resolve keystore path: %w", err)
	}

	creds, err := config.LoadCredentials(ksPath)
	if err != nil {
		return fmt.Errorf("configure: load existing credentials: %w", err)
	}

	if configureLLMKey != "" {
		creds.LLMAPIKey = configureLLMKey
	}
	if configureAlpacaKey != "" {
		creds.AlpacaKeyID = configureAlpacaKey
	}
	if configureAlpacaSecret != "" {
		creds.AlpacaSecretKey = configureAlpacaSecret
	}
	if configureWatchlist != "" {
		creds.Watchlist = parseWatchlist(configureWatchlist)
	}

	if err := config.SaveCredentials(ksPath, creds); err != nil {
		return fmt.Errorf("configure: save credentials: %w", err)
	}

	fmt.Printf("Credentials saved to %s\n", ksPath)
	return nil
}
