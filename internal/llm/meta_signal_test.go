package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeCompositeSignal(sigType bullarc.SignalType, confidence float64) bullarc.Signal {
	return bullarc.Signal{
		Type:        sigType,
		Confidence:  confidence,
		Indicator:   "composite",
		Symbol:      "AAPL",
		Timestamp:   time.Now(),
		Explanation: "3 buy, 1 sell, 0 hold signals",
	}
}

func makeIndicatorValues() map[string][]bullarc.IndicatorValue {
	now := time.Now()
	return map[string][]bullarc.IndicatorValue{
		"RSI_14": {
			{Time: now, Value: 65.5},
		},
		"MACD_12_26_9": {
			{Time: now, Value: 0.23, Extra: map[string]float64{"signal": 0.15, "histogram": 0.08}},
		},
	}
}

// TestMetaSignalPrompt_ContainsRequiredFields verifies the prompt includes symbol,
// price, indicator values, and composite signal fields.
func TestMetaSignalPrompt_ContainsRequiredFields(t *testing.T) {
	composite := makeCompositeSignal(bullarc.SignalBuy, 75.0)
	indVals := makeIndicatorValues()

	prompt := MetaSignalPrompt("AAPL", indVals, composite, 150.25)

	assert.Contains(t, prompt, "AAPL")
	assert.Contains(t, prompt, "150.2500")
	assert.Contains(t, prompt, "RSI_14")
	assert.Contains(t, prompt, "MACD_12_26_9")
	assert.Contains(t, prompt, "BUY")
	assert.Contains(t, prompt, "75%")
	assert.Contains(t, prompt, "BUY|SELL|HOLD")
}

// TestMetaSignalPrompt_IncludesExtraFields verifies that indicator Extra fields
// (e.g. MACD signal line) are included in the prompt.
func TestMetaSignalPrompt_IncludesExtraFields(t *testing.T) {
	indVals := map[string][]bullarc.IndicatorValue{
		"MACD_12_26_9": {
			{Time: time.Now(), Value: 0.5, Extra: map[string]float64{"signal": 0.3, "histogram": 0.2}},
		},
	}
	composite := makeCompositeSignal(bullarc.SignalHold, 50.0)

	prompt := MetaSignalPrompt("TSLA", indVals, composite, 200.0)

	assert.Contains(t, prompt, "MACD_12_26_9")
	assert.Contains(t, prompt, "signal=")
	assert.Contains(t, prompt, "histogram=")
}

// TestMetaSignalPrompt_EmptyIndicatorValues handles the case of no indicator values gracefully.
func TestMetaSignalPrompt_EmptyIndicatorValues(t *testing.T) {
	composite := makeCompositeSignal(bullarc.SignalHold, 50.0)

	prompt := MetaSignalPrompt("MSFT", nil, composite, 300.0)

	assert.Contains(t, prompt, "MSFT")
	assert.Contains(t, prompt, "300.0000")
	assert.True(t, len(prompt) > 0)
}

// TestParseMetaSignalResponse_BuySignal verifies parsing a valid BUY response.
func TestParseMetaSignalResponse_BuySignal(t *testing.T) {
	text := `{"signal":"BUY","confidence":82,"reasoning":"Strong uptrend with RSI below overbought. MACD histogram expanding positively."}`

	sig, ok := parseMetaSignalResponse("AAPL", text)

	require.True(t, ok)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.Equal(t, 82.0, sig.Confidence)
	assert.Equal(t, "LLMMetaSignal", sig.Indicator)
	assert.Equal(t, "AAPL", sig.Symbol)
	assert.Contains(t, sig.Explanation, "Strong uptrend")
}

// TestParseMetaSignalResponse_SellSignal verifies parsing a valid SELL response.
func TestParseMetaSignalResponse_SellSignal(t *testing.T) {
	text := `{"signal":"SELL","confidence":70,"reasoning":"Bearish divergence detected."}`

	sig, ok := parseMetaSignalResponse("TSLA", text)

	require.True(t, ok)
	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.Equal(t, 70.0, sig.Confidence)
}

// TestParseMetaSignalResponse_HoldSignal verifies parsing a valid HOLD response.
func TestParseMetaSignalResponse_HoldSignal(t *testing.T) {
	text := `{"signal":"HOLD","confidence":55,"reasoning":"Mixed signals, no clear direction."}`

	sig, ok := parseMetaSignalResponse("MSFT", text)

	require.True(t, ok)
	assert.Equal(t, bullarc.SignalHold, sig.Type)
	assert.Equal(t, 55.0, sig.Confidence)
}

// TestParseMetaSignalResponse_WithPreamble verifies that JSON embedded in preamble is extracted.
func TestParseMetaSignalResponse_WithPreamble(t *testing.T) {
	text := `Based on my analysis: {"signal":"BUY","confidence":90,"reasoning":"Bullish."} I hope this helps.`

	sig, ok := parseMetaSignalResponse("AAPL", text)

	require.True(t, ok)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.Equal(t, 90.0, sig.Confidence)
}

// TestParseMetaSignalResponse_InvalidJSON returns false on malformed JSON.
func TestParseMetaSignalResponse_InvalidJSON(t *testing.T) {
	sig, ok := parseMetaSignalResponse("AAPL", "not json at all")

	assert.False(t, ok)
	assert.Equal(t, bullarc.Signal{}, sig)
}

// TestParseMetaSignalResponse_InvalidSignalType returns false on an unrecognised signal.
func TestParseMetaSignalResponse_InvalidSignalType(t *testing.T) {
	text := `{"signal":"MAYBE","confidence":60,"reasoning":"Unsure."}`

	sig, ok := parseMetaSignalResponse("AAPL", text)

	assert.False(t, ok)
	assert.Equal(t, bullarc.Signal{}, sig)
}

// TestParseMetaSignalResponse_ConfidenceClamped_High verifies confidence > 100 is clamped.
func TestParseMetaSignalResponse_ConfidenceClamped_High(t *testing.T) {
	text := `{"signal":"BUY","confidence":150,"reasoning":"Very sure."}`

	sig, ok := parseMetaSignalResponse("AAPL", text)

	require.True(t, ok)
	assert.Equal(t, 100.0, sig.Confidence)
}

// TestParseMetaSignalResponse_ConfidenceClamped_Low verifies confidence < 0 is clamped.
func TestParseMetaSignalResponse_ConfidenceClamped_Low(t *testing.T) {
	text := `{"signal":"SELL","confidence":-10,"reasoning":"Negative."}`

	sig, ok := parseMetaSignalResponse("AAPL", text)

	require.True(t, ok)
	assert.Equal(t, 0.0, sig.Confidence)
}

// TestGenerateMetaSignal_Success verifies the full call returns a valid signal.
func TestGenerateMetaSignal_Success(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"signal":"BUY","confidence":78,"reasoning":"All indicators align bullishly."}`},
		},
	}
	composite := makeCompositeSignal(bullarc.SignalBuy, 72.0)

	sig, ok := GenerateMetaSignal(context.Background(), "AAPL", makeIndicatorValues(), composite, 155.0, provider)

	require.True(t, ok)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.Equal(t, 78.0, sig.Confidence)
	assert.Equal(t, "LLMMetaSignal", sig.Indicator)
	assert.Equal(t, "AAPL", sig.Symbol)
	assert.Equal(t, 1, provider.calls)
}

// TestGenerateMetaSignal_LLMError returns (zero, false) on provider error.
func TestGenerateMetaSignal_LLMError(t *testing.T) {
	provider := &stubLLMProvider{
		errs: []error{errors.New("timeout")},
	}
	composite := makeCompositeSignal(bullarc.SignalBuy, 70.0)

	sig, ok := GenerateMetaSignal(context.Background(), "AAPL", makeIndicatorValues(), composite, 155.0, provider)

	assert.False(t, ok)
	assert.Equal(t, bullarc.Signal{}, sig)
}

// TestGenerateMetaSignal_InvalidResponse returns (zero, false) on invalid LLM response.
func TestGenerateMetaSignal_InvalidResponse(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `not valid json`},
		},
	}
	composite := makeCompositeSignal(bullarc.SignalHold, 50.0)

	sig, ok := GenerateMetaSignal(context.Background(), "AAPL", makeIndicatorValues(), composite, 155.0, provider)

	assert.False(t, ok)
	assert.Equal(t, bullarc.Signal{}, sig)
}

// TestMetaSignalPrompt_SkipsEmptyValues verifies that indicators with no values
// are omitted from the prompt gracefully.
func TestMetaSignalPrompt_SkipsEmptyValues(t *testing.T) {
	indVals := map[string][]bullarc.IndicatorValue{
		"RSI_14":    {{Time: time.Now(), Value: 55.0}},
		"EMPTY_IND": {}, // should be skipped
	}
	composite := makeCompositeSignal(bullarc.SignalBuy, 60.0)

	prompt := MetaSignalPrompt("AAPL", indVals, composite, 100.0)

	assert.Contains(t, prompt, "RSI_14")
	// EMPTY_IND has no values so it should not appear with a numeric value
	assert.False(t, strings.Contains(prompt, "EMPTY_IND: "), "indicator with no values should be skipped")
}
