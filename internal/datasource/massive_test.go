package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMassiveSource_Meta(t *testing.T) {
	src := NewMassiveSource("key")
	meta := src.Meta()
	assert.Equal(t, "massive", meta.Name)
	assert.NotEmpty(t, meta.Description)
}

func TestMassiveSource_intervalToMassiveParams(t *testing.T) {
	cases := []struct {
		interval   string
		multiplier string
		timespan   string
	}{
		{"1m", "1", "minute"},
		{"5m", "5", "minute"},
		{"15m", "15", "minute"},
		{"1h", "1", "hour"},
		{"1Day", "1", "day"},
		{"", "1", "day"},
		{"unknown", "1", "day"},
	}
	for _, tc := range cases {
		m, ts := intervalToMassiveParams(tc.interval)
		assert.Equal(t, tc.multiplier, m, "interval=%q multiplier", tc.interval)
		assert.Equal(t, tc.timespan, ts, "interval=%q timespan", tc.interval)
	}
}

func TestMassiveSource_Fetch_Success(t *testing.T) {
	ts1 := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	ts2 := time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v2/aggs/ticker/AAPL/range/1/day/")
		assert.Equal(t, "test-key", r.URL.Query().Get("apiKey"))
		assert.Equal(t, "true", r.URL.Query().Get("adjusted"))
		assert.Equal(t, "asc", r.URL.Query().Get("sort"))
		assert.Equal(t, "50000", r.URL.Query().Get("limit"))

		w.Header().Set("Content-Type", "application/json")
		resp := massiveAggsResponse{
			Status:       "OK",
			ResultsCount: 2,
			Results: []massiveBar{
				{Timestamp: ts1.UnixMilli(), Open: 149.95, High: 150.48, Low: 149.73, Close: 149.83, Volume: 2753969},
				{Timestamp: ts2.UnixMilli(), Open: 149.73, High: 150.81, Low: 149.56, Close: 150.73, Volume: 6745338},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	src := NewMassiveSource("test-key", WithMassiveBaseURL(srv.URL))
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{
		Symbol:   "AAPL",
		Start:    ts1,
		End:      ts2,
		Interval: "1Day",
	})
	require.NoError(t, err)
	require.Len(t, bars, 2)

	assert.Equal(t, ts1, bars[0].Time)
	assert.Equal(t, 149.95, bars[0].Open)
	assert.Equal(t, 150.48, bars[0].High)
	assert.Equal(t, 149.73, bars[0].Low)
	assert.Equal(t, 149.83, bars[0].Close)
	assert.Equal(t, float64(2753969), bars[0].Volume)
}

func TestMassiveSource_Fetch_Pagination(t *testing.T) {
	var page atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		n := page.Add(1)
		if n == 1 {
			resp := massiveAggsResponse{
				Status:       "OK",
				ResultsCount: 1,
				Results: []massiveBar{
					{Timestamp: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100},
				},
				NextURL: fmt.Sprintf("http://%s/v2/aggs/next?cursor=abc123", r.Host),
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			resp := massiveAggsResponse{
				Status:       "OK",
				ResultsCount: 1,
				Results: []massiveBar{
					{Timestamp: time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC).UnixMilli(), Open: 2, High: 3, Low: 1, Close: 2.5, Volume: 200},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	src := NewMassiveSource("key", WithMassiveBaseURL(srv.URL))
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Len(t, bars, 2)
	assert.Equal(t, int32(2), page.Load(), "expected exactly 2 page requests")
}

func TestMassiveSource_Fetch_RetryOn429(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := massiveAggsResponse{
			Status:       "OK",
			ResultsCount: 1,
			Results: []massiveBar{
				{Timestamp: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 3, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}
	src := NewMassiveSource("key", WithMassiveBaseURL(srv.URL), WithMassiveRetry(cfg))

	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Len(t, bars, 1)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestMassiveSource_Fetch_RetryExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 2, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}
	src := NewMassiveSource("key", WithMassiveBaseURL(srv.URL), WithMassiveRetry(cfg))

	_, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestMassiveSource_Fetch_SymbolNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	src := NewMassiveSource("key", WithMassiveBaseURL(srv.URL))
	_, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "XXXX"})
	require.Error(t, err)
	requireCode(t, err, "SYMBOL_NOT_FOUND")
}

func TestMassiveSource_Fetch_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	src := NewMassiveSource("key")
	_, err := src.Fetch(ctx, bullarc.DataQuery{Symbol: "AAPL"})
	require.Error(t, err)
}

func TestMassiveSource_Fetch_ErrorResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"status":"ERROR","error":"Invalid API key"}`)
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 1, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}
	src := NewMassiveSource("bad-key", WithMassiveBaseURL(srv.URL), WithMassiveRetry(cfg))
	_, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
	assert.Contains(t, err.Error(), "Invalid API key")
}

func TestMassiveSource_Fetch_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"OK","resultsCount":0,"results":[]}`)
	}))
	defer srv.Close()

	src := NewMassiveSource("key", WithMassiveBaseURL(srv.URL))
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Empty(t, bars)
}

func TestMassiveSource_Fetch_NullResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"OK","resultsCount":0}`)
	}))
	defer srv.Close()

	src := NewMassiveSource("key", WithMassiveBaseURL(srv.URL))
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Empty(t, bars)
}

func TestMassiveSource_Fetch_QueryParams(t *testing.T) {
	start := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 7, 31, 0, 0, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/range/1/hour/2024-07-01/2024-07-31")

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"OK","resultsCount":0,"results":[]}`)
	}))
	defer srv.Close()

	src := NewMassiveSource("key", WithMassiveBaseURL(srv.URL))
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{
		Symbol:   "AAPL",
		Start:    start,
		End:      end,
		Interval: "1h",
	})
	require.NoError(t, err)
	assert.Empty(t, bars)
}

func TestMassiveSource_appendAPIKey(t *testing.T) {
	src := NewMassiveSource("mykey")

	got := src.appendAPIKey("https://api.polygon.io/v2/aggs/next?cursor=abc")
	assert.Equal(t, "https://api.polygon.io/v2/aggs/next?cursor=abc&apiKey=mykey", got)

	got = src.appendAPIKey("https://api.polygon.io/v2/aggs/next?cursor=abc&apiKey=mykey")
	assert.Equal(t, "https://api.polygon.io/v2/aggs/next?cursor=abc&apiKey=mykey", got)

	got = src.appendAPIKey("https://api.polygon.io/v2/aggs/next")
	assert.Equal(t, "https://api.polygon.io/v2/aggs/next?apiKey=mykey", got)
}
