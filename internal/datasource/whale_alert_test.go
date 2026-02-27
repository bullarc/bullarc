package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildWhaleAlertResponse returns a minimal Whale Alert API response JSON.
func buildWhaleAlertResponse(txns []whaleAlertTransaction) whaleAlertResponse {
	return whaleAlertResponse{
		Result:       "success",
		Count:        len(txns),
		Transactions: txns,
	}
}

func sampleWhaleAlertTx(symbol string, amountUSD float64, fromType, toType string) whaleAlertTransaction {
	return whaleAlertTransaction{
		Blockchain: "bitcoin",
		Symbol:     symbol,
		Hash:       "abc123hash",
		From:       whaleAlertEntity{Address: "addr1", Owner: fromType, OwnerType: fromType},
		To:         whaleAlertEntity{Address: "addr2", Owner: toType, OwnerType: toType},
		Timestamp:  time.Now().Unix(),
		Amount:     50.0,
		AmountUSD:  amountUSD,
	}
}

func TestWhaleAlertTracker_ImplementsWhaleAlertSource(t *testing.T) {
	var _ bullarc.WhaleAlertSource = (*WhaleAlertTracker)(nil)
}

func TestWhaleAlertTracker_NoAPIKey_ReturnsNotConfigured(t *testing.T) {
	tracker := NewWhaleAlertTracker("")
	_, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.Error(t, err)

	var e *bullarc.Error
	require.True(t, errors.As(err, &e))
	assert.Equal(t, "NOT_CONFIGURED", e.Code)
}

func TestWhaleAlertTracker_FetchWhaleAlerts_MatchingSymbol(t *testing.T) {
	ts := time.Now().UTC().Truncate(time.Second)
	tx := whaleAlertTransaction{
		Symbol:    "btc",
		Hash:      "txhash1",
		From:      whaleAlertEntity{Owner: "unknown", OwnerType: "unknown"},
		To:        whaleAlertEntity{Owner: "binance", OwnerType: "exchange"},
		Timestamp: ts.Unix(),
		Amount:    100.0,
		AmountUSD: 5_000_000,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, whaleAlertTransactionsPath, r.URL.Path)
		assert.Equal(t, "testkey", r.URL.Query().Get("api_key"))
		assert.Equal(t, "1000000", r.URL.Query().Get("min_value"))

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildWhaleAlertResponse([]whaleAlertTransaction{tx}))
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("testkey", WithWhaleAlertBaseURL(srv.URL))
	alerts, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, alerts, 1)

	a := alerts[0]
	assert.Equal(t, "BTC", a.Symbol)
	assert.Equal(t, "txhash1", a.TxHash)
	assert.InDelta(t, 100.0, a.Amount, 0.001)
	assert.InDelta(t, 5_000_000.0, a.AmountUSD, 0.001)
	assert.Equal(t, "unknown", a.FromType)
	assert.Equal(t, "exchange", a.ToType)
	assert.Equal(t, "binance", a.ToEntity)
	assert.Equal(t, ts, a.Timestamp)
}

func TestWhaleAlertTracker_FetchWhaleAlerts_FiltersNonMatchingSymbols(t *testing.T) {
	txns := []whaleAlertTransaction{
		sampleWhaleAlertTx("btc", 3_000_000, "unknown", "exchange"),
		sampleWhaleAlertTx("eth", 2_000_000, "unknown", "wallet"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildWhaleAlertResponse(txns))
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("testkey", WithWhaleAlertBaseURL(srv.URL))
	alerts, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	assert.Equal(t, "BTC", alerts[0].Symbol)
}

func TestWhaleAlertTracker_FetchWhaleAlerts_EmptyTransactions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildWhaleAlertResponse(nil))
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("testkey", WithWhaleAlertBaseURL(srv.URL))
	alerts, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.NotNil(t, alerts)
	assert.Empty(t, alerts)
}

func TestWhaleAlertTracker_FetchWhaleAlerts_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "invalid api key")
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("badkey", WithWhaleAlertBaseURL(srv.URL))
	_, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.Error(t, err)
	requireCode(t, err, "NOT_CONFIGURED")
}

func TestWhaleAlertTracker_FetchWhaleAlerts_Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "forbidden")
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("badkey", WithWhaleAlertBaseURL(srv.URL))
	_, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.Error(t, err)
	requireCode(t, err, "NOT_CONFIGURED")
}

func TestWhaleAlertTracker_FetchWhaleAlerts_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("testkey", WithWhaleAlertBaseURL(srv.URL))
	_, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.Error(t, err)
	requireCode(t, err, "RATE_LIMIT_EXCEEDED")
}

func TestWhaleAlertTracker_FetchWhaleAlerts_Non200Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "service unavailable")
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("testkey", WithWhaleAlertBaseURL(srv.URL))
	_, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestWhaleAlertTracker_FetchWhaleAlerts_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not json at all")
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("testkey", WithWhaleAlertBaseURL(srv.URL))
	_, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestWhaleAlertTracker_FetchWhaleAlerts_NetworkError(t *testing.T) {
	tracker := NewWhaleAlertTracker("testkey", WithWhaleAlertBaseURL("http://127.0.0.1:1"))
	_, err := tracker.FetchWhaleAlerts(context.Background(), "BTC/USD", time.Now().Add(-time.Hour))
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestWhaleAlertTracker_FetchWhaleAlerts_CaseInsensitiveSymbolMatch(t *testing.T) {
	txns := []whaleAlertTransaction{
		sampleWhaleAlertTx("BTC", 2_000_000, "unknown", "exchange"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildWhaleAlertResponse(txns))
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("testkey", WithWhaleAlertBaseURL(srv.URL))
	// query with lowercase symbol — should still match
	alerts, err := tracker.FetchWhaleAlerts(context.Background(), "btc/usd", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	assert.Equal(t, "BTC", alerts[0].Symbol)
}

func TestWhaleAlertTracker_FetchWhaleAlerts_WalletToType(t *testing.T) {
	txns := []whaleAlertTransaction{
		sampleWhaleAlertTx("eth", 1_500_000, "coinbase", "wallet"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(buildWhaleAlertResponse(txns))
	}))
	defer srv.Close()

	tracker := NewWhaleAlertTracker("testkey", WithWhaleAlertBaseURL(srv.URL))
	alerts, err := tracker.FetchWhaleAlerts(context.Background(), "ETH/USD", time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	assert.Equal(t, "wallet", alerts[0].ToType)
}

func TestNormaliseEntityType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"exchange", "exchange"},
		{"Exchange", "exchange"},
		{"EXCHANGE", "exchange"},
		{"wallet", "wallet"},
		{"cold_wallet", "wallet"},
		{"unknown", "unknown"},
		{"", "unknown"},
		{"miner", "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := normaliseEntityType(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveEntityName(t *testing.T) {
	tests := []struct {
		entity whaleAlertEntity
		want   string
	}{
		{whaleAlertEntity{Owner: "binance", OwnerType: "exchange"}, "binance"},
		{whaleAlertEntity{Owner: "unknown", OwnerType: "exchange"}, "exchange"},
		{whaleAlertEntity{Owner: "", OwnerType: "wallet"}, "wallet"},
		{whaleAlertEntity{Owner: "", OwnerType: ""}, "unknown"},
	}
	for _, tc := range tests {
		got := resolveEntityName(tc.entity)
		assert.Equal(t, tc.want, got)
	}
}
