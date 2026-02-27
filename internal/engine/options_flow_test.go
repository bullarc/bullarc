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

// stubOptionsSource is a test double for bullarc.OptionsSource.
type stubOptionsSource struct {
	events []bullarc.OptionsActivity
	err    error
}

func (s *stubOptionsSource) FetchOptionsActivity(_ context.Context, _ string, _ bullarc.OptionsConfig) ([]bullarc.OptionsActivity, error) {
	return s.events, s.err
}

// makeCallActivity returns a single call-side unusual options event.
func makeCallActivity(symbol string, premium float64) bullarc.OptionsActivity {
	return bullarc.OptionsActivity{
		Symbol:       symbol,
		Strike:       150.0,
		Expiration:   time.Date(2024, 6, 21, 0, 0, 0, 0, time.UTC),
		Direction:    "call",
		Volume:       500,
		OpenInterest: 100,
		Premium:      premium,
		ActivityType: bullarc.OptionsActivityBlock,
	}
}

// makePutActivity returns a single put-side unusual options event.
func makePutActivity(symbol string, premium float64) bullarc.OptionsActivity {
	return bullarc.OptionsActivity{
		Symbol:       symbol,
		Strike:       145.0,
		Expiration:   time.Date(2024, 6, 21, 0, 0, 0, 0, time.UTC),
		Direction:    "put",
		Volume:       600,
		OpenInterest: 100,
		Premium:      premium,
		ActivityType: bullarc.OptionsActivitySweep,
	}
}

// newEngineWithBarsAndOptions builds an Engine with all default indicators,
// a stub data source, and a stub options source.
func newEngineWithBarsAndOptions(bars []bullarc.OHLCV, opts *stubOptionsSource) *engine.Engine {
	e := newEngineWithBars(bars)
	e.RegisterOptionsSource(opts)
	return e
}

// TestAnalyze_OptionsFlow_NilSource_NoOptionsSignal verifies that without a
// registered options source the analysis still completes normally.
func TestAnalyze_OptionsFlow_NilSource_NoOptionsSignal(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	// No options_activity signal in the result.
	for _, s := range result.Signals {
		assert.NotEqual(t, "options_activity", s.Indicator,
			"options_activity signal must not appear when no source is registered")
	}
}

// TestAnalyze_OptionsFlow_NoActivity_SignalOmitted verifies that when the
// options source returns no unusual events the signal is omitted from aggregation.
func TestAnalyze_OptionsFlow_NoActivity_SignalOmitted(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	src := &stubOptionsSource{events: nil}
	e := newEngineWithBarsAndOptions(bars, src)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	for _, s := range result.Signals {
		assert.NotEqual(t, "options_activity", s.Indicator,
			"options_activity signal must be omitted when no events are returned")
	}
}

// TestAnalyze_OptionsFlow_CallDominant_BuySignal verifies that call-dominant
// unusual activity produces a BUY options_activity signal in the result.
func TestAnalyze_OptionsFlow_CallDominant_BuySignal(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	src := &stubOptionsSource{
		events: []bullarc.OptionsActivity{
			makeCallActivity("AAPL", 300_000),
			makePutActivity("AAPL", 100_000),
		},
	}
	e := newEngineWithBarsAndOptions(bars, src)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	var optSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "options_activity" {
			optSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, optSig, "options_activity signal must be present in result")
	assert.Equal(t, bullarc.SignalBuy, optSig.Type)
	assert.Equal(t, "AAPL", optSig.Symbol)
	assert.InDelta(t, 75.0, optSig.Confidence, 0.01) // 300k/400k = 75%
}

// TestAnalyze_OptionsFlow_PutDominant_SellSignal verifies that put-dominant
// unusual activity produces a SELL options_activity signal in the result.
func TestAnalyze_OptionsFlow_PutDominant_SellSignal(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	src := &stubOptionsSource{
		events: []bullarc.OptionsActivity{
			makePutActivity("TSLA", 500_000),
			makeCallActivity("TSLA", 100_000),
		},
	}
	e := newEngineWithBars(bars)
	e.RegisterOptionsSource(src)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "TSLA"})
	require.NoError(t, err)

	var optSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "options_activity" {
			optSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, optSig)
	assert.Equal(t, bullarc.SignalSell, optSig.Type)
	assert.InDelta(t, 83.333, optSig.Confidence, 0.01) // 500k/600k
}

// TestAnalyze_OptionsFlow_ParticipatesInAggregate verifies that the options
// signal contributes to the composite aggregation result.
func TestAnalyze_OptionsFlow_ParticipatesInAggregate(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)

	// Run baseline without options source.
	eBase := newEngineWithBars(bars)
	baseResult, err := eBase.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, baseResult.Signals)

	// Run with a strong BUY options signal.
	src := &stubOptionsSource{
		events: []bullarc.OptionsActivity{
			makeCallActivity("AAPL", 1_000_000),
		},
	}
	e := newEngineWithBarsAndOptions(bars, src)
	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)
	require.NotEmpty(t, result.Signals)

	// The composite (first signal) should be produced and options signal present.
	composite := result.Signals[0]
	assert.Equal(t, "composite", composite.Indicator)

	var found bool
	for _, s := range result.Signals {
		if s.Indicator == "options_activity" {
			found = true
			break
		}
	}
	assert.True(t, found, "options_activity signal must appear in result signals")
}

// TestAnalyze_OptionsFlow_WeightScalesConfidence verifies that the options flow
// weight multiplier is applied to the signal confidence before aggregation.
func TestAnalyze_OptionsFlow_WeightScalesConfidence(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	src := &stubOptionsSource{
		events: []bullarc.OptionsActivity{
			makeCallActivity("AAPL", 300_000),
			makePutActivity("AAPL", 100_000),
		},
	}

	// Weight 2.0 should double the raw confidence of 75 to 100 (capped).
	e := newEngineWithBarsAndOptions(bars, src)
	e.SetOptionsFlowWeight(2.0)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	var optSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "options_activity" {
			optSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, optSig)
	// 75 * 2 = 150 → capped at 100.
	assert.InDelta(t, 100.0, optSig.Confidence, 0.01)
}

// TestAnalyze_OptionsFlow_HalfWeight verifies that weight < 1.0 reduces confidence.
func TestAnalyze_OptionsFlow_HalfWeight(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	src := &stubOptionsSource{
		events: []bullarc.OptionsActivity{
			makeCallActivity("AAPL", 200_000),
			makePutActivity("AAPL", 200_000),
		},
	}

	// Raw confidence is 50 (equal call/put) → 0.5 weight → 25.
	e := newEngineWithBarsAndOptions(bars, src)
	e.SetOptionsFlowWeight(0.5)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	var optSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "options_activity" {
			optSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, optSig)
	assert.InDelta(t, 25.0, optSig.Confidence, 0.01)
}

// TestAnalyze_OptionsFlow_FetchError_SignalOmitted verifies that a fetch error
// is treated as a non-fatal event and the signal is simply omitted.
func TestAnalyze_OptionsFlow_FetchError_SignalOmitted(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	src := &stubOptionsSource{err: bullarc.ErrNotConfigured}
	e := newEngineWithBarsAndOptions(bars, src)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err, "fetch error must not propagate to caller")
	require.NotEmpty(t, result.Signals)

	for _, s := range result.Signals {
		assert.NotEqual(t, "options_activity", s.Indicator)
	}
}

// TestAnalyze_OptionsFlow_SetOptionsConfig verifies that SetOptionsConfig is
// wired through: the config is accepted without panicking.
func TestAnalyze_OptionsFlow_SetOptionsConfig(t *testing.T) {
	bars := trendingBars(100, 100, 1.0)
	src := &stubOptionsSource{
		events: []bullarc.OptionsActivity{makeCallActivity("AAPL", 200_000)},
	}
	e := newEngineWithBarsAndOptions(bars, src)
	e.SetOptionsConfig(bullarc.OptionsConfig{
		PremiumThreshold:   50_000,
		HistoricalPCRatios: []float64{1.0, 1.0, 1.0},
	})

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	var found bool
	for _, s := range result.Signals {
		if s.Indicator == "options_activity" {
			found = true
			break
		}
	}
	assert.True(t, found, "options_activity signal expected")
}
