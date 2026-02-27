package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bullarc/bullarc/internal/config"
	"github.com/bullarc/bullarc/internal/journal"
	"github.com/bullarc/bullarc/internal/llm"
)

const defaultJournalPath = "journal.json"

var journalCmd = &cobra.Command{
	Use:   "journal",
	Short: "Trade journal commands (view, query, and review closed paper trades)",
	Long: `Trade journal commands let you inspect closed paper trades and run
an LLM-powered learning-loop review to improve your strategy.`,
}

var journalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all journal entries",
	RunE:  runJournalList,
}

var journalQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query journal entries by symbol, date, P&L, or direction",
	RunE:  runJournalQuery,
}

var journalReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Request an LLM-powered review of journal performance (requires 20+ trades)",
	RunE:  runJournalReview,
}

var (
	journalPath      string
	journalSymbol    string
	journalSince     string
	journalUntil     string
	journalWinners   bool
	journalLosers    bool
	journalDirection string
	journalLLMKey    string
)

func init() {
	// Shared path flag.
	journalCmd.PersistentFlags().StringVar(&journalPath, "path", defaultJournalPath, "path to journal JSON file")

	// query flags.
	journalQueryCmd.Flags().StringVarP(&journalSymbol, "symbol", "s", "", "filter by symbol")
	journalQueryCmd.Flags().StringVar(&journalSince, "since", "", "filter entries on or after date (YYYY-MM-DD)")
	journalQueryCmd.Flags().StringVar(&journalUntil, "until", "", "filter entries on or before date (YYYY-MM-DD)")
	journalQueryCmd.Flags().BoolVar(&journalWinners, "winners", false, "show only profitable trades")
	journalQueryCmd.Flags().BoolVar(&journalLosers, "losers", false, "show only losing trades")
	journalQueryCmd.Flags().StringVarP(&journalDirection, "direction", "d", "", "filter by signal direction: BUY or SELL")

	// review flags.
	journalReviewCmd.Flags().StringVar(&journalLLMKey, "llm-key", "", "Anthropic API key (overrides ANTHROPIC_API_KEY env var)")

	journalCmd.AddCommand(journalListCmd)
	journalCmd.AddCommand(journalQueryCmd)
	journalCmd.AddCommand(journalReviewCmd)
}

func runJournalList(cmd *cobra.Command, _ []string) error {
	j, err := openJournal(journalPath, "")
	if err != nil {
		return err
	}
	_ = cmd.Context()
	printJournalEntries(j.All())
	return nil
}

func runJournalQuery(cmd *cobra.Command, _ []string) error {
	j, err := openJournal(journalPath, "")
	if err != nil {
		return err
	}
	_ = cmd.Context()

	filter, err := buildQueryFilter()
	if err != nil {
		return err
	}

	printJournalEntries(j.Query(filter))
	return nil
}

func runJournalReview(cmd *cobra.Command, _ []string) error {
	llmKey := resolveJournalLLMKey(journalLLMKey)

	j, err := openJournal(journalPath, llmKey)
	if err != nil {
		return err
	}

	review, err := j.Review(cmd.Context())
	if err != nil {
		return fmt.Errorf("journal review: %w", err)
	}

	fmt.Println(review)
	return nil
}

// openJournal creates a Journal backed by path. If llmKey is non-empty an
// Anthropic LLM provider is registered for journal review.
func openJournal(path string, llmKey string) (*journal.Journal, error) {
	var j *journal.Journal
	var err error

	if llmKey != "" {
		ap := llm.NewAnthropicProvider(llmKey, "")
		j, err = journal.New(path, ap)
	} else {
		j, err = journal.New(path, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("open journal %s: %w", path, err)
	}
	return j, nil
}

// resolveJournalLLMKey resolves the LLM API key using the standard resolution order:
// flag > ANTHROPIC_API_KEY env var > keystore.
func resolveJournalLLMKey(flagKey string) string {
	if flagKey != "" {
		return flagKey
	}
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		return k
	}
	ksPath, err := config.DefaultKeystorePath()
	if err != nil {
		return ""
	}
	creds, err := config.LoadCredentials(ksPath)
	if err != nil {
		return ""
	}
	return creds.LLMAPIKey
}

// openJournalWithContext is used by cmd_paper to open the journal with an
// optional LLM provider built from the engine's resolved LLM key.
func openJournalWithContext(_ context.Context, path, llmKey string) (*journal.Journal, error) {
	return openJournal(path, llmKey)
}

func buildQueryFilter() (journal.QueryFilter, error) {
	f := journal.QueryFilter{
		Symbol:      journalSymbol,
		WinnersOnly: journalWinners,
		LosersOnly:  journalLosers,
		Direction:   strings.ToUpper(journalDirection),
	}

	if journalSince != "" {
		t, err := time.Parse("2006-01-02", journalSince)
		if err != nil {
			return journal.QueryFilter{}, fmt.Errorf("invalid --since date %q: use YYYY-MM-DD", journalSince)
		}
		f.StartTime = t
	}

	if journalUntil != "" {
		t, err := time.Parse("2006-01-02", journalUntil)
		if err != nil {
			return journal.QueryFilter{}, fmt.Errorf("invalid --until date %q: use YYYY-MM-DD", journalUntil)
		}
		// Include the whole day.
		f.EndTime = t.Add(24*time.Hour - time.Nanosecond)
	}

	return f, nil
}

func printJournalEntries(entries []journal.Entry) {
	if len(entries) == 0 {
		fmt.Println("no journal entries found")
		return
	}

	fmt.Printf("%-12s %-6s %-8s %10s %10s %8s %8s %10s %s\n",
		"SYMBOL", "DIR", "IND", "ENTRY", "EXIT", "P&L", "P&L%", "HOLD", "STATUS")
	fmt.Printf("%-12s %-6s %-8s %10s %10s %8s %8s %10s %s\n",
		"------", "---", "---", "-----", "----", "---", "----", "----", "------")

	for _, e := range entries {
		status := "LOSS"
		if e.IsWinner() {
			status = "WIN"
		}
		holdStr := formatDuration(e.HoldingPeriod)
		indName := e.EntrySignal.Indicator
		if len(indName) > 8 {
			indName = indName[:8]
		}
		fmt.Printf("%-12s %-6s %-8s %10.2f %10.2f %8.2f %7.2f%% %10s %s\n",
			e.Symbol,
			string(e.EntrySignal.Type),
			indName,
			e.EntryPrice,
			e.ExitPrice,
			e.PnL,
			e.PnLPct,
			holdStr,
			status,
		)
	}

	wins, losses := 0, 0
	totalPnL := 0.0
	for _, e := range entries {
		totalPnL += e.PnL
		if e.IsWinner() {
			wins++
		} else {
			losses++
		}
	}

	fmt.Printf("\nTotal: %d entries | Winners: %d | Losers: %d | Total P&L: %.2f\n",
		len(entries), wins, losses, totalPnL)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}
