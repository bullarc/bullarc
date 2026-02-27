package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bullarc/bullarc"
)

// Valid market regime identifiers returned by DetectRegime.
const (
	// RegimeLowVolTrending indicates low and falling volatility with a clear trend.
	// Wide stops are appropriate and full position size is acceptable.
	RegimeLowVolTrending = "low_vol_trending"
	// RegimeHighVolTrending indicates high or rising volatility with momentum.
	// Momentum strategies work but position size should be reduced.
	RegimeHighVolTrending = "high_vol_trending"
	// RegimeMeanReverting indicates moderate volatility with price oscillating around the mean.
	// Fade extremes with tight stops.
	RegimeMeanReverting = "mean_reverting"
	// RegimeCrisis indicates an extreme volatility spike with large drawdown.
	// Minimal exposure and capital protection are the priority.
	RegimeCrisis = "crisis"
)

// regimeLLMResponse is the expected JSON schema from the regime detection LLM call.
type regimeLLMResponse struct {
	Regime    string `json:"regime"`
	Reasoning string `json:"reasoning"`
}

// RegimeDetectionPrompt builds a prompt asking the LLM to classify the current
// market regime based on volatility metrics.
func RegimeDetectionPrompt(symbol string, atrTrendPct, bbBandwidth, recentDrawdownPct float64) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(
		"You are a quantitative financial analyst specializing in market regime classification.\n"+
			"Classify the current market regime for %s based on the following volatility metrics.\n\n",
		symbol,
	))

	b.WriteString("=== Volatility Metrics ===\n")
	b.WriteString(fmt.Sprintf("ATR Trend (20-period change %%): %.2f%%\n", atrTrendPct))
	b.WriteString(fmt.Sprintf("Bollinger Band Bandwidth: %.4f\n", bbBandwidth))
	b.WriteString(fmt.Sprintf("Recent Drawdown (20-day): %.2f%%\n", recentDrawdownPct))

	b.WriteString(`
=== Regime Definitions ===
- low_vol_trending: Low and falling volatility, clear trend direction (wide stops OK, full position size)
- high_vol_trending: High or rising volatility with momentum (momentum works, reduce size)
- mean_reverting: Moderate volatility, price oscillating around mean (fade extremes, tight stops)
- crisis: Extreme volatility spike, large drawdown, protect capital (minimal exposure)

Based on the volatility metrics above, classify the current regime.
Respond with ONLY a JSON object in this exact format:
{"regime": "low_vol_trending|high_vol_trending|mean_reverting|crisis", "reasoning": "1-2 sentence explanation"}

- regime: exactly one of: "low_vol_trending", "high_vol_trending", "mean_reverting", "crisis"
- reasoning: 1-2 sentences explaining the classification based on the metrics`)

	return b.String()
}

// parseRegimeResponse parses the LLM JSON response into a regime string.
// Returns ("", false) on invalid JSON or unrecognised regime value.
func parseRegimeResponse(text string) (string, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end <= start {
		slog.Warn("no JSON object found in regime detection response")
		return "", false
	}
	text = text[start : end+1]

	var raw regimeLLMResponse
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		slog.Warn("failed to parse regime detection LLM response", "err", err)
		return "", false
	}

	switch raw.Regime {
	case RegimeLowVolTrending, RegimeHighVolTrending, RegimeMeanReverting, RegimeCrisis:
		return raw.Regime, true
	default:
		slog.Warn("unrecognised regime in LLM response", "regime", raw.Regime)
		return "", false
	}
}

// DetectRegime calls the LLM provider to classify the current market regime
// based on volatility metrics. Returns ("", false) if the LLM call fails or
// returns an unrecognised regime value.
func DetectRegime(
	ctx context.Context,
	symbol string,
	atrTrendPct float64,
	bbBandwidth float64,
	recentDrawdownPct float64,
	provider bullarc.LLMProvider,
) (string, bool) {
	prompt := RegimeDetectionPrompt(symbol, atrTrendPct, bbBandwidth, recentDrawdownPct)
	resp, err := provider.Complete(ctx, bullarc.LLMRequest{
		Prompt:    prompt,
		MaxTokens: 256,
	})
	if err != nil {
		slog.Warn("regime detection LLM call failed", "symbol", symbol, "err", err)
		return "", false
	}
	return parseRegimeResponse(resp.Text)
}
