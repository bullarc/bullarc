package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubSocialTracker is a test double for bullarc.SocialTracker.
type stubSocialTracker struct {
	metrics []bullarc.SocialMetrics
	err     error
}

func (s *stubSocialTracker) FetchSocialMetrics(_ context.Context, _ []string) ([]bullarc.SocialMetrics, error) {
	return s.metrics, s.err
}

// elevatedMetrics returns a SocialMetrics slice with IsElevated=true for sym.
func elevatedMetrics(sym string) []bullarc.SocialMetrics {
	return []bullarc.SocialMetrics{
		{
			Symbol:     sym,
			Mentions:   500,
			Velocity:   4.0,
			IsElevated: true,
			Timestamp:  time.Now(),
		},
	}
}

// normalMetrics returns a SocialMetrics slice with IsElevated=false for sym.
func normalMetrics(sym string) []bullarc.SocialMetrics {
	return []bullarc.SocialMetrics{
		{
			Symbol:     sym,
			Mentions:   50,
			Velocity:   1.0,
			IsElevated: false,
			Timestamp:  time.Now(),
		},
	}
}

// TestAnalyze_SocialRiskFlag_ElevatedAttention verifies that when social
// attention is elevated the composite signal carries the risk flag and the
// confidence is reduced by the default 10 % penalty.
func TestAnalyze_SocialRiskFlag_ElevatedAttention(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	e := newEngineWithBars(bars)
	e.RegisterSocialTracker(&stubSocialTracker{metrics: elevatedMetrics("AAPL")})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	require.Equal(t, "composite", composite.Indicator)
	assert.Contains(t, composite.RiskFlags, signal.RiskFlagElevatedSocialAttention,
		"composite signal should carry the elevated_social_attention flag")
}

// TestAnalyze_SocialRiskFlag_ConfidenceReduced verifies the default 10%
// confidence penalty is applied when the symbol is elevated.
func TestAnalyze_SocialRiskFlag_ConfidenceReduced(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)

	// Run once without social tracker to get baseline confidence.
	eBase := newEngineWithBars(bars)
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)
	baseConfidence := baseResult.Signals[0].Confidence

	// Run with elevated social tracker.
	e := newEngineWithBars(bars)
	e.RegisterSocialTracker(&stubSocialTracker{metrics: elevatedMetrics("AAPL")})
	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	expected := baseConfidence * 0.9
	assert.InDelta(t, expected, composite.Confidence, 0.001,
		"confidence should be reduced by 10%% (base=%.2f)", baseConfidence)
}

// TestAnalyze_SocialRiskFlag_DirectionUnchanged verifies the signal direction
// is preserved when the risk flag is applied.
func TestAnalyze_SocialRiskFlag_DirectionUnchanged(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)

	eBase := newEngineWithBars(bars)
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)
	baseType := baseResult.Signals[0].Type

	e := newEngineWithBars(bars)
	e.RegisterSocialTracker(&stubSocialTracker{metrics: elevatedMetrics("AAPL")})
	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Equal(t, baseType, result.Signals[0].Type,
		"signal direction must not change when social risk flag is applied")
}

// TestAnalyze_SocialRiskFlag_NoDataNoFlag verifies that when social data is
// absent (not elevated) no risk flag is attached and confidence is unaffected.
func TestAnalyze_SocialRiskFlag_NoDataNoFlag(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)

	eBase := newEngineWithBars(bars)
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)
	baseConfidence := baseResult.Signals[0].Confidence

	e := newEngineWithBars(bars)
	e.RegisterSocialTracker(&stubSocialTracker{metrics: normalMetrics("AAPL")})
	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.Empty(t, composite.RiskFlags, "no risk flag when attention is not elevated")
	assert.InDelta(t, baseConfidence, composite.Confidence, 0.001,
		"confidence should be unchanged when not elevated")
}

// TestAnalyze_SocialRiskFlag_NoTrackerNoFlag verifies that without a registered
// social tracker the composite carries no risk flags.
func TestAnalyze_SocialRiskFlag_NoTrackerNoFlag(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	composite := result.Signals[0]
	assert.Empty(t, composite.RiskFlags, "no risk flag when no social tracker is registered")
}

// TestAnalyze_SocialRiskFlag_CustomPenalty verifies that a custom confidence
// penalty is applied when configured.
func TestAnalyze_SocialRiskFlag_CustomPenalty(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)

	eBase := newEngineWithBars(bars)
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)
	baseConfidence := baseResult.Signals[0].Confidence

	// Use a 20% custom penalty.
	e := newEngineWithBars(bars)
	e.RegisterSocialTracker(&stubSocialTracker{metrics: elevatedMetrics("AAPL")})
	e.SetSocialConfidencePenalty(20)
	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	expected := baseConfidence * 0.8
	assert.InDelta(t, expected, result.Signals[0].Confidence, 0.001,
		"confidence should be reduced by 20%% (base=%.2f)", baseConfidence)
}

// TestAnalyze_SocialRiskFlag_EmptyMetrics verifies that when the tracker
// returns an empty slice (symbol not found) no flag is attached.
func TestAnalyze_SocialRiskFlag_EmptyMetrics(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	e := newEngineWithBars(bars)
	e.RegisterSocialTracker(&stubSocialTracker{metrics: nil})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Empty(t, result.Signals[0].RiskFlags, "no flag when tracker returns empty metrics")
}
