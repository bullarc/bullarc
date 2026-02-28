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

	"github.com/bullarc/bullarc"
)

const massiveDefaultBaseURL = "https://api.massive.com"

// MassiveSource fetches historical OHLCV bars from the Massive market data API.
type MassiveSource struct {
	apiKey  string
	baseURL string
	client  *http.Client
	retry   retryConfig
}

// MassiveOption is a functional option for MassiveSource.
type MassiveOption func(*MassiveSource)

// WithMassiveHTTPClient sets a custom HTTP client on the MassiveSource.
func WithMassiveHTTPClient(c *http.Client) MassiveOption {
	return func(s *MassiveSource) { s.client = c }
}

// WithMassiveBaseURL overrides the Massive API base URL (useful for testing).
func WithMassiveBaseURL(u string) MassiveOption {
	return func(s *MassiveSource) { s.baseURL = u }
}

// WithMassiveRetry overrides the retry configuration.
func WithMassiveRetry(cfg retryConfig) MassiveOption {
	return func(s *MassiveSource) { s.retry = cfg }
}

// NewMassiveSource creates a MassiveSource authenticated with the given API key.
func NewMassiveSource(apiKey string, opts ...MassiveOption) *MassiveSource {
	s := &MassiveSource{
		apiKey:  apiKey,
		baseURL: massiveDefaultBaseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
		retry:   defaultRetryConfig,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Meta returns metadata for the Massive data source.
func (s *MassiveSource) Meta() bullarc.DataSourceMeta {
	return bullarc.DataSourceMeta{
		Name:        "massive",
		Description: "Fetches historical OHLCV bars from the Massive market data API",
	}
}

type massiveBar struct {
	Timestamp int64   `json:"t"`
	Open      float64 `json:"o"`
	High      float64 `json:"h"`
	Low       float64 `json:"l"`
	Close     float64 `json:"c"`
	Volume    float64 `json:"v"`
}

type massiveAggsResponse struct {
	Status       string       `json:"status"`
	ResultsCount int          `json:"resultsCount"`
	Results      []massiveBar `json:"results"`
	NextURL      string       `json:"next_url"`
}

// Fetch retrieves OHLCV bars for query.Symbol from the Massive API, paginating as needed.
func (s *MassiveSource) Fetch(ctx context.Context, query bullarc.DataQuery) ([]bullarc.OHLCV, error) {
	slog.Info("fetching bars from massive",
		"symbol", query.Symbol,
		"start", query.Start,
		"end", query.End,
		"interval", query.Interval)

	multiplier, timespan := intervalToMassiveParams(query.Interval)

	from := query.Start.Format("2006-01-02")
	to := query.End.Format("2006-01-02")
	if query.Start.IsZero() {
		from = time.Now().AddDate(0, 0, -200).Format("2006-01-02")
	}
	if query.End.IsZero() {
		to = time.Now().Format("2006-01-02")
	}

	endpoint := fmt.Sprintf("%s/v2/aggs/ticker/%s/range/%s/%s/%s/%s?apiKey=%s&adjusted=true&sort=asc&limit=50000",
		s.baseURL,
		url.PathEscape(query.Symbol),
		multiplier, timespan, from, to,
		url.QueryEscape(s.apiKey))

	var all []bullarc.OHLCV

	for {
		if err := ctx.Err(); err != nil {
			return nil, bullarc.ErrTimeout.Wrap(err)
		}

		var resp massiveAggsResponse
		err := withRetry(ctx, s.retry, func() error {
			var ferr error
			resp, ferr = s.fetchPage(ctx, endpoint)
			return ferr
		})
		if err != nil {
			return nil, wrapMassiveError(err, query.Symbol)
		}

		for _, b := range resp.Results {
			all = append(all, bullarc.OHLCV{
				Time:   time.UnixMilli(b.Timestamp).UTC(),
				Open:   b.Open,
				High:   b.High,
				Low:    b.Low,
				Close:  b.Close,
				Volume: b.Volume,
			})
		}

		if resp.NextURL == "" {
			break
		}
		endpoint = s.appendAPIKey(resp.NextURL)
	}

	slog.Info("fetched bars from massive", "symbol", query.Symbol, "count", len(all))
	return all, nil
}

// fetchPage fetches a single page of aggregates from the given endpoint URL.
func (s *MassiveSource) fetchPage(ctx context.Context, endpoint string) (massiveAggsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return massiveAggsResponse{}, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("build request: %w", err))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return massiveAggsResponse{}, bullarc.ErrTimeout.Wrap(err)
		}
		return massiveAggsResponse{}, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("http request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return massiveAggsResponse{}, &httpStatusError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	var result massiveAggsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return massiveAggsResponse{}, bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("decode response: %w", err))
	}
	return result, nil
}

// appendAPIKey ensures the apiKey query parameter is present in the URL.
func (s *MassiveSource) appendAPIKey(rawURL string) string {
	if strings.Contains(rawURL, "apiKey=") {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "apiKey=" + url.QueryEscape(s.apiKey)
}

// wrapMassiveError converts raw errors from page fetches into bullarc sentinel errors.
func wrapMassiveError(err error, symbol string) error {
	var he *httpStatusError
	if errors.As(err, &he) {
		if he.StatusCode == http.StatusNotFound {
			return bullarc.ErrSymbolNotFound.Wrap(fmt.Errorf("symbol %s not found", symbol))
		}
		return bullarc.ErrDataSourceUnavailable.Wrap(he)
	}
	return err
}

// intervalToMassiveParams maps bullarc interval strings to Massive multiplier and timespan.
func intervalToMassiveParams(interval string) (multiplier string, timespan string) {
	switch interval {
	case "1m":
		return "1", "minute"
	case "5m":
		return "5", "minute"
	case "15m":
		return "15", "minute"
	case "1h":
		return "1", "hour"
	case "1Day":
		return "1", "day"
	default:
		return "1", "day"
	}
}
