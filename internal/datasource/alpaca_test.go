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

	"github.com/bullarcdev/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlpacaSource_Meta(t *testing.T) {
	src := NewAlpacaSource("key", "secret")
	meta := src.Meta()
	assert.Equal(t, "alpaca", meta.Name)
	assert.NotEmpty(t, meta.Description)
}

func TestAlpacaSource_intervalToAlpacaTimeframe(t *testing.T) {
	cases := []struct {
		interval  string
		timeframe string
	}{
		{"1m", "1Min"},
		{"5m", "5Min"},
		{"15m", "15Min"},
		{"1h", "1Hour"},
		{"1d", "1Day"},
		{"", "1Day"},
		{"unknown", "1Day"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.timeframe, intervalToAlpacaTimeframe(tc.interval), "interval=%q", tc.interval)
	}
}

func TestAlpacaSource_Fetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("APCA-API-KEY-ID"))
		assert.Equal(t, "test-secret", r.Header.Get("APCA-API-SECRET-KEY"))
		assert.Equal(t, "/v2/stocks/AAPL/bars", r.URL.Path)
		assert.Equal(t, "1Day", r.URL.Query().Get("timeframe"))

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"bars": [
				{"t":"2024-07-01T00:00:00Z","o":149.95,"h":150.48,"l":149.73,"c":149.83,"v":2753969},
				{"t":"2024-07-02T00:00:00Z","o":149.73,"h":150.81,"l":149.56,"c":150.73,"v":6745338}
			],
			"symbol": "AAPL"
		}`)
	}))
	defer srv.Close()

	src := NewAlpacaSource("test-key", "test-secret", WithBaseURL(srv.URL))
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.NoError(t, err)
	require.Len(t, bars, 2)

	assert.Equal(t, time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), bars[0].Time)
	assert.Equal(t, 149.95, bars[0].Open)
	assert.Equal(t, 150.48, bars[0].High)
	assert.Equal(t, 149.73, bars[0].Low)
	assert.Equal(t, 149.83, bars[0].Close)
	assert.Equal(t, float64(2753969), bars[0].Volume)
}

func TestAlpacaSource_Fetch_Pagination(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page++
		if page == 1 {
			token := "page2token"
			resp := alpacaBarsResponse{
				Bars: []alpacaBar{
					{Time: time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC), Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100},
				},
				Symbol:        "AAPL",
				NextPageToken: &token,
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			assert.Equal(t, "page2token", r.URL.Query().Get("page_token"))
			resp := alpacaBarsResponse{
				Bars: []alpacaBar{
					{Time: time.Date(2024, 7, 2, 0, 0, 0, 0, time.UTC), Open: 2, High: 3, Low: 1, Close: 2.5, Volume: 200},
				},
				Symbol: "AAPL",
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	src := NewAlpacaSource("key", "secret", WithBaseURL(srv.URL))
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Len(t, bars, 2)
	assert.Equal(t, 2, page, "expected exactly 2 page requests")
}

func TestAlpacaSource_Fetch_RetryOn429(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"bars":[{"t":"2024-07-01T00:00:00Z","o":1,"h":2,"l":0.5,"c":1.5,"v":100}],"symbol":"AAPL"}`)
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 3, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}
	src := NewAlpacaSource("key", "secret", WithBaseURL(srv.URL), WithRetry(cfg))

	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Len(t, bars, 1)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestAlpacaSource_Fetch_RetryExhausted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 2, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}
	src := NewAlpacaSource("key", "secret", WithBaseURL(srv.URL), WithRetry(cfg))

	_, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "AAPL"})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestAlpacaSource_Fetch_SymbolNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	src := NewAlpacaSource("key", "secret", WithBaseURL(srv.URL))
	_, err := src.Fetch(context.Background(), bullarc.DataQuery{Symbol: "XXXX"})
	require.Error(t, err)
	requireCode(t, err, "SYMBOL_NOT_FOUND")
}

func TestAlpacaSource_Fetch_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	src := NewAlpacaSource("key", "secret")
	_, err := src.Fetch(ctx, bullarc.DataQuery{Symbol: "AAPL"})
	require.Error(t, err)
}

func TestAlpacaSource_Fetch_QueryParams(t *testing.T) {
	start := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 7, 31, 0, 0, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		assert.Equal(t, "1Hour", q.Get("timeframe"))
		assert.Equal(t, start.Format(time.RFC3339), q.Get("start"))
		assert.Equal(t, end.Format(time.RFC3339), q.Get("end"))

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"bars":[],"symbol":"AAPL"}`)
	}))
	defer srv.Close()

	src := NewAlpacaSource("key", "secret", WithBaseURL(srv.URL))
	bars, err := src.Fetch(context.Background(), bullarc.DataQuery{
		Symbol:   "AAPL",
		Start:    start,
		End:      end,
		Interval: "1h",
	})
	require.NoError(t, err)
	assert.Empty(t, bars)
}
