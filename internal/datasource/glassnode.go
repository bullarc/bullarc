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
	glassnodeDefaultBaseURL = "https://api.glassnode.com"
	glassnodeNetFlowPath    = "/v1/metrics/distribution/exchange_net_position_change"
)

// GlassnodeTracker fetches on-chain exchange net flow data from Glassnode.
// It supports crypto pairs (e.g., BTC/USD, ETH/USD) and silently skips
// non-crypto symbols. An API key must be provided; if absent, all calls
// return ErrNotConfigured.
type GlassnodeTracker struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// GlassnodeOption is a functional option for GlassnodeTracker.
type GlassnodeOption func(*GlassnodeTracker)

// WithGlassnodeHTTPClient sets a custom HTTP client on the GlassnodeTracker.
func WithGlassnodeHTTPClient(c *http.Client) GlassnodeOption {
	return func(t *GlassnodeTracker) { t.client = c }
}

// WithGlassnodeBaseURL overrides the Glassnode API base URL (useful for testing).
func WithGlassnodeBaseURL(u string) GlassnodeOption {
	return func(t *GlassnodeTracker) { t.baseURL = u }
}

// NewGlassnodeTracker creates a GlassnodeTracker with the provided API key.
// Pass an empty apiKey to create a tracker that always returns ErrNotConfigured.
func NewGlassnodeTracker(apiKey string, opts ...GlassnodeOption) *GlassnodeTracker {
	t := &GlassnodeTracker{
		apiKey:  apiKey,
		baseURL: glassnodeDefaultBaseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// glassnodeDataPoint is a single time-series entry from the Glassnode API.
type glassnodeDataPoint struct {
	T int64   `json:"t"` // unix timestamp (seconds)
	V float64 `json:"v"` // value
}

// FetchChainMetrics returns on-chain exchange net flow data for each crypto
// symbol in the list. Non-crypto symbols are silently skipped. Returns
// ErrNotConfigured when the API key is absent.
func (t *GlassnodeTracker) FetchChainMetrics(ctx context.Context, symbols []string) ([]bullarc.ChainMetrics, error) {
	if t.apiKey == "" {
		return nil, bullarc.ErrNotConfigured.Wrap(fmt.Errorf("glassnode api key is required"))
	}

	var results []bullarc.ChainMetrics
	for _, sym := range symbols {
		if !isCryptoSymbol(sym) {
			slog.Debug("glassnode: skipping non-crypto symbol", "symbol", sym)
			continue
		}

		asset := extractCryptoAsset(sym)
		m, err := t.fetchForAsset(ctx, sym, asset)
		if err != nil {
			return nil, err
		}
		if m != nil {
			results = append(results, *m)
		}
	}
	return results, nil
}

// fetchForAsset fetches the latest net flow metric for a single asset ticker.
func (t *GlassnodeTracker) fetchForAsset(ctx context.Context, symbol, asset string) (*bullarc.ChainMetrics, error) {
	params := url.Values{}
	params.Set("a", asset)
	params.Set("api_key", t.apiKey)
	params.Set("i", "24h")
	params.Set("limit", "1")

	endpoint := t.baseURL + glassnodeNetFlowPath + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("glassnode: build request: %w", err))
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("glassnode: request failed: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, bullarc.ErrNotConfigured.Wrap(fmt.Errorf("glassnode: auth error %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("glassnode: http %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	var points []glassnodeDataPoint
	if err := json.NewDecoder(resp.Body).Decode(&points); err != nil {
		return nil, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("glassnode: decode response: %w", err))
	}

	if len(points) == 0 {
		slog.Warn("glassnode: empty response", "symbol", symbol, "asset", asset)
		return nil, nil
	}

	latest := points[len(points)-1]
	direction := bullarc.FlowDirectionOutflow
	if latest.V > 0 {
		direction = bullarc.FlowDirectionInflow
	}

	slog.Info("fetched chain metrics from glassnode",
		"symbol", symbol,
		"asset", asset,
		"net_flow", latest.V,
		"direction", direction)

	return &bullarc.ChainMetrics{
		Symbol:        strings.ToUpper(symbol),
		NetFlow:       latest.V,
		FlowDirection: direction,
		Timestamp:     time.Unix(latest.T, 0).UTC(),
	}, nil
}

// extractCryptoAsset returns the base asset ticker from a symbol like "BTC/USD" → "BTC".
// For symbols without a separator, the full symbol is returned as-is.
func extractCryptoAsset(symbol string) string {
	upper := strings.ToUpper(symbol)
	if idx := strings.Index(upper, "/"); idx != -1 {
		return upper[:idx]
	}
	return upper
}
