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
	"strconv"
	"strings"
	"time"

	"github.com/bullarc/bullarc"
)

const alpacaNewsBaseURL = "https://data.alpaca.markets"
const alpacaNewsPath = "/v1beta1/news"
const alpacaNewsPageLimit = 50

// AlpacaNewsSource fetches news articles from the Alpaca Markets News API.
type AlpacaNewsSource struct {
	keyID     string
	secretKey string
	baseURL   string
	client    *http.Client
	retry     retryConfig
}

// AlpacaNewsOption is a functional option for AlpacaNewsSource.
type AlpacaNewsOption func(*AlpacaNewsSource)

// WithNewsHTTPClient sets a custom HTTP client on the AlpacaNewsSource.
func WithNewsHTTPClient(c *http.Client) AlpacaNewsOption {
	return func(s *AlpacaNewsSource) { s.client = c }
}

// WithNewsBaseURL overrides the Alpaca News API base URL (useful for testing).
func WithNewsBaseURL(u string) AlpacaNewsOption {
	return func(s *AlpacaNewsSource) { s.baseURL = u }
}

// WithNewsRetry overrides the retry configuration for the news source.
func WithNewsRetry(cfg retryConfig) AlpacaNewsOption {
	return func(s *AlpacaNewsSource) { s.retry = cfg }
}

// NewAlpacaNewsSource creates an AlpacaNewsSource authenticated with the given credentials.
func NewAlpacaNewsSource(keyID, secretKey string, opts ...AlpacaNewsOption) *AlpacaNewsSource {
	s := &AlpacaNewsSource{
		keyID:     keyID,
		secretKey: secretKey,
		baseURL:   alpacaNewsBaseURL,
		client:    &http.Client{Timeout: 30 * time.Second},
		retry:     defaultRetryConfig,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// alpacaNewsArticle is the raw JSON representation of an article from the Alpaca News API.
type alpacaNewsArticle struct {
	ID        int      `json:"id"`
	Headline  string   `json:"headline"`
	Summary   string   `json:"summary"`
	Source    string   `json:"source"`
	Symbols   []string `json:"symbols"`
	CreatedAt string   `json:"created_at"`
}

// alpacaNewsResponse is the paginated response from the Alpaca News API.
type alpacaNewsResponse struct {
	News          []alpacaNewsArticle `json:"news"`
	NextPageToken *string             `json:"next_page_token"`
}

// FetchNews retrieves news articles for the given symbols published since the given time.
// If symbols is empty all news is fetched. Returns an empty slice (not an error) when
// no articles are found.
func (s *AlpacaNewsSource) FetchNews(ctx context.Context, symbols []string, since time.Time) ([]bullarc.NewsArticle, error) {
	slog.Info("fetching news from alpaca", "symbols", symbols, "since", since)

	var all []bullarc.NewsArticle
	var pageToken string

	for {
		if err := ctx.Err(); err != nil {
			return nil, bullarc.ErrTimeout.Wrap(err)
		}

		params := url.Values{}
		params.Set("limit", strconv.Itoa(alpacaNewsPageLimit))
		params.Set("sort", "desc")
		if len(symbols) > 0 {
			params.Set("symbols", strings.Join(symbols, ","))
		}
		if !since.IsZero() {
			params.Set("start", since.UTC().Format(time.RFC3339))
		}
		if pageToken != "" {
			params.Set("page_token", pageToken)
		}

		var resp alpacaNewsResponse
		err := withRetry(ctx, s.retry, func() error {
			return s.fetchPage(ctx, params, &resp)
		})
		if err != nil {
			return nil, wrapNewsError(err)
		}

		for _, a := range resp.News {
			article, err := convertArticle(a)
			if err != nil {
				slog.Warn("skipping article with invalid timestamp",
					"id", a.ID, "created_at", a.CreatedAt, "err", err)
				continue
			}
			all = append(all, article)
		}

		if resp.NextPageToken == nil || *resp.NextPageToken == "" {
			break
		}
		pageToken = *resp.NextPageToken
	}

	slog.Info("fetched news from alpaca", "symbols", symbols, "count", len(all))
	return all, nil
}

// fetchPage issues a GET request to the Alpaca News API and decodes the response.
func (s *AlpacaNewsSource) fetchPage(ctx context.Context, params url.Values, out *alpacaNewsResponse) error {
	endpoint := fmt.Sprintf("%s%s?%s", s.baseURL, alpacaNewsPath, params.Encode())
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

// convertArticle maps a raw API article to the bullarc.NewsArticle type.
func convertArticle(a alpacaNewsArticle) (bullarc.NewsArticle, error) {
	t, err := time.Parse(time.RFC3339, a.CreatedAt)
	if err != nil {
		return bullarc.NewsArticle{}, fmt.Errorf("parse created_at %q: %w", a.CreatedAt, err)
	}
	symbols := a.Symbols
	if symbols == nil {
		symbols = []string{}
	}
	return bullarc.NewsArticle{
		ID:          strconv.Itoa(a.ID),
		Headline:    a.Headline,
		Summary:     a.Summary,
		Source:      a.Source,
		Symbols:     symbols,
		PublishedAt: t,
	}, nil
}

// wrapNewsError converts raw errors from news page fetches into bullarc sentinel errors.
func wrapNewsError(err error) error {
	var he *httpStatusError
	if errors.As(err, &he) {
		if he.StatusCode == http.StatusTooManyRequests {
			return bullarc.ErrRateLimitExceeded.Wrap(he)
		}
		return bullarc.ErrDataSourceUnavailable.Wrap(he)
	}
	return err
}
