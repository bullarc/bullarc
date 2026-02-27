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

// makeIndicatorHistory returns a map of indicator values with n entries each,
// starting from baseTime and incrementing by one day.
func makeIndicatorHistory(n int) map[string][]bullarc.IndicatorValue {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rsi := make([]bullarc.IndicatorValue, n)
	macd := make([]bullarc.IndicatorValue, n)
	for i := range rsi {
		t := now.AddDate(0, 0, i)
		rsi[i] = bullarc.IndicatorValue{Time: t, Value: 50.0 + float64(i)*0.3}
		macd[i] = bullarc.IndicatorValue{
			Time:  t,
			Value: 0.1 + float64(i)*0.01,
			Extra: map[string]float64{"signal": 0.05, "histogram": 0.05},
		}
	}
	return map[string][]bullarc.IndicatorValue{
		"RSI_14":      rsi,
		"MACD_12_26_9": macd,
	}
}

// TestAnomalyDetectionPrompt_ContainsSymbol verifies the prompt mentions the symbol.
func TestAnomalyDetectionPrompt_ContainsSymbol(t *testing.T) {
	prompt := AnomalyDetectionPrompt("AAPL", makeIndicatorHistory(30))
	assert.Contains(t, prompt, "AAPL")
}

// TestAnomalyDetectionPrompt_ContainsIndicatorNames verifies indicator names appear.
func TestAnomalyDetectionPrompt_ContainsIndicatorNames(t *testing.T) {
	prompt := AnomalyDetectionPrompt("TSLA", makeIndicatorHistory(30))
	assert.Contains(t, prompt, "RSI_14")
	assert.Contains(t, prompt, "MACD_12_26_9")
}

// TestAnomalyDetectionPrompt_ContainsAnomalyTypes verifies the prompt instructs the
// LLM to look for all required anomaly categories.
func TestAnomalyDetectionPrompt_ContainsAnomalyTypes(t *testing.T) {
	prompt := AnomalyDetectionPrompt("AAPL", makeIndicatorHistory(30))
	assert.Contains(t, prompt, "Bearish divergence")
	assert.Contains(t, prompt, "Bullish divergence")
	assert.Contains(t, prompt, "Volatility squeeze")
	assert.Contains(t, prompt, "Volume anomaly")
}

// TestAnomalyDetectionPrompt_LimitsTo30Days verifies that only 30 data points per
// indicator appear in the prompt, even when more history is provided.
func TestAnomalyDetectionPrompt_LimitsTo30Days(t *testing.T) {
	// 60 entries — the prompt should only include the last 30.
	indVals := makeIndicatorHistory(60)
	prompt := AnomalyDetectionPrompt("AAPL", indVals)

	// The oldest date of the RSI history starts at 2024-01-01 but only the
	// last 30 days (2024-02-01 .. 2024-02-29) should appear.
	assert.NotContains(t, prompt, "2024-01-01",
		"oldest entries beyond 30-day window must be excluded from the prompt")
	assert.Contains(t, prompt, "2024-02-29",
		"the 60th (last) entry date must appear in the prompt")
}

// TestAnomalyDetectionPrompt_EmptyIndicators returns a non-empty prompt with instructions.
func TestAnomalyDetectionPrompt_EmptyIndicators(t *testing.T) {
	prompt := AnomalyDetectionPrompt("MSFT", nil)
	assert.Contains(t, prompt, "MSFT")
	assert.Greater(t, len(prompt), 100)
}

// TestAnomalyDetectionPrompt_IncludesExtraFields verifies Extra fields are included.
func TestAnomalyDetectionPrompt_IncludesExtraFields(t *testing.T) {
	indVals := map[string][]bullarc.IndicatorValue{
		"MACD_12_26_9": {
			{Time: time.Now(), Value: 0.5, Extra: map[string]float64{"signal": 0.3, "histogram": 0.2}},
		},
	}
	prompt := AnomalyDetectionPrompt("AAPL", indVals)
	assert.Contains(t, prompt, "signal=")
	assert.Contains(t, prompt, "histogram=")
}

// TestAnomalyDetectionPrompt_ContainsJSONSchemaInstruction verifies the JSON response
// format instructions are present.
func TestAnomalyDetectionPrompt_ContainsJSONSchemaInstruction(t *testing.T) {
	prompt := AnomalyDetectionPrompt("AAPL", makeIndicatorHistory(10))
	assert.Contains(t, prompt, `"anomalies"`)
	assert.Contains(t, prompt, "severity")
	assert.Contains(t, prompt, "affected_indicators")
}

// TestParseAnomalyResponse_SingleAnomaly verifies parsing a single anomaly.
func TestParseAnomalyResponse_SingleAnomaly(t *testing.T) {
	text := `{"anomalies":[{"type":"bearish_divergence","description":"Price rising while RSI falling.","severity":"high","affected_indicators":["RSI_14"]}]}`

	anomalies, ok := parseAnomalyResponse(text)

	require.True(t, ok)
	require.Len(t, anomalies, 1)
	assert.Equal(t, "bearish_divergence", anomalies[0].Type)
	assert.Equal(t, "Price rising while RSI falling.", anomalies[0].Description)
	assert.Equal(t, bullarc.AnomalySeverityHigh, anomalies[0].Severity)
	assert.Equal(t, []string{"RSI_14"}, anomalies[0].AffectedIndicators)
}

// TestParseAnomalyResponse_MultipleAnomalies verifies parsing multiple anomalies.
func TestParseAnomalyResponse_MultipleAnomalies(t *testing.T) {
	text := `{"anomalies":[
		{"type":"bearish_divergence","description":"RSI declining.","severity":"high","affected_indicators":["RSI_14","MACD_12_26_9"]},
		{"type":"volatility_squeeze","description":"BB bandwidth narrowing.","severity":"medium","affected_indicators":["BB_20_2.0"]}
	]}`

	anomalies, ok := parseAnomalyResponse(text)

	require.True(t, ok)
	require.Len(t, anomalies, 2)
	assert.Equal(t, "bearish_divergence", anomalies[0].Type)
	assert.Equal(t, bullarc.AnomalySeverityHigh, anomalies[0].Severity)
	assert.Equal(t, "volatility_squeeze", anomalies[1].Type)
	assert.Equal(t, bullarc.AnomalySeverityMedium, anomalies[1].Severity)
}

// TestParseAnomalyResponse_EmptyAnomalies verifies parsing when no anomalies found.
func TestParseAnomalyResponse_EmptyAnomalies(t *testing.T) {
	text := `{"anomalies":[]}`

	anomalies, ok := parseAnomalyResponse(text)

	require.True(t, ok)
	assert.Empty(t, anomalies)
}

// TestParseAnomalyResponse_AllSeverityLevels verifies all severity values are handled.
func TestParseAnomalyResponse_AllSeverityLevels(t *testing.T) {
	text := `{"anomalies":[
		{"type":"a","description":"d","severity":"low","affected_indicators":[]},
		{"type":"b","description":"d","severity":"medium","affected_indicators":[]},
		{"type":"c","description":"d","severity":"high","affected_indicators":[]}
	]}`

	anomalies, ok := parseAnomalyResponse(text)

	require.True(t, ok)
	require.Len(t, anomalies, 3)
	assert.Equal(t, bullarc.AnomalySeverityLow, anomalies[0].Severity)
	assert.Equal(t, bullarc.AnomalySeverityMedium, anomalies[1].Severity)
	assert.Equal(t, bullarc.AnomalySeverityHigh, anomalies[2].Severity)
}

// TestParseAnomalyResponse_UnknownSeverityDefaultsToLow verifies unrecognised severity
// is normalised to "low".
func TestParseAnomalyResponse_UnknownSeverityDefaultsToLow(t *testing.T) {
	text := `{"anomalies":[{"type":"x","description":"d","severity":"critical","affected_indicators":[]}]}`

	anomalies, ok := parseAnomalyResponse(text)

	require.True(t, ok)
	require.Len(t, anomalies, 1)
	assert.Equal(t, bullarc.AnomalySeverityLow, anomalies[0].Severity)
}

// TestParseAnomalyResponse_WithPreamble verifies JSON embedded in surrounding text is parsed.
func TestParseAnomalyResponse_WithPreamble(t *testing.T) {
	text := `Here is my analysis: {"anomalies":[{"type":"volume_anomaly","description":"Volume spike.","severity":"medium","affected_indicators":["OBV"]}]} Hope this helps.`

	anomalies, ok := parseAnomalyResponse(text)

	require.True(t, ok)
	require.Len(t, anomalies, 1)
	assert.Equal(t, "volume_anomaly", anomalies[0].Type)
}

// TestParseAnomalyResponse_InvalidJSON returns (nil, false) on malformed input.
func TestParseAnomalyResponse_InvalidJSON(t *testing.T) {
	anomalies, ok := parseAnomalyResponse("not json at all")

	assert.False(t, ok)
	assert.Nil(t, anomalies)
}

// TestParseAnomalyResponse_NoJSONObject returns (nil, false) when no braces found.
func TestParseAnomalyResponse_NoJSONObject(t *testing.T) {
	anomalies, ok := parseAnomalyResponse("anomalies: none found")

	assert.False(t, ok)
	assert.Nil(t, anomalies)
}

// TestParseAnomalyResponse_AffectedIndicatorsNotNil verifies empty affected_indicators
// becomes an empty slice, not nil.
func TestParseAnomalyResponse_AffectedIndicatorsNotNil(t *testing.T) {
	text := `{"anomalies":[{"type":"x","description":"d","severity":"low","affected_indicators":[]}]}`

	anomalies, ok := parseAnomalyResponse(text)

	require.True(t, ok)
	require.Len(t, anomalies, 1)
	assert.NotNil(t, anomalies[0].AffectedIndicators)
}

// TestDetectAnomalies_Success verifies the full DetectAnomalies call returns anomalies.
func TestDetectAnomalies_Success(t *testing.T) {
	responseJSON := `{"anomalies":[{"type":"bearish_divergence","description":"Price up, RSI down.","severity":"high","affected_indicators":["RSI_14"]}]}`
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{{Text: responseJSON}},
	}

	anomalies, ok := DetectAnomalies(context.Background(), "AAPL", makeIndicatorHistory(30), provider)

	require.True(t, ok)
	require.Len(t, anomalies, 1)
	assert.Equal(t, "bearish_divergence", anomalies[0].Type)
	assert.Equal(t, bullarc.AnomalySeverityHigh, anomalies[0].Severity)
	assert.Equal(t, 1, provider.calls)
}

// TestDetectAnomalies_LLMError returns (nil, false) on provider error.
func TestDetectAnomalies_LLMError(t *testing.T) {
	provider := &stubLLMProvider{
		errs: []error{errors.New("connection timeout")},
	}

	anomalies, ok := DetectAnomalies(context.Background(), "AAPL", makeIndicatorHistory(30), provider)

	assert.False(t, ok)
	assert.Nil(t, anomalies)
}

// TestDetectAnomalies_InvalidResponseReturnsFalse returns (nil, false) on invalid JSON.
func TestDetectAnomalies_InvalidResponseReturnsFalse(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{{Text: "not valid json"}},
	}

	anomalies, ok := DetectAnomalies(context.Background(), "AAPL", makeIndicatorHistory(30), provider)

	assert.False(t, ok)
	assert.Nil(t, anomalies)
}

// TestDetectAnomalies_PromptSentToProvider verifies the prompt passed to the LLM
// contains the symbol and the "last 30 days" header.
func TestDetectAnomalies_PromptSentToProvider(t *testing.T) {
	var capturedPrompt string
	provider := &capturingLLMProvider{
		response: bullarc.LLMResponse{Text: `{"anomalies":[]}`},
		capture:  func(req bullarc.LLMRequest) { capturedPrompt = req.Prompt },
	}

	_, _ = DetectAnomalies(context.Background(), "NVDA", makeIndicatorHistory(30), provider)

	assert.Contains(t, capturedPrompt, "NVDA")
	assert.Contains(t, capturedPrompt, "30 days")
	assert.True(t, strings.Contains(capturedPrompt, "RSI_14") || strings.Contains(capturedPrompt, "MACD"))
}

// capturingLLMProvider is a test double that captures the LLMRequest for inspection.
type capturingLLMProvider struct {
	response bullarc.LLMResponse
	capture  func(bullarc.LLMRequest)
}

func (c *capturingLLMProvider) Name() string { return "capturing" }

func (c *capturingLLMProvider) Complete(_ context.Context, req bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	if c.capture != nil {
		c.capture(req)
	}
	return c.response, nil
}
