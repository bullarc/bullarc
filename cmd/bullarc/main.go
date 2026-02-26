package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bullarc",
	Short: "Financial analysis engine powered by technical indicators and LLMs",
	Long: `bullarc is a modular financial analysis engine that combines
technical indicators, market data sources, and large language models
to produce actionable trading signals and market insights.`,
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}
