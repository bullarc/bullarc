package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bullarc/bullarc"
)

const (
	whaleAlertDefaultBaseURL  = "https://api.whale-alert.io"
	whaleAlertTransactionsPath = "/v1/transactions"

	// whaleAlertMinUSD is the minimum USD transaction value reported as a whale alert.
	whaleAlertMinUSD = 1_000_000
)

// WhaleAlertTracker polls the Whale Alert API for large on-chain transactions
// (>$1M) involving a given crypto symbol. An API key is required; without one
// all calls return ErrNotConfigured.
type WhaleAlertTracker struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// WhaleAlertOption is a functional option for WhaleAlertTracker.
type WhaleAlertOption func(*WhaleAlertTracker)

// WithWhaleAlertHTTPClient sets a custom HTTP client on the WhaleAlertTracker.
func WithWhaleAlertHTTPClient(c *http.Client) WhaleAlertOption {
	return func(t *WhaleAlertTracker) { t.client = c }
}

// WithWhaleAlertBaseURL overrides the Whale Alert API base URL (useful for testing).
func WithWhaleAlertBaseURL(u string) WhaleAlertOption {
	return func(t *WhaleAlertTracker) { t.baseURL = u }
}

// NewWhaleAlertTracker creates a WhaleAlertTracker authenticated with apiKey.
// Pass an empty apiKey to create a tracker that always returns ErrNotConfigured.
func NewWhaleAlertTracker(apiKey string, opts ...WhaleAlertOption) *WhaleAlertTracker {
	t := &WhaleAlertTracker{
		apiKey:  apiKey,
		baseURL: whaleAlertDefaultBaseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// whaleAlertEntity represents a transaction endpoint (from/to) in the API response.
type whaleAlertEntity struct {
	Address   string `json:"address"`
	Owner     string `json:"owner"`
	OwnerType string `json:"owner_type"`
}

// whaleAlertTransaction is a single transaction record from the Whale Alert API.
type whaleAlertTransaction struct {
	Blockchain string           `json:"blockchain"`
	Symbol     string           `json:"symbol"`
	Hash       string           `json:"hash"`
	From       whaleAlertEntity `json:"from"`
	To         whaleAlertEntity `json:"to"`
	Timestamp  int64            `json:"timestamp"`
	Amount     float64          `json:"amount"`
	AmountUSD  float64          `json:"amount_usd"`
}

// whaleAlertResponse is the top-level response from the Whale Alert API.
type whaleAlertResponse struct {
	Result       string                  `json:"result"`
	Count        int                     `json:"count"`
	Transactions []whaleAlertTransaction `json:"transactions"`
}

// FetchWhaleAlerts returns all transactions for symbol with AmountUSD > $1M
// that occurred at or after since. Symbol is matched case-insensitively against
// the base asset (e.g., "BTC/USD" matches API symbol "btc"). When no large
// transactions exist in the window, an empty (non-nil) slice is returned.
// Returns ErrNotConfigured when the API key is absent.
func (t *WhaleAlertTracker) FetchWhaleAlerts(ctx context.Context, symbol string, since time.Time) ([]bullarc.WhaleTransaction, error) {
	if t.apiKey == "" {
		return nil, bullarc.ErrNotConfigured.Wrap(fmt.Errorf("whale alert api key is required"))
	}

	asset := strings.ToLower(extractCryptoAsset(symbol))

	params := url.Values{}
	params.Set("api_key", t.apiKey)
	params.Set("min_value", fmt.Sprintf("%d", whaleAlertMinUSD))
	params.Set("start", fmt.Sprintf("%d", since.Unix()))

	endpoint := t.baseURL + whaleAlertTransactionsPath + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("whale alert: build request: %w", err))
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("whale alert: request failed: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, bullarc.ErrNotConfigured.Wrap(fmt.Errorf("whale alert: auth error %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, bullarc.ErrRateLimitExceeded.Wrap(fmt.Errorf("whale alert: rate limit exceeded"))
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("whale alert: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	var apiResp whaleAlertResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("whale alert: decode response: %w", err))
	}

	results := make([]bullarc.WhaleTransaction, 0, len(apiResp.Transactions))
	for _, tx := range apiResp.Transactions {
		if strings.ToLower(tx.Symbol) != asset {
			continue
		}
		results = append(results, bullarc.WhaleTransaction{
			Amount:     tx.Amount,
			AmountUSD:  tx.AmountUSD,
			Symbol:     strings.ToUpper(tx.Symbol),
			FromEntity: resolveEntityName(tx.From),
			FromType:   normaliseEntityType(tx.From.OwnerType),
			ToEntity:   resolveEntityName(tx.To),
			ToType:     normaliseEntityType(tx.To.OwnerType),
			TxHash:     tx.Hash,
			Timestamp:  time.Unix(tx.Timestamp, 0).UTC(),
		})
	}

	slog.Info("fetched whale alerts",
		"symbol", symbol,
		"asset", asset,
		"since", since,
		"total_api_txns", len(apiResp.Transactions),
		"matched", len(results))

	return results, nil
}

// resolveEntityName returns the owner name if set, otherwise falls back to
// the owner type (e.g., "unknown" or "wallet").
func resolveEntityName(e whaleAlertEntity) string {
	if e.Owner != "" && e.Owner != "unknown" {
		return e.Owner
	}
	if e.OwnerType != "" {
		return e.OwnerType
	}
	return "unknown"
}

// normaliseEntityType maps raw Whale Alert owner_type values to the canonical
// set used in bullarc: "exchange", "wallet", or "unknown".
func normaliseEntityType(ownerType string) string {
	switch strings.ToLower(ownerType) {
	case "exchange":
		return "exchange"
	case "wallet", "cold_wallet":
		return "wallet"
	default:
		return "unknown"
	}
}
