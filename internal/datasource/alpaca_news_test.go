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

// sampleNewsResponse returns a minimal valid Alpaca News API response.
func sampleNewsResponse(nextPageToken *string) alpacaNewsResponse {
	return alpacaNewsResponse{
		News: []alpacaNewsArticle{
			{
				ID:        1001,
				Headline:  "Apple releases new product",
				Summary:   "Apple Inc. announced a new product line today.",
				Source:    "benzinga",
				Symbols:   []string{"AAPL"},
				CreatedAt: "2024-07-01T12:00:00Z",
			},
			{
				ID:        1002,
				Headline:  "Tesla quarterly results beat estimates",
				Summary:   "Tesla reported strong quarterly earnings.",
				Source:    "reuters",
				Symbols:   []string{"TSLA"},
				CreatedAt: "2024-07-01T10:00:00Z",
			},
		},
		NextPageToken: nextPageToken,
	}
}

func TestAlpacaNewsSource_FetchNews_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-key", r.Header.Get("APCA-API-KEY-ID"))
		assert.Equal(t, "test-secret", r.Header.Get("APCA-API-SECRET-KEY"))
		assert.Equal(t, "/v1beta1/news", r.URL.Path)
		assert.Equal(t, "AAPL,TSLA", r.URL.Query().Get("symbols"))
		assert.Equal(t, "desc", r.URL.Query().Get("sort"))

		w.Header().Set("Content-Type", "application/json")
		resp := sampleNewsResponse(nil)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	src := NewAlpacaNewsSource("test-key", "test-secret", WithNewsBaseURL(srv.URL))
	since := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	articles, err := src.FetchNews(context.Background(), []string{"AAPL", "TSLA"}, since)
	require.NoError(t, err)
	require.Len(t, articles, 2)

	assert.Equal(t, "1001", articles[0].ID)
	assert.Equal(t, "Apple releases new product", articles[0].Headline)
	assert.Equal(t, "Apple Inc. announced a new product line today.", articles[0].Summary)
	assert.Equal(t, "benzinga", articles[0].Source)
	assert.Equal(t, []string{"AAPL"}, articles[0].Symbols)
	assert.Equal(t, time.Date(2024, 7, 1, 12, 0, 0, 0, time.UTC), articles[0].PublishedAt)

	assert.Equal(t, "1002", articles[1].ID)
	assert.Equal(t, []string{"TSLA"}, articles[1].Symbols)
}

func TestAlpacaNewsSource_FetchNews_EmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"news":[],"next_page_token":null}`)
	}))
	defer srv.Close()

	src := NewAlpacaNewsSource("key", "secret", WithNewsBaseURL(srv.URL))
	articles, err := src.FetchNews(context.Background(), []string{"UNKNOWN"}, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, articles)
}

func TestAlpacaNewsSource_FetchNews_NoSymbols(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.Query().Get("symbols"), "symbols param should be absent when not provided")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"news":[],"next_page_token":null}`)
	}))
	defer srv.Close()

	src := NewAlpacaNewsSource("key", "secret", WithNewsBaseURL(srv.URL))
	articles, err := src.FetchNews(context.Background(), nil, time.Now().Add(-24*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, articles)
}

func TestAlpacaNewsSource_FetchNews_Pagination(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page++
		if page == 1 {
			token := "page2token"
			resp := alpacaNewsResponse{
				News: []alpacaNewsArticle{
					{ID: 101, Headline: "Article 1", Source: "src", Symbols: []string{"AAPL"}, CreatedAt: "2024-07-01T12:00:00Z"},
				},
				NextPageToken: &token,
			}
			json.NewEncoder(w).Encode(resp)
		} else {
			assert.Equal(t, "page2token", r.URL.Query().Get("page_token"))
			resp := alpacaNewsResponse{
				News: []alpacaNewsArticle{
					{ID: 102, Headline: "Article 2", Source: "src", Symbols: []string{"AAPL"}, CreatedAt: "2024-07-01T10:00:00Z"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer srv.Close()

	src := NewAlpacaNewsSource("key", "secret", WithNewsBaseURL(srv.URL))
	articles, err := src.FetchNews(context.Background(), []string{"AAPL"}, time.Time{})
	require.NoError(t, err)
	assert.Len(t, articles, 2)
	assert.Equal(t, 2, page, "expected exactly 2 page requests")
	assert.Equal(t, "101", articles[0].ID)
	assert.Equal(t, "102", articles[1].ID)
}

func TestAlpacaNewsSource_FetchNews_SinceParam(t *testing.T) {
	since := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, since.Format(time.RFC3339), r.URL.Query().Get("start"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"news":[],"next_page_token":null}`)
	}))
	defer srv.Close()

	src := NewAlpacaNewsSource("key", "secret", WithNewsBaseURL(srv.URL))
	_, err := src.FetchNews(context.Background(), []string{"AAPL"}, since)
	require.NoError(t, err)
}

func TestAlpacaNewsSource_FetchNews_ZeroSinceOmitsStartParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.Query().Get("start"), "start param should be absent for zero time")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"news":[],"next_page_token":null}`)
	}))
	defer srv.Close()

	src := NewAlpacaNewsSource("key", "secret", WithNewsBaseURL(srv.URL))
	_, err := src.FetchNews(context.Background(), []string{"AAPL"}, time.Time{})
	require.NoError(t, err)
}

func TestAlpacaNewsSource_FetchNews_RetryOn429(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"news":[],"next_page_token":null}`)
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 3, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}
	src := NewAlpacaNewsSource("key", "secret", WithNewsBaseURL(srv.URL), WithNewsRetry(cfg))

	articles, err := src.FetchNews(context.Background(), []string{"AAPL"}, time.Time{})
	require.NoError(t, err)
	assert.Empty(t, articles)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestAlpacaNewsSource_FetchNews_RateLimitExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"message":"rate limit exceeded"}`)
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 2, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}
	src := NewAlpacaNewsSource("key", "secret", WithNewsBaseURL(srv.URL), WithNewsRetry(cfg))

	_, err := src.FetchNews(context.Background(), []string{"AAPL"}, time.Time{})
	require.Error(t, err)
	requireCode(t, err, "RATE_LIMIT_EXCEEDED")
}

func TestAlpacaNewsSource_FetchNews_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 2, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}
	src := NewAlpacaNewsSource("key", "secret", WithNewsBaseURL(srv.URL), WithNewsRetry(cfg))

	_, err := src.FetchNews(context.Background(), []string{"AAPL"}, time.Time{})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestAlpacaNewsSource_FetchNews_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"forbidden"}`)
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 1, baseDelay: time.Millisecond, maxDelay: 5 * time.Millisecond}
	src := NewAlpacaNewsSource("bad-key", "bad-secret", WithNewsBaseURL(srv.URL), WithNewsRetry(cfg))

	_, err := src.FetchNews(context.Background(), []string{"AAPL"}, time.Time{})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
	assert.Contains(t, err.Error(), "forbidden")
}

func TestAlpacaNewsSource_FetchNews_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	src := NewAlpacaNewsSource("key", "secret")
	_, err := src.FetchNews(ctx, []string{"AAPL"}, time.Time{})
	require.Error(t, err)
}

func TestAlpacaNewsSource_FetchNews_SymbolsNilBecomesEmptySlice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := alpacaNewsResponse{
			News: []alpacaNewsArticle{
				{ID: 200, Headline: "General news", Source: "src", Symbols: nil, CreatedAt: "2024-07-01T12:00:00Z"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	src := NewAlpacaNewsSource("key", "secret", WithNewsBaseURL(srv.URL))
	articles, err := src.FetchNews(context.Background(), []string{"AAPL"}, time.Time{})
	require.NoError(t, err)
	require.Len(t, articles, 1)
	assert.NotNil(t, articles[0].Symbols, "Symbols should never be nil")
	assert.Empty(t, articles[0].Symbols)
}

func TestAlpacaNewsSource_FetchNews_ImplementsNewsSource(t *testing.T) {
	var _ bullarc.NewsSource = (*AlpacaNewsSource)(nil)
}
