package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Prompt builder tests ──────────────────────────────────────────────────────

func makeMultiStepIndicatorValues() map[string][]bullarc.IndicatorValue {
	now := time.Now()
	return map[string][]bullarc.IndicatorValue{
		"RSI_14":       {{Time: now, Value: 62.3}},
		"MACD_12_26_9": {{Time: now, Value: 0.45, Extra: map[string]float64{"signal": 0.30, "histogram": 0.15}}},
	}
}

func makeMultiStepComposite(sigType bullarc.SignalType, confidence float64) bullarc.Signal {
	return bullarc.Signal{
		Type:        sigType,
		Confidence:  confidence,
		Indicator:   "composite",
		Symbol:      "AAPL",
		Timestamp:   time.Now(),
		Explanation: "2 buy, 0 sell",
	}
}

// TestTechnicalThesisPrompt_ContainsRequiredFields verifies the step-1 prompt
// includes symbol, price, indicator values, and the response schema.
func TestTechnicalThesisPrompt_ContainsRequiredFields(t *testing.T) {
	composite := makeMultiStepComposite(bullarc.SignalBuy, 70.0)
	indVals := makeMultiStepIndicatorValues()

	prompt := TechnicalThesisPrompt("AAPL", indVals, composite, 152.50)

	assert.Contains(t, prompt, "AAPL")
	assert.Contains(t, prompt, "152.5000")
	assert.Contains(t, prompt, "RSI_14")
	assert.Contains(t, prompt, "MACD_12_26_9")
	assert.Contains(t, prompt, "BUY")
	assert.Contains(t, prompt, "signal")
	assert.Contains(t, prompt, "confidence")
	assert.Contains(t, prompt, "thesis")
}

// TestTechnicalThesisPrompt_IncludesExtraFields verifies MACD extra fields appear in prompt.
func TestTechnicalThesisPrompt_IncludesExtraFields(t *testing.T) {
	indVals := makeMultiStepIndicatorValues()
	composite := makeMultiStepComposite(bullarc.SignalHold, 50.0)

	prompt := TechnicalThesisPrompt("TSLA", indVals, composite, 200.0)

	assert.Contains(t, prompt, "signal=")
	assert.Contains(t, prompt, "histogram=")
}

// TestNewsThesisPrompt_ContainsHeadlines verifies the step-2 prompt includes
// all provided headlines with their sentiment scores.
func TestNewsThesisPrompt_ContainsHeadlines(t *testing.T) {
	headlines := []ScoredHeadline{
		{Headline: "Apple beats earnings estimates", Sentiment: "bullish", Confidence: 85},
		{Headline: "Supply chain disruption fears", Sentiment: "bearish", Confidence: 70},
	}

	prompt := NewsThesisPrompt("AAPL", headlines)

	assert.Contains(t, prompt, "AAPL")
	assert.Contains(t, prompt, "Apple beats earnings estimates")
	assert.Contains(t, prompt, "bullish")
	assert.Contains(t, prompt, "Supply chain disruption fears")
	assert.Contains(t, prompt, "bearish")
	assert.Contains(t, prompt, "sentiment_trend")
	assert.Contains(t, prompt, "thesis")
}

// TestNewsThesisPrompt_EmptyHeadlines builds without crashing on empty slice.
func TestNewsThesisPrompt_EmptyHeadlines(t *testing.T) {
	prompt := NewsThesisPrompt("MSFT", nil)

	assert.Contains(t, prompt, "MSFT")
	assert.True(t, len(prompt) > 0)
}

// TestSynthesisPrompt_ContainesBothTheses verifies the step-3 prompt includes
// both the technical and news thesis texts.
func TestSynthesisPrompt_ContainsBothTheses(t *testing.T) {
	techThesis := "RSI at 62 shows moderate bullish momentum. MACD histogram is expanding positively."
	newsThesis := "Recent earnings beat boosted sentiment. Supply chain risks remain a concern."

	prompt := SynthesisPrompt("AAPL", techThesis, newsThesis)

	assert.Contains(t, prompt, "AAPL")
	assert.Contains(t, prompt, techThesis)
	assert.Contains(t, prompt, newsThesis)
	assert.Contains(t, prompt, "signal")
	assert.Contains(t, prompt, "reasoning")
	assert.Contains(t, prompt, "3-5 sentence")
}

// TestSynthesisPrompt_NoNewsFallbackText verifies that when newsThesis is empty,
// the prompt states no news data is available.
func TestSynthesisPrompt_NoNewsFallbackText(t *testing.T) {
	prompt := SynthesisPrompt("TSLA", "Technical thesis text.", "")

	assert.Contains(t, prompt, "No recent news data available")
}

// ── Parse response tests ──────────────────────────────────────────────────────

// TestParseTechnicalThesisResponse_ValidBuy verifies parsing a valid BUY thesis.
func TestParseTechnicalThesisResponse_ValidBuy(t *testing.T) {
	text := `{"signal":"BUY","confidence":78,"direction":"bullish","key_levels":"support at 150","confluence":"RSI and MACD agree","thesis":"Momentum is building."}`

	resp, ok := parseTechnicalThesisResponse("AAPL", text)

	require.True(t, ok)
	assert.Equal(t, "BUY", resp.Signal)
	assert.Equal(t, 78, resp.Confidence)
	assert.Equal(t, "bullish", resp.Direction)
	assert.Equal(t, "Momentum is building.", resp.Thesis)
}

// TestParseTechnicalThesisResponse_InvalidSignal returns false on unrecognised signal.
func TestParseTechnicalThesisResponse_InvalidSignal(t *testing.T) {
	text := `{"signal":"MAYBE","confidence":60,"direction":"neutral","key_levels":"","confluence":"","thesis":"unsure"}`

	_, ok := parseTechnicalThesisResponse("AAPL", text)

	assert.False(t, ok)
}

// TestParseTechnicalThesisResponse_InvalidJSON returns false on malformed JSON.
func TestParseTechnicalThesisResponse_InvalidJSON(t *testing.T) {
	_, ok := parseTechnicalThesisResponse("AAPL", "not json")

	assert.False(t, ok)
}

// TestParseTechnicalThesisResponse_WithPreamble extracts JSON embedded in text.
func TestParseTechnicalThesisResponse_WithPreamble(t *testing.T) {
	text := `Here is my analysis: {"signal":"SELL","confidence":65,"direction":"bearish","key_levels":"resistance at 180","confluence":"RSI divergence","thesis":"Bearish setup."}`

	resp, ok := parseTechnicalThesisResponse("AAPL", text)

	require.True(t, ok)
	assert.Equal(t, "SELL", resp.Signal)
}

// TestParseNewsThesisResponse_Valid verifies parsing a valid news thesis.
func TestParseNewsThesisResponse_Valid(t *testing.T) {
	text := `{"sentiment_trend":"bullish","catalysts":"earnings beat","risks":"macro uncertainty","thesis":"Positive news flow."}`

	resp, ok := parseNewsThesisResponse("AAPL", text)

	require.True(t, ok)
	assert.Equal(t, "bullish", resp.SentimentTrend)
	assert.Equal(t, "Positive news flow.", resp.Thesis)
}

// TestParseNewsThesisResponse_InvalidJSON returns false on malformed JSON.
func TestParseNewsThesisResponse_InvalidJSON(t *testing.T) {
	_, ok := parseNewsThesisResponse("AAPL", "bad json")

	assert.False(t, ok)
}

// TestParseSynthesisResponse_ValidBuy verifies parsing a valid synthesis BUY response.
func TestParseSynthesisResponse_ValidBuy(t *testing.T) {
	text := `{"signal":"BUY","confidence":82,"reasoning":"Technical indicators are bullish with RSI at 62 and MACD expanding. News sentiment is positive following earnings. Both factors support a buy decision. Risk is moderate given macro uncertainty. Overall conviction is high."}`

	resp, ok := parseSynthesisResponse("AAPL", text)

	require.True(t, ok)
	assert.Equal(t, "BUY", resp.Signal)
	assert.Equal(t, 82, resp.Confidence)
	assert.True(t, len(resp.Reasoning) > 0)
}

// TestParseSynthesisResponse_InvalidSignal returns false on unrecognised signal.
func TestParseSynthesisResponse_InvalidSignal(t *testing.T) {
	text := `{"signal":"UNKNOWN","confidence":70,"reasoning":"unsure"}`

	_, ok := parseSynthesisResponse("AAPL", text)

	assert.False(t, ok)
}

// TestParseSynthesisResponse_ConfidenceClamped_High verifies confidence > 100 is clamped.
func TestParseSynthesisResponse_ConfidenceClamped_High(t *testing.T) {
	text := `{"signal":"BUY","confidence":150,"reasoning":"Very sure about this."}`

	resp, ok := parseSynthesisResponse("AAPL", text)

	require.True(t, ok)
	// Clamping happens in RunMultiStepChain, not in parse; parser returns raw value.
	assert.Equal(t, 150, resp.Confidence)
}

// ── RunMultiStepChain tests ───────────────────────────────────────────────────

// multiStepLLMStub returns configurable responses per call index.
type multiStepLLMStub struct {
	responses []bullarc.LLMResponse
	errs      []error
	calls     int
}

func (s *multiStepLLMStub) Name() string { return "multi-step-stub" }

func (s *multiStepLLMStub) Complete(_ context.Context, _ bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return bullarc.LLMResponse{}, s.errs[i]
	}
	if i < len(s.responses) {
		return s.responses[i], nil
	}
	return bullarc.LLMResponse{Text: `{"signal":"HOLD","confidence":50,"reasoning":"Default."}`}, nil
}

func techThesisJSON(signal string, confidence int, direction, keyLevels, confluence, thesis string) bullarc.LLMResponse {
	return bullarc.LLMResponse{
		Text: fmt.Sprintf(`{"signal":%q,"confidence":%d,"direction":%q,"key_levels":%q,"confluence":%q,"thesis":%q}`,
			signal, confidence, direction, keyLevels, confluence, thesis),
	}
}

func newsThesisJSON(sentimentTrend, catalysts, risks, thesis string) bullarc.LLMResponse {
	return bullarc.LLMResponse{
		Text: fmt.Sprintf(`{"sentiment_trend":%q,"catalysts":%q,"risks":%q,"thesis":%q}`,
			sentimentTrend, catalysts, risks, thesis),
	}
}

func synthesisJSON(signal string, confidence int, reasoning string) bullarc.LLMResponse {
	return bullarc.LLMResponse{
		Text: fmt.Sprintf(`{"signal":%q,"confidence":%d,"reasoning":%q}`, signal, confidence, reasoning),
	}
}

// TestRunMultiStepChain_FullChainWithNews verifies the happy path with all three steps.
func TestRunMultiStepChain_FullChainWithNews(t *testing.T) {
	provider := &multiStepLLMStub{
		responses: []bullarc.LLMResponse{
			techThesisJSON("BUY", 75, "bullish", "support 150", "RSI and MACD agree", "Momentum building."),
			newsThesisJSON("bullish", "earnings beat", "macro risk", "Positive news flow."),
			synthesisJSON("BUY", 80, "Technical and news factors both bullish. RSI at 62 is strong. Earnings beat confirms momentum. Combined conviction is high. Buy recommended."),
		},
	}
	composite := makeMultiStepComposite(bullarc.SignalBuy, 72.0)
	headlines := []ScoredHeadline{
		{Headline: "Apple beats estimates", Sentiment: "bullish", Confidence: 85},
	}

	sig, reasoning, ok := RunMultiStepChain(
		context.Background(), "AAPL",
		makeMultiStepIndicatorValues(), composite, 152.0,
		headlines, provider,
	)

	require.True(t, ok)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.InDelta(t, 80.0, sig.Confidence, 0.01)
	assert.Equal(t, "LLMMultiStep", sig.Indicator)
	assert.Equal(t, "AAPL", sig.Symbol)
	assert.True(t, len(reasoning) > 0)
	assert.Equal(t, 3, provider.calls, "all three LLM steps must be called")
}

// TestRunMultiStepChain_NoNews skips step 2 when no headlines are provided.
func TestRunMultiStepChain_NoNews(t *testing.T) {
	provider := &multiStepLLMStub{
		responses: []bullarc.LLMResponse{
			techThesisJSON("HOLD", 55, "neutral", "150", "mixed signals", "Sideways action."),
			synthesisJSON("HOLD", 55, "Technical analysis shows neutral. No news data available. Mixed indicator signals suggest holding. Awaiting clearer direction. Maintain current position."),
		},
	}
	composite := makeMultiStepComposite(bullarc.SignalHold, 50.0)

	sig, reasoning, ok := RunMultiStepChain(
		context.Background(), "AAPL",
		makeMultiStepIndicatorValues(), composite, 152.0,
		nil, provider,
	)

	require.True(t, ok)
	assert.Equal(t, bullarc.SignalHold, sig.Type)
	assert.True(t, len(reasoning) > 0)
	assert.Equal(t, 2, provider.calls, "step 2 must be skipped with no headlines")
}

// TestRunMultiStepChain_Step1Fails returns (zero, "", false) when step 1 fails.
func TestRunMultiStepChain_Step1Fails(t *testing.T) {
	provider := &multiStepLLMStub{
		errs: []error{errors.New("connection refused")},
	}
	composite := makeMultiStepComposite(bullarc.SignalBuy, 70.0)

	sig, reasoning, ok := RunMultiStepChain(
		context.Background(), "AAPL",
		makeMultiStepIndicatorValues(), composite, 150.0,
		nil, provider,
	)

	assert.False(t, ok)
	assert.Equal(t, bullarc.Signal{}, sig)
	assert.Empty(t, reasoning)
	assert.Equal(t, 1, provider.calls)
}

// TestRunMultiStepChain_Step1InvalidJSON returns (zero, "", false) on invalid step-1 JSON.
func TestRunMultiStepChain_Step1InvalidJSON(t *testing.T) {
	provider := &multiStepLLMStub{
		responses: []bullarc.LLMResponse{{Text: "not valid json"}},
	}
	composite := makeMultiStepComposite(bullarc.SignalBuy, 70.0)

	_, _, ok := RunMultiStepChain(
		context.Background(), "AAPL",
		makeMultiStepIndicatorValues(), composite, 150.0,
		nil, provider,
	)

	assert.False(t, ok)
}

// TestRunMultiStepChain_Step3FailsFallsBackToStep1 verifies fallback to step-1
// signal when step 3 (synthesis) fails.
func TestRunMultiStepChain_Step3FailsFallsBackToStep1(t *testing.T) {
	provider := &multiStepLLMStub{
		responses: []bullarc.LLMResponse{
			techThesisJSON("BUY", 70, "bullish", "150", "RSI strong", "Bullish technical setup."),
			synthesisJSON("INVALID_WILL_NOT_PARSE", 0, ""), // step 3 returns invalid signal
		},
	}
	composite := makeMultiStepComposite(bullarc.SignalBuy, 65.0)

	sig, reasoning, ok := RunMultiStepChain(
		context.Background(), "AAPL",
		makeMultiStepIndicatorValues(), composite, 150.0,
		nil, provider,
	)

	require.True(t, ok, "fallback to step 1 should still return ok=true")
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.InDelta(t, 70.0, sig.Confidence, 0.01)
	assert.Equal(t, "LLMMultiStep", sig.Indicator)
	assert.Equal(t, "Bullish technical setup.", reasoning)
}

// TestRunMultiStepChain_Step3ErrorFallsBackToStep1 verifies fallback when step 3
// returns an LLM error.
func TestRunMultiStepChain_Step3ErrorFallsBackToStep1(t *testing.T) {
	errs := make([]error, 3)
	errs[2] = errors.New("synthesis LLM error")

	provider := &multiStepLLMStub{
		responses: []bullarc.LLMResponse{
			techThesisJSON("SELL", 65, "bearish", "resistance 180", "divergence", "Bearish technical setup."),
			newsThesisJSON("bearish", "", "supply chain risk", "Negative news."),
		},
		errs: errs,
	}
	headlines := []ScoredHeadline{
		{Headline: "Supply chain fears", Sentiment: "bearish", Confidence: 75},
	}
	composite := makeMultiStepComposite(bullarc.SignalSell, 60.0)

	sig, reasoning, ok := RunMultiStepChain(
		context.Background(), "AAPL",
		makeMultiStepIndicatorValues(), composite, 180.0,
		headlines, provider,
	)

	require.True(t, ok)
	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.Equal(t, "Bearish technical setup.", reasoning)
}

// TestRunMultiStepChain_Step2FailContinues verifies that step 2 failure does not
// abort the chain — synthesis proceeds with no news thesis.
func TestRunMultiStepChain_Step2FailContinues(t *testing.T) {
	errs := make([]error, 3)
	errs[1] = errors.New("news LLM error")

	provider := &multiStepLLMStub{
		responses: []bullarc.LLMResponse{
			techThesisJSON("BUY", 72, "bullish", "150", "aligned", "Strong momentum."),
			{}, // unused (errs[1] fires first)
			synthesisJSON("BUY", 75, "Technical momentum confirmed. News data unavailable but indicators support buying. RSI and MACD aligned. Synthesis based on technical factors only. Conviction is moderate."),
		},
		errs: errs,
	}
	headlines := []ScoredHeadline{
		{Headline: "Test headline", Sentiment: "bullish", Confidence: 80},
	}
	composite := makeMultiStepComposite(bullarc.SignalBuy, 68.0)

	sig, reasoning, ok := RunMultiStepChain(
		context.Background(), "AAPL",
		makeMultiStepIndicatorValues(), composite, 152.0,
		headlines, provider,
	)

	require.True(t, ok)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.True(t, len(reasoning) > 0)
}

// TestRunMultiStepChain_ConfidenceClamped verifies that confidence > 100 is clamped.
func TestRunMultiStepChain_ConfidenceClamped(t *testing.T) {
	provider := &multiStepLLMStub{
		responses: []bullarc.LLMResponse{
			techThesisJSON("BUY", 80, "bullish", "150", "aligned", "Bullish."),
			synthesisJSON("BUY", 150, "Very confident synthesis."),
		},
	}
	composite := makeMultiStepComposite(bullarc.SignalBuy, 80.0)

	sig, _, ok := RunMultiStepChain(
		context.Background(), "AAPL",
		makeMultiStepIndicatorValues(), composite, 152.0,
		nil, provider,
	)

	require.True(t, ok)
	assert.InDelta(t, 100.0, sig.Confidence, 0.01, "confidence must be clamped to 100")
}

// TestRunMultiStepChain_SignalSymbolPropagated verifies the symbol is set on the signal.
func TestRunMultiStepChain_SignalSymbolPropagated(t *testing.T) {
	provider := &multiStepLLMStub{
		responses: []bullarc.LLMResponse{
			techThesisJSON("SELL", 60, "bearish", "200", "divergence", "Bearish."),
			synthesisJSON("SELL", 65, "Bearish synthesis from technical and fundamental data. Indicators confirm downtrend. News sentiment supports selling. Risk-reward favors short. Exit longs."),
		},
	}
	composite := makeMultiStepComposite(bullarc.SignalSell, 55.0)
	composite.Symbol = "TSLA"

	sig, _, ok := RunMultiStepChain(
		context.Background(), "TSLA",
		makeMultiStepIndicatorValues(), composite, 200.0,
		nil, provider,
	)

	require.True(t, ok)
	assert.Equal(t, "TSLA", sig.Symbol)
	assert.Equal(t, "LLMMultiStep", sig.Indicator)
	assert.False(t, sig.Timestamp.IsZero())
}

// TestSynthesisPrompt_ContainsNoNewsText verifies the fallback text when
// newsThesis is empty string.
func TestSynthesisPrompt_NoNewsFallback(t *testing.T) {
	prompt := SynthesisPrompt("AAPL", "Tech thesis here.", "")

	assert.Contains(t, prompt, "No recent news data available")
	assert.True(t, strings.Contains(prompt, "AAPL"))
}
