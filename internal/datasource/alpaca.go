package datasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/bullarcdev/bullarc"
)

const alpacaDefaultBaseURL = "https://data.alpaca.markets"

// AlpacaSource fetches real-time and historical OHLCV bars from Alpaca Markets.
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

// Fetch retrieves OHLCV bars for query.Symbol from Alpaca, paginating as needed.
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

		var resp alpacaBarsResponse
		err := withRetry(ctx, s.retry, func() error {
			return s.fetchPage(ctx, query.Symbol, params, &resp)
		})
		if err != nil {
			return nil, wrapAlpacaError(err, query.Symbol)
		}

		for _, b := range resp.Bars {
			all = append(all, bullarc.OHLCV{
				Time:   b.Time,
				Open:   b.Open,
				High:   b.High,
				Low:    b.Low,
				Close:  b.Close,
				Volume: b.Volume,
			})
		}

		if resp.NextPageToken == nil || *resp.NextPageToken == "" {
			break
		}
		pageToken = *resp.NextPageToken
	}

	slog.Info("fetched bars from alpaca", "symbol", query.Symbol, "count", len(all))
	return all, nil
}

func (s *AlpacaSource) fetchPage(ctx context.Context, symbol string, params url.Values, out *alpacaBarsResponse) error {
	endpoint := fmt.Sprintf("%s/v2/stocks/%s/bars?%s",
		s.baseURL, url.PathEscape(symbol), params.Encode())

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
		return &httpStatusError{StatusCode: resp.StatusCode}
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("decode response: %w", err))
	}
	return nil
}

// wrapAlpacaError converts raw errors from fetchPage into bullarc sentinel errors.
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
