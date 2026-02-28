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

// --- helpers ---

func makeContract(contractType string, strikePrice float64, expDate string, volume, oi float64, ask, bid float64) polygonOptionsContract {
	return polygonOptionsContract{
		Details: polygonOptionsDetails{
			ContractType:   contractType,
			ExpirationDate: expDate,
			StrikePrice:    strikePrice,
			Ticker:         fmt.Sprintf("O:AAPL%sC%08.0f", expDate, strikePrice*1000),
		},
		Day:          polygonOptionsDayStats{Volume: volume},
		LastQuote:    polygonOptionsLastQuote{Ask: ask, Bid: bid, Midpoint: (ask + bid) / 2},
		OpenInterest: oi,
	}
}

func polygonOptionsServer(t *testing.T, results []polygonOptionsContract) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "testkey", r.URL.Query().Get("apiKey"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(polygonOptionsSnapshotResponse{
			Status:  "OK",
			Results: results,
		})
	}))
}

// --- interface compliance ---

func TestPolygonOptionsSource_ImplementsOptionsSource(t *testing.T) {
	var _ bullarc.OptionsSource = (*PolygonOptionsSource)(nil)
}

// --- no api key ---

func TestPolygonOptionsSource_NoAPIKey_ReturnsNotConfigured(t *testing.T) {
	src := NewPolygonOptionsSource("")
	_, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.Error(t, err)
	requireCode(t, err, "NOT_CONFIGURED")
}

// --- crypto skip ---

func TestPolygonOptionsSource_CryptoSymbol_ReturnsNil(t *testing.T) {
	src := NewPolygonOptionsSource("testkey")
	events, err := src.FetchOptionsActivity(context.Background(), "BTC/USD", bullarc.OptionsConfig{})
	require.NoError(t, err)
	assert.Nil(t, events)
}

// --- unusual volume detection ---

func TestPolygonOptionsSource_UnusualVolume_Detected(t *testing.T) {
	// volume (600) > 3 × OI (100) → unusual_volume
	// price = 0.25 midpoint → premium = 0.25 * 600 * 100 = 15_000 < default threshold
	contracts := []polygonOptionsContract{
		makeContract("call", 150.0, "2024-03-15", 600, 100, 0.3, 0.2),
	}
	srv := polygonOptionsServer(t, contracts)
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.NoError(t, err)
	require.Len(t, events, 1)

	e := events[0]
	assert.Equal(t, "AAPL", e.Symbol)
	assert.Equal(t, 150.0, e.Strike)
	assert.Equal(t, "call", e.Direction)
	assert.Equal(t, int64(600), e.Volume)
	assert.Equal(t, int64(100), e.OpenInterest)
	assert.Equal(t, bullarc.OptionsActivityUnusualVolume, e.ActivityType)
	assert.Equal(t, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC), e.Expiration)
}

// --- block trade detection ---

func TestPolygonOptionsSource_BlockTrade_Detected(t *testing.T) {
	// premium = 5.0 * 200 * 100 = 100_000 ≥ default threshold
	// volume (200) <= 3 × OI (300) → block only
	contracts := []polygonOptionsContract{
		makeContract("put", 140.0, "2024-06-21", 200, 300, 5.0, 5.0),
	}
	srv := polygonOptionsServer(t, contracts)
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{
		PremiumThreshold: 100_000,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)

	e := events[0]
	assert.Equal(t, "put", e.Direction)
	assert.Equal(t, bullarc.OptionsActivityBlock, e.ActivityType)
	assert.InDelta(t, 100_000.0, e.Premium, 1)
}

// --- sweep detection (block + unusual volume) ---

func TestPolygonOptionsSource_Sweep_Detected(t *testing.T) {
	// volume (900) > 3 × OI (100) AND premium = 5.0 * 900 * 100 = 450_000 → sweep
	contracts := []polygonOptionsContract{
		makeContract("call", 155.0, "2024-09-20", 900, 100, 5.0, 4.8),
	}
	srv := polygonOptionsServer(t, contracts)
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{
		PremiumThreshold: 100_000,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.Equal(t, bullarc.OptionsActivitySweep, events[0].ActivityType)
}

// --- custom premium threshold ---

func TestPolygonOptionsSource_CustomPremiumThreshold(t *testing.T) {
	// premium = 2.0 * 100 * 100 = 20_000 < default 100_000, but > custom 10_000
	// volume (100) <= 3 × OI (200) → block only
	contracts := []polygonOptionsContract{
		makeContract("call", 160.0, "2024-12-20", 100, 200, 2.0, 2.0),
	}
	srv := polygonOptionsServer(t, contracts)
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))

	// With default threshold: should not be flagged
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.NoError(t, err)
	assert.Empty(t, events)

	// With lower threshold: should be flagged
	srv2 := polygonOptionsServer(t, contracts)
	defer srv2.Close()
	src2 := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv2.URL))
	events2, err := src2.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{
		PremiumThreshold: 10_000,
	})
	require.NoError(t, err)
	require.Len(t, events2, 1)
	assert.Equal(t, bullarc.OptionsActivityBlock, events2[0].ActivityType)
}

// --- no unusual activity ---

func TestPolygonOptionsSource_NormalContracts_NotFlagged(t *testing.T) {
	// volume (50) < 3 × OI (1000), premium = 1.5 * 50 * 100 = 7500 < 100000
	contracts := []polygonOptionsContract{
		makeContract("call", 150.0, "2024-03-15", 50, 1000, 1.5, 1.4),
		makeContract("put", 145.0, "2024-03-15", 30, 800, 1.0, 0.9),
	}
	srv := polygonOptionsServer(t, contracts)
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.NoError(t, err)
	assert.Empty(t, events)
}

// --- zero volume skip ---

func TestPolygonOptionsSource_ZeroVolume_Skipped(t *testing.T) {
	contracts := []polygonOptionsContract{
		makeContract("call", 150.0, "2024-03-15", 0, 100, 2.0, 1.8),
	}
	srv := polygonOptionsServer(t, contracts)
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.NoError(t, err)
	assert.Empty(t, events)
}

// --- empty results ---

func TestPolygonOptionsSource_EmptyResults_ReturnsNil(t *testing.T) {
	srv := polygonOptionsServer(t, nil)
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.NoError(t, err)
	assert.Nil(t, events)
}

// --- http error cases ---

func TestPolygonOptionsSource_Unauthorized_ReturnsNotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "invalid api key")
	}))
	defer srv.Close()

	src := NewPolygonOptionsSource("badkey", WithPolygonOptionsBaseURL(srv.URL))
	_, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.Error(t, err)
	requireCode(t, err, "NOT_CONFIGURED")
}

func TestPolygonOptionsSource_Forbidden_ReturnsNotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	src := NewPolygonOptionsSource("badkey", WithPolygonOptionsBaseURL(srv.URL))
	_, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.Error(t, err)
	requireCode(t, err, "NOT_CONFIGURED")
}

func TestPolygonOptionsSource_RateLimited_ReturnsRateLimitExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	_, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.Error(t, err)
	requireCode(t, err, "RATE_LIMIT_EXCEEDED")
}

func TestPolygonOptionsSource_NotFound_ReturnsSymbolNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "ticker not found")
	}))
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	_, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.Error(t, err)
	requireCode(t, err, "SYMBOL_NOT_FOUND")
}

func TestPolygonOptionsSource_ServerError_ReturnsDataSourceUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	_, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestPolygonOptionsSource_InvalidJSON_ReturnsDataSourceUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not json")
	}))
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	_, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestPolygonOptionsSource_NetworkError_ReturnsDataSourceUnavailable(t *testing.T) {
	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL("http://127.0.0.1:1"))
	_, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

// --- pagination ---

func TestPolygonOptionsSource_Pagination_FetchesAllPages(t *testing.T) {
	page1 := []polygonOptionsContract{
		makeContract("call", 150.0, "2024-03-15", 600, 100, 2.0, 1.8),
	}
	page2 := []polygonOptionsContract{
		makeContract("call", 155.0, "2024-03-15", 700, 100, 3.0, 2.9),
	}

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			// First page returns a next_url pointer.
			json.NewEncoder(w).Encode(polygonOptionsSnapshotResponse{
				Status:  "OK",
				Results: page1,
				NextURL: "http://" + r.Host + "/v3/snapshot/options/AAPL?cursor=page2",
			})
		} else {
			json.NewEncoder(w).Encode(polygonOptionsSnapshotResponse{
				Status:  "OK",
				Results: page2,
			})
		}
	}))
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{})
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Len(t, events, 2)
}

// --- PC ratio anomaly ---

func TestPolygonOptionsSource_PCRatioAnomaly_FlagsExtraPutContracts(t *testing.T) {
	// Put volume (1000) >> call volume (100) → PC ratio = 10.0
	// Historical average ≈ 1.0 with low std dev → clearly anomalous
	contracts := []polygonOptionsContract{
		makeContract("put", 140.0, "2024-06-21", 1000, 900, 1.0, 0.9),  // volume >= OI → should be included
		makeContract("call", 150.0, "2024-06-21", 100, 2000, 1.0, 0.9), // call side: not flagged
	}
	srv := polygonOptionsServer(t, contracts)
	defer srv.Close()

	// Historical PC ratios around 1.0 — current 10.0 is many σ away.
	historical := []float64{1.0, 1.05, 0.95, 1.02, 0.98, 1.01, 0.99, 1.03, 0.97, 1.0,
		1.02, 0.98, 1.01, 0.99, 1.0, 1.03, 0.97, 1.0, 1.02, 0.98}

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{
		HistoricalPCRatios: historical,
	})
	require.NoError(t, err)

	// Put contract with volume >= OI should be flagged.
	require.Len(t, events, 1)
	assert.Equal(t, "put", events[0].Direction)
	assert.Equal(t, bullarc.OptionsActivityUnusualVolume, events[0].ActivityType)
}

func TestPolygonOptionsSource_PCRatioAnomaly_InsufficientHistory_Skipped(t *testing.T) {
	// Only 1 historical data point → anomaly check skipped.
	contracts := []polygonOptionsContract{
		makeContract("put", 140.0, "2024-06-21", 1000, 900, 1.0, 0.9),
		makeContract("call", 150.0, "2024-06-21", 100, 2000, 1.0, 0.9),
	}
	srv := polygonOptionsServer(t, contracts)
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{
		HistoricalPCRatios: []float64{1.0}, // only 1 entry → skipped
	})
	require.NoError(t, err)
	// No unusual volume or block criteria met, so nothing flagged.
	assert.Empty(t, events)
}

// --- premium calculation ---

func TestPolygonOptionsSource_Premium_UsesAskWhenBidIsZero(t *testing.T) {
	contracts := []polygonOptionsContract{{
		Details: polygonOptionsDetails{
			ContractType:   "call",
			ExpirationDate: "2024-03-15",
			StrikePrice:    150.0,
			Ticker:         "O:AAPL240315C00150000",
		},
		Day:          polygonOptionsDayStats{Volume: 200},
		LastQuote:    polygonOptionsLastQuote{Ask: 5.0, Bid: 0, Midpoint: 0},
		OpenInterest: 50,
	}}
	srv := polygonOptionsServer(t, contracts)
	defer srv.Close()

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	events, err := src.FetchOptionsActivity(context.Background(), "AAPL", bullarc.OptionsConfig{
		PremiumThreshold: 100_000,
	})
	require.NoError(t, err)
	// premium = 5.0 * 200 * 100 = 100_000 (exactly at threshold, volume = 4x OI)
	require.Len(t, events, 1)
	assert.InDelta(t, 100_000.0, events[0].Premium, 1)
}

// --- helper function unit tests ---

func TestClassifyOptionsActivity(t *testing.T) {
	assert.Equal(t, bullarc.OptionsActivitySweep, classifyOptionsActivity(true, true))
	assert.Equal(t, bullarc.OptionsActivityBlock, classifyOptionsActivity(true, false))
	assert.Equal(t, bullarc.OptionsActivityUnusualVolume, classifyOptionsActivity(false, true))
	assert.Equal(t, bullarc.OptionsActivityUnusualVolume, classifyOptionsActivity(false, false))
}

func TestContractPrice(t *testing.T) {
	// Midpoint takes priority.
	assert.Equal(t, 2.0, contractPrice(polygonOptionsLastQuote{Ask: 3.0, Bid: 1.0, Midpoint: 2.0}))
	// Ask+bid average when no midpoint.
	assert.Equal(t, 2.0, contractPrice(polygonOptionsLastQuote{Ask: 3.0, Bid: 1.0, Midpoint: 0}))
	// Only ask available.
	assert.Equal(t, 3.0, contractPrice(polygonOptionsLastQuote{Ask: 3.0, Bid: 0, Midpoint: 0}))
	// Only bid available.
	assert.Equal(t, 1.0, contractPrice(polygonOptionsLastQuote{Ask: 0, Bid: 1.0, Midpoint: 0}))
}

func TestPCRatioAnomaly(t *testing.T) {
	historical := []float64{1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0, 1.0}

	// Clear anomaly: put/call = 10 → far above mean of 1.0
	anomalous, dominant := pcRatioAnomaly(1000, 100, historical)
	assert.True(t, anomalous)
	assert.Equal(t, "put", dominant)

	// Call-side anomaly: put/call = 0.1 → far below mean of 1.0
	anomalous2, dominant2 := pcRatioAnomaly(10, 100, historical)
	assert.True(t, anomalous2)
	assert.Equal(t, "call", dominant2)

	// Normal: put/call = 1.0 → no anomaly
	anomalous3, _ := pcRatioAnomaly(100, 100, historical)
	assert.False(t, anomalous3)

	// Insufficient history: not enough data
	anomalous4, _ := pcRatioAnomaly(1000, 100, []float64{1.0})
	assert.False(t, anomalous4)

	// No call volume
	anomalous5, _ := pcRatioAnomaly(1000, 0, historical)
	assert.False(t, anomalous5)
}

func TestAppendAPIKey(t *testing.T) {
	assert.Equal(t, "https://example.com?apiKey=k", appendAPIKey("https://example.com", "k"))
	assert.Equal(t, "https://example.com?cursor=1&apiKey=k", appendAPIKey("https://example.com?cursor=1", "k"))
	assert.Equal(t, "https://example.com?apiKey=existing", appendAPIKey("https://example.com?apiKey=existing", "k"))
}

// --- context cancellation ---

func TestPolygonOptionsSource_ContextCancelled_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve a page with a next_url so the pagination loop runs.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(polygonOptionsSnapshotResponse{
			Status:  "OK",
			Results: []polygonOptionsContract{},
			NextURL: "http://" + r.Host + "/v3/snapshot/options/AAPL?cursor=page2",
		})
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	src := NewPolygonOptionsSource("testkey", WithPolygonOptionsBaseURL(srv.URL))
	_, err := src.FetchOptionsActivity(ctx, "AAPL", bullarc.OptionsConfig{})
	require.Error(t, err)

	var be *bullarc.Error
	require.True(t, errors.As(err, &be))
	assert.Equal(t, "TIMEOUT", be.Code)
}
