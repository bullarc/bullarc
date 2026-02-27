package engine_test

import (
	"context"
	"testing"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCorrelationLLMProvider returns configurable correlation check responses.
type stubCorrelationLLMProvider struct {
	responses []string
	calls     int
	err       error
}

func (s *stubCorrelationLLMProvider) Name() string { return "stub-correlation" }

func (s *stubCorrelationLLMProvider) Complete(_ context.Context, _ bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	if s.err != nil {
		return bullarc.LLMResponse{}, s.err
	}
	i := s.calls
	s.calls++
	if i < len(s.responses) {
		return bullarc.LLMResponse{Text: s.responses[i]}, nil
	}
	return bullarc.LLMResponse{Text: `{"correlated": false, "overlap": "low", "reasoning": "default"}`}, nil
}

// newCorrelationEngine builds an engine with default indicators, a stub data
// source, a stub LLM provider, and correlation checking enabled.
func newCorrelationEngine(bars []bullarc.OHLCV, provider bullarc.LLMProvider) *engine.Engine {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	e.RegisterLLMProvider(provider)
	e.SetCorrelationConfig(engine.CorrelationConfig{Enabled: true})
	return e
}

// TestCorrelationCheck_HighOverlap_AddsRiskFlag verifies that when the LLM
// returns high overlap, the "high_correlation" risk flag is added to the
// composite signal.
func TestCorrelationCheck_HighOverlap_AddsRiskFlag(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	provider := &stubCorrelationLLMProvider{
		responses: []string{
			`{"correlated": true, "overlap": "high", "reasoning": "Same sector."}`,
		},
	}
	e := newCorrelationEngine(bars, provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol:    "NVDA",
		Portfolio: []string{"AMD", "INTC"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.Contains(t, composite.RiskFlags, engine.RiskFlagHighCorrelation,
		"composite signal should carry the high_correlation flag when overlap is high")
}

// TestCorrelationCheck_MediumOverlap_NoRiskFlag verifies that medium overlap
// does not add the "high_correlation" risk flag.
func TestCorrelationCheck_MediumOverlap_NoRiskFlag(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	provider := &stubCorrelationLLMProvider{
		responses: []string{
			`{"correlated": false, "overlap": "medium", "reasoning": "Same broad sector."}`,
		},
	}
	e := newCorrelationEngine(bars, provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol:    "XOM",
		Portfolio: []string{"CVX"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.NotContains(t, composite.RiskFlags, engine.RiskFlagHighCorrelation,
		"medium overlap should not add high_correlation flag")
}

// TestCorrelationCheck_LowOverlap_NoRiskFlag verifies that low overlap does not
// add the "high_correlation" risk flag.
func TestCorrelationCheck_LowOverlap_NoRiskFlag(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	provider := &stubCorrelationLLMProvider{
		responses: []string{
			`{"correlated": false, "overlap": "low", "reasoning": "Different sectors entirely."}`,
		},
	}
	e := newCorrelationEngine(bars, provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol:    "XOM",
		Portfolio: []string{"AAPL", "MSFT"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.NotContains(t, composite.RiskFlags, engine.RiskFlagHighCorrelation,
		"low overlap should not add high_correlation flag")
}

// TestCorrelationCheck_EmptyPortfolio_Skipped verifies that when Portfolio is
// empty the correlation check is skipped and no flag is added.
func TestCorrelationCheck_EmptyPortfolio_Skipped(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	provider := &stubCorrelationLLMProvider{
		responses: []string{
			`{"correlated": true, "overlap": "high", "reasoning": "Would be correlated."}`,
		},
	}
	e := newCorrelationEngine(bars, provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol:    "NVDA",
		Portfolio: []string{}, // empty — check must be skipped
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.NotContains(t, composite.RiskFlags, engine.RiskFlagHighCorrelation,
		"empty portfolio should skip correlation check")
	assert.Equal(t, 0, provider.calls, "LLM must not be called for empty portfolio")
}

// TestCorrelationCheck_NilPortfolio_Skipped verifies that a nil portfolio also
// skips the correlation check.
func TestCorrelationCheck_NilPortfolio_Skipped(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	provider := &stubCorrelationLLMProvider{
		responses: []string{
			`{"correlated": true, "overlap": "high", "reasoning": "Would be correlated."}`,
		},
	}
	e := newCorrelationEngine(bars, provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol: "NVDA",
		// Portfolio omitted (nil)
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Equal(t, 0, provider.calls, "LLM must not be called when Portfolio is nil")
}

// TestCorrelationCheck_DisabledConfig_Skipped verifies that when
// CorrelationConfig.Enabled is false the check is not performed.
func TestCorrelationCheck_DisabledConfig_Skipped(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	provider := &stubCorrelationLLMProvider{
		responses: []string{
			`{"correlated": true, "overlap": "high", "reasoning": "Would be correlated."}`,
		},
	}
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	e.RegisterLLMProvider(provider)
	// CorrelationConfig is left at its zero value (Enabled=false).

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol:    "NVDA",
		Portfolio: []string{"AMD", "INTC"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.NotContains(t, composite.RiskFlags, engine.RiskFlagHighCorrelation,
		"disabled correlation check must not add the risk flag")
	assert.Equal(t, 0, provider.calls, "LLM must not be called when correlation check is disabled")
}

// TestCorrelationCheck_NoLLMProvider_Skipped verifies that when no LLM provider
// is registered the correlation check is silently skipped.
func TestCorrelationCheck_NoLLMProvider_Skipped(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	e.SetCorrelationConfig(engine.CorrelationConfig{Enabled: true})
	// No LLM provider registered.

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol:    "NVDA",
		Portfolio: []string{"AMD"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.NotContains(t, composite.RiskFlags, engine.RiskFlagHighCorrelation,
		"no LLM provider means correlation check is silently skipped")
}

// TestCorrelationCheck_LLMFailure_NoFlag verifies that an LLM failure during
// correlation check leaves the composite signal unmodified.
func TestCorrelationCheck_LLMFailure_NoFlag(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	provider := &stubCorrelationLLMProvider{
		err: bullarc.ErrLLMUnavailable,
	}
	e := newCorrelationEngine(bars, provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol:    "NVDA",
		Portfolio: []string{"AMD"},
	})
	require.NoError(t, err, "LLM failure must not propagate to caller")
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.NotContains(t, composite.RiskFlags, engine.RiskFlagHighCorrelation,
		"LLM failure should not add the risk flag")
}

// TestCorrelationCheck_SignalDirectionUnchanged verifies that adding the
// high_correlation flag does not modify the composite signal direction.
func TestCorrelationCheck_SignalDirectionUnchanged(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)

	// Get baseline direction without correlation check.
	eBase := newEngineWithBars(bars)
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "NVDA"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)
	baseType := baseResult.Signals[0].Type

	provider := &stubCorrelationLLMProvider{
		responses: []string{
			`{"correlated": true, "overlap": "high", "reasoning": "Same sector."}`,
		},
	}
	e := newCorrelationEngine(bars, provider)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{
		Symbol:    "NVDA",
		Portfolio: []string{"AMD"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Equal(t, baseType, result.Signals[0].Type,
		"correlation risk flag must not change signal direction")
}
