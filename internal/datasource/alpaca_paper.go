package datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bullarc/bullarc"
)

const (
	alpacaPaperBaseURL = "https://paper-api.alpaca.markets"
	paperTradingLabel  = "[PAPER TRADING]"
)

// AlpacaPaperTrader implements bullarc.PaperTrader using the Alpaca paper trading API.
// All orders and positions are simulated and clearly labeled as paper trading.
type AlpacaPaperTrader struct {
	keyID     string
	secretKey string
	baseURL   string
	client    *http.Client
}

// AlpacaPaperOption is a functional option for AlpacaPaperTrader.
type AlpacaPaperOption func(*AlpacaPaperTrader)

// WithPaperHTTPClient sets a custom HTTP client on the AlpacaPaperTrader.
func WithPaperHTTPClient(c *http.Client) AlpacaPaperOption {
	return func(t *AlpacaPaperTrader) { t.client = c }
}

// WithPaperBaseURL overrides the Alpaca paper trading base URL (useful for testing).
func WithPaperBaseURL(u string) AlpacaPaperOption {
	return func(t *AlpacaPaperTrader) { t.baseURL = u }
}

// NewAlpacaPaperTrader creates an AlpacaPaperTrader authenticated with the given credentials.
// keyID and secretKey must be Alpaca paper trading credentials from paper-api.alpaca.markets.
func NewAlpacaPaperTrader(keyID, secretKey string, opts ...AlpacaPaperOption) *AlpacaPaperTrader {
	t := &AlpacaPaperTrader{
		keyID:     keyID,
		secretKey: secretKey,
		baseURL:   alpacaPaperBaseURL,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// alpacaOrderRequest is the payload sent to POST /v2/orders.
type alpacaOrderRequest struct {
	Symbol      string `json:"symbol"`
	Qty         string `json:"qty"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
}

// alpacaOrderResponse is the response from POST /v2/orders and DELETE /v2/positions/{symbol}.
type alpacaOrderResponse struct {
	ID             string     `json:"id"`
	Symbol         string     `json:"symbol"`
	Side           string     `json:"side"`
	Qty            string     `json:"qty"`
	FilledQty      string     `json:"filled_qty"`
	FilledAvgPrice *string    `json:"filled_avg_price"`
	FilledAt       *time.Time `json:"filled_at"`
	Status         string     `json:"status"`
}

// alpacaPositionResponse is one entry from GET /v2/positions.
type alpacaPositionResponse struct {
	Symbol         string `json:"symbol"`
	Qty            string `json:"qty"`
	AvgEntryPrice  string `json:"avg_entry_price"`
	CurrentPrice   string `json:"current_price"`
	UnrealizedPL   string `json:"unrealized_pl"`
	UnrealizedPLPC string `json:"unrealized_plpc"`
}

// alpacaAccountResponse is the response from GET /v2/account.
type alpacaAccountResponse struct {
	Equity string `json:"equity"`
}

// PlaceOrder submits a market order to the Alpaca paper trading API.
// The order is logged with symbol, direction, quantity, price, timestamp,
// signal confidence, and explanation, all prefixed with [PAPER TRADING].
func (t *AlpacaPaperTrader) PlaceOrder(ctx context.Context, order bullarc.Order) (bullarc.OrderResult, error) {
	if order.Qty <= 0 {
		return bullarc.OrderResult{}, bullarc.ErrInvalidParameter.Wrap(fmt.Errorf("order qty must be positive, got %f", order.Qty))
	}

	qty := strconv.FormatFloat(order.Qty, 'f', 6, 64)
	// Trim trailing zeros for cleaner API request.
	qty = strings.TrimRight(qty, "0")
	qty = strings.TrimRight(qty, ".")

	reqBody := alpacaOrderRequest{
		Symbol:      order.Symbol,
		Qty:         qty,
		Side:        string(order.Side),
		Type:        "market",
		TimeInForce: "day",
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return bullarc.OrderResult{}, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("marshal order: %w", err))
	}

	endpoint := fmt.Sprintf("%s/v2/orders", t.baseURL)
	var resp alpacaOrderResponse
	if err := t.doRequest(ctx, http.MethodPost, endpoint, data, &resp); err != nil {
		return bullarc.OrderResult{}, wrapPaperError(err)
	}

	result := t.toOrderResult(resp)

	slog.Info(paperTradingLabel+" order placed",
		"symbol", result.Symbol,
		"side", result.Side,
		"qty", result.Qty,
		"filled_price", result.FilledPrice,
		"filled_at", result.FilledAt,
		"status", result.Status,
		"signal_confidence", order.SignalConfidence,
		"signal_explanation", order.SignalExplanation)

	return result, nil
}

// GetPositions returns all open paper trading positions with their current
// market value and unrealized P&L. Output is labeled as paper trading.
func (t *AlpacaPaperTrader) GetPositions(ctx context.Context) ([]bullarc.Position, error) {
	endpoint := fmt.Sprintf("%s/v2/positions", t.baseURL)
	var raw []alpacaPositionResponse
	if err := t.doRequest(ctx, http.MethodGet, endpoint, nil, &raw); err != nil {
		return nil, wrapPaperError(err)
	}

	positions := make([]bullarc.Position, 0, len(raw))
	for _, r := range raw {
		p, err := toPosition(r)
		if err != nil {
			slog.Warn(paperTradingLabel+" failed to parse position, skipping",
				"symbol", r.Symbol, "err", err)
			continue
		}
		positions = append(positions, p)
	}

	slog.Info(paperTradingLabel+" positions fetched", "count", len(positions))
	return positions, nil
}

// ClosePosition closes the open paper trading position for the given symbol.
// The resulting closing order is logged.
func (t *AlpacaPaperTrader) ClosePosition(ctx context.Context, symbol string) (bullarc.OrderResult, error) {
	if symbol == "" {
		return bullarc.OrderResult{}, bullarc.ErrInvalidParameter.Wrap(fmt.Errorf("symbol must not be empty"))
	}

	endpoint := fmt.Sprintf("%s/v2/positions/%s", t.baseURL, url.PathEscape(symbol))
	var resp alpacaOrderResponse
	if err := t.doRequest(ctx, http.MethodDelete, endpoint, nil, &resp); err != nil {
		return bullarc.OrderResult{}, wrapPaperError(err)
	}

	result := t.toOrderResult(resp)
	slog.Info(paperTradingLabel+" position closed",
		"symbol", symbol,
		"qty", result.Qty,
		"filled_price", result.FilledPrice,
		"status", result.Status)

	return result, nil
}

// CloseAll closes all open paper trading positions immediately.
// This is the kill switch: all simulated positions are liquidated.
func (t *AlpacaPaperTrader) CloseAll(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/v2/positions", t.baseURL)
	// DELETE /v2/positions returns an array of order responses (one per position closed).
	// We read and discard the body; errors are still surfaced.
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("build close-all request: %w", err))
	}
	req.Header.Set("APCA-API-KEY-ID", t.keyID)
	req.Header.Set("APCA-API-SECRET-KEY", t.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return bullarc.ErrTimeout.Wrap(err)
		}
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("close-all request: %w", err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusMultiStatus {
		if resp.StatusCode == http.StatusUnprocessableEntity {
			// 422 is returned when there are no positions to close — treat as success.
			slog.Info(paperTradingLabel + " no open positions to close")
			return nil
		}
		return bullarc.ErrDataSourceUnavailable.Wrap(
			&httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))})
	}

	slog.Info(paperTradingLabel + " all positions closed")
	return nil
}

// GetAccountEquity returns the current portfolio equity from the Alpaca account.
// This is used to calculate position sizes from the risk metrics percentage.
func (t *AlpacaPaperTrader) GetAccountEquity(ctx context.Context) (float64, error) {
	endpoint := fmt.Sprintf("%s/v2/account", t.baseURL)
	var resp alpacaAccountResponse
	if err := t.doRequest(ctx, http.MethodGet, endpoint, nil, &resp); err != nil {
		return 0, wrapPaperError(err)
	}

	equity, err := strconv.ParseFloat(resp.Equity, 64)
	if err != nil {
		return 0, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("parse equity %q: %w", resp.Equity, err))
	}
	return equity, nil
}

// doRequest executes an authenticated HTTP request, encodes body as JSON when
// non-nil, and decodes the JSON response into out.
func (t *AlpacaPaperTrader) doRequest(ctx context.Context, method, endpoint string, body []byte, out any) error {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("build request: %w", err))
	}
	req.Header.Set("APCA-API-KEY-ID", t.keyID)
	req.Header.Set("APCA-API-SECRET-KEY", t.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return bullarc.ErrTimeout.Wrap(err)
		}
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("http request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(respBody))}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("decode response: %w", err))
	}
	return nil
}

// toOrderResult converts an alpacaOrderResponse to bullarc.OrderResult.
func (t *AlpacaPaperTrader) toOrderResult(r alpacaOrderResponse) bullarc.OrderResult {
	qty, _ := strconv.ParseFloat(r.FilledQty, 64)
	if qty == 0 {
		qty, _ = strconv.ParseFloat(r.Qty, 64)
	}

	var filledPrice float64
	if r.FilledAvgPrice != nil {
		filledPrice, _ = strconv.ParseFloat(*r.FilledAvgPrice, 64)
	}

	var filledAt time.Time
	if r.FilledAt != nil {
		filledAt = *r.FilledAt
	}

	return bullarc.OrderResult{
		OrderID:     r.ID,
		Symbol:      r.Symbol,
		Side:        bullarc.OrderSide(r.Side),
		Qty:         qty,
		FilledPrice: filledPrice,
		FilledAt:    filledAt,
		Status:      r.Status,
	}
}

// toPosition converts an alpacaPositionResponse to bullarc.Position.
func toPosition(r alpacaPositionResponse) (bullarc.Position, error) {
	qty, err := strconv.ParseFloat(r.Qty, 64)
	if err != nil {
		return bullarc.Position{}, fmt.Errorf("parse qty: %w", err)
	}
	avgEntry, err := strconv.ParseFloat(r.AvgEntryPrice, 64)
	if err != nil {
		return bullarc.Position{}, fmt.Errorf("parse avg_entry_price: %w", err)
	}
	currentPrice, err := strconv.ParseFloat(r.CurrentPrice, 64)
	if err != nil {
		return bullarc.Position{}, fmt.Errorf("parse current_price: %w", err)
	}
	unrealizedPL, err := strconv.ParseFloat(r.UnrealizedPL, 64)
	if err != nil {
		return bullarc.Position{}, fmt.Errorf("parse unrealized_pl: %w", err)
	}
	unrealizedPLPC, err := strconv.ParseFloat(r.UnrealizedPLPC, 64)
	if err != nil {
		return bullarc.Position{}, fmt.Errorf("parse unrealized_plpc: %w", err)
	}

	return bullarc.Position{
		Symbol:           r.Symbol,
		Qty:              qty,
		AvgEntryPrice:    avgEntry,
		CurrentPrice:     currentPrice,
		UnrealizedPnL:    unrealizedPL,
		UnrealizedPnLPct: unrealizedPLPC,
	}, nil
}

// wrapPaperError converts raw errors from paper trading API calls into bullarc sentinel errors.
func wrapPaperError(err error) error {
	var he *httpStatusError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case http.StatusNotFound:
			return bullarc.ErrSymbolNotFound.Wrap(fmt.Errorf("position not found: %w", he))
		case http.StatusTooManyRequests:
			return bullarc.ErrRateLimitExceeded.Wrap(he)
		case http.StatusUnauthorized, http.StatusForbidden:
			return bullarc.ErrNotConfigured.Wrap(fmt.Errorf("alpaca paper trading credentials invalid: %w", he))
		}
		return bullarc.ErrDataSourceUnavailable.Wrap(he)
	}
	return err
}
