package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bullarc",
	Short: "Financial analysis engine powered by technical indicators and LLMs",
	Long: `bullarc is a modular financial analysis engine that combines
technical indicators, market data sources, and large language models
to produce actionable trading signals and market insights.`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		format := flagOrEnv(cmd, "log-format", "BULLARC_LOG_FORMAT", "text")
		levelStr := flagOrEnv(cmd, "log-level", "BULLARC_LOG_LEVEL", "info")

		level, err := parseLogLevel(levelStr)
		if err != nil {
			return err
		}

		opts := &slog.HandlerOptions{Level: level}

		var handler slog.Handler
		switch strings.ToLower(format) {
		case "text":
			handler = slog.NewTextHandler(os.Stderr, opts)
		case "json":
			handler = slog.NewJSONHandler(os.Stderr, opts)
		default:
			return fmt.Errorf("invalid log format %q: must be \"text\" or \"json\"", format)
		}

		slog.SetDefault(slog.New(handler))
		return nil
	},
}

// parseLogLevel maps a string name to the corresponding slog.Level.
func parseLogLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q: must be one of \"debug\", \"info\", \"warn\", \"error\"", s)
	}
}

// flagOrEnv returns the flag value if it was explicitly set, otherwise falls
// back to the environment variable, and finally to the provided default.
func flagOrEnv(cmd *cobra.Command, flagName, envKey, defaultVal string) string {
	if f := cmd.Flag(flagName); f != nil && f.Changed {
		return f.Value.String()
	}
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return defaultVal
}

func init() {
	rootCmd.PersistentFlags().String("log-format", "text", `log output format ("text" or "json")`)
	rootCmd.PersistentFlags().String("log-level", "info", `log level ("debug", "info", "warn", "error")`)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(backtestCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configureCmd)
	rootCmd.AddCommand(paperCmd)
	rootCmd.AddCommand(journalCmd)
	rootCmd.AddCommand(demoCmd)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}
