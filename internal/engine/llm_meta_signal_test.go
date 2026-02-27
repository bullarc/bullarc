package engine_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubMetaLLM is an LLMProvider stub that returns configurable responses for
// engine-level LLM meta-signal tests. It tracks calls so tests can assert
// call counts.
type stubMetaLLM struct {
	responses []bullarc.LLMResponse
	err       error
	calls     int
}

func (s *stubMetaLLM) Name() string { return "stub-meta" }

func (s *stubMetaLLM) Complete(_ context.Context, _ bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	if s.err != nil {
		return bullarc.LLMResponse{}, s.err
	}
	i := s.calls
	s.calls++
	if i < len(s.responses) {
		return s.responses[i], nil
	}
	return bullarc.LLMResponse{Text: `{"signal":"HOLD","confidence":50,"reasoning":"Default."}`}, nil
}

func metaSignalJSON(signal string, confidence int, reasoning string) bullarc.LLMResponse {
	return bullarc.LLMResponse{
		Text: fmt.Sprintf(`{"signal":%q,"confidence":%d,"reasoning":%q}`, signal, confidence, reasoning),
	}
}

// TestAnalyze_LLMMetaSignalIncluded verifies that when UseLLM is true and an LLM
// provider is registered, a LLMMetaSignal appears in the result signals.
func TestAnalyze_LLMMetaSignalIncluded(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("BUY", 80, "Strong bullish alignment across indicators."),
			// Second response for the explanation call.
			{Text: "Bullish trend confirmed by multiple indicators."},
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	var metaSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "LLMMetaSignal" {
			metaSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, metaSig, "LLMMetaSignal must be present when UseLLM=true and provider is set")
	assert.Equal(t, "AAPL", metaSig.Symbol)
	assert.Equal(t, bullarc.SignalBuy, metaSig.Type)
}

// TestAnalyze_LLMMetaSignalOmittedWhenNoProvider verifies that the LLMMetaSignal
// is absent when no LLM provider is registered, and the technical composite is
// still produced.
func TestAnalyze_LLMMetaSignalOmittedWhenNoProvider(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals, "technical composite must still be produced")

	for _, sig := range result.Signals {
		assert.NotEqual(t, "LLMMetaSignal", sig.Indicator,
			"LLMMetaSignal must not appear when no LLM provider is registered")
	}
}

// TestAnalyze_LLMMetaSignalOmittedWhenUseLLMFalse verifies that the LLMMetaSignal
// is absent when UseLLM is false, even if a provider is registered.
func TestAnalyze_LLMMetaSignalOmittedWhenUseLLMFalse(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.RegisterLLMProvider(&stubMetaLLM{})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: false,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	for _, sig := range result.Signals {
		assert.NotEqual(t, "LLMMetaSignal", sig.Indicator,
			"LLMMetaSignal must not appear when UseLLM=false")
	}
}

// TestAnalyze_LLMMetaSignalErrorOmitsSignal verifies that an LLM error causes the
// meta-signal to be omitted without failing Analyze.
func TestAnalyze_LLMMetaSignalErrorOmitsSignal(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubMetaLLM{err: errors.New("connection refused")}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err, "LLM error must not propagate from Analyze")
	require.NotEmpty(t, result.Signals, "technical composite must still be produced")

	for _, sig := range result.Signals {
		assert.NotEqual(t, "LLMMetaSignal", sig.Indicator)
	}
}

// TestAnalyze_LLMMetaSignalInvalidResponseOmitsSignal verifies that an invalid
// JSON response causes the meta-signal to be omitted without failing Analyze.
func TestAnalyze_LLMMetaSignalInvalidResponseOmitsSignal(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{{Text: "not valid json"}},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err, "invalid LLM response must not propagate from Analyze")
	require.NotEmpty(t, result.Signals)

	for _, sig := range result.Signals {
		assert.NotEqual(t, "LLMMetaSignal", sig.Indicator)
	}
}

// TestAnalyze_LLMMetaSignalDefaultWeight verifies that the default weight (2x) is
// applied: a meta-signal confidence of 50 becomes 100 (capped).
func TestAnalyze_LLMMetaSignalDefaultWeight(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("BUY", 50, "Moderate bullish signal."),
			{Text: "Explanation."},
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	var metaSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "LLMMetaSignal" {
			metaSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, metaSig)
	// Default weight = 2.0: 50 * 2.0 = 100 (capped at 100).
	assert.InDelta(t, 100.0, metaSig.Confidence, 0.01,
		"default weight 2.0 should double the raw confidence (capped at 100)")
}

// TestAnalyze_LLMMetaSignalCustomWeight verifies that a custom weight is applied
// to the meta-signal confidence.
func TestAnalyze_LLMMetaSignalCustomWeight(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetLLMMetaSignalWeight(0.5) // halve the influence

	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("SELL", 80, "Bearish divergence."),
			{Text: "Explanation."},
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	var metaSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "LLMMetaSignal" {
			metaSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, metaSig)
	// Weight = 0.5: 80 * 0.5 = 40.
	assert.InDelta(t, 40.0, metaSig.Confidence, 0.01,
		"weight 0.5 should halve the raw LLM confidence")
}

// TestAnalyze_LLMMetaSignalParticipatesInAggregate verifies that the LLM meta-signal
// is added to the signals slice before aggregation, thereby influencing the composite.
// An uptrend produces a strong BUY composite. Adding an LLM SELL vote reduces the BUY
// dominance and lowers the composite confidence.
func TestAnalyze_LLMMetaSignalParticipatesInAggregate(t *testing.T) {
	bars := trendingBars(100, 100, 1.0) // steep uptrend → strong BUY from indicators
	e := newEngineWithBars(bars)

	// First analysis: no LLM provider — establishes the baseline BUY confidence.
	baseResult, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: false,
	})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)
	baseConfidence := baseResult.Signals[0].Confidence
	assert.Equal(t, bullarc.SignalBuy, baseResult.Signals[0].Type)

	// Second analysis: LLM returns a SELL vote, which should reduce BUY confidence.
	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("SELL", 100, "Contrarian sell signal."),
			{Text: "Explanation."},
		},
	}
	e.RegisterLLMProvider(provider)

	llmResult, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, llmResult.Signals)

	// The LLMMetaSignal must be present in the result.
	var metaSig *bullarc.Signal
	for i := range llmResult.Signals {
		if llmResult.Signals[i].Indicator == "LLMMetaSignal" {
			metaSig = &llmResult.Signals[i]
			break
		}
	}
	require.NotNil(t, metaSig, "LLMMetaSignal must be present in result.Signals")

	// The LLM SELL vote must reduce the composite BUY confidence.
	llmCompositeConfidence := llmResult.Signals[0].Confidence
	assert.Less(t, llmCompositeConfidence, baseConfidence,
		"LLM SELL vote should reduce BUY composite confidence from %.2f", baseConfidence)
}

// TestAnalyze_LLMMetaSignalSymbolPropagated verifies the symbol is set correctly.
func TestAnalyze_LLMMetaSignalSymbolPropagated(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("BUY", 70, "Bullish."),
			{Text: "Explanation."},
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "TSLA",
		UseLLM: true,
	})
	require.NoError(t, err)

	for _, sig := range result.Signals {
		assert.Equal(t, "TSLA", sig.Symbol,
			"all signals including LLMMetaSignal must carry the request symbol")
	}
}

// TestAnalyze_LLMMetaSignalCompositeStillFirst verifies the composite signal is
// always at index 0 even when the LLM meta-signal is present.
func TestAnalyze_LLMMetaSignalCompositeStillFirst(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("BUY", 75, "Bullish signals confirmed."),
			{Text: "Explanation."},
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Equal(t, "composite", result.Signals[0].Indicator,
		"composite must always be the first signal")
}
