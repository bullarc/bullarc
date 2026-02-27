package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleTradestieResponse returns a minimal valid Tradestie Reddit API response.
func sampleTradestieResponse() []tradestieEntry {
	return []tradestieEntry{
		{Ticker: "GME", NoOfComments: 500, Sentiment: "Bullish", SentimentScore: 0.72},
		{Ticker: "AMC", NoOfComments: 200, Sentiment: "Bearish", SentimentScore: -0.30},
		{Ticker: "TSLA", NoOfComments: 150, Sentiment: "Bullish", SentimentScore: 0.45},
	}
}

// sampleApeWisdomResponse returns a minimal valid ApeWisdom API response.
func sampleApeWisdomResponse() apewisdomResponse {
	return apewisdomResponse{
		Results: []apewisdomResult{
			{Ticker: "GME", Mentions: 500, Rank: 1},
			{Ticker: "AMC", Mentions: 200, Rank: 2},
			{Ticker: "TSLA", Mentions: 150, Rank: 3},
		},
	}
}

func TestRedditTracker_ImplementsSocialTracker(t *testing.T) {
	var _ bullarc.SocialTracker = (*RedditTracker)(nil)
}

func TestRedditTracker_FetchSocialMetrics_Tradestie_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, tradestieRedditPath, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleTradestieResponse())
	}))
	defer srv.Close()

	tracker := NewRedditTracker(WithRedditBaseURL(srv.URL))
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME", "TSLA"})
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	bySymbol := make(map[string]bullarc.SocialMetrics)
	for _, m := range metrics {
		bySymbol[m.Symbol] = m
	}

	gme := bySymbol["GME"]
	assert.Equal(t, "GME", gme.Symbol)
	assert.Equal(t, 500, gme.Mentions)
	assert.InDelta(t, 0.72, gme.Sentiment, 0.001)
	assert.Equal(t, 1, gme.Rank)
	assert.False(t, gme.Timestamp.IsZero())

	tsla := bySymbol["TSLA"]
	assert.Equal(t, "TSLA", tsla.Symbol)
	assert.Equal(t, 150, tsla.Mentions)
	assert.Equal(t, 3, tsla.Rank)
}

func TestRedditTracker_FetchSocialMetrics_Tradestie_SymbolNotInResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleTradestieResponse())
	}))
	defer srv.Close()

	tracker := NewRedditTracker(WithRedditBaseURL(srv.URL))
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"UNKNOWN"})
	require.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestRedditTracker_FetchSocialMetrics_Tradestie_CaseInsensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleTradestieResponse())
	}))
	defer srv.Close()

	tracker := NewRedditTracker(WithRedditBaseURL(srv.URL))
	// lowercase symbol should still match
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"gme"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "GME", metrics[0].Symbol)
}

func TestRedditTracker_FetchSocialMetrics_Tradestie_APIUnavailable(t *testing.T) {
	// Point to a server that immediately closes connections.
	tracker := NewRedditTracker(WithRedditBaseURL("http://127.0.0.1:1"))
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME"})
	require.NoError(t, err, "unavailable API must not return an error")
	assert.Empty(t, metrics)
}

func TestRedditTracker_FetchSocialMetrics_Tradestie_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, `service unavailable`)
	}))
	defer srv.Close()

	tracker := NewRedditTracker(WithRedditBaseURL(srv.URL))
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME"})
	require.NoError(t, err, "non-200 response must not return an error")
	assert.Empty(t, metrics)
}

func TestRedditTracker_FetchSocialMetrics_Tradestie_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `not json`)
	}))
	defer srv.Close()

	tracker := NewRedditTracker(WithRedditBaseURL(srv.URL))
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME"})
	require.NoError(t, err, "invalid JSON must not return an error")
	assert.Empty(t, metrics)
}

func TestRedditTracker_FetchSocialMetrics_ApeWisdom_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, apewisdomPath, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleApeWisdomResponse())
	}))
	defer srv.Close()

	tracker := NewApeWisdomTracker(WithRedditBaseURL(srv.URL))
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME", "AMC"})
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	bySymbol := make(map[string]bullarc.SocialMetrics)
	for _, m := range metrics {
		bySymbol[m.Symbol] = m
	}

	gme := bySymbol["GME"]
	assert.Equal(t, 500, gme.Mentions)
	assert.Equal(t, 1, gme.Rank)
	assert.Equal(t, 0.0, gme.Sentiment) // ApeWisdom has no sentiment score

	amc := bySymbol["AMC"]
	assert.Equal(t, 200, amc.Mentions)
	assert.Equal(t, 2, amc.Rank)
}

func TestRedditTracker_FetchSocialMetrics_ApeWisdom_APIUnavailable(t *testing.T) {
	tracker := NewApeWisdomTracker(WithRedditBaseURL("http://127.0.0.1:1"))
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME"})
	require.NoError(t, err, "unavailable API must not return an error")
	assert.Empty(t, metrics)
}

func TestRedditTracker_Velocity_NoHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]tradestieEntry{
			{Ticker: "GME", NoOfComments: 300, SentimentScore: 0.5},
		})
	}))
	defer srv.Close()

	tracker := NewRedditTracker(WithRedditBaseURL(srv.URL))
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	// First fetch: no historical data, velocity should be 1.0 (neutral).
	assert.InDelta(t, 1.0, metrics[0].Velocity, 0.001)
	assert.False(t, metrics[0].IsElevated)
}

func TestRedditTracker_Velocity_ComputedFromHistory(t *testing.T) {
	tracker := NewRedditTracker()
	now := time.Now().UTC()

	// Manually inject 7 days of history with low mention counts.
	tracker.mu.Lock()
	for i := 1; i <= 7; i++ {
		day := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		tracker.history["GME"] = append(tracker.history["GME"], mentionSnapshot{
			Date:     day,
			Mentions: 100,
		})
	}
	tracker.mu.Unlock()

	// Current mentions are 3x the 7-day average: exactly at default threshold.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]tradestieEntry{
			{Ticker: "GME", NoOfComments: 300, SentimentScore: 0.5},
		})
	}))
	defer srv.Close()
	tracker.baseURL = srv.URL

	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	assert.InDelta(t, 3.0, metrics[0].Velocity, 0.001)
	assert.True(t, metrics[0].IsElevated, "velocity at 3x threshold should be elevated")
}

func TestRedditTracker_Velocity_BelowThreshold(t *testing.T) {
	tracker := NewRedditTracker()
	now := time.Now().UTC()

	tracker.mu.Lock()
	for i := 1; i <= 3; i++ {
		day := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		tracker.history["TSLA"] = append(tracker.history["TSLA"], mentionSnapshot{
			Date:     day,
			Mentions: 200,
		})
	}
	tracker.mu.Unlock()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]tradestieEntry{
			{Ticker: "TSLA", NoOfComments: 210, SentimentScore: 0.1},
		})
	}))
	defer srv.Close()
	tracker.baseURL = srv.URL

	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"TSLA"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	assert.InDelta(t, 1.05, metrics[0].Velocity, 0.01)
	assert.False(t, metrics[0].IsElevated)
}

func TestRedditTracker_Velocity_CustomThreshold(t *testing.T) {
	tracker := NewRedditTracker(WithSpikeThreshold(2.0))
	now := time.Now().UTC()

	tracker.mu.Lock()
	for i := 1; i <= 3; i++ {
		day := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		tracker.history["GME"] = append(tracker.history["GME"], mentionSnapshot{
			Date:     day,
			Mentions: 100,
		})
	}
	tracker.mu.Unlock()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]tradestieEntry{
			{Ticker: "GME", NoOfComments: 250, SentimentScore: 0.8},
		})
	}))
	defer srv.Close()
	tracker.baseURL = srv.URL

	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	assert.InDelta(t, 2.5, metrics[0].Velocity, 0.01)
	// threshold is 2x, velocity is 2.5x → elevated
	assert.True(t, metrics[0].IsElevated)
}

func TestRedditTracker_Velocity_OldHistoryPruned(t *testing.T) {
	tracker := NewRedditTracker()
	now := time.Now().UTC()

	// Inject a snapshot older than 7 days.
	tracker.mu.Lock()
	oldDay := now.AddDate(0, 0, -8).Truncate(24 * time.Hour)
	tracker.history["AMC"] = []mentionSnapshot{
		{Date: oldDay, Mentions: 9999},
	}
	tracker.mu.Unlock()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]tradestieEntry{
			{Ticker: "AMC", NoOfComments: 100, SentimentScore: 0.2},
		})
	}))
	defer srv.Close()
	tracker.baseURL = srv.URL

	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"AMC"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	// Old entry pruned; only today's data remains → neutral velocity.
	assert.InDelta(t, 1.0, metrics[0].Velocity, 0.001)
	assert.False(t, metrics[0].IsElevated)
}

func TestRedditTracker_Velocity_ZeroAverage(t *testing.T) {
	tracker := NewRedditTracker()
	now := time.Now().UTC()

	// Historical data with zero mentions.
	tracker.mu.Lock()
	for i := 1; i <= 3; i++ {
		day := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		tracker.history["XYZ"] = append(tracker.history["XYZ"], mentionSnapshot{
			Date:     day,
			Mentions: 0,
		})
	}
	tracker.mu.Unlock()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]tradestieEntry{
			{Ticker: "XYZ", NoOfComments: 50, SentimentScore: 0.1},
		})
	}))
	defer srv.Close()
	tracker.baseURL = srv.URL

	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"XYZ"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.True(t, metrics[0].IsElevated, "spike from zero should be elevated")
}

func TestRedditTracker_MultipleSymbols_IndependentHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]tradestieEntry{
			{Ticker: "GME", NoOfComments: 300, SentimentScore: 0.6},
			{Ticker: "TSLA", NoOfComments: 50, SentimentScore: 0.2},
		})
	}))
	defer srv.Close()

	tracker := NewRedditTracker(WithRedditBaseURL(srv.URL))
	now := time.Now().UTC()

	// Give GME a high baseline (so 300 is not elevated) and TSLA a low baseline.
	tracker.mu.Lock()
	for i := 1; i <= 3; i++ {
		day := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
		tracker.history["GME"] = append(tracker.history["GME"], mentionSnapshot{Date: day, Mentions: 1000})
		tracker.history["TSLA"] = append(tracker.history["TSLA"], mentionSnapshot{Date: day, Mentions: 10})
	}
	tracker.mu.Unlock()

	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{"GME", "TSLA"})
	require.NoError(t, err)
	require.Len(t, metrics, 2)

	bySymbol := make(map[string]bullarc.SocialMetrics)
	for _, m := range metrics {
		bySymbol[m.Symbol] = m
	}

	assert.False(t, bySymbol["GME"].IsElevated, "GME at 0.3x should not be elevated")
	assert.True(t, bySymbol["TSLA"].IsElevated, "TSLA at 5x should be elevated")
}

func TestRedditTracker_Provider(t *testing.T) {
	assert.Equal(t, "tradestie", NewRedditTracker().Provider())
	assert.Equal(t, "apewisdom", NewApeWisdomTracker().Provider())
}

func TestRedditTracker_SpikeThreshold(t *testing.T) {
	assert.Equal(t, DefaultSpikeThreshold, NewRedditTracker().SpikeThreshold())
	assert.Equal(t, 5.0, NewRedditTracker(WithSpikeThreshold(5.0)).SpikeThreshold())
}

func TestRedditTracker_EmptySymbols(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleTradestieResponse())
	}))
	defer srv.Close()

	tracker := NewRedditTracker(WithRedditBaseURL(srv.URL))
	metrics, err := tracker.FetchSocialMetrics(context.Background(), []string{})
	require.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestValidateSocialProvider(t *testing.T) {
	assert.NoError(t, validateSocialProvider("tradestie"))
	assert.NoError(t, validateSocialProvider("apewisdom"))

	err := validateSocialProvider("unknown")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unknown"))
}
