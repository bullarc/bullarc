// Package journal implements trade journal storage and learning-loop review.
package journal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/bullarc/bullarc"
)

// Entry represents a single closed paper trade logged in the journal.
type Entry struct {
	ID              string                              `json:"id"`
	Symbol          string                              `json:"symbol"`
	EntrySignal     bullarc.Signal                      `json:"entry_signal"`
	ExitSignal      bullarc.Signal                      `json:"exit_signal"`
	EntryPrice      float64                             `json:"entry_price"`
	ExitPrice       float64                             `json:"exit_price"`
	Qty             float64                             `json:"qty"`
	PnL             float64                             `json:"pnl"`
	PnLPct          float64                             `json:"pnl_pct"`
	HoldingPeriod   time.Duration                       `json:"holding_period_ns"`
	EntryIndicators map[string][]bullarc.IndicatorValue `json:"entry_indicators,omitempty"`
	ExitIndicators  map[string][]bullarc.IndicatorValue `json:"exit_indicators,omitempty"`
	EntryTime       time.Time                           `json:"entry_time"`
	ExitTime        time.Time                           `json:"exit_time"`
}

// IsWinner reports whether the trade produced a positive P&L.
func (e Entry) IsWinner() bool { return e.PnL > 0 }

// QueryFilter specifies criteria for filtering journal entries.
type QueryFilter struct {
	Symbol    string
	StartTime time.Time
	EndTime   time.Time
	// WinnersOnly when true returns only profitable trades.
	WinnersOnly bool
	// LosersOnly when true returns only loss-making trades.
	LosersOnly bool
	// Direction filters by signal direction: "BUY", "SELL", or "" for all.
	Direction string
}

// Journal stores closed trade entries and supports querying and LLM-powered review.
type Journal struct {
	mu       sync.RWMutex
	path     string
	entries  []Entry
	provider bullarc.LLMProvider
}

// New creates a Journal backed by the given file path.
// Existing entries are loaded from disk on creation.
// provider may be nil; when nil, Review returns ErrNotConfigured.
func New(path string, provider bullarc.LLMProvider) (*Journal, error) {
	j := &Journal{path: path, provider: provider}
	if err := j.load(); err != nil {
		return nil, err
	}
	return j, nil
}

// Log appends a closed trade entry to the journal and persists it to disk.
func (j *Journal) Log(entry Entry) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if entry.ID == "" {
		entry.ID = fmt.Sprintf("%s-%d", entry.Symbol, time.Now().UnixNano())
	}
	j.entries = append(j.entries, entry)
	return j.save()
}

// Query returns journal entries matching the given filter.
// An empty filter returns all entries.
func (j *Journal) Query(f QueryFilter) []Entry {
	j.mu.RLock()
	defer j.mu.RUnlock()

	var out []Entry
	for _, e := range j.entries {
		if !matchesFilter(e, f) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// All returns all journal entries in insertion order.
func (j *Journal) All() []Entry {
	j.mu.RLock()
	defer j.mu.RUnlock()
	cp := make([]Entry, len(j.entries))
	copy(cp, j.entries)
	return cp
}

// Len returns the total number of logged entries.
func (j *Journal) Len() int {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return len(j.entries)
}

// minReviewEntries is the minimum number of closed trades required before
// the learning-loop review can be requested.
const minReviewEntries = 20

// Review sends the journal to the configured LLM provider and returns a
// plain-English analysis of trade performance, indicator effectiveness, and
// suggested adjustments.
//
// Returns ErrNotConfigured when no LLM provider is set.
// Returns an error describing insufficient data when fewer than 20 trades are logged.
func (j *Journal) Review(ctx context.Context) (string, error) {
	j.mu.RLock()
	provider := j.provider
	entries := j.entries
	j.mu.RUnlock()

	if provider == nil {
		return "", bullarc.ErrNotConfigured.Wrap(fmt.Errorf("no LLM provider configured for journal review"))
	}
	if len(entries) < minReviewEntries {
		return "", fmt.Errorf("journal review requires at least %d closed trades, have %d", minReviewEntries, len(entries))
	}

	prompt := buildReviewPrompt(entries)
	resp, err := provider.Complete(ctx, bullarc.LLMRequest{
		Prompt:    prompt,
		MaxTokens: 1024,
	})
	if err != nil {
		return "", fmt.Errorf("journal review LLM call failed: %w", err)
	}
	return resp.Text, nil
}

// SetProvider replaces the LLM provider used for journal review.
func (j *Journal) SetProvider(p bullarc.LLMProvider) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.provider = p
}

// load reads entries from j.path. If the file does not exist, no error is
// returned and the journal starts empty.
func (j *Journal) load() error {
	data, err := os.ReadFile(j.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("journal: read %s: %w", j.path, err)
	}
	if len(data) == 0 {
		return nil
	}
	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("journal: parse %s: %w", j.path, err)
	}
	j.entries = entries
	return nil
}

// save writes all entries to j.path atomically via a temp file.
func (j *Journal) save() error {
	data, err := json.MarshalIndent(j.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("journal: marshal entries: %w", err)
	}
	tmpPath := j.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("journal: write temp file: %w", err)
	}
	if err := os.Rename(tmpPath, j.path); err != nil {
		return fmt.Errorf("journal: rename temp to journal: %w", err)
	}
	return nil
}

// matchesFilter reports whether entry satisfies all criteria in f.
func matchesFilter(e Entry, f QueryFilter) bool {
	if f.Symbol != "" && e.Symbol != f.Symbol {
		return false
	}
	if !f.StartTime.IsZero() && e.EntryTime.Before(f.StartTime) {
		return false
	}
	if !f.EndTime.IsZero() && e.EntryTime.After(f.EndTime) {
		return false
	}
	if f.WinnersOnly && !e.IsWinner() {
		return false
	}
	if f.LosersOnly && e.IsWinner() {
		return false
	}
	if f.Direction != "" && string(e.EntrySignal.Type) != f.Direction {
		return false
	}
	return true
}

// buildReviewPrompt constructs the LLM prompt for the journal review.
func buildReviewPrompt(entries []Entry) string {
	wins, losses := 0, 0
	totalPnL := 0.0
	indicatorWinCounts := make(map[string]int)
	indicatorTotalCounts := make(map[string]int)

	for _, e := range entries {
		totalPnL += e.PnL
		indicatorTotalCounts[e.EntrySignal.Indicator]++
		if e.IsWinner() {
			wins++
			indicatorWinCounts[e.EntrySignal.Indicator]++
		} else {
			losses++
		}
	}

	winRate := 0.0
	if len(entries) > 0 {
		winRate = float64(wins) / float64(len(entries)) * 100
	}

	// Build a JSON summary of the trades for the LLM.
	type tradeSummary struct {
		Symbol       string   `json:"symbol"`
		Direction    string   `json:"direction"`
		Confidence   float64  `json:"confidence"`
		Indicator    string   `json:"indicator"`
		EntryPrice   float64  `json:"entry_price"`
		ExitPrice    float64  `json:"exit_price"`
		PnLPct       float64  `json:"pnl_pct"`
		HoldingHours float64  `json:"holding_hours"`
		Winner       bool     `json:"winner"`
		ExitReason   string   `json:"exit_reason"`
		RiskFlags    []string `json:"risk_flags,omitempty"`
	}

	summaries := make([]tradeSummary, 0, len(entries))
	for _, e := range entries {
		summaries = append(summaries, tradeSummary{
			Symbol:       e.Symbol,
			Direction:    string(e.EntrySignal.Type),
			Confidence:   e.EntrySignal.Confidence,
			Indicator:    e.EntrySignal.Indicator,
			EntryPrice:   e.EntryPrice,
			ExitPrice:    e.ExitPrice,
			PnLPct:       e.PnLPct,
			HoldingHours: e.HoldingPeriod.Hours(),
			Winner:       e.IsWinner(),
			ExitReason:   e.ExitSignal.Explanation,
			RiskFlags:    e.EntrySignal.RiskFlags,
		})
	}

	tradesJSON, _ := json.MarshalIndent(summaries, "", "  ")

	indicatorStats := ""
	for ind, total := range indicatorTotalCounts {
		w := indicatorWinCounts[ind]
		rate := float64(w) / float64(total) * 100
		indicatorStats += fmt.Sprintf("  - %s: %d trades, %.0f%% win rate\n", ind, total, rate)
	}

	return fmt.Sprintf(`You are a trading coach reviewing a paper trading journal for a non-technical user.

Overall statistics:
- Total closed trades: %d
- Winners: %d (%.1f%% win rate)
- Losers: %d
- Total P&L: %.2f

Indicator performance:
%s
Trade details:
%s

Please provide a clear, plain-English analysis covering:
1. Which indicator configurations produced the best results and why
2. What patterns or conditions led to losses
3. Specific, actionable suggestions to improve indicator weights or thresholds
4. Any risk management observations (e.g., holding period patterns, stop-loss effectiveness)

Write for a non-technical user. Avoid jargon. Be concise and direct.`,
		len(entries), wins, winRate, losses, totalPnL,
		indicatorStats, string(tradesJSON))
}

// NewEntry constructs an Entry from buy and sell order results plus analysis
// context captured at entry and exit time. It is a convenience constructor
// used by the paper trading executor.
func NewEntry(
	buyResult bullarc.OrderResult,
	sellResult bullarc.OrderResult,
	entrySignal bullarc.Signal,
	exitSignal bullarc.Signal,
	entryIndicators map[string][]bullarc.IndicatorValue,
	exitIndicators map[string][]bullarc.IndicatorValue,
) Entry {
	entryTime := buyResult.FilledAt
	if entryTime.IsZero() {
		entryTime = entrySignal.Timestamp
	}
	exitTime := sellResult.FilledAt
	if exitTime.IsZero() {
		exitTime = exitSignal.Timestamp
	}
	if exitTime.IsZero() {
		exitTime = time.Now()
	}
	if entryTime.IsZero() {
		entryTime = exitTime
	}

	holdingPeriod := exitTime.Sub(entryTime)

	pnl := 0.0
	pnlPct := 0.0
	if sellResult.FilledPrice > 0 && buyResult.FilledPrice > 0 && buyResult.Qty > 0 {
		pnl = (sellResult.FilledPrice - buyResult.FilledPrice) * buyResult.Qty
		pnlPct = (sellResult.FilledPrice - buyResult.FilledPrice) / buyResult.FilledPrice * 100
	}

	return Entry{
		Symbol:          buyResult.Symbol,
		EntrySignal:     entrySignal,
		ExitSignal:      exitSignal,
		EntryPrice:      buyResult.FilledPrice,
		ExitPrice:       sellResult.FilledPrice,
		Qty:             buyResult.Qty,
		PnL:             pnl,
		PnLPct:          pnlPct,
		HoldingPeriod:   holdingPeriod,
		EntryIndicators: entryIndicators,
		ExitIndicators:  exitIndicators,
		EntryTime:       entryTime,
		ExitTime:        exitTime,
	}
}
