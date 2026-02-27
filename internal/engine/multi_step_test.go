package engine_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// multiStepLLMProvider is a configurable LLM stub for multi-step engine tests.
// Each Complete call consumes the next response in the slice.
type multiStepLLMProvider struct {
	responses []bullarc.LLMResponse
	errs      []error
	calls     int
}

func (s *multiStepLLMProvider) Name() string { return "multi-step-engine-stub" }

func (s *multiStepLLMProvider) Complete(_ context.Context, _ bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return bullarc.LLMResponse{}, s.errs[i]
	}
	if i < len(s.responses) {
		return s.responses[i], nil
	}
	return bullarc.LLMResponse{Text: `{"signal":"HOLD","confidence":50,"reasoning":"Default fallback."}`}, nil
}

// multiStepTechThesis builds a JSON technical thesis response.
func multiStepTechThesis(signal string, confidence int, thesis string) bullarc.LLMResponse {
	return bullarc.LLMResponse{
		Text: fmt.Sprintf(`{"signal":%q,"confidence":%d,"direction":"bullish","key_levels":"150","confluence":"aligned","thesis":%q}`,
			signal, confidence, thesis),
	}
}

// multiStepNewsThesis builds a JSON news thesis response.
func multiStepNewsThesis(thesis string) bullarc.LLMResponse {
	return bullarc.LLMResponse{
		Text: fmt.Sprintf(`{"sentiment_trend":"bullish","catalysts":"earnings","risks":"macro","thesis":%q}`, thesis),
	}
}

// multiStepSynthesis builds a JSON synthesis response.
func multiStepSynthesis(signal string, confidence int, reasoning string) bullarc.LLMResponse {
	return bullarc.LLMResponse{
		Text: fmt.Sprintf(`{"signal":%q,"confidence":%d,"reasoning":%q}`, signal, confidence, reasoning),
	}
}

// ── Core acceptance criteria tests ───────────────────────────────────────────

// TestAnalyze_MultiStep_ThreeStepsExecuted verifies that when multi-step mode is
// enabled, exactly three LLM calls are made (tech thesis, news thesis, synthesis).
func TestAnalyze_MultiStep_ThreeStepsExecuted(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	// Register a news source so the news thesis step is triggered.
	articles := []bullarc.NewsArticle{
		makeNewsArticle("ms1"),
	}
	sentimentProvider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{sentimentJSON("bullish", 80)},
	}
	scorer := llm.NewSentimentScorer(sentimentProvider)
	e.RegisterNewsSource(&stubNewsSource{articles: articles})
	e.RegisterSentimentScorer(scorer)

	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 75, "Strong technical momentum."),
			multiStepNewsThesis("Positive news flow driven by earnings."),
			multiStepSynthesis("BUY", 82, "Technical and news factors both bullish. RSI is strong. Earnings beat confirms uptrend. Combined conviction supports buying. Target upside."),
			{Text: `{"anomalies":[]}`}, // anomaly detection (4th call)
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	// 3 chain calls + 1 anomaly detection = 4 total LLM calls via multiProvider.
	assert.Equal(t, 4, provider.calls, "multi-step mode must execute all three chain steps plus anomaly detection")
	_ = result
}

// TestAnalyze_MultiStep_SynthesisSignalPresent verifies the LLMMultiStep signal
// appears in result.Signals when multi-step mode is enabled.
func TestAnalyze_MultiStep_SynthesisSignalPresent(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 75, "Bullish momentum."),
			multiStepSynthesis("BUY", 80, "Both technical indicators and news support bullish view. RSI at 65 shows strength. MACD histogram expanding positively. Combined conviction is high. Buy recommended."),
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	var multiSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "LLMMultiStep" {
			multiSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, multiSig, "LLMMultiStep signal must be present when multi-step mode is enabled")
	assert.Equal(t, "AAPL", multiSig.Symbol)
	assert.Equal(t, bullarc.SignalBuy, multiSig.Type)
	assert.InDelta(t, 100.0, multiSig.Confidence, 0.01, "default weight 2x: 80*2=160 capped at 100")
}

// TestAnalyze_MultiStep_ReasoningStoredInLLMAnalysis verifies that the synthesis
// reasoning is stored in AnalysisResult.LLMAnalysis.
func TestAnalyze_MultiStep_ReasoningStoredInLLMAnalysis(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	expectedReasoning := "Technical indicators confirm uptrend. RSI shows bullish momentum. News flow is positive. Combined signal is bullish. High conviction buy."
	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 75, "Bullish technical setup."),
			multiStepSynthesis("BUY", 78, expectedReasoning),
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	assert.Equal(t, expectedReasoning, result.LLMAnalysis,
		"synthesis reasoning must be stored in LLMAnalysis")
}

// TestAnalyze_MultiStep_ReplacesLLMMetaSignal verifies that when multi-step mode is
// enabled, LLMMetaSignal is NOT present (they are mutually exclusive).
func TestAnalyze_MultiStep_ReplacesLLMMetaSignal(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("HOLD", 55, "Sideways action."),
			multiStepSynthesis("HOLD", 55, "Technical and news neutral. Mixed indicator readings. No clear catalyst for a move. Holding current position is prudent. Watch for a breakout."),
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	for _, sig := range result.Signals {
		assert.NotEqual(t, "LLMMetaSignal", sig.Indicator,
			"LLMMetaSignal must not appear when multi-step mode is enabled")
	}
}

// TestAnalyze_MultiStep_NoNewsSkipsStep2 verifies that step 2 is skipped when
// no news source is registered (or no recent articles are available).
func TestAnalyze_MultiStep_NoNewsSkipsStep2(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	// No news source registered — only tech thesis (step 1) and synthesis (step 3).
	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 72, "Bullish setup."),
			multiStepSynthesis("BUY", 74, "Technical analysis is the primary driver. No news data available. RSI and MACD are aligned. Technical thesis supports buying. Proceed with caution."),
			{Text: `{"anomalies":[]}`}, // anomaly detection (3rd call)
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	// 2 chain calls (no news, step 2 skipped) + 1 anomaly detection = 3 total.
	assert.Equal(t, 3, provider.calls,
		"without news source: tech thesis + synthesis + anomaly detection = 3 calls")
	assert.NotEmpty(t, result.LLMAnalysis)
}

// TestAnalyze_MultiStep_NoNewsSource_Step2Skipped verifies that step 2 is also
// skipped when a news source is registered but returns no recent articles.
func TestAnalyze_MultiStep_EmptyNews_Step2Skipped(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	provider := &stubSentimentLLM{}
	scorer := llm.NewSentimentScorer(provider)
	e.RegisterNewsSource(&stubNewsSource{articles: nil})
	e.RegisterSentimentScorer(scorer)

	multiProvider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 72, "Bullish setup."),
			multiStepSynthesis("BUY", 74, "Technical analysis drives this view. No news available. RSI is positive. MACD confirms trend. Technical factors support buy."),
			{Text: `{"anomalies":[]}`}, // anomaly detection (3rd call)
		},
	}
	e.RegisterLLMProvider(multiProvider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	// 2 chain calls (no articles, step 2 skipped) + 1 anomaly detection = 3.
	assert.Equal(t, 3, multiProvider.calls,
		"no recent articles: tech thesis + synthesis + anomaly detection = 3 calls")
	assert.NotEmpty(t, result.LLMAnalysis)
}

// TestAnalyze_MultiStep_Step1Failure_OmitsLLMAnalysis verifies that when step 1
// (technical thesis) fails, no LLM analysis is included and Analyze still succeeds.
func TestAnalyze_MultiStep_Step1Failure_OmitsLLMAnalysis(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	provider := &multiStepLLMProvider{
		errs: []error{errors.New("LLM unavailable")},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err, "step 1 failure must not propagate from Analyze")
	require.NotEmpty(t, result.Signals, "technical composite must still be produced")

	assert.Empty(t, result.LLMAnalysis,
		"LLMAnalysis must be empty when step 1 fails")

	for _, sig := range result.Signals {
		assert.NotEqual(t, "LLMMultiStep", sig.Indicator,
			"LLMMultiStep must not appear when step 1 fails")
	}
}

// TestAnalyze_MultiStep_Step3Failure_FallsBackToStep1 verifies that when step 3
// (synthesis) fails, the engine falls back to the step-1 signal and sets
// LLMAnalysis to the technical thesis.
func TestAnalyze_MultiStep_Step3Failure_FallsBackToStep1(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	techThesis := "Bullish momentum confirmed by RSI and MACD."
	errs := make([]error, 3)
	errs[1] = errors.New("synthesis failed") // step 3 fails (index 1 without news = step 3)

	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 70, techThesis),
		},
		errs: errs,
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err, "step 3 failure must not propagate from Analyze")

	// The fallback step-1 signal should be present.
	var multiSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "LLMMultiStep" {
			multiSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, multiSig, "LLMMultiStep fallback signal must be present")
	assert.Equal(t, bullarc.SignalBuy, multiSig.Type)
	assert.Equal(t, techThesis, result.LLMAnalysis,
		"LLMAnalysis must contain the step-1 thesis as fallback")
}

// TestAnalyze_MultiStep_DisabledFallsBackToMetaSignal verifies that when
// multi-step mode is disabled, the engine uses the regular LLMMetaSignal.
func TestAnalyze_MultiStep_DisabledFallsBackToMetaSignal(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	// Multi-step mode is NOT enabled — default behaviour.

	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("BUY", 75, "Strong bullish alignment."),
			{Text: "Bullish trend confirmed."},
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
	require.NotNil(t, metaSig,
		"regular LLMMetaSignal must be present when multi-step mode is disabled")

	for _, sig := range result.Signals {
		assert.NotEqual(t, "LLMMultiStep", sig.Indicator,
			"LLMMultiStep must not appear when multi-step mode is disabled")
	}
}

// TestAnalyze_MultiStep_UseLLMFalse_ChainNotRun verifies multi-step chain is
// skipped when UseLLM is false.
func TestAnalyze_MultiStep_UseLLMFalse_ChainNotRun(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	provider := &multiStepLLMProvider{}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: false, // LLM disabled
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Equal(t, 0, provider.calls,
		"no LLM calls should be made when UseLLM=false")

	for _, sig := range result.Signals {
		assert.NotEqual(t, "LLMMultiStep", sig.Indicator)
	}
}

// TestAnalyze_MultiStep_NoLLMProvider_ChainNotRun verifies multi-step chain is
// skipped when no LLM provider is registered.
func TestAnalyze_MultiStep_NoLLMProvider_ChainNotRun(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)
	// No provider registered.

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	for _, sig := range result.Signals {
		assert.NotEqual(t, "LLMMultiStep", sig.Indicator)
		assert.NotEqual(t, "LLMMetaSignal", sig.Indicator)
	}
}

// TestAnalyze_MultiStep_WithNews_ThreeStepsAndHeadlinesPassed verifies the full
// multi-step chain with news: scored headlines are passed to step 2 correctly.
func TestAnalyze_MultiStep_WithNews_FullChain(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	articles := []bullarc.NewsArticle{
		{
			ID:          "n1",
			Headline:    "Apple reports record revenue",
			Summary:     "Revenue up 20%",
			Source:      "test",
			Symbols:     []string{"AAPL"},
			PublishedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID:          "n2",
			Headline:    "Supply chain risks persist",
			Summary:     "Ongoing issues",
			Source:      "test",
			Symbols:     []string{"AAPL"},
			PublishedAt: time.Now().Add(-1 * time.Hour),
		},
	}
	sentimentProvider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{
			sentimentJSON("bullish", 85),
			sentimentJSON("bearish", 60),
		},
	}
	scorer := llm.NewSentimentScorer(sentimentProvider)
	e.RegisterNewsSource(&stubNewsSource{articles: articles})
	e.RegisterSentimentScorer(scorer)

	reasoning := "Technical momentum is strong with RSI bullish. News is mixed: earnings positive but supply chain risk noted. Combined view is cautiously bullish. Indicator confluence supports buying. Monitor supply chain situation."
	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 72, "Bullish technical signals."),
			multiStepNewsThesis("Mixed news with positive earnings and supply risk."),
			multiStepSynthesis("BUY", 68, reasoning),
			{Text: `{"anomalies":[]}`}, // anomaly detection call
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	// 3 chain calls + 1 anomaly detection call = 4 total LLM calls.
	assert.Equal(t, 4, provider.calls, "all three chain steps plus anomaly detection must run")
	assert.Equal(t, reasoning, result.LLMAnalysis)

	var multiSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "LLMMultiStep" {
			multiSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, multiSig)
	assert.Equal(t, bullarc.SignalBuy, multiSig.Type)
}

// TestAnalyze_MultiStep_CompositeStillFirst verifies the composite signal is
// always at index 0 in multi-step mode.
func TestAnalyze_MultiStep_CompositeStillFirst(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 75, "Bullish."),
			multiStepSynthesis("BUY", 78, "Technical momentum and fundamental outlook are aligned. RSI confirms uptrend. MACD is expanding. No significant news headwinds. Buy recommended."),
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
		"composite must always be the first signal in multi-step mode")
}

// TestAnalyze_MultiStep_SymbolPropagated verifies the symbol is set on all signals.
func TestAnalyze_MultiStep_SymbolPropagated(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("SELL", 65, "Bearish setup."),
			multiStepSynthesis("SELL", 67, "Technical analysis is bearish for TSLA. RSI divergence detected. MACD turning down. No news to offset the bearish signal. Sell recommended."),
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
			"all signals including LLMMultiStep must carry the request symbol")
	}
}

// TestAnalyze_MultiStep_WeightApplied verifies that the llmMetaSignalWeight is
// applied to the multi-step synthesis confidence.
func TestAnalyze_MultiStep_WeightApplied(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)
	e.SetLLMMetaSignalWeight(0.5) // halve the confidence

	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 80, "Bullish."),
			multiStepSynthesis("BUY", 80, "Bullish combined view. Technical and news aligned. RSI strong. MACD confirms. Moderate conviction buy."),
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	var multiSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "LLMMultiStep" {
			multiSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, multiSig)
	// Weight 0.5: 80 * 0.5 = 40.
	assert.InDelta(t, 40.0, multiSig.Confidence, 0.01,
		"weight 0.5 should halve the synthesis confidence from 80 to 40")
}

// TestAnalyze_MultiStep_AnomalyDetectionStillRuns verifies that anomaly detection
// still runs in multi-step mode when UseLLM is true (it's independent of the chain).
func TestAnalyze_MultiStep_AnomalyDetectionStillRuns(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.SetMultiStepMode(true)

	anomalyJSON := `{"anomalies":[{"type":"bearish_divergence","description":"RSI diverging from price.","severity":"medium","affected_indicators":["RSI_14"]}]}`
	provider := &multiStepLLMProvider{
		responses: []bullarc.LLMResponse{
			multiStepTechThesis("BUY", 72, "Bullish despite divergence."),
			multiStepSynthesis("BUY", 70, "Technical bias is bullish overall. Divergence is a risk to watch. News neutral. Indicator confluence is moderate. Proceed with caution."),
			{Text: anomalyJSON}, // anomaly detection call
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	require.Len(t, result.Anomalies, 1,
		"anomaly detection must still run in multi-step mode")
	assert.Equal(t, "bearish_divergence", result.Anomalies[0].Type)
}
