package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bullarc/bullarc"
)

const (
	tradestieDefaultBaseURL = "https://tradestie.com"
	tradestieRedditPath     = "/api/v1/apps/reddit"

	apewisdomDefaultBaseURL = "https://apewisdom.io"
	apewisdomPath           = "/api/v1.0/filter/all-stocks"

	// DefaultSocialPollInterval is the default interval between Reddit API polls.
	DefaultSocialPollInterval = 15 * time.Minute
	// DefaultSpikeThreshold is the default mention velocity multiplier that triggers elevated status.
	DefaultSpikeThreshold = 3.0
	// socialHistoryDays is the rolling window size in days used for velocity computation.
	socialHistoryDays = 7
)

// mentionSnapshot records a day's peak mention count for a symbol.
type mentionSnapshot struct {
	Date     time.Time
	Mentions int
}

// RedditTracker polls a Reddit mention aggregation API (Tradestie or ApeWisdom) and
// tracks mention velocity over a 7-day rolling window per symbol.
type RedditTracker struct {
	provider       string
	baseURL        string
	spikeThreshold float64
	client         *http.Client

	mu      sync.Mutex
	history map[string][]mentionSnapshot // symbol -> rolling 7-day daily snapshots
}

// RedditTrackerOption is a functional option for RedditTracker.
type RedditTrackerOption func(*RedditTracker)

// WithRedditHTTPClient sets a custom HTTP client on the RedditTracker.
func WithRedditHTTPClient(c *http.Client) RedditTrackerOption {
	return func(t *RedditTracker) { t.client = c }
}

// WithRedditBaseURL overrides the API base URL (useful for testing).
func WithRedditBaseURL(u string) RedditTrackerOption {
	return func(t *RedditTracker) { t.baseURL = u }
}

// WithSpikeThreshold sets the velocity multiplier at which a symbol is flagged as elevated.
func WithSpikeThreshold(threshold float64) RedditTrackerOption {
	return func(t *RedditTracker) { t.spikeThreshold = threshold }
}

// NewRedditTracker creates a RedditTracker using the Tradestie provider.
func NewRedditTracker(opts ...RedditTrackerOption) *RedditTracker {
	t := &RedditTracker{
		provider:       "tradestie",
		baseURL:        tradestieDefaultBaseURL,
		spikeThreshold: DefaultSpikeThreshold,
		client:         &http.Client{Timeout: 15 * time.Second},
		history:        make(map[string][]mentionSnapshot),
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// NewApeWisdomTracker creates a RedditTracker using the ApeWisdom provider.
func NewApeWisdomTracker(opts ...RedditTrackerOption) *RedditTracker {
	t := &RedditTracker{
		provider:       "apewisdom",
		baseURL:        apewisdomDefaultBaseURL,
		spikeThreshold: DefaultSpikeThreshold,
		client:         &http.Client{Timeout: 15 * time.Second},
		history:        make(map[string][]mentionSnapshot),
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// tradestieEntry is a single record from the Tradestie Reddit API response.
type tradestieEntry struct {
	Ticker         string  `json:"ticker"`
	NoOfComments   int     `json:"no_of_comments"`
	Sentiment      string  `json:"sentiment"`
	SentimentScore float64 `json:"sentiment_score"`
}

// apewisdomResult is a single result from the ApeWisdom API response.
type apewisdomResult struct {
	Ticker   string `json:"ticker"`
	Mentions int    `json:"mentions"`
	Rank     int    `json:"rank"`
}

// apewisdomResponse is the top-level ApeWisdom API response.
type apewisdomResponse struct {
	Results []apewisdomResult `json:"results"`
}

// FetchSocialMetrics retrieves Reddit mention data for the given symbols.
// When the API is unavailable, a warning is logged and an empty slice is returned (no error).
func (t *RedditTracker) FetchSocialMetrics(ctx context.Context, symbols []string) ([]bullarc.SocialMetrics, error) {
	slog.Info("fetching social metrics", "provider", t.provider, "symbols", symbols)
	switch t.provider {
	case "apewisdom":
		return t.fetchFromApeWisdom(ctx, symbols)
	default:
		return t.fetchFromTradestie(ctx, symbols)
	}
}

// fetchFromTradestie fetches social metrics from the Tradestie Reddit API.
func (t *RedditTracker) fetchFromTradestie(ctx context.Context, symbols []string) ([]bullarc.SocialMetrics, error) {
	endpoint := t.baseURL + tradestieRedditPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		slog.Warn("social tracker: failed to build tradestie request", "err", err)
		return nil, nil
	}

	resp, err := t.client.Do(req)
	if err != nil {
		slog.Warn("social tracker: tradestie api unavailable", "err", err)
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		slog.Warn("social tracker: tradestie api non-200",
			"status", resp.StatusCode,
			"body", strings.TrimSpace(string(body)))
		return nil, nil
	}

	var entries []tradestieEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		slog.Warn("social tracker: failed to decode tradestie response", "err", err)
		return nil, nil
	}

	byTicker := make(map[string]tradestieEntry, len(entries))
	rankByTicker := make(map[string]int, len(entries))
	for i, e := range entries {
		upper := strings.ToUpper(e.Ticker)
		byTicker[upper] = e
		rankByTicker[upper] = i + 1
	}

	now := time.Now().UTC()
	metrics := t.buildMetrics(symbols, now, func(sym string) (mentions int, sentiment float64, rank int, ok bool) {
		e, found := byTicker[sym]
		if !found {
			return 0, 0, 0, false
		}
		return e.NoOfComments, e.SentimentScore, rankByTicker[sym], true
	})

	slog.Info("fetched social metrics from tradestie", "symbols", symbols, "found", len(metrics))
	return metrics, nil
}

// fetchFromApeWisdom fetches social metrics from the ApeWisdom API.
func (t *RedditTracker) fetchFromApeWisdom(ctx context.Context, symbols []string) ([]bullarc.SocialMetrics, error) {
	endpoint := t.baseURL + apewisdomPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		slog.Warn("social tracker: failed to build apewisdom request", "err", err)
		return nil, nil
	}

	resp, err := t.client.Do(req)
	if err != nil {
		slog.Warn("social tracker: apewisdom api unavailable", "err", err)
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		slog.Warn("social tracker: apewisdom api non-200",
			"status", resp.StatusCode,
			"body", strings.TrimSpace(string(body)))
		return nil, nil
	}

	var apiResp apewisdomResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		slog.Warn("social tracker: failed to decode apewisdom response", "err", err)
		return nil, nil
	}

	byTicker := make(map[string]apewisdomResult, len(apiResp.Results))
	for _, r := range apiResp.Results {
		byTicker[strings.ToUpper(r.Ticker)] = r
	}

	now := time.Now().UTC()
	metrics := t.buildMetrics(symbols, now, func(sym string) (mentions int, sentiment float64, rank int, ok bool) {
		r, found := byTicker[sym]
		if !found {
			return 0, 0, 0, false
		}
		return r.Mentions, 0, r.Rank, true
	})

	slog.Info("fetched social metrics from apewisdom", "symbols", symbols, "found", len(metrics))
	return metrics, nil
}

// buildMetrics constructs SocialMetrics for each requested symbol using the provided lookup function.
func (t *RedditTracker) buildMetrics(
	symbols []string,
	now time.Time,
	lookup func(sym string) (mentions int, sentiment float64, rank int, ok bool),
) []bullarc.SocialMetrics {
	var metrics []bullarc.SocialMetrics
	for _, raw := range symbols {
		sym := strings.ToUpper(raw)
		mentions, sentiment, rank, ok := lookup(sym)
		if !ok {
			continue
		}
		velocity, isElevated := t.updateHistory(sym, mentions, now)
		metrics = append(metrics, bullarc.SocialMetrics{
			Symbol:     sym,
			Mentions:   mentions,
			Sentiment:  sentiment,
			Rank:       rank,
			Velocity:   velocity,
			IsElevated: isElevated,
			Timestamp:  now,
		})
	}
	return metrics
}

// updateHistory records today's mention count and returns the velocity ratio and elevated flag.
// velocity = current / 7-day-average (excluding today). isElevated = velocity >= spikeThreshold.
func (t *RedditTracker) updateHistory(symbol string, currentMentions int, now time.Time) (velocity float64, isElevated bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := now.AddDate(0, 0, -socialHistoryDays)
	todayDate := now.Truncate(24 * time.Hour)

	// Prune entries older than the rolling window.
	prev := t.history[symbol]
	filtered := prev[:0]
	for _, snap := range prev {
		if !snap.Date.Before(cutoff) {
			filtered = append(filtered, snap)
		}
	}

	// Upsert today's snapshot.
	updated := false
	for i, snap := range filtered {
		if snap.Date.Equal(todayDate) {
			filtered[i].Mentions = currentMentions
			updated = true
			break
		}
	}
	if !updated {
		filtered = append(filtered, mentionSnapshot{Date: todayDate, Mentions: currentMentions})
	}
	t.history[symbol] = filtered

	// Compute 7-day average from historical days (excluding today).
	var sumPast int
	pastCount := 0
	for _, snap := range filtered {
		if !snap.Date.Equal(todayDate) {
			sumPast += snap.Mentions
			pastCount++
		}
	}

	if pastCount == 0 {
		// Only today's data; velocity is neutral.
		return 1.0, false
	}

	avg := float64(sumPast) / float64(pastCount)
	if avg == 0 {
		if currentMentions > 0 {
			return t.spikeThreshold, true
		}
		return 0, false
	}

	v := float64(currentMentions) / avg
	return v, v >= t.spikeThreshold
}

// SpikeThreshold returns the configured spike threshold for this tracker.
func (t *RedditTracker) SpikeThreshold() float64 {
	return t.spikeThreshold
}

// Provider returns the configured provider name ("tradestie" or "apewisdom").
func (t *RedditTracker) Provider() string {
	return t.provider
}

// validateSocialProvider returns a descriptive error string if the provider name is invalid.
func validateSocialProvider(provider string) error {
	switch provider {
	case "tradestie", "apewisdom":
		return nil
	default:
		return fmt.Errorf("unknown social provider %q: must be \"tradestie\" or \"apewisdom\"", provider)
	}
}
