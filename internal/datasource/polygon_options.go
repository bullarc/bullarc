package datasource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bullarc/bullarc"
)

const (
	polygonOptionsDefaultBaseURL = "https://api.polygon.io"
	polygonOptionsSnapshotPath   = "/v3/snapshot/options"

	// defaultPremiumThreshold is the fallback premium threshold (USD) when OptionsConfig.PremiumThreshold is zero.
	defaultPremiumThreshold = 100_000.0
	// defaultVolumeOIMultiple is the volume/open-interest multiplier that triggers unusual-volume detection.
	defaultVolumeOIMultiple = 3.0
	// defaultPCRatioStdDevs is the number of standard deviations from the mean PC ratio that triggers anomaly detection.
	defaultPCRatioStdDevs = 1.5
	// polygonOptionsPageLimit is the maximum number of contracts per API page.
	polygonOptionsPageLimit = 250
)

// PolygonOptionsSource fetches options chain data from Polygon.io and detects
// unusual activity events for equity symbols.
type PolygonOptionsSource struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// PolygonOptionsOption is a functional option for PolygonOptionsSource.
type PolygonOptionsOption func(*PolygonOptionsSource)

// WithPolygonOptionsHTTPClient sets a custom HTTP client on the PolygonOptionsSource.
func WithPolygonOptionsHTTPClient(c *http.Client) PolygonOptionsOption {
	return func(s *PolygonOptionsSource) { s.client = c }
}

// WithPolygonOptionsBaseURL overrides the Polygon.io API base URL (useful for testing).
func WithPolygonOptionsBaseURL(u string) PolygonOptionsOption {
	return func(s *PolygonOptionsSource) { s.baseURL = u }
}

// NewPolygonOptionsSource creates a PolygonOptionsSource authenticated with the given API key.
// Pass an empty apiKey to create a source that always returns ErrNotConfigured.
func NewPolygonOptionsSource(apiKey string, opts ...PolygonOptionsOption) *PolygonOptionsSource {
	s := &PolygonOptionsSource{
		apiKey:  apiKey,
		baseURL: polygonOptionsDefaultBaseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// polygonOptionsDetails holds per-contract static details from the snapshot response.
type polygonOptionsDetails struct {
	ContractType   string  `json:"contract_type"`
	ExpirationDate string  `json:"expiration_date"`
	StrikePrice    float64 `json:"strike_price"`
	Ticker         string  `json:"ticker"`
}

// polygonOptionsDayStats holds intraday statistics for a contract.
type polygonOptionsDayStats struct {
	Volume float64 `json:"volume"`
}

// polygonOptionsLastQuote holds the latest quote data for a contract.
type polygonOptionsLastQuote struct {
	Ask      float64 `json:"ask"`
	Bid      float64 `json:"bid"`
	Midpoint float64 `json:"midpoint"`
}

// polygonOptionsContract is a single options contract record from the Polygon.io snapshot API.
type polygonOptionsContract struct {
	Details      polygonOptionsDetails   `json:"details"`
	Day          polygonOptionsDayStats  `json:"day"`
	LastQuote    polygonOptionsLastQuote `json:"last_quote"`
	OpenInterest float64                 `json:"open_interest"`
}

// polygonOptionsSnapshotResponse is the top-level response from the Polygon.io options snapshot API.
type polygonOptionsSnapshotResponse struct {
	Results   []polygonOptionsContract `json:"results"`
	Status    string                   `json:"status"`
	NextURL   string                   `json:"next_url"`
	RequestID string                   `json:"request_id"`
}

// FetchOptionsActivity fetches the options chain for symbol from Polygon.io and returns
// contracts that exhibit unusual activity according to the following rules:
//
//   - Volume > 3× open interest → unusual_volume
//   - Total premium > cfg.PremiumThreshold (default $100K) → block; if also unusual_volume → sweep
//   - Aggregate put/call ratio deviates >1.5 standard deviations from the mean of
//     cfg.HistoricalPCRatios → dominant-direction contracts with volume > open interest
//     are also included (classified as unusual_volume when not already flagged)
//
// Crypto symbols (containing "/") are silently skipped and nil is returned.
// Returns ErrNotConfigured when the API key is absent.
func (s *PolygonOptionsSource) FetchOptionsActivity(ctx context.Context, symbol string, cfg bullarc.OptionsConfig) ([]bullarc.OptionsActivity, error) {
	if s.apiKey == "" {
		return nil, bullarc.ErrNotConfigured.Wrap(fmt.Errorf("polygon options: api key is required"))
	}

	if isCryptoSymbol(symbol) {
		slog.Debug("polygon options: skipping crypto symbol", "symbol", symbol)
		return nil, nil
	}

	threshold := cfg.PremiumThreshold
	if threshold <= 0 {
		threshold = defaultPremiumThreshold
	}

	contracts, err := s.fetchAllContracts(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if len(contracts) == 0 {
		return nil, nil
	}

	// Aggregate put/call volumes to check for PC ratio anomaly.
	var putVolume, callVolume float64
	for _, c := range contracts {
		switch strings.ToLower(c.Details.ContractType) {
		case "put":
			putVolume += c.Day.Volume
		case "call":
			callVolume += c.Day.Volume
		}
	}

	pcAnomalous, dominantDirection := pcRatioAnomaly(putVolume, callVolume, cfg.HistoricalPCRatios)

	var activities []bullarc.OptionsActivity
	for _, c := range contracts {
		if c.Day.Volume <= 0 {
			continue
		}

		direction := strings.ToLower(c.Details.ContractType)
		if direction != "call" && direction != "put" {
			continue
		}

		price := contractPrice(c.LastQuote)
		premium := price * c.Day.Volume * 100 // one contract = 100 shares

		vol := int64(c.Day.Volume)
		oi := int64(c.OpenInterest)

		unusualVol := oi > 0 && float64(vol) > defaultVolumeOIMultiple*float64(oi)
		isBlock := premium >= threshold
		// PC-ratio anomaly: flag dominant-direction contracts with volume >= OI.
		pcDriven := pcAnomalous && direction == dominantDirection && oi > 0 && float64(vol) >= float64(oi)

		if !unusualVol && !isBlock && !pcDriven {
			continue
		}

		exp, err := time.Parse("2006-01-02", c.Details.ExpirationDate)
		if err != nil {
			slog.Warn("polygon options: skipping contract with unparseable expiration",
				"ticker", c.Details.Ticker,
				"expiration_date", c.Details.ExpirationDate)
			continue
		}

		activities = append(activities, bullarc.OptionsActivity{
			Symbol:       strings.ToUpper(symbol),
			Strike:       c.Details.StrikePrice,
			Expiration:   exp,
			Direction:    direction,
			Volume:       vol,
			OpenInterest: oi,
			Premium:      premium,
			ActivityType: classifyOptionsActivity(isBlock, unusualVol),
		})
	}

	slog.Info("polygon options: completed unusual activity scan",
		"symbol", symbol,
		"contracts_fetched", len(contracts),
		"unusual_count", len(activities),
		"put_volume", putVolume,
		"call_volume", callVolume,
		"pc_anomalous", pcAnomalous)

	return activities, nil
}

// fetchAllContracts paginates through the Polygon.io options snapshot endpoint
// and returns all contract records for the given underlying symbol.
func (s *PolygonOptionsSource) fetchAllContracts(ctx context.Context, symbol string) ([]polygonOptionsContract, error) {
	params := url.Values{}
	params.Set("apiKey", s.apiKey)
	params.Set("limit", fmt.Sprintf("%d", polygonOptionsPageLimit))

	endpoint := fmt.Sprintf("%s%s/%s?%s",
		s.baseURL, polygonOptionsSnapshotPath, url.PathEscape(symbol), params.Encode())

	var all []polygonOptionsContract
	for endpoint != "" {
		if err := ctx.Err(); err != nil {
			return nil, bullarc.ErrTimeout.Wrap(err)
		}

		var resp polygonOptionsSnapshotResponse
		if err := s.doGet(ctx, endpoint, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Results...)

		if resp.NextURL == "" {
			break
		}
		endpoint = appendAPIKey(resp.NextURL, s.apiKey)
	}
	return all, nil
}

// doGet executes a GET request and JSON-decodes the response into out.
func (s *PolygonOptionsSource) doGet(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("polygon options: build request: %w", err))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("polygon options: http request: %w", err))
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through to decode
	case http.StatusUnauthorized, http.StatusForbidden:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return bullarc.ErrNotConfigured.Wrap(fmt.Errorf("polygon options: auth error %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body))))
	case http.StatusTooManyRequests:
		return bullarc.ErrRateLimitExceeded.Wrap(fmt.Errorf("polygon options: rate limit exceeded"))
	case http.StatusNotFound:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return bullarc.ErrSymbolNotFound.Wrap(fmt.Errorf("polygon options: symbol not found: %s",
			strings.TrimSpace(string(body))))
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("polygon options: http %d: %s",
			resp.StatusCode, strings.TrimSpace(string(body))))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return bullarc.ErrDataSourceUnavailable.Wrap(fmt.Errorf("polygon options: decode response: %w", err))
	}
	return nil
}

// classifyOptionsActivity returns the OptionsActivityType based on detection flags.
// When both premium and volume criteria are met the event is classified as a sweep
// (aggressive institutional order with high urgency).
func classifyOptionsActivity(isBlock, unusualVol bool) bullarc.OptionsActivityType {
	if isBlock && unusualVol {
		return bullarc.OptionsActivitySweep
	}
	if isBlock {
		return bullarc.OptionsActivityBlock
	}
	return bullarc.OptionsActivityUnusualVolume
}

// contractPrice returns the best available mid-price for a contract.
// Uses the explicit midpoint if non-zero, otherwise averages ask and bid.
func contractPrice(q polygonOptionsLastQuote) float64 {
	if q.Midpoint > 0 {
		return q.Midpoint
	}
	if q.Ask > 0 && q.Bid > 0 {
		return (q.Ask + q.Bid) / 2
	}
	if q.Ask > 0 {
		return q.Ask
	}
	return q.Bid
}

// pcRatioAnomaly reports whether the current put/call ratio (derived from putVol / callVol)
// deviates more than defaultPCRatioStdDevs standard deviations from the mean of historical.
// It also returns the direction ("put" or "call") that is dominant in the current ratio.
// Returns (false, "") when fewer than 2 historical values are available.
func pcRatioAnomaly(putVol, callVol float64, historical []float64) (anomalous bool, dominant string) {
	if callVol <= 0 || len(historical) < 2 {
		return false, ""
	}

	current := putVol / callVol

	var mean float64
	for _, r := range historical {
		mean += r
	}
	mean /= float64(len(historical))

	var variance float64
	for _, r := range historical {
		d := r - mean
		variance += d * d
	}
	variance /= float64(len(historical))
	stdDev := math.Sqrt(variance)

	// When stdDev == 0 every historical value equals the mean;
	// any deviation from that exact mean is treated as anomalous.
	if stdDev == 0 {
		if current == mean {
			return false, ""
		}
	} else if math.Abs(current-mean) <= defaultPCRatioStdDevs*stdDev {
		return false, ""
	}

	if current > mean {
		return true, "put"
	}
	return true, "call"
}

// appendAPIKey appends the apiKey query parameter to nextURL if not already present.
func appendAPIKey(nextURL, apiKey string) string {
	if strings.Contains(nextURL, "apiKey=") {
		return nextURL
	}
	if strings.Contains(nextURL, "?") {
		return nextURL + "&apiKey=" + apiKey
	}
	return nextURL + "?apiKey=" + apiKey
}
