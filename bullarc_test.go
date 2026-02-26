package bullarc_test

import (
	"errors"
	"testing"
	"time"

	"github.com/bullarcdev/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmoke_OHLCVFields(t *testing.T) {
	bar := bullarc.OHLCV{
		Time:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Open:   100.0,
		High:   110.0,
		Low:    95.0,
		Close:  105.0,
		Volume: 1000.0,
	}

	assert.Equal(t, 100.0, bar.Open)
	assert.Equal(t, 110.0, bar.High)
	assert.Equal(t, 95.0, bar.Low)
	assert.Equal(t, 105.0, bar.Close)
	assert.Equal(t, 1000.0, bar.Volume)
}

func TestSmoke_BarAlias(t *testing.T) {
	var bar bullarc.Bar
	bar.Close = 42.0
	assert.Equal(t, 42.0, bar.Close)
}

func TestSmoke_SignalTypes(t *testing.T) {
	assert.Equal(t, bullarc.SignalType("BUY"), bullarc.SignalBuy)
	assert.Equal(t, bullarc.SignalType("SELL"), bullarc.SignalSell)
	assert.Equal(t, bullarc.SignalType("HOLD"), bullarc.SignalHold)
}

func TestSmoke_Signal(t *testing.T) {
	sig := bullarc.Signal{
		Type:       bullarc.SignalBuy,
		Confidence: 0.85,
		Indicator:  "RSI",
		Symbol:     "AAPL",
		Timestamp:  time.Now(),
	}

	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.Equal(t, 0.85, sig.Confidence)
	assert.Equal(t, "RSI", sig.Indicator)
}

func TestSmoke_ErrorString(t *testing.T) {
	err := bullarc.ErrInsufficientData
	assert.Equal(t, "INSUFFICIENT_DATA: not enough data bars for computation", err.Error())
}

func TestSmoke_ErrorWrap(t *testing.T) {
	inner := errors.New("connection refused")
	wrapped := bullarc.ErrDataSourceUnavailable.Wrap(inner)

	require.NotNil(t, wrapped)
	assert.Contains(t, wrapped.Error(), "connection refused")
	assert.Contains(t, wrapped.Error(), "DATA_SOURCE_UNAVAILABLE")
	assert.ErrorIs(t, wrapped, inner)
}

func TestSmoke_ErrorUnwrapNil(t *testing.T) {
	err := &bullarc.Error{Code: "TEST", Message: "test error"}
	assert.Nil(t, err.Unwrap())
}

func TestSmoke_AnalysisRequest(t *testing.T) {
	req := bullarc.AnalysisRequest{
		Symbol:     "TSLA",
		Indicators: []string{"SMA", "RSI"},
		UseLLM:     true,
	}

	assert.Equal(t, "TSLA", req.Symbol)
	assert.Len(t, req.Indicators, 2)
	assert.True(t, req.UseLLM)
}

func TestSmoke_DataQuery(t *testing.T) {
	q := bullarc.DataQuery{
		Symbol:   "MSFT",
		Start:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		End:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		Interval: "1d",
	}

	assert.Equal(t, "MSFT", q.Symbol)
	assert.Equal(t, "1d", q.Interval)
}

func TestSmoke_LLMRequest(t *testing.T) {
	req := bullarc.LLMRequest{
		Prompt:      "Analyze AAPL",
		MaxTokens:   500,
		Temperature: 0.7,
	}

	assert.Equal(t, "Analyze AAPL", req.Prompt)
	assert.Equal(t, 500, req.MaxTokens)
}

func TestSmoke_AllSentinelErrors(t *testing.T) {
	errs := []*bullarc.Error{
		bullarc.ErrInsufficientData,
		bullarc.ErrInvalidParameter,
		bullarc.ErrDataSourceUnavailable,
		bullarc.ErrLLMUnavailable,
		bullarc.ErrSymbolNotFound,
		bullarc.ErrTimeout,
	}

	for _, e := range errs {
		assert.NotEmpty(t, e.Code)
		assert.NotEmpty(t, e.Message)
		assert.NotEmpty(t, e.Error())
	}
}
