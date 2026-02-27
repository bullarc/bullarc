package engine

import (
	"context"
	"log/slog"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/llm"
)

// RiskFlagHighCorrelation is the risk flag attached to the composite signal
// when the new position is highly correlated with the existing portfolio.
const RiskFlagHighCorrelation = "high_correlation"

// CorrelationConfig controls LLM-based portfolio correlation checking.
type CorrelationConfig struct {
	// Enabled activates portfolio correlation checking. When false, the check
	// is skipped regardless of whether a portfolio is provided in the request.
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// applyCorrelationRiskFlag attaches a "high_correlation" risk flag to sig when
// overlap is "high". The signal direction and confidence are never modified.
func applyCorrelationRiskFlag(sig bullarc.Signal, overlap string) bullarc.Signal {
	if overlap != "high" {
		return sig
	}
	flags := make([]string, len(sig.RiskFlags), len(sig.RiskFlags)+1)
	copy(flags, sig.RiskFlags)
	flags = append(flags, RiskFlagHighCorrelation)
	sig.RiskFlags = flags
	return sig
}

// checkPortfolioCorrelation runs the LLM correlation check when all preconditions
// are met and, if high overlap is detected, attaches the "high_correlation" risk
// flag to the composite signal. Returns the (possibly modified) composite signal.
func (e *Engine) checkPortfolioCorrelation(
	ctx context.Context,
	symbol string,
	portfolio []string,
	composite bullarc.Signal,
	snap engineSnapshot,
) bullarc.Signal {
	if !snap.correlationConfig.Enabled {
		return composite
	}
	if len(portfolio) == 0 {
		return composite
	}
	if snap.llmProvider == nil {
		return composite
	}

	correlated, overlap, ok := llm.CheckCorrelation(ctx, symbol, portfolio, snap.llmProvider)
	if !ok {
		slog.Warn("correlation check failed, skipping risk flag", "symbol", symbol)
		return composite
	}

	slog.Info("correlation check complete",
		"symbol", symbol,
		"portfolio_size", len(portfolio),
		"correlated", correlated,
		"overlap", overlap)

	return applyCorrelationRiskFlag(composite, overlap)
}
