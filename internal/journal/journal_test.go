package journal_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/journal"
)

// stubLLM is a minimal LLMProvider stub for testing.
type stubLLM struct {
	response string
	err      error
}

func (s *stubLLM) Name() string { return "stub" }
func (s *stubLLM) Complete(_ context.Context, _ bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	if s.err != nil {
		return bullarc.LLMResponse{}, s.err
	}
	return bullarc.LLMResponse{Text: s.response, Model: "stub"}, nil
}

func tmpJournalPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "journal.json")
}

func makeEntry(symbol string, pnl float64, direction bullarc.SignalType) journal.Entry {
	now := time.Now()
	return journal.Entry{
		Symbol: symbol,
		EntrySignal: bullarc.Signal{
			Type:        direction,
			Confidence:  75.0,
			Indicator:   "RSI_14",
			Symbol:      symbol,
			Timestamp:   now.Add(-time.Hour),
			Explanation: "RSI oversold",
		},
		ExitSignal: bullarc.Signal{
			Type:        bullarc.SignalSell,
			Confidence:  60.0,
			Indicator:   "RSI_14",
			Symbol:      symbol,
			Timestamp:   now,
			Explanation: "RSI overbought",
		},
		EntryPrice:    100.0,
		ExitPrice:     100.0 + pnl,
		Qty:           1.0,
		PnL:           pnl,
		PnLPct:        pnl,
		HoldingPeriod: time.Hour,
		EntryTime:     now.Add(-time.Hour),
		ExitTime:      now,
	}
}

func TestJournal_LogAndAll(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)

	e1 := makeEntry("AAPL", 5.0, bullarc.SignalBuy)
	e2 := makeEntry("TSLA", -3.0, bullarc.SignalBuy)

	require.NoError(t, j.Log(e1))
	require.NoError(t, j.Log(e2))

	all := j.All()
	assert.Len(t, all, 2)
	assert.Equal(t, "AAPL", all[0].Symbol)
	assert.Equal(t, "TSLA", all[1].Symbol)
}

func TestJournal_Len(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)

	assert.Equal(t, 0, j.Len())
	require.NoError(t, j.Log(makeEntry("AAPL", 5.0, bullarc.SignalBuy)))
	assert.Equal(t, 1, j.Len())
}

func TestJournal_Persistence(t *testing.T) {
	path := tmpJournalPath(t)

	j1, err := journal.New(path, nil)
	require.NoError(t, err)
	require.NoError(t, j1.Log(makeEntry("GOOG", 10.0, bullarc.SignalBuy)))

	// Reload from same path.
	j2, err := journal.New(path, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, j2.Len())
	all := j2.All()
	assert.Equal(t, "GOOG", all[0].Symbol)
}

func TestJournal_PersistenceFileFormat(t *testing.T) {
	path := tmpJournalPath(t)
	j, err := journal.New(path, nil)
	require.NoError(t, err)
	require.NoError(t, j.Log(makeEntry("AMZN", 7.5, bullarc.SignalBuy)))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var entries []map[string]any
	require.NoError(t, json.Unmarshal(data, &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "AMZN", entries[0]["symbol"])
}

func TestJournal_Query_BySymbol(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)
	require.NoError(t, j.Log(makeEntry("AAPL", 5.0, bullarc.SignalBuy)))
	require.NoError(t, j.Log(makeEntry("TSLA", -3.0, bullarc.SignalBuy)))

	results := j.Query(journal.QueryFilter{Symbol: "AAPL"})
	assert.Len(t, results, 1)
	assert.Equal(t, "AAPL", results[0].Symbol)
}

func TestJournal_Query_WinnersOnly(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)
	require.NoError(t, j.Log(makeEntry("AAPL", 5.0, bullarc.SignalBuy)))
	require.NoError(t, j.Log(makeEntry("TSLA", -3.0, bullarc.SignalBuy)))
	require.NoError(t, j.Log(makeEntry("GOOG", 2.0, bullarc.SignalBuy)))

	winners := j.Query(journal.QueryFilter{WinnersOnly: true})
	assert.Len(t, winners, 2)
	for _, w := range winners {
		assert.True(t, w.IsWinner())
	}
}

func TestJournal_Query_LosersOnly(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)
	require.NoError(t, j.Log(makeEntry("AAPL", 5.0, bullarc.SignalBuy)))
	require.NoError(t, j.Log(makeEntry("TSLA", -3.0, bullarc.SignalBuy)))

	losers := j.Query(journal.QueryFilter{LosersOnly: true})
	assert.Len(t, losers, 1)
	assert.Equal(t, "TSLA", losers[0].Symbol)
	assert.False(t, losers[0].IsWinner())
}

func TestJournal_Query_ByDateRange(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)

	now := time.Now()

	e1 := makeEntry("AAPL", 5.0, bullarc.SignalBuy)
	e1.EntryTime = now.Add(-48 * time.Hour)
	e1.ExitTime = now.Add(-47 * time.Hour)

	e2 := makeEntry("TSLA", 3.0, bullarc.SignalBuy)
	e2.EntryTime = now.Add(-2 * time.Hour)
	e2.ExitTime = now.Add(-time.Hour)

	require.NoError(t, j.Log(e1))
	require.NoError(t, j.Log(e2))

	// Query last 24 hours.
	results := j.Query(journal.QueryFilter{StartTime: now.Add(-24 * time.Hour)})
	assert.Len(t, results, 1)
	assert.Equal(t, "TSLA", results[0].Symbol)
}

func TestJournal_Query_ByDirection(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)
	require.NoError(t, j.Log(makeEntry("AAPL", 5.0, bullarc.SignalBuy)))
	require.NoError(t, j.Log(makeEntry("TSLA", -3.0, bullarc.SignalSell)))

	buys := j.Query(journal.QueryFilter{Direction: "BUY"})
	assert.Len(t, buys, 1)
	assert.Equal(t, "AAPL", buys[0].Symbol)

	sells := j.Query(journal.QueryFilter{Direction: "SELL"})
	assert.Len(t, sells, 1)
	assert.Equal(t, "TSLA", sells[0].Symbol)
}

func TestJournal_Query_EmptyFilter(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)
	require.NoError(t, j.Log(makeEntry("AAPL", 5.0, bullarc.SignalBuy)))
	require.NoError(t, j.Log(makeEntry("TSLA", -3.0, bullarc.SignalBuy)))

	all := j.Query(journal.QueryFilter{})
	assert.Len(t, all, 2)
}

func TestJournal_Review_NoProvider(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)

	_, err = j.Review(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_CONFIGURED")
}

func TestJournal_Review_InsufficientTrades(t *testing.T) {
	stub := &stubLLM{response: "great analysis"}
	j, err := journal.New(tmpJournalPath(t), stub)
	require.NoError(t, err)

	// Add only 5 trades (< 20 required).
	for i := 0; i < 5; i++ {
		require.NoError(t, j.Log(makeEntry("AAPL", float64(i), bullarc.SignalBuy)))
	}

	_, err = j.Review(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "20")
}

func TestJournal_Review_WithEnoughTrades(t *testing.T) {
	stub := &stubLLM{response: "Use RSI_14 more, it performed best. Avoid holding overnight."}
	j, err := journal.New(tmpJournalPath(t), stub)
	require.NoError(t, err)

	// Add 20 trades.
	for i := 0; i < 20; i++ {
		pnl := float64(i%3) - 1.0 // mix of wins/losses
		require.NoError(t, j.Log(makeEntry("AAPL", pnl, bullarc.SignalBuy)))
	}

	review, err := j.Review(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, review)
	assert.Contains(t, review, "RSI_14")
}

func TestJournal_Review_LLMError(t *testing.T) {
	stub := &stubLLM{err: bullarc.ErrLLMUnavailable.Wrap(nil)}
	j, err := journal.New(tmpJournalPath(t), stub)
	require.NoError(t, err)

	for i := 0; i < 20; i++ {
		require.NoError(t, j.Log(makeEntry("AAPL", float64(i), bullarc.SignalBuy)))
	}

	_, err = j.Review(context.Background())
	assert.Error(t, err)
}

func TestJournal_EmptyFile(t *testing.T) {
	path := tmpJournalPath(t)
	// Write an empty file.
	require.NoError(t, os.WriteFile(path, []byte{}, 0o600))

	j, err := journal.New(path, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, j.Len())
}

func TestJournal_NonExistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	j, err := journal.New(path, nil)
	require.NoError(t, err)
	assert.Equal(t, 0, j.Len())
}

func TestJournal_SetProvider(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)

	// Without provider — review fails.
	for i := 0; i < 20; i++ {
		require.NoError(t, j.Log(makeEntry("AAPL", float64(i), bullarc.SignalBuy)))
	}
	_, err = j.Review(context.Background())
	assert.Error(t, err)

	// Set provider — review succeeds.
	j.SetProvider(&stubLLM{response: "good analysis"})
	review, err := j.Review(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "good analysis", review)
}

func TestNewEntry(t *testing.T) {
	now := time.Now()
	buyResult := bullarc.OrderResult{
		Symbol:      "AAPL",
		Side:        bullarc.OrderSideBuy,
		Qty:         10.0,
		FilledPrice: 150.0,
		FilledAt:    now.Add(-2 * time.Hour),
		Status:      "filled",
	}
	sellResult := bullarc.OrderResult{
		Symbol:      "AAPL",
		Side:        bullarc.OrderSideSell,
		Qty:         10.0,
		FilledPrice: 160.0,
		FilledAt:    now,
		Status:      "filled",
	}
	entrySignal := bullarc.Signal{
		Type:       bullarc.SignalBuy,
		Confidence: 80.0,
		Indicator:  "RSI_14",
		Symbol:     "AAPL",
	}
	exitSignal := bullarc.Signal{
		Type:        bullarc.SignalSell,
		Confidence:  70.0,
		Indicator:   "RSI_14",
		Symbol:      "AAPL",
		Explanation: "RSI overbought",
	}

	entry := journal.NewEntry(buyResult, sellResult, entrySignal, exitSignal, nil, nil)

	assert.Equal(t, "AAPL", entry.Symbol)
	assert.Equal(t, 150.0, entry.EntryPrice)
	assert.Equal(t, 160.0, entry.ExitPrice)
	assert.Equal(t, 10.0, entry.Qty)
	assert.InDelta(t, 100.0, entry.PnL, 0.001)         // (160-150) * 10
	assert.InDelta(t, 6.666, entry.PnLPct, 0.001)       // (160-150)/150 * 100
	assert.Equal(t, 2*time.Hour, entry.HoldingPeriod)
	assert.True(t, entry.IsWinner())
}

func TestEntry_IsWinner(t *testing.T) {
	winner := journal.Entry{PnL: 10.0}
	loser := journal.Entry{PnL: -5.0}
	breakeven := journal.Entry{PnL: 0.0}

	assert.True(t, winner.IsWinner())
	assert.False(t, loser.IsWinner())
	assert.False(t, breakeven.IsWinner())
}

func TestJournal_IDAutoAssigned(t *testing.T) {
	j, err := journal.New(tmpJournalPath(t), nil)
	require.NoError(t, err)

	e := makeEntry("AAPL", 5.0, bullarc.SignalBuy)
	e.ID = "" // clear ID to test auto-assignment
	require.NoError(t, j.Log(e))

	all := j.All()
	require.Len(t, all, 1)
	assert.NotEmpty(t, all[0].ID)
}
