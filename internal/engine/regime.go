package engine

import (
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/llm"
)

// DefaultRegimeCacheDuration is the default duration for which a regime
// classification is cached before triggering a new LLM call.
const DefaultRegimeCacheDuration = time.Hour

// RegimeConfig controls LLM-based market regime detection and composite signal
// confidence adjustment.
type RegimeConfig struct {
	// Enabled activates regime detection. When false, regime detection is
	// skipped and AnalysisResult.Regime is left empty.
	Enabled bool `json:"enabled" yaml:"enabled"`
	// CacheDuration controls how long a regime classification is reused before
	// making another LLM call. Defaults to DefaultRegimeCacheDuration when zero.
	CacheDuration time.Duration `json:"cache_duration" yaml:"cache_duration"`
	// BBIndicatorName is the name of the Bollinger Bands indicator whose bandwidth
	// values are used for regime metrics. Defaults to "BB_20_2.0" when empty.
	BBIndicatorName string `json:"bb_indicator_name" yaml:"bb_indicator_name"`
	// ATRIndicatorName is the name of the ATR indicator whose values are used
	// for ATR trend computation. Defaults to "ATR_14" when empty.
	ATRIndicatorName string `json:"atr_indicator_name" yaml:"atr_indicator_name"`
}

// defaultRegimeConfig returns a RegimeConfig with standard defaults (disabled).
func defaultRegimeConfig() RegimeConfig {
	return RegimeConfig{
		Enabled:          false,
		CacheDuration:    DefaultRegimeCacheDuration,
		BBIndicatorName:  "BB_20_2.0",
		ATRIndicatorName: "ATR_14",
	}
}

// regimeCacheEntry holds a cached regime and its expiry time.
type regimeCacheEntry struct {
	regime    string
	expiresAt time.Time
}

// regimeCache is a thread-safe per-symbol cache for regime classifications.
type regimeCache struct {
	mu      sync.Mutex
	entries map[string]regimeCacheEntry
}

// newRegimeCache creates an empty regime cache.
func newRegimeCache() *regimeCache {
	return &regimeCache{entries: make(map[string]regimeCacheEntry)}
}

// get returns the cached regime for symbol if the entry exists and has not expired.
// Returns ("", false) when no valid cache entry exists.
func (c *regimeCache) get(symbol string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[symbol]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.regime, true
}

// set stores a regime classification for symbol with the given TTL.
func (c *regimeCache) set(symbol, regime string, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[symbol] = regimeCacheEntry{
		regime:    regime,
		expiresAt: time.Now().Add(ttl),
	}
}

// applyRegimeMultiplier scales composite signal confidence based on the detected
// regime and clamps the result to [0, 100].
//
// Multipliers:
//   - low_vol_trending:  1.0 (no change)
//   - high_vol_trending: 0.8
//   - mean_reverting:    0.7
//   - crisis:            0.5
func applyRegimeMultiplier(regime string, confidence float64) float64 {
	var mult float64
	switch regime {
	case llm.RegimeLowVolTrending:
		mult = 1.0
	case llm.RegimeHighVolTrending:
		mult = 0.8
	case llm.RegimeMeanReverting:
		mult = 0.7
	case llm.RegimeCrisis:
		mult = 0.5
	default:
		return confidence
	}
	result := confidence * mult
	if result < 0 {
		result = 0
	}
	if result > 100 {
		result = 100
	}
	return result
}

// volatilityMetrics holds the three metrics sent to the LLM for regime detection.
type volatilityMetrics struct {
	atrTrendPct       float64
	bbBandwidth       float64
	recentDrawdownPct float64
}

// regimeWindowSize is the number of bars/values used for rolling metric computation.
const regimeWindowSize = 20

// computeVolatilityMetrics derives ATR trend, Bollinger bandwidth, and recent
// drawdown from computed indicator values and raw bars.
//
// Returns (zero, false) when fewer than regimeWindowSize bars are available.
func computeVolatilityMetrics(
	bars []bullarc.OHLCV,
	indicatorValues map[string][]bullarc.IndicatorValue,
	atrName, bbName string,
) (volatilityMetrics, bool) {
	if len(bars) < regimeWindowSize {
		return volatilityMetrics{}, false
	}

	// ── ATR trend ─────────────────────────────────────────────────────────────
	// Percentage change of ATR over the most recent regimeWindowSize data points.
	atrTrendPct := 0.0
	if atrVals, ok := indicatorValues[atrName]; ok && len(atrVals) >= 2 {
		window := atrVals
		if len(window) > regimeWindowSize {
			window = window[len(window)-regimeWindowSize:]
		}
		first := window[0].Value
		last := window[len(window)-1].Value
		if first > 0 {
			atrTrendPct = (last - first) / first * 100
		}
	}

	// ── Bollinger Bandwidth ───────────────────────────────────────────────────
	// Average bandwidth from the most recent regimeWindowSize BB values.
	bbBandwidth := 0.0
	if bbVals, ok := indicatorValues[bbName]; ok && len(bbVals) > 0 {
		window := bbVals
		if len(window) > regimeWindowSize {
			window = window[len(window)-regimeWindowSize:]
		}
		var sumBW float64
		for _, v := range window {
			if bw, ok := v.Extra["bandwidth"]; ok {
				sumBW += bw
			}
		}
		bbBandwidth = sumBW / float64(len(window))
	}

	// ── Recent Drawdown ───────────────────────────────────────────────────────
	// Max drawdown from peak over the last regimeWindowSize bars.
	recentBars := bars
	if len(recentBars) > regimeWindowSize {
		recentBars = recentBars[len(recentBars)-regimeWindowSize:]
	}
	peak := recentBars[0].High
	maxDD := 0.0
	for _, b := range recentBars {
		if b.High > peak {
			peak = b.High
		}
		if peak > 0 {
			dd := (peak - b.Low) / peak * 100
			if dd > maxDD {
				maxDD = dd
			}
		}
	}

	// Guard against NaN/Inf from degenerate inputs.
	if math.IsNaN(atrTrendPct) || math.IsInf(atrTrendPct, 0) {
		atrTrendPct = 0
	}
	if math.IsNaN(bbBandwidth) || math.IsInf(bbBandwidth, 0) {
		bbBandwidth = 0
	}
	if math.IsNaN(maxDD) || math.IsInf(maxDD, 0) {
		maxDD = 0
	}

	slog.Info("volatility metrics computed",
		"atr_trend_pct", atrTrendPct,
		"bb_bandwidth", bbBandwidth,
		"recent_drawdown_pct", maxDD)

	return volatilityMetrics{
		atrTrendPct:       atrTrendPct,
		bbBandwidth:       bbBandwidth,
		recentDrawdownPct: maxDD,
	}, true
}
