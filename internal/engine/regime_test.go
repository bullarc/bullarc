package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRegimeLLMProvider is a test double for bullarc.LLMProvider that returns
// configurable regime detection responses.
type stubRegimeLLMProvider struct {
	responses []string
	calls     int
	err       error
}

func (s *stubRegimeLLMProvider) Name() string { return "stub-regime" }

func (s *stubRegimeLLMProvider) Complete(_ context.Context, _ bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	if s.err != nil {
		return bullarc.LLMResponse{}, s.err
	}
	i := s.calls
	s.calls++
	if i < len(s.responses) {
		return bullarc.LLMResponse{Text: s.responses[i]}, nil
	}
	return bullarc.LLMResponse{Text: `{"regime":"low_vol_trending","reasoning":"default"}`}, nil
}

// newRegimeEngine builds an engine with all default indicators, a stub data source,
// and regime detection configured with the given provider.
func newRegimeEngine(bars []bullarc.OHLCV, provider bullarc.LLMProvider, cacheDur time.Duration) *engine.Engine {
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	e.RegisterLLMProvider(provider)
	e.SetRegimeConfig(engine.RegimeConfig{
		Enabled:          true,
		CacheDuration:    cacheDur,
		ATRIndicatorName: "ATR_14",
		BBIndicatorName:  "BB_20_2.0",
	})
	return e
}

// regime20Bars returns 20+ bars with real volatility (spread > 0) suitable for
// computing ATR and Bollinger Bands.
func regime20Bars(n int) []bullarc.OHLCV {
	closes := make([]float64, n)
	for i := range closes {
		closes[i] = 100.0 + float64(i)*0.5
	}
	return makeATRBars(closes, 2.0) // 2.0 spread gives non-trivial ATR
}

// TestRegimeDetection_DisabledByDefault verifies that regime detection is off
// by default and AnalysisResult.Regime is empty.
func TestRegimeDetection_DisabledByDefault(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	provider := &stubRegimeLLMProvider{
		responses: []string{`{"regime":"crisis","reasoning":"test"}`},
	}
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	e.RegisterLLMProvider(provider)
	// Do NOT call SetRegimeConfig — defaults should have Enabled=false.

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	assert.Empty(t, result.Regime, "regime must be empty when detection is disabled")
	assert.Equal(t, 0, provider.calls, "LLM must not be called when regime detection is disabled")
}

// TestRegimeDetection_NoLLMProvider_RegimeEmpty verifies that regime detection
// is skipped when no LLM provider is registered, leaving confidence unmodified.
func TestRegimeDetection_NoLLMProvider_RegimeEmpty(t *testing.T) {
	bars := regime20Bars(100)
	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	e.RegisterDataSource(&stubDataSource{bars: bars})
	// No LLM provider registered.
	e.SetRegimeConfig(engine.RegimeConfig{
		Enabled:          true,
		ATRIndicatorName: "ATR_14",
		BBIndicatorName:  "BB_20_2.0",
	})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	assert.Empty(t, result.Regime, "regime must be empty when no LLM provider is configured")
}

// TestRegimeDetection_SetOnResult verifies that when regime detection is enabled
// and the LLM returns a valid regime, it is stored in AnalysisResult.Regime.
func TestRegimeDetection_SetOnResult(t *testing.T) {
	bars := regime20Bars(100)
	provider := &stubRegimeLLMProvider{
		responses: []string{`{"regime":"high_vol_trending","reasoning":"ATR rising."}`},
	}
	e := newRegimeEngine(bars, provider, time.Hour)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	assert.Equal(t, "high_vol_trending", result.Regime)
}

// TestRegimeDetection_LowVolTrending_NoConfidenceChange verifies that
// low_vol_trending does not change composite signal confidence.
func TestRegimeDetection_LowVolTrending_NoConfidenceChange(t *testing.T) {
	bars := regime20Bars(100)
	provider := &stubRegimeLLMProvider{
		responses: []string{`{"regime":"low_vol_trending","reasoning":"Low vol."}`},
	}
	e := newRegimeEngine(bars, provider, time.Hour)

	// Get baseline confidence without regime detection.
	eBase := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		eBase.RegisterIndicator(ind)
	}
	eBase.RegisterDataSource(&stubDataSource{bars: bars})
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Equal(t, "low_vol_trending", result.Regime)
	// low_vol_trending multiplier is 1.0, so confidence should be unchanged.
	assert.InDelta(t, baseResult.Signals[0].Confidence, result.Signals[0].Confidence, 0.001,
		"low_vol_trending should not change composite confidence")
}

// TestRegimeDetection_HighVolTrending_ReducesConfidence verifies that
// high_vol_trending reduces composite signal confidence by factor 0.8.
func TestRegimeDetection_HighVolTrending_ReducesConfidence(t *testing.T) {
	bars := regime20Bars(100)
	provider := &stubRegimeLLMProvider{
		responses: []string{`{"regime":"high_vol_trending","reasoning":"High vol."}`},
	}
	e := newRegimeEngine(bars, provider, time.Hour)

	// Get baseline confidence without regime.
	eBase := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		eBase.RegisterIndicator(ind)
	}
	eBase.RegisterDataSource(&stubDataSource{bars: bars})
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Equal(t, "high_vol_trending", result.Regime)
	expected := baseResult.Signals[0].Confidence * 0.8
	assert.InDelta(t, expected, result.Signals[0].Confidence, 0.001,
		"high_vol_trending should reduce confidence by 0.8x")
}

// TestRegimeDetection_MeanReverting_ReducesConfidence verifies that
// mean_reverting reduces composite signal confidence by factor 0.7.
func TestRegimeDetection_MeanReverting_ReducesConfidence(t *testing.T) {
	bars := regime20Bars(100)
	provider := &stubRegimeLLMProvider{
		responses: []string{`{"regime":"mean_reverting","reasoning":"Ranging."}`},
	}
	e := newRegimeEngine(bars, provider, time.Hour)

	eBase := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		eBase.RegisterIndicator(ind)
	}
	eBase.RegisterDataSource(&stubDataSource{bars: bars})
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Equal(t, "mean_reverting", result.Regime)
	expected := baseResult.Signals[0].Confidence * 0.7
	assert.InDelta(t, expected, result.Signals[0].Confidence, 0.001,
		"mean_reverting should reduce confidence by 0.7x")
}

// TestRegimeDetection_Crisis_ReducesConfidence verifies that crisis reduces
// composite signal confidence by factor 0.5.
func TestRegimeDetection_Crisis_ReducesConfidence(t *testing.T) {
	bars := regime20Bars(100)
	provider := &stubRegimeLLMProvider{
		responses: []string{`{"regime":"crisis","reasoning":"Extreme vol."}`},
	}
	e := newRegimeEngine(bars, provider, time.Hour)

	eBase := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		eBase.RegisterIndicator(ind)
	}
	eBase.RegisterDataSource(&stubDataSource{bars: bars})
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	assert.Equal(t, "crisis", result.Regime)
	expected := baseResult.Signals[0].Confidence * 0.5
	assert.InDelta(t, expected, result.Signals[0].Confidence, 0.001,
		"crisis should reduce confidence by 0.5x")
}

// TestRegimeDetection_Cache_PreventsDuplicateLLMCall verifies that the second
// Analyze call for the same symbol uses the cached regime rather than calling
// the LLM again.
func TestRegimeDetection_Cache_PreventsDuplicateLLMCall(t *testing.T) {
	bars := regime20Bars(100)
	provider := &stubRegimeLLMProvider{
		responses: []string{
			`{"regime":"crisis","reasoning":"First call."}`,
			`{"regime":"low_vol_trending","reasoning":"Second call — should not happen."}`,
		},
	}
	// Long cache TTL so the second call hits the cache.
	e := newRegimeEngine(bars, provider, 10*time.Minute)

	result1, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Equal(t, "crisis", result1.Regime)

	result2, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Equal(t, "crisis", result2.Regime, "cached regime should be reused")
	assert.Equal(t, 1, provider.calls, "LLM should be called only once when cache is valid")
}

// TestRegimeDetection_Cache_Expiry_CallsLLMAgain verifies that an expired cache
// entry causes a new LLM call on the next Analyze.
func TestRegimeDetection_Cache_Expiry_CallsLLMAgain(t *testing.T) {
	bars := regime20Bars(100)
	provider := &stubRegimeLLMProvider{
		responses: []string{
			`{"regime":"crisis","reasoning":"First call."}`,
			`{"regime":"mean_reverting","reasoning":"Second call after expiry."}`,
		},
	}
	// Very short TTL so the entry expires immediately.
	e := newRegimeEngine(bars, provider, 1*time.Nanosecond)

	result1, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Equal(t, "crisis", result1.Regime)

	// Ensure the entry has expired.
	time.Sleep(2 * time.Millisecond)

	result2, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	assert.Equal(t, "mean_reverting", result2.Regime, "regime should be re-detected after cache expiry")
	assert.Equal(t, 2, provider.calls, "LLM should be called twice after cache expiry")
}

// TestRegimeDetection_InsufficientBars_RegimeEmpty verifies that regime detection
// is skipped when fewer than 20 bars are available, leaving Regime empty.
func TestRegimeDetection_InsufficientBars_RegimeEmpty(t *testing.T) {
	// Only 10 bars — below the 20-bar minimum for regime metrics.
	bars := regime20Bars(10)
	provider := &stubRegimeLLMProvider{
		responses: []string{`{"regime":"crisis","reasoning":"test"}`},
	}
	e := newRegimeEngine(bars, provider, time.Hour)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	assert.Empty(t, result.Regime, "regime must be empty when fewer than 20 bars are available")
	assert.Equal(t, 0, provider.calls, "LLM must not be called when insufficient data")
}

// TestRegimeDetection_LLMFailure_RegimeEmpty verifies that an LLM failure leaves
// Regime empty and does not affect composite signal confidence.
func TestRegimeDetection_LLMFailure_RegimeEmpty(t *testing.T) {
	bars := regime20Bars(100)
	provider := &stubRegimeLLMProvider{
		err: bullarc.ErrLLMUnavailable,
	}
	e := newRegimeEngine(bars, provider, time.Hour)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err, "LLM failure must not propagate to caller")

	assert.Empty(t, result.Regime, "regime must be empty when LLM call fails")
}

// TestRegimeDetection_AllFourRegimes verifies that each valid regime string is
// accepted by the engine and stored correctly in AnalysisResult.Regime.
func TestRegimeDetection_AllFourRegimes(t *testing.T) {
	regimes := []string{"low_vol_trending", "high_vol_trending", "mean_reverting", "crisis"}
	bars := regime20Bars(100)

	for _, regime := range regimes {
		t.Run(regime, func(t *testing.T) {
			provider := &stubRegimeLLMProvider{
				responses: []string{
					`{"regime":"` + regime + `","reasoning":"test"}`,
				},
			}
			e := newRegimeEngine(bars, provider, time.Hour)
			result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "TEST"})
			require.NoError(t, err)
			require.NotEmpty(t, result.Signals)
			assert.Equal(t, regime, result.Regime)
		})
	}
}
