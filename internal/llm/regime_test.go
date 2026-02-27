package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegimeDetectionPrompt_ContainsRequiredFields verifies the prompt includes
// symbol, all three metrics, and the four regime definitions.
func TestRegimeDetectionPrompt_ContainsRequiredFields(t *testing.T) {
	prompt := RegimeDetectionPrompt("AAPL", 12.5, 0.0423, 8.3)

	assert.Contains(t, prompt, "AAPL")
	assert.Contains(t, prompt, "12.50%")
	assert.Contains(t, prompt, "0.0423")
	assert.Contains(t, prompt, "8.30%")
	assert.Contains(t, prompt, "low_vol_trending")
	assert.Contains(t, prompt, "high_vol_trending")
	assert.Contains(t, prompt, "mean_reverting")
	assert.Contains(t, prompt, "crisis")
}

// TestRegimeDetectionPrompt_NegativeATRTrend verifies negative ATR trend is formatted correctly.
func TestRegimeDetectionPrompt_NegativeATRTrend(t *testing.T) {
	prompt := RegimeDetectionPrompt("BTC", -5.7, 0.08, 3.2)

	assert.Contains(t, prompt, "-5.70%")
}

// TestParseRegimeResponse_LowVolTrending verifies parsing of low_vol_trending.
func TestParseRegimeResponse_LowVolTrending(t *testing.T) {
	text := `{"regime":"low_vol_trending","reasoning":"ATR is falling and BB bandwidth is narrow."}`

	regime, ok := parseRegimeResponse(text)

	require.True(t, ok)
	assert.Equal(t, RegimeLowVolTrending, regime)
}

// TestParseRegimeResponse_HighVolTrending verifies parsing of high_vol_trending.
func TestParseRegimeResponse_HighVolTrending(t *testing.T) {
	text := `{"regime":"high_vol_trending","reasoning":"ATR rising with momentum."}`

	regime, ok := parseRegimeResponse(text)

	require.True(t, ok)
	assert.Equal(t, RegimeHighVolTrending, regime)
}

// TestParseRegimeResponse_MeanReverting verifies parsing of mean_reverting.
func TestParseRegimeResponse_MeanReverting(t *testing.T) {
	text := `{"regime":"mean_reverting","reasoning":"Price oscillating with moderate volatility."}`

	regime, ok := parseRegimeResponse(text)

	require.True(t, ok)
	assert.Equal(t, RegimeMeanReverting, regime)
}

// TestParseRegimeResponse_Crisis verifies parsing of crisis.
func TestParseRegimeResponse_Crisis(t *testing.T) {
	text := `{"regime":"crisis","reasoning":"Extreme volatility and large drawdown."}`

	regime, ok := parseRegimeResponse(text)

	require.True(t, ok)
	assert.Equal(t, RegimeCrisis, regime)
}

// TestParseRegimeResponse_WithPreamble verifies JSON extraction from surrounding text.
func TestParseRegimeResponse_WithPreamble(t *testing.T) {
	text := `Based on the metrics: {"regime":"crisis","reasoning":"High drawdown."} End.`

	regime, ok := parseRegimeResponse(text)

	require.True(t, ok)
	assert.Equal(t, RegimeCrisis, regime)
}

// TestParseRegimeResponse_InvalidJSON returns false on malformed JSON.
func TestParseRegimeResponse_InvalidJSON(t *testing.T) {
	regime, ok := parseRegimeResponse("not json at all")

	assert.False(t, ok)
	assert.Empty(t, regime)
}

// TestParseRegimeResponse_UnrecognisedRegime returns false for unknown regime strings.
func TestParseRegimeResponse_UnrecognisedRegime(t *testing.T) {
	text := `{"regime":"unknown_regime","reasoning":"Something else."}`

	regime, ok := parseRegimeResponse(text)

	assert.False(t, ok)
	assert.Empty(t, regime)
}

// TestParseRegimeResponse_EmptyString returns false on empty input.
func TestParseRegimeResponse_EmptyString(t *testing.T) {
	regime, ok := parseRegimeResponse("")

	assert.False(t, ok)
	assert.Empty(t, regime)
}

// TestDetectRegime_Success verifies that a valid provider response is parsed correctly.
func TestDetectRegime_Success(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"regime":"high_vol_trending","reasoning":"ATR trend rising with momentum."}`},
		},
	}

	regime, ok := DetectRegime(context.Background(), "AAPL", 15.0, 0.05, 4.2, provider)

	require.True(t, ok)
	assert.Equal(t, RegimeHighVolTrending, regime)
	assert.Equal(t, 1, provider.calls)
}

// TestDetectRegime_LLMError returns ("", false) on provider error.
func TestDetectRegime_LLMError(t *testing.T) {
	provider := &stubLLMProvider{
		errs: []error{errors.New("connection timeout")},
	}

	regime, ok := DetectRegime(context.Background(), "AAPL", 5.0, 0.03, 2.0, provider)

	assert.False(t, ok)
	assert.Empty(t, regime)
}

// TestDetectRegime_InvalidResponse returns ("", false) on unparseable response.
func TestDetectRegime_InvalidResponse(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `not valid json`},
		},
	}

	regime, ok := DetectRegime(context.Background(), "AAPL", 5.0, 0.03, 2.0, provider)

	assert.False(t, ok)
	assert.Empty(t, regime)
}

// TestDetectRegime_AllRegimes verifies that all four regimes can be returned.
func TestDetectRegime_AllRegimes(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		expectedRegime string
	}{
		{"low_vol_trending", `{"regime":"low_vol_trending","reasoning":"Low vol."}`, RegimeLowVolTrending},
		{"high_vol_trending", `{"regime":"high_vol_trending","reasoning":"High vol."}`, RegimeHighVolTrending},
		{"mean_reverting", `{"regime":"mean_reverting","reasoning":"Ranging."}`, RegimeMeanReverting},
		{"crisis", `{"regime":"crisis","reasoning":"Extreme vol."}`, RegimeCrisis},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := &stubLLMProvider{
				responses: []bullarc.LLMResponse{{Text: tc.response}},
			}
			regime, ok := DetectRegime(context.Background(), "TEST", 0, 0, 0, provider)
			require.True(t, ok)
			assert.Equal(t, tc.expectedRegime, regime)
		})
	}
}
