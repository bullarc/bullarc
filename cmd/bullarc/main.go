package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(backtestCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(paperCmd)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}
