package llm

import (
	"fmt"
	"strings"

	"github.com/bullarc/bullarc"
)

// BacktestPrompt builds a plain English prompt from a BacktestResult,
// asking the LLM to explain the backtest performance covering overall strategy
// results, notable winning and losing periods, and indicator contributions.
func BacktestPrompt(result bullarc.BacktestResult) string {
	var b strings.Builder
	s := result.Summary

	b.WriteString(fmt.Sprintf(
		"You are a financial analyst assistant. Explain the following backtest results for %s in plain English suitable for a retail investor. "+
			"Cover: (1) overall strategy performance, (2) notable winning and losing periods, "+
			"(3) which signals contributed most to the results. Be concise (4-6 sentences).\n\n",
		result.Symbol,
	))

	b.WriteString("=== Backtest Summary ===\n")
	b.WriteString(fmt.Sprintf("Total signals: %d (Buy: %d, Sell: %d, Hold: %d)\n",
		s.TotalSignals, s.BuyCount, s.SellCount, s.HoldCount))
	b.WriteString(fmt.Sprintf("Simulated return: %.2f%%\n", s.SimReturn))
	b.WriteString(fmt.Sprintf("Maximum drawdown: %.2f%%\n", s.MaxDrawdown))
	b.WriteString(fmt.Sprintf("Win rate: %.1f%%\n", s.WinRate))

	if len(result.BarSignals) > 0 {
		b.WriteString("\n=== Signal Timeline (sample) ===\n")
		const maxSample = 10
		signals := result.BarSignals
		var shown []bullarc.BarSignal
		if len(signals) <= maxSample*2 {
			shown = signals
		} else {
			shown = append(shown, signals[:maxSample]...)
			shown = append(shown, signals[len(signals)-maxSample:]...)
		}
		for _, bs := range shown {
			b.WriteString(fmt.Sprintf("  %s: %s (confidence=%.0f%%, close=%.2f)\n",
				bs.Bar.Time.Format("2006-01-02"),
				bs.Signal.Type,
				bs.Signal.Confidence,
				bs.Bar.Close,
			))
		}
	}

	b.WriteString("\nProvide a plain English explanation of the backtest performance.")
	return b.String()
}

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
