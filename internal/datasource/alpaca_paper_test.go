package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlpacaPaperTrader_PlaceOrder_Buy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v2/orders", r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get("APCA-API-KEY-ID"))
		assert.Equal(t, "test-secret", r.Header.Get("APCA-API-SECRET-KEY"))

		var req alpacaOrderRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "AAPL", req.Symbol)
		assert.Equal(t, "10", req.Qty)
		assert.Equal(t, "buy", req.Side)
		assert.Equal(t, "market", req.Type)
		assert.Equal(t, "day", req.TimeInForce)

		filledAt := time.Date(2024, 7, 1, 12, 0, 0, 0, time.UTC)
		filledPrice := "150.25"
		resp := alpacaOrderResponse{
			ID:             "order-123",
			Symbol:         "AAPL",
			Side:           "buy",
			Qty:            "10",
			FilledQty:      "10",
			FilledAvgPrice: &filledPrice,
			FilledAt:       &filledAt,
			Status:         "filled",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("test-key", "test-secret", WithPaperBaseURL(srv.URL))
	result, err := trader.PlaceOrder(context.Background(), bullarc.Order{
		Symbol:            "AAPL",
		Side:              bullarc.OrderSideBuy,
		Qty:               10,
		SignalConfidence:  75.0,
		SignalExplanation: "RSI oversold",
	})
	require.NoError(t, err)
	assert.Equal(t, "order-123", result.OrderID)
	assert.Equal(t, "AAPL", result.Symbol)
	assert.Equal(t, bullarc.OrderSideBuy, result.Side)
	assert.Equal(t, float64(10), result.Qty)
	assert.Equal(t, 150.25, result.FilledPrice)
	assert.Equal(t, "filled", result.Status)
}

func TestAlpacaPaperTrader_PlaceOrder_Sell(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req alpacaOrderRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "sell", req.Side)

		filledPrice := "152.00"
		filledAt := time.Now()
		resp := alpacaOrderResponse{
			ID:             "order-456",
			Symbol:         "AAPL",
			Side:           "sell",
			Qty:            "5",
			FilledQty:      "5",
			FilledAvgPrice: &filledPrice,
			FilledAt:       &filledAt,
			Status:         "filled",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	result, err := trader.PlaceOrder(context.Background(), bullarc.Order{
		Symbol: "AAPL",
		Side:   bullarc.OrderSideSell,
		Qty:    5,
	})
	require.NoError(t, err)
	assert.Equal(t, bullarc.OrderSideSell, result.Side)
	assert.Equal(t, float64(5), result.Qty)
}

func TestAlpacaPaperTrader_PlaceOrder_ZeroQty(t *testing.T) {
	trader := NewAlpacaPaperTrader("key", "secret")
	_, err := trader.PlaceOrder(context.Background(), bullarc.Order{
		Symbol: "AAPL",
		Side:   bullarc.OrderSideBuy,
		Qty:    0,
	})
	require.Error(t, err)
	requireCode(t, err, "INVALID_PARAMETER")
}

func TestAlpacaPaperTrader_PlaceOrder_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"forbidden"}`)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("bad-key", "bad-secret", WithPaperBaseURL(srv.URL))
	_, err := trader.PlaceOrder(context.Background(), bullarc.Order{Symbol: "AAPL", Side: bullarc.OrderSideBuy, Qty: 1})
	require.Error(t, err)
	requireCode(t, err, "NOT_CONFIGURED")
}

func TestAlpacaPaperTrader_PlaceOrder_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	trader := NewAlpacaPaperTrader("key", "secret")
	_, err := trader.PlaceOrder(ctx, bullarc.Order{Symbol: "AAPL", Side: bullarc.OrderSideBuy, Qty: 1})
	require.Error(t, err)
}

func TestAlpacaPaperTrader_GetPositions_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v2/positions", r.URL.Path)

		positions := []alpacaPositionResponse{
			{
				Symbol:         "AAPL",
				Qty:            "10",
				AvgEntryPrice:  "150.00",
				CurrentPrice:   "155.00",
				UnrealizedPL:   "50.00",
				UnrealizedPLPC: "0.0333",
			},
			{
				Symbol:         "MSFT",
				Qty:            "5",
				AvgEntryPrice:  "300.00",
				CurrentPrice:   "290.00",
				UnrealizedPL:   "-50.00",
				UnrealizedPLPC: "-0.0333",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(positions)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	positions, err := trader.GetPositions(context.Background())
	require.NoError(t, err)
	require.Len(t, positions, 2)

	assert.Equal(t, "AAPL", positions[0].Symbol)
	assert.Equal(t, float64(10), positions[0].Qty)
	assert.Equal(t, 150.00, positions[0].AvgEntryPrice)
	assert.Equal(t, 155.00, positions[0].CurrentPrice)
	assert.Equal(t, 50.00, positions[0].UnrealizedPnL)
	assert.InDelta(t, 0.0333, positions[0].UnrealizedPnLPct, 0.0001)

	assert.Equal(t, "MSFT", positions[1].Symbol)
	assert.Equal(t, -50.00, positions[1].UnrealizedPnL)
}

func TestAlpacaPaperTrader_GetPositions_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	positions, err := trader.GetPositions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, positions)
}

func TestAlpacaPaperTrader_ClosePosition_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v2/positions/AAPL", r.URL.Path)

		filledPrice := "155.50"
		filledAt := time.Now()
		resp := alpacaOrderResponse{
			ID:             "close-order-789",
			Symbol:         "AAPL",
			Side:           "sell",
			Qty:            "10",
			FilledQty:      "10",
			FilledAvgPrice: &filledPrice,
			FilledAt:       &filledAt,
			Status:         "filled",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	result, err := trader.ClosePosition(context.Background(), "AAPL")
	require.NoError(t, err)
	assert.Equal(t, "close-order-789", result.OrderID)
	assert.Equal(t, "AAPL", result.Symbol)
	assert.Equal(t, "filled", result.Status)
}

func TestAlpacaPaperTrader_ClosePosition_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"position not found"}`)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	_, err := trader.ClosePosition(context.Background(), "XXXX")
	require.Error(t, err)
	requireCode(t, err, "SYMBOL_NOT_FOUND")
}

func TestAlpacaPaperTrader_ClosePosition_EmptySymbol(t *testing.T) {
	trader := NewAlpacaPaperTrader("key", "secret")
	_, err := trader.ClosePosition(context.Background(), "")
	require.Error(t, err)
	requireCode(t, err, "INVALID_PARAMETER")
}

func TestAlpacaPaperTrader_CloseAll_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/v2/positions", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	err := trader.CloseAll(context.Background())
	require.NoError(t, err)
}

func TestAlpacaPaperTrader_CloseAll_NoPositions(t *testing.T) {
	// Alpaca returns 422 when there are no positions to close — should be treated as success.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"no open positions"}`)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	err := trader.CloseAll(context.Background())
	require.NoError(t, err)
}

func TestAlpacaPaperTrader_CloseAll_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"internal error"}`)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	err := trader.CloseAll(context.Background())
	require.Error(t, err)
	requireCode(t, err, "DATA_SOURCE_UNAVAILABLE")
}

func TestAlpacaPaperTrader_GetAccountEquity_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v2/account", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"equity":"100000.50"}`)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	equity, err := trader.GetAccountEquity(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 100000.50, equity)
}

func TestAlpacaPaperTrader_PlaceOrder_QtyFormatting(t *testing.T) {
	// Verify fractional quantities are sent correctly (e.g. for crypto).
	var capturedQty string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req alpacaOrderRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		capturedQty = req.Qty

		filledPrice := "60000"
		filledAt := time.Now()
		resp := alpacaOrderResponse{
			ID: "o1", Symbol: "BTC/USD", Side: "buy",
			Qty: capturedQty, FilledQty: capturedQty,
			FilledAvgPrice: &filledPrice, FilledAt: &filledAt, Status: "filled",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	trader := NewAlpacaPaperTrader("key", "secret", WithPaperBaseURL(srv.URL))
	_, err := trader.PlaceOrder(context.Background(), bullarc.Order{
		Symbol: "BTC/USD",
		Side:   bullarc.OrderSideBuy,
		Qty:    0.00123,
	})
	require.NoError(t, err)
	assert.Equal(t, "0.00123", capturedQty)
}

func TestAlpacaPaperTrader_ImplementsPaperTrader(t *testing.T) {
	// Compile-time check: AlpacaPaperTrader implements bullarc.PaperTrader.
	var _ bullarc.PaperTrader = NewAlpacaPaperTrader("key", "secret")
}
