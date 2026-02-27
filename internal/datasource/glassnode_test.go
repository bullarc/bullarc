package datasource

import (
	"context"
	"encoding/json"
	"errors"
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

// sampleGlassnodeResponse returns a minimal valid Glassnode net-flow API response.
func sampleGlassnodeResponse(t int64, v float64) []glassnodeDataPoint {
	return []glassnodeDataPoint{{T: t, V: v}}
}

func TestGlassnodeTracker_ImplementsChainFlowSource(t *testing.T) {
	var _ bullarc.ChainFlowSource = (*GlassnodeTracker)(nil)
}

func TestGlassnodeTracker_FetchChainMetrics_NoAPIKey(t *testing.T) {
	tracker := NewGlassnodeTracker("")
	_, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD"})
	require.Error(t, err)

	var e *bullarc.Error
	require.True(t, errors.As(err, &e))
	assert.Equal(t, "NOT_CONFIGURED", e.Code)
}

func TestGlassnodeTracker_FetchChainMetrics_NonCryptoSymbolSkipped(t *testing.T) {
	// Server should never be called for non-crypto symbols.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for non-crypto symbols")
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	metrics, err := tracker.FetchChainMetrics(context.Background(), []string{"AAPL", "MSFT", "TSLA"})
	require.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestGlassnodeTracker_FetchChainMetrics_Inflow(t *testing.T) {
	ts := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, glassnodeNetFlowPath, r.URL.Path)
		assert.Equal(t, "BTC", r.URL.Query().Get("a"))
		assert.Equal(t, "testkey", r.URL.Query().Get("api_key"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleGlassnodeResponse(ts.Unix(), 1234.56))
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	metrics, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	m := metrics[0]
	assert.Equal(t, "BTC/USD", m.Symbol)
	assert.InDelta(t, 1234.56, m.NetFlow, 0.001)
	assert.Equal(t, bullarc.FlowDirectionInflow, m.FlowDirection)
	assert.Equal(t, ts, m.Timestamp)
}

func TestGlassnodeTracker_FetchChainMetrics_Outflow(t *testing.T) {
	ts := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleGlassnodeResponse(ts.Unix(), -500.0))
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	metrics, err := tracker.FetchChainMetrics(context.Background(), []string{"ETH/USD"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	m := metrics[0]
	assert.Equal(t, "ETH/USD", m.Symbol)
	assert.InDelta(t, -500.0, m.NetFlow, 0.001)
	assert.Equal(t, bullarc.FlowDirectionOutflow, m.FlowDirection)
}

func TestGlassnodeTracker_FetchChainMetrics_ZeroFlowIsOutflow(t *testing.T) {
	ts := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleGlassnodeResponse(ts.Unix(), 0.0))
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	metrics, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, bullarc.FlowDirectionOutflow, metrics[0].FlowDirection)
}

func TestGlassnodeTracker_FetchChainMetrics_MultipleSymbols_MixedTypes(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		asset := r.URL.Query().Get("a")
		var v float64
		switch asset {
		case "BTC":
			v = 1000.0
		case "ETH":
			v = -200.0
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleGlassnodeResponse(time.Now().Unix(), v))
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	// Mix of crypto and non-crypto.
	metrics, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD", "AAPL", "ETH/USD", "MSFT"})
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	assert.Equal(t, 2, callCount, "only crypto symbols should trigger API calls")

	bySymbol := make(map[string]bullarc.ChainMetrics)
	for _, m := range metrics {
		bySymbol[m.Symbol] = m
	}

	assert.Equal(t, bullarc.FlowDirectionInflow, bySymbol["BTC/USD"].FlowDirection)
	assert.Equal(t, bullarc.FlowDirectionOutflow, bySymbol["ETH/USD"].FlowDirection)
}

func TestGlassnodeTracker_FetchChainMetrics_Non200Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "service unavailable")
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	_, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD"})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestGlassnodeTracker_FetchChainMetrics_UnauthorizedReturnsNotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "invalid api key")
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("badkey", WithGlassnodeBaseURL(srv.URL))
	_, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD"})
	require.Error(t, err)
	requireCode(t, err, "NOT_CONFIGURED")
}

func TestGlassnodeTracker_FetchChainMetrics_ForbiddenReturnsNotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "forbidden")
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("badkey", WithGlassnodeBaseURL(srv.URL))
	_, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD"})
	require.Error(t, err)
	requireCode(t, err, "NOT_CONFIGURED")
}

func TestGlassnodeTracker_FetchChainMetrics_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not json")
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	_, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD"})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestGlassnodeTracker_FetchChainMetrics_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "[]")
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	metrics, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD"})
	require.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestGlassnodeTracker_FetchChainMetrics_NetworkError(t *testing.T) {
	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL("http://127.0.0.1:1"))
	_, err := tracker.FetchChainMetrics(context.Background(), []string{"BTC/USD"})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestGlassnodeTracker_FetchChainMetrics_CaseInsensitiveSymbol(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Asset should be uppercased.
		assert.Equal(t, "BTC", r.URL.Query().Get("a"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleGlassnodeResponse(time.Now().Unix(), 100.0))
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	metrics, err := tracker.FetchChainMetrics(context.Background(), []string{"btc/usd"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Equal(t, "BTC/USD", metrics[0].Symbol)
}

func TestGlassnodeTracker_FetchChainMetrics_EmptySymbolList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for empty symbol list")
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	metrics, err := tracker.FetchChainMetrics(context.Background(), []string{})
	require.NoError(t, err)
	assert.Empty(t, metrics)
}

func TestExtractCryptoAsset(t *testing.T) {
	tests := []struct {
		symbol string
		want   string
	}{
		{"BTC/USD", "BTC"},
		{"ETH/USD", "ETH"},
		{"btc/usd", "BTC"},
		{"SOL/USDT", "SOL"},
		{"BTC", "BTC"},
		{"eth", "ETH"},
	}
	for _, tc := range tests {
		t.Run(tc.symbol, func(t *testing.T) {
			got := extractCryptoAsset(tc.symbol)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestGlassnodeTracker_FetchChainMetrics_SymbolUppercasedInResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sampleGlassnodeResponse(time.Now().Unix(), 50.0))
	}))
	defer srv.Close()

	tracker := NewGlassnodeTracker("testkey", WithGlassnodeBaseURL(srv.URL))
	metrics, err := tracker.FetchChainMetrics(context.Background(), []string{"eth/usd"})
	require.NoError(t, err)
	require.Len(t, metrics, 1)

	// Symbol in result must be uppercased.
	assert.Equal(t, "ETH/USD", metrics[0].Symbol)
	assert.False(t, strings.Contains(metrics[0].Symbol, "eth"), "symbol should be uppercase")
}
