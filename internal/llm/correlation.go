package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/bullarc/bullarc"
)

// correlationLLMResponse is the expected JSON schema from the correlation check LLM call.
type correlationLLMResponse struct {
	Correlated bool   `json:"correlated"`
	Overlap    string `json:"overlap"`
	Reasoning  string `json:"reasoning"`
}

// CorrelationCheckPrompt builds a prompt asking the LLM whether adding symbol
// to the given portfolio adds diversification or duplicates existing exposure.
func CorrelationCheckPrompt(symbol string, portfolio []string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(
		"You are a portfolio risk analyst specializing in correlation and diversification.\n"+
			"Assess whether adding %s to the following portfolio adds diversification or duplicates existing exposure.\n\n",
		symbol,
	))

	b.WriteString("=== Current Portfolio ===\n")
	for _, s := range portfolio {
		b.WriteString(fmt.Sprintf("- %s\n", s))
	}

	b.WriteString(fmt.Sprintf("\n=== New Symbol to Evaluate ===\n%s\n", symbol))

	b.WriteString(`
Assess the correlation and sector/asset-class overlap between the new symbol and the existing portfolio.
Respond with ONLY a JSON object in this exact format:
{"correlated": true|false, "overlap": "high|medium|low", "reasoning": "..."}

- correlated: true if the new position significantly duplicates existing exposure, false if it adds diversification
- overlap: "high" if strongly correlated with portfolio holdings, "medium" if moderately correlated, "low" if largely independent
- reasoning: 1-2 sentences explaining the correlation assessment`)

	return b.String()
}

// parseCorrelationResponse parses the LLM JSON response into correlation fields.
// Returns (false, "", false) on invalid JSON or unrecognised overlap value.
func parseCorrelationResponse(text string) (correlated bool, overlap string, ok bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end <= start {
		slog.Warn("no JSON object found in correlation check response")
		return false, "", false
	}
	text = text[start : end+1]

	var raw correlationLLMResponse
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		slog.Warn("failed to parse correlation check LLM response", "err", err)
		return false, "", false
	}

	switch raw.Overlap {
	case "high", "medium", "low":
	default:
		slog.Warn("unrecognised overlap in correlation check response", "overlap", raw.Overlap)
		return false, "", false
	}

	return raw.Correlated, raw.Overlap, true
}

// CheckCorrelation calls the LLM provider to assess whether adding symbol to
// portfolio duplicates existing exposure. Returns (false, "", false) if the LLM
// call fails or returns an unrecognised response.
func CheckCorrelation(
	ctx context.Context,
	symbol string,
	portfolio []string,
	provider bullarc.LLMProvider,
) (correlated bool, overlap string, ok bool) {
	prompt := CorrelationCheckPrompt(symbol, portfolio)
	resp, err := provider.Complete(ctx, bullarc.LLMRequest{
		Prompt:    prompt,
		MaxTokens: 256,
	})
	if err != nil {
		slog.Warn("correlation check LLM call failed", "symbol", symbol, "err", err)
		return false, "", false
	}
	return parseCorrelationResponse(resp.Text)
}
