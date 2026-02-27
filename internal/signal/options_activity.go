package signal

import (
	"fmt"
	"time"

	"github.com/bullarc/bullarc"
)

const (
	// RiskFlagUnusualCallActivity is set when unusual call-side options activity is detected.
	RiskFlagUnusualCallActivity = "unusual_call_activity"
	// RiskFlagUnusualPutActivity is set when unusual put-side options activity is detected.
	RiskFlagUnusualPutActivity = "unusual_put_activity"
)

// OptionsActivitySignal generates a trading signal from a slice of unusual options activity events.
//
// Rules:
//   - Empty events → nil (no signal).
//   - More call premium than put premium → BUY (institutional call buying is bullish).
//   - More put premium than call premium → SELL (institutional put buying is bearish).
//   - Equal premium on both sides → HOLD.
//
// Confidence scales with the directional dominance of premium (0–100).
// The symbol is taken directly from the first event; all events are assumed
// to be for the same underlying.
func OptionsActivitySignal(events []bullarc.OptionsActivity) *bullarc.Signal {
	if len(events) == 0 {
		return nil
	}

	var callPremium, putPremium float64
	var sweepCount, blockCount, unusualVolCount int
	for _, e := range events {
		switch e.Direction {
		case "call":
			callPremium += e.Premium
		case "put":
			putPremium += e.Premium
		}
		switch e.ActivityType {
		case bullarc.OptionsActivitySweep:
			sweepCount++
		case bullarc.OptionsActivityBlock:
			blockCount++
		case bullarc.OptionsActivityUnusualVolume:
			unusualVolCount++
		}
	}

	total := callPremium + putPremium
	if total == 0 {
		return nil
	}

	var sigType bullarc.SignalType
	var dominant float64
	var riskFlag string

	switch {
	case callPremium > putPremium:
		sigType = bullarc.SignalBuy
		dominant = callPremium
		riskFlag = RiskFlagUnusualCallActivity
	case putPremium > callPremium:
		sigType = bullarc.SignalSell
		dominant = putPremium
		riskFlag = RiskFlagUnusualPutActivity
	default:
		sigType = bullarc.SignalHold
		dominant = callPremium
		riskFlag = RiskFlagUnusualCallActivity
	}

	confidence := (dominant / total) * 100

	symbol := events[0].Symbol
	return &bullarc.Signal{
		Type:        sigType,
		Confidence:  confidence,
		Indicator:   "options_activity",
		Symbol:      symbol,
		Timestamp:   time.Now().UTC(),
		Explanation: buildOptionsExplanation(sigType, len(events), callPremium, putPremium, sweepCount, blockCount, unusualVolCount),
		RiskFlags:   []string{riskFlag},
		Metadata: map[string]any{
			"event_count":       len(events),
			"call_premium":      callPremium,
			"put_premium":       putPremium,
			"total_premium":     total,
			"sweep_count":       sweepCount,
			"block_count":       blockCount,
			"unusual_vol_count": unusualVolCount,
		},
	}
}

func buildOptionsExplanation(sigType bullarc.SignalType, eventCount int, callPremium, putPremium float64, sweepCount, blockCount, unusualVolCount int) string {
	direction := "mixed options flow"
	switch sigType {
	case bullarc.SignalBuy:
		direction = "unusual call buying"
	case bullarc.SignalSell:
		direction = "unusual put buying"
	}
	return fmt.Sprintf(
		"%d unusual options event(s): calls $%.0f / puts $%.0f (%d sweeps, %d blocks, %d unusual vol): %s",
		eventCount, callPremium, putPremium, sweepCount, blockCount, unusualVolCount, direction,
	)
}
