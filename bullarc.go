package bullarc

import (
	"context"
	"time"
)

// OHLCV represents a single candlestick/bar of market data.
type OHLCV struct {
	Time   time.Time `json:"time"`
	Open   float64   `json:"open"`
	High   float64   `json:"high"`
	Low    float64   `json:"low"`
	Close  float64   `json:"close"`
	Volume float64   `json:"volume"`
}

// Bar is an alias for OHLCV.
type Bar = OHLCV

// SignalType represents the type of trading signal.
type SignalType string

const (
	SignalBuy  SignalType = "BUY"
	SignalSell SignalType = "SELL"
	SignalHold SignalType = "HOLD"
)

// Signal represents a trading signal produced by analysis.
type Signal struct {
	Type        SignalType     `json:"type"`
	Confidence  float64        `json:"confidence"`
	Indicator   string         `json:"indicator"`
	Symbol      string         `json:"symbol"`
	Timestamp   time.Time      `json:"timestamp"`
	Explanation string         `json:"explanation"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	RiskFlags   []string       `json:"risk_flags,omitempty"`
}

// IndicatorMeta describes an indicator's metadata.
type IndicatorMeta struct {
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Category     string         `json:"category"`
	Parameters   map[string]any `json:"parameters"`
	WarmupPeriod int            `json:"warmup_period"`
}

// IndicatorValue is a single computed indicator value at a point in time.
type IndicatorValue struct {
	Time  time.Time          `json:"time"`
	Value float64            `json:"value"`
	Extra map[string]float64 `json:"extra,omitempty"`
}

// Indicator computes technical indicator values from OHLCV bars.
type Indicator interface {
	Meta() IndicatorMeta
	Compute(bars []OHLCV) ([]IndicatorValue, error)
}

// DataSourceMeta describes a data source.
type DataSourceMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DataQuery specifies parameters for fetching market data.
type DataQuery struct {
	Symbol   string    `json:"symbol"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Interval string    `json:"interval"`
}

// DataSource fetches market data from an external provider.
type DataSource interface {
	Meta() DataSourceMeta
	Fetch(ctx context.Context, query DataQuery) ([]OHLCV, error)
}

// LLMRequest represents a request to an LLM provider.
type LLMRequest struct {
	Prompt      string  `json:"prompt"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// LLMResponse represents a response from an LLM provider.
type LLMResponse struct {
	Text       string `json:"text"`
	TokensUsed int    `json:"tokens_used"`
	Model      string `json:"model"`
}

// LLMProvider integrates with a large language model for analysis.
type LLMProvider interface {
	Name() string
	Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)
}

// AnalysisRequest specifies what to analyze.
type AnalysisRequest struct {
	Symbol     string   `json:"symbol"`
	Indicators []string `json:"indicators,omitempty"`
	UseLLM     bool     `json:"use_llm"`
}

// AnomalySeverity indicates the significance level of a detected anomaly.
type AnomalySeverity string

const (
	AnomalySeverityLow    AnomalySeverity = "low"
	AnomalySeverityMedium AnomalySeverity = "medium"
	AnomalySeverityHigh   AnomalySeverity = "high"
)

// Anomaly represents a detected divergence or unusual pattern in market data.
type Anomaly struct {
	Type               string          `json:"type"`
	Description        string          `json:"description"`
	Severity           AnomalySeverity `json:"severity"`
	AffectedIndicators []string        `json:"affected_indicators"`
}

// AnalysisResult contains the full result of an analysis run.
type AnalysisResult struct {
	Symbol          string                      `json:"symbol"`
	Signals         []Signal                    `json:"signals"`
	IndicatorValues map[string][]IndicatorValue `json:"indicator_values,omitempty"`
	LLMAnalysis     string                      `json:"llm_analysis,omitempty"`
	Anomalies       []Anomaly                   `json:"anomalies,omitempty"`
	Timestamp       time.Time                   `json:"timestamp"`
}

// Engine orchestrates analysis using indicators, data sources, and LLM providers.
type Engine interface {
	Analyze(ctx context.Context, req AnalysisRequest) (AnalysisResult, error)
	RegisterIndicator(ind Indicator)
	RegisterDataSource(ds DataSource)
	RegisterLLMProvider(llm LLMProvider)
}

// NewsArticle represents a single news article from a news data source.
type NewsArticle struct {
	ID          string    `json:"id"`
	Headline    string    `json:"headline"`
	Summary     string    `json:"summary"`
	Source      string    `json:"source"`
	Symbols     []string  `json:"symbols"`
	PublishedAt time.Time `json:"published_at"`
}

// NewsSource fetches news articles for given symbols.
type NewsSource interface {
	FetchNews(ctx context.Context, symbols []string, since time.Time) ([]NewsArticle, error)
}

// SocialMetrics contains Reddit mention data for a tracked symbol.
type SocialMetrics struct {
	Symbol     string    `json:"symbol"`
	Mentions   int       `json:"mentions"`
	Sentiment  float64   `json:"sentiment"`
	Rank       int       `json:"rank"`
	Velocity   float64   `json:"velocity"`
	IsElevated bool      `json:"is_elevated"`
	Timestamp  time.Time `json:"timestamp"`
}

// SocialTracker fetches Reddit mention metrics for one or more symbols.
type SocialTracker interface {
	FetchSocialMetrics(ctx context.Context, symbols []string) ([]SocialMetrics, error)
}

// FlowDirection indicates the direction of exchange net flow.
type FlowDirection string

const (
	// FlowDirectionInflow means coins moved into exchanges (bearish pressure).
	FlowDirectionInflow FlowDirection = "inflow"
	// FlowDirectionOutflow means coins moved out of exchanges (accumulation/bullish).
	FlowDirectionOutflow FlowDirection = "outflow"
)

// ChainMetrics contains on-chain exchange flow data for a crypto asset.
type ChainMetrics struct {
	Symbol        string        `json:"symbol"`
	NetFlow       float64       `json:"net_flow"`
	FlowDirection FlowDirection `json:"flow_direction"`
	Timestamp     time.Time     `json:"timestamp"`
}

// ChainFlowSource fetches on-chain exchange flow metrics for crypto assets.
type ChainFlowSource interface {
	FetchChainMetrics(ctx context.Context, symbols []string) ([]ChainMetrics, error)
}

// Sentinel errors.
var (
	ErrInsufficientData      = &Error{Code: "INSUFFICIENT_DATA", Message: "not enough data bars for computation"}
	ErrInvalidParameter      = &Error{Code: "INVALID_PARAMETER", Message: "invalid indicator parameter"}
	ErrDataSourceUnavailable = &Error{Code: "DATA_SOURCE_UNAVAILABLE", Message: "data source is unavailable"}
	ErrLLMUnavailable        = &Error{Code: "LLM_UNAVAILABLE", Message: "LLM provider is unavailable"}
	ErrSymbolNotFound        = &Error{Code: "SYMBOL_NOT_FOUND", Message: "symbol not found"}
	ErrTimeout               = &Error{Code: "TIMEOUT", Message: "operation timed out"}
	ErrRateLimitExceeded     = &Error{Code: "RATE_LIMIT_EXCEEDED", Message: "API rate limit exceeded"}
	ErrNotConfigured         = &Error{Code: "NOT_CONFIGURED", Message: "provider is not configured"}
)

// Error is a structured error with a code and message.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Code + ": " + e.Message + ": " + e.Err.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Wrap(err error) *Error {
	return &Error{Code: e.Code, Message: e.Message, Err: err}
}
