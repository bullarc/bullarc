package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/bullarc/bullarc"
)

// anomalyLLMResponse is the expected JSON schema from the anomaly detection LLM call.
type anomalyLLMResponse struct {
	Anomalies []anomalyItem `json:"anomalies"`
}

type anomalyItem struct {
	Type               string   `json:"type"`
	Description        string   `json:"description"`
	Severity           string   `json:"severity"`
	AffectedIndicators []string `json:"affected_indicators"`
}

// AnomalyDetectionPrompt builds a prompt asking the LLM to identify divergences
// and anomalies in the last 30 days of historical indicator values for a symbol.
func AnomalyDetectionPrompt(symbol string, indicatorValues map[string][]bullarc.IndicatorValue) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(
		"You are a quantitative financial analyst specializing in divergence and anomaly detection.\n"+
			"Analyze the following daily indicator history for %s and identify any divergences or anomalies.\n\n",
		symbol,
	))

	b.WriteString("Look for:\n")
	b.WriteString("- Bearish divergence: price trending up while RSI or MACD trending down\n")
	b.WriteString("- Bullish divergence: price trending down while RSI or MACD trending up\n")
	b.WriteString("- Volatility squeeze: Bollinger Band bandwidth narrowing significantly\n")
	b.WriteString("- Volume anomaly: volume expanding while price remains flat\n")
	b.WriteString("- Any other unusual patterns or regime changes\n\n")

	b.WriteString("=== Indicator History (last 30 days) ===\n")

	// Sort indicator names for deterministic prompt output.
	names := make([]string, 0, len(indicatorValues))
	for name := range indicatorValues {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		values := indicatorValues[name]
		if len(values) == 0 {
			continue
		}
		// Limit to last 30 entries.
		start := len(values) - 30
		if start < 0 {
			start = 0
		}
		window := values[start:]

		b.WriteString(fmt.Sprintf("\n%s:\n", name))
		for _, v := range window {
			line := fmt.Sprintf("  %s: %.4f", v.Time.Format("2006-01-02"), v.Value)
			for k, ev := range v.Extra {
				line += fmt.Sprintf(", %s=%.4f", k, ev)
			}
			b.WriteString(line + "\n")
		}
	}

	b.WriteString(`
Respond with ONLY a JSON object in this exact format:
{"anomalies":[{"type":"string","description":"string","severity":"low|medium|high","affected_indicators":["string"]}]}
If no anomalies are detected, return {"anomalies":[]}.
- type: short identifier for the anomaly (e.g. "bearish_divergence", "volatility_squeeze", "volume_anomaly")
- description: 1-2 sentence description of the anomaly
- severity: "low", "medium", or "high"
- affected_indicators: list of indicator names involved`)

	return b.String()
}

// parseAnomalyResponse parses the LLM JSON response into a slice of bullarc.Anomaly.
// Returns (nil, false) on invalid JSON. Unrecognised severity values default to "low".
func parseAnomalyResponse(text string) ([]bullarc.Anomaly, bool) {
	// Extract JSON object from surrounding text (LLMs often add preamble).
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end <= start {
		slog.Warn("no JSON object found in anomaly detection response")
		return nil, false
	}
	text = text[start : end+1]

	var raw anomalyLLMResponse
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		slog.Warn("failed to parse anomaly detection LLM response", "err", err)
		return nil, false
	}

	anomalies := make([]bullarc.Anomaly, 0, len(raw.Anomalies))
	for _, item := range raw.Anomalies {
		severity := bullarc.AnomalySeverity(item.Severity)
		switch severity {
		case bullarc.AnomalySeverityLow, bullarc.AnomalySeverityMedium, bullarc.AnomalySeverityHigh:
		default:
			slog.Warn("unrecognised anomaly severity, defaulting to low",
				"severity", item.Severity, "type", item.Type)
			severity = bullarc.AnomalySeverityLow
		}

		indicators := item.AffectedIndicators
		if indicators == nil {
			indicators = []string{}
		}

		anomalies = append(anomalies, bullarc.Anomaly{
			Type:               item.Type,
			Description:        item.Description,
			Severity:           severity,
			AffectedIndicators: indicators,
		})
	}
	return anomalies, true
}

// DetectAnomalies calls the LLM provider to identify divergences and anomalies
// in the historical indicator values. It uses the last 30 data points of each
// indicator. Returns (nil, false) if the LLM call fails or returns invalid JSON.
func DetectAnomalies(
	ctx context.Context,
	symbol string,
	indicatorValues map[string][]bullarc.IndicatorValue,
	provider bullarc.LLMProvider,
) ([]bullarc.Anomaly, bool) {
	prompt := AnomalyDetectionPrompt(symbol, indicatorValues)
	resp, err := provider.Complete(ctx, bullarc.LLMRequest{
		Prompt:    prompt,
		MaxTokens: 1024,
	})
	if err != nil {
		slog.Warn("anomaly detection LLM call failed", "symbol", symbol, "err", err)
		return nil, false
	}
	return parseAnomalyResponse(resp.Text)
}
