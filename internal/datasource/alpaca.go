package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bullarcdev/bullarc"
)

const alpacaDefaultBaseURL = "https://data.alpaca.markets"

// AlpacaSource fetches real-time and historical OHLCV bars from Alpaca Markets.
// It supports both equity symbols (e.g., "AAPL") and crypto pairs (e.g., "BTC/USD"),
// routing each to the appropriate Alpaca API endpoint automatically.
type AlpacaSource struct {
	keyID     string
	secretKey string
	baseURL   string
	client    *http.Client
	retry     retryConfig
}

// AlpacaOption is a functional option for AlpacaSource.
type AlpacaOption func(*AlpacaSource)

// WithHTTPClient sets a custom HTTP client on the AlpacaSource.
func WithHTTPClient(c *http.Client) AlpacaOption {
	return func(s *AlpacaSource) { s.client = c }
}

// WithBaseURL overrides the Alpaca API base URL (useful for testing).
func WithBaseURL(u string) AlpacaOption {
	return func(s *AlpacaSource) { s.baseURL = u }
}

// WithRetry overrides the retry configuration.
func WithRetry(cfg retryConfig) AlpacaOption {
	return func(s *AlpacaSource) { s.retry = cfg }
}

// NewAlpacaSource creates an AlpacaSource authenticated with the given credentials.
func NewAlpacaSource(keyID, secretKey string, opts ...AlpacaOption) *AlpacaSource {
	s := &AlpacaSource{
		keyID:     keyID,
		secretKey: secretKey,
		baseURL:   alpacaDefaultBaseURL,
		client:    &http.Client{Timeout: 30 * time.Second},
		retry:     defaultRetryConfig,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Meta returns metadata for the Alpaca data source.
func (s *AlpacaSource) Meta() bullarc.DataSourceMeta {
	return bullarc.DataSourceMeta{
		Name:        "alpaca",
		Description: "Fetches real-time and historical OHLCV bars from Alpaca Markets",
	}
}

// isCryptoSymbol reports whether symbol is a crypto trading pair (e.g., "BTC/USD").
func isCryptoSymbol(symbol string) bool {
	return strings.Contains(symbol, "/")
}

type alpacaBar struct {
	Time   time.Time `json:"t"`
	Open   float64   `json:"o"`
	High   float64   `json:"h"`
	Low    float64   `json:"l"`
	Close  float64   `json:"c"`
	Volume float64   `json:"v"`
}

type alpacaBarsResponse struct {
	Bars          []alpacaBar `json:"bars"`
	Symbol        string      `json:"symbol"`
	NextPageToken *string     `json:"next_page_token"`
}

type alpacaCryptoBar struct {
	Time   time.Time `json:"t"`
	Open   float64   `json:"o"`
	High   float64   `json:"h"`
	Low    float64   `json:"l"`
	Close  float64   `json:"c"`
	Volume float64   `json:"v"`
}

type alpacaCryptoBarsResponse struct {
	Bars          map[string][]alpacaCryptoBar `json:"bars"`
	NextPageToken *string                      `json:"next_page_token"`
}

// pageResult holds bars and pagination token from a single page fetch.
type pageResult struct {
	bars          []bullarc.OHLCV
	nextPageToken *string
}

// Fetch retrieves OHLCV bars for query.Symbol from Alpaca, paginating as needed.
// Crypto pairs (e.g., "BTC/USD") are routed to the crypto bars endpoint;
// equity symbols (e.g., "AAPL") use the stocks bars endpoint.
func (s *AlpacaSource) Fetch(ctx context.Context, query bullarc.DataQuery) ([]bullarc.OHLCV, error) {
	slog.Info("fetching bars from alpaca",
		"symbol", query.Symbol,
		"start", query.Start,
		"end", query.End,
		"interval", query.Interval)

	timeframe := intervalToAlpacaTimeframe(query.Interval)

	var all []bullarc.OHLCV
	var pageToken string

	for {
		if err := ctx.Err(); err != nil {
			return nil, bullarc.ErrTimeout.Wrap(err)
		}

		params := url.Values{}
		params.Set("timeframe", timeframe)
		params.Set("limit", "1000")
		if !query.Start.IsZero() {
			params.Set("start", query.Start.UTC().Format(time.RFC3339))
		}
		if !query.End.IsZero() {
			params.Set("end", query.End.UTC().Format(time.RFC3339))
		}
		if pageToken != "" {
			params.Set("page_token", pageToken)
		}

		var result pageResult
		err := withRetry(ctx, s.retry, func() error {
			var ferr error
			result, ferr = s.fetchOnePage(ctx, query.Symbol, params)
			return ferr
		})
		if err != nil {
			return nil, wrapAlpacaError(err, query.Symbol)
		}

		all = append(all, result.bars...)
		if result.nextPageToken == nil || *result.nextPageToken == "" {
			break
		}
		pageToken = *result.nextPageToken
	}

	slog.Info("fetched bars from alpaca", "symbol", query.Symbol, "count", len(all))
	return all, nil
}

// fetchOnePage dispatches to the stocks or crypto endpoint based on the symbol type.
func (s *AlpacaSource) fetchOnePage(ctx context.Context, symbol string, params url.Values) (pageResult, error) {
	if isCryptoSymbol(symbol) {
		var resp alpacaCryptoBarsResponse
		if err := s.fetchCryptoPage(ctx, symbol, params, &resp); err != nil {
			return pageResult{}, err
		}
		bars := make([]bullarc.OHLCV, 0, len(resp.Bars[symbol]))
		for _, b := range resp.Bars[symbol] {
			bars = append(bars, bullarc.OHLCV{
				Time: b.Time, Open: b.Open, High: b.High,
				Low: b.Low, Close: b.Close, Volume: b.Volume,
			})
		}
		return pageResult{bars: bars, nextPageToken: resp.NextPageToken}, nil
	}

	var resp alpacaBarsResponse
	if err := s.fetchPage(ctx, symbol, params, &resp); err != nil {
		return pageResult{}, err
	}
	bars := make([]bullarc.OHLCV, 0, len(resp.Bars))
	for _, b := range resp.Bars {
		bars = append(bars, bullarc.OHLCV{
			Time: b.Time, Open: b.Open, High: b.High,
			Low: b.Low, Close: b.Close, Volume: b.Volume,
		})
	}
	return pageResult{bars: bars, nextPageToken: resp.NextPageToken}, nil
}

// fetchPage fetches one page of bars from the Alpaca stocks endpoint.
func (s *AlpacaSource) fetchPage(ctx context.Context, symbol string, params url.Values, out *alpacaBarsResponse) error {
	endpoint := fmt.Sprintf("%s/v2/stocks/%s/bars?%s",
		s.baseURL, url.PathEscape(symbol), params.Encode())
	return s.doGet(ctx, endpoint, out)
}

// fetchCryptoPage fetches one page of bars from the Alpaca crypto endpoint.
func (s *AlpacaSource) fetchCryptoPage(ctx context.Context, symbol string, params url.Values, out *alpacaCryptoBarsResponse) error {
	p := make(url.Values, len(params)+1)
	for k, v := range params {
		p[k] = v
	}
	p.Set("symbols", symbol)
	endpoint := fmt.Sprintf("%s/v1beta3/crypto/us/bars?%s", s.baseURL, p.Encode())
	return s.doGet(ctx, endpoint, out)
}

// doGet executes an authenticated GET request and decodes the JSON response into out.
// On non-200 status, up to 512 bytes of the response body are read and included
// in the returned httpStatusError for clearer diagnostics.
func (s *AlpacaSource) doGet(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("build request: %w", err))
	}
	req.Header.Set("APCA-API-KEY-ID", s.keyID)
	req.Header.Set("APCA-API-SECRET-KEY", s.secretKey)

	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return bullarc.ErrTimeout.Wrap(err)
		}
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("http request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("decode response: %w", err))
	}
	return nil
}

// wrapAlpacaError converts raw errors from page fetches into bullarc sentinel errors.
func wrapAlpacaError(err error, symbol string) error {
	var he *httpStatusError
	if errors.As(err, &he) {
		if he.StatusCode == http.StatusNotFound {
			return bullarc.ErrSymbolNotFound.Wrap(fmt.Errorf("symbol %s not found", symbol))
		}
		return bullarc.ErrDataSourceUnavailable.Wrap(he)
	}
	return err
}

// intervalToAlpacaTimeframe maps bullarc interval strings to Alpaca timeframe values.
func intervalToAlpacaTimeframe(interval string) string {
	switch interval {
	case "1m":
		return "1Min"
	case "5m":
		return "5Min"
	case "15m":
		return "15Min"
	case "1h":
		return "1Hour"
	default:
		return "1Day"
	}
}
