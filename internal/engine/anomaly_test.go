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

// anomalyJSON returns a well-formed LLM response with the given anomalies encoded
// as JSON, ready to be used as a stubMetaLLM response.
func anomalyJSON(anomalies []bullarc.Anomaly) bullarc.LLMResponse {
	if len(anomalies) == 0 {
		return bullarc.LLMResponse{Text: `{"anomalies":[]}`}
	}
	var items string
	for i, a := range anomalies {
		if i > 0 {
			items += ","
		}
		indicators := `[]`
		if len(a.AffectedIndicators) > 0 {
			var parts string
			for j, ind := range a.AffectedIndicators {
				if j > 0 {
					parts += ","
				}
				parts += fmt.Sprintf("%q", ind)
			}
			indicators = "[" + parts + "]"
		}
		items += fmt.Sprintf(
			`{"type":%q,"description":%q,"severity":%q,"affected_indicators":%s}`,
			a.Type, a.Description, string(a.Severity), indicators,
		)
	}
	return bullarc.LLMResponse{Text: fmt.Sprintf(`{"anomalies":[%s]}`, items)}
}

// TestAnalyze_AnomaliesPopulatedWhenUseLLM verifies that anomalies are included
// in the result when UseLLM is true, an LLM provider is registered, and there
// are enough bars.
func TestAnalyze_AnomaliesPopulatedWhenUseLLM(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	want := []bullarc.Anomaly{
		{
			Type:               "bearish_divergence",
			Description:        "Price up while RSI declining.",
			Severity:           bullarc.AnomalySeverityHigh,
			AffectedIndicators: []string{"RSI_14"},
		},
	}
	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("BUY", 75, "Bullish."),
			{Text: "Explanation."},
			anomalyJSON(want),
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	require.NotNil(t, result.Anomalies)
	require.Len(t, result.Anomalies, 1)
	assert.Equal(t, "bearish_divergence", result.Anomalies[0].Type)
	assert.Equal(t, bullarc.AnomalySeverityHigh, result.Anomalies[0].Severity)
	assert.Equal(t, []string{"RSI_14"}, result.Anomalies[0].AffectedIndicators)
}

// TestAnalyze_AnomaliesNilWhenUseLLMFalse verifies that anomaly detection is
// skipped entirely when UseLLM is false.
func TestAnalyze_AnomaliesNilWhenUseLLMFalse(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)
	e.RegisterLLMProvider(&stubMetaLLM{})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: false,
	})
	require.NoError(t, err)
	assert.Nil(t, result.Anomalies,
		"Anomalies must be nil when UseLLM=false")
}

// TestAnalyze_AnomaliesNilWhenNoProvider verifies that anomaly detection is
// skipped when no LLM provider is registered.
func TestAnalyze_AnomaliesNilWhenNoProvider(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)
	assert.Nil(t, result.Anomalies,
		"Anomalies must be nil when no LLM provider is registered")
}

// TestAnalyze_AnomaliesEmptyWhenInsufficientBars verifies that the system returns
// an empty (non-nil) anomaly list when fewer than 10 bars are available.
func TestAnalyze_AnomaliesEmptyWhenInsufficientBars(t *testing.T) {
	// 5 bars — below the 10-bar minimum.
	bars := trendingBars(5, 100, 0.5)
	e := newEngineWithBars(bars)
	// Provider should not be called for anomaly detection.
	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			// Meta-signal and explanation calls may still happen (indicators may not
			// have enough warmup, but the engine proceeds).
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	assert.NotNil(t, result.Anomalies,
		"Anomalies must be an empty slice (not nil) when bars < 10")
	assert.Empty(t, result.Anomalies,
		"Anomalies must be empty when bars < 10")
}

// TestAnalyze_AnomaliesEmptyWhenLLMFails verifies that a failing LLM call for
// anomaly detection returns an empty list without failing Analyze.
func TestAnalyze_AnomaliesEmptyWhenLLMFails(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubMetaLLM{err: errors.New("service unavailable")}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err, "LLM failure must not propagate from Analyze")
	assert.NotNil(t, result.Anomalies)
	assert.Empty(t, result.Anomalies)
}

// TestAnalyze_AnomaliesEmptyWhenLLMReturnsNoAnomalies verifies an explicit empty
// anomaly list is returned when the LLM finds nothing unusual.
func TestAnalyze_AnomaliesEmptyWhenLLMReturnsNoAnomalies(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("HOLD", 50, "No clear direction."),
			{Text: "Explanation."},
			anomalyJSON(nil),
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)
	assert.NotNil(t, result.Anomalies)
	assert.Empty(t, result.Anomalies)
}

// TestAnalyze_MultipleAnomalies verifies that multiple anomalies are all preserved.
func TestAnalyze_MultipleAnomalies(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	want := []bullarc.Anomaly{
		{
			Type:               "bearish_divergence",
			Description:        "Price up while RSI declining.",
			Severity:           bullarc.AnomalySeverityHigh,
			AffectedIndicators: []string{"RSI_14"},
		},
		{
			Type:               "volatility_squeeze",
			Description:        "Bollinger Band bandwidth narrowing.",
			Severity:           bullarc.AnomalySeverityMedium,
			AffectedIndicators: []string{"BB_20_2.0"},
		},
		{
			Type:               "volume_anomaly",
			Description:        "Volume expanding while price flat.",
			Severity:           bullarc.AnomalySeverityLow,
			AffectedIndicators: []string{},
		},
	}
	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("SELL", 65, "Bearish patterns detected."),
			{Text: "Explanation."},
			anomalyJSON(want),
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)

	require.Len(t, result.Anomalies, 3)
	assert.Equal(t, "bearish_divergence", result.Anomalies[0].Type)
	assert.Equal(t, "volatility_squeeze", result.Anomalies[1].Type)
	assert.Equal(t, "volume_anomaly", result.Anomalies[2].Type)
}

// TestAnalyze_AnomaliesDoNotAffectSignals verifies that anomaly detection does
// not modify the signals slice.
func TestAnalyze_AnomaliesDoNotAffectSignals(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubMetaLLM{
		responses: []bullarc.LLMResponse{
			metaSignalJSON("BUY", 80, "Bullish."),
			{Text: "Explanation."},
			anomalyJSON([]bullarc.Anomaly{
				{Type: "bearish_divergence", Description: "RSI falling.", Severity: bullarc.AnomalySeverityHigh, AffectedIndicators: []string{"RSI_14"}},
			}),
		},
	}
	e.RegisterLLMProvider(provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "AAPL",
		UseLLM: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	// The composite must still be first.
	assert.Equal(t, "composite", result.Signals[0].Indicator)
	// Anomalies must not appear in the signals slice.
	for _, sig := range result.Signals {
		assert.NotEqual(t, "anomaly", sig.Indicator)
	}
}
