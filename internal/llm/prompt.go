package llm

import (
	"fmt"
	"strings"

	"github.com/bullarc/bullarc"
)

// AnalysisPrompt builds a plain English prompt from an AnalysisResult,
// asking the LLM to summarise the technical signals in plain language.
func AnalysisPrompt(result bullarc.AnalysisResult) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("You are a financial analyst assistant. Explain the following technical analysis for %s in plain English suitable for a retail investor. Be concise (2-4 sentences).\n\n", result.Symbol))

	if len(result.Signals) > 0 {
		composite := result.Signals[0]
		b.WriteString(fmt.Sprintf("Overall signal: %s (confidence %.0f%%)\n", composite.Type, composite.Confidence))
		b.WriteString(fmt.Sprintf("Summary: %s\n", composite.Explanation))
	}

	if len(result.Signals) > 1 {
		b.WriteString("\nIndicator signals:\n")
		for _, s := range result.Signals[1:] {
			b.WriteString(fmt.Sprintf("- %s: %s (confidence %.0f%%)\n", s.Indicator, s.Type, s.Confidence))
		}
	}

	b.WriteString("\nProvide a plain English explanation of what these signals mean for the stock.")
	return b.String()
}
