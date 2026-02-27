package signal_test

import (
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/signal"
	"github.com/stretchr/testify/assert"
)

func baseComposite(t bullarc.SignalType, confidence float64) bullarc.Signal {
	return bullarc.Signal{
		Type:        t,
		Confidence:  confidence,
		Indicator:   "composite",
		Symbol:      "AAPL",
		Timestamp:   time.Now(),
		Explanation: "test",
	}
}

// TestApplySocialRiskFlag_NotElevated verifies that a non-elevated symbol
// leaves the signal unchanged.
func TestApplySocialRiskFlag_NotElevated(t *testing.T) {
	sig := baseComposite(bullarc.SignalBuy, 75.0)
	got := signal.ApplySocialRiskFlag(sig, false, 10)
	assert.Equal(t, bullarc.SignalBuy, got.Type)
	assert.Equal(t, 75.0, got.Confidence)
	assert.Empty(t, got.RiskFlags)
}

// TestApplySocialRiskFlag_Elevated verifies that an elevated symbol attaches
// the risk flag and reduces confidence by the configured percentage.
func TestApplySocialRiskFlag_Elevated(t *testing.T) {
	sig := baseComposite(bullarc.SignalBuy, 80.0)
	got := signal.ApplySocialRiskFlag(sig, true, 10)
	assert.Equal(t, bullarc.SignalBuy, got.Type, "direction must not change")
	assert.Contains(t, got.RiskFlags, signal.RiskFlagElevatedSocialAttention)
	assert.InDelta(t, 72.0, got.Confidence, 0.001, "80 * 0.9 = 72")
}

// TestApplySocialRiskFlag_ZeroPenalty verifies that a zero penalty still
// attaches the flag but does not reduce confidence.
func TestApplySocialRiskFlag_ZeroPenalty(t *testing.T) {
	sig := baseComposite(bullarc.SignalSell, 60.0)
	got := signal.ApplySocialRiskFlag(sig, true, 0)
	assert.Contains(t, got.RiskFlags, signal.RiskFlagElevatedSocialAttention)
	assert.Equal(t, 60.0, got.Confidence)
}

// TestApplySocialRiskFlag_DirectionPreserved verifies the signal type is
// unchanged for all three signal types.
func TestApplySocialRiskFlag_DirectionPreserved(t *testing.T) {
	for _, st := range []bullarc.SignalType{bullarc.SignalBuy, bullarc.SignalSell, bullarc.SignalHold} {
		sig := baseComposite(st, 70.0)
		got := signal.ApplySocialRiskFlag(sig, true, 10)
		assert.Equal(t, st, got.Type, "type must not change for %s", st)
	}
}

// TestApplySocialRiskFlag_ExistingFlagsPreserved verifies that pre-existing
// risk flags are retained when a new one is added.
func TestApplySocialRiskFlag_ExistingFlagsPreserved(t *testing.T) {
	sig := baseComposite(bullarc.SignalBuy, 70.0)
	sig.RiskFlags = []string{"some_other_flag"}
	got := signal.ApplySocialRiskFlag(sig, true, 10)
	assert.Contains(t, got.RiskFlags, "some_other_flag")
	assert.Contains(t, got.RiskFlags, signal.RiskFlagElevatedSocialAttention)
}

// TestApplySocialRiskFlag_ConfidenceClampedAtZero verifies that even with a
// 100% penalty the confidence cannot go below zero.
func TestApplySocialRiskFlag_ConfidenceClampedAtZero(t *testing.T) {
	sig := baseComposite(bullarc.SignalBuy, 50.0)
	got := signal.ApplySocialRiskFlag(sig, true, 200)
	assert.GreaterOrEqual(t, got.Confidence, 0.0)
}
