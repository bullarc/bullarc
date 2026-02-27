package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bullarc/bullarc"
)

// metaSignalLLMResponse is the expected JSON schema from the LLM meta-signal call.
type metaSignalLLMResponse struct {
	Signal     string `json:"signal"`
	Confidence int    `json:"confidence"`
	Reasoning  string `json:"reasoning"`
}

// MetaSignalPrompt builds a prompt asking the LLM to synthesize all indicator
// values and the preliminary composite signal into a structured BUY/SELL/HOLD
// assessment. The current price of the symbol is included as context.
func MetaSignalPrompt(symbol string, indicatorValues map[string][]bullarc.IndicatorValue, composite bullarc.Signal, currentPrice float64) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(
		"You are a quantitative financial analyst. Synthesize the following technical analysis for %s into a single structured trading signal.\n\n",
		symbol,
	))
	b.WriteString(fmt.Sprintf("Current price: %.4f\n\n", currentPrice))

	b.WriteString("=== Indicator Values (latest) ===\n")
	for name, values := range indicatorValues {
		if len(values) == 0 {
			continue
		}
		latest := values[len(values)-1]
		if len(latest.Extra) > 0 {
			b.WriteString(fmt.Sprintf("  %s: %.4f", name, latest.Value))
			for k, v := range latest.Extra {
				b.WriteString(fmt.Sprintf(", %s=%.4f", k, v))
			}
			b.WriteString("\n")
		} else {
			b.WriteString(fmt.Sprintf("  %s: %.4f\n", name, latest.Value))
		}
	}

	b.WriteString("\n=== Technical Composite Signal ===\n")
	b.WriteString(fmt.Sprintf("Direction: %s\n", composite.Type))
	b.WriteString(fmt.Sprintf("Confidence: %.0f%%\n", composite.Confidence))
	b.WriteString(fmt.Sprintf("Summary: %s\n", composite.Explanation))

	b.WriteString(`
Based on the above technical analysis, provide your meta-assessment as a structured JSON object.
Respond with ONLY a JSON object in this exact format:
{"signal": "BUY|SELL|HOLD", "confidence": 0-100, "reasoning": "2-3 sentence explanation"}

- signal: "BUY", "SELL", or "HOLD"
- confidence: integer 0-100 reflecting your conviction
- reasoning: 2-3 sentences synthesizing the key signals and your rationale`)

	return b.String()
}

// parseMetaSignalResponse parses the LLM JSON response into a bullarc.Signal.
// On invalid JSON or unrecognised signal type, it returns (zero, false) and logs a warning.
func parseMetaSignalResponse(symbol, text string) (bullarc.Signal, bool) {
	// Extract JSON object from surrounding text (LLMs often add preamble).
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end > start {
		text = text[start : end+1]
	}

	var raw metaSignalLLMResponse
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		slog.Warn("failed to parse LLM meta-signal response", "symbol", symbol, "err", err)
		return bullarc.Signal{}, false
	}

	sigType := bullarc.SignalType(raw.Signal)
	switch sigType {
	case bullarc.SignalBuy, bullarc.SignalSell, bullarc.SignalHold:
	default:
		slog.Warn("invalid signal type in LLM meta-signal response",
			"symbol", symbol, "signal", raw.Signal)
		return bullarc.Signal{}, false
	}

	confidence := float64(raw.Confidence)
	if confidence < 0 {
		confidence = 0
	} else if confidence > 100 {
		confidence = 100
	}

	return bullarc.Signal{
		Type:        sigType,
		Confidence:  confidence,
		Indicator:   "LLMMetaSignal",
		Symbol:      symbol,
		Timestamp:   time.Now(),
		Explanation: raw.Reasoning,
	}, true
}

// GenerateMetaSignal calls the LLM provider to synthesize all indicator values
// and the preliminary composite signal into a structured BUY/SELL/HOLD assessment.
// Returns (zero Signal, false) if the LLM call fails or returns an invalid response.
func GenerateMetaSignal(
	ctx context.Context,
	symbol string,
	indicatorValues map[string][]bullarc.IndicatorValue,
	composite bullarc.Signal,
	currentPrice float64,
	provider bullarc.LLMProvider,
) (bullarc.Signal, bool) {
	prompt := MetaSignalPrompt(symbol, indicatorValues, composite, currentPrice)
	resp, err := provider.Complete(ctx, bullarc.LLMRequest{
		Prompt:    prompt,
		MaxTokens: 512,
	})
	if err != nil {
		slog.Warn("LLM meta-signal call failed", "symbol", symbol, "err", err)
		return bullarc.Signal{}, false
	}
	return parseMetaSignalResponse(symbol, resp.Text)
}
