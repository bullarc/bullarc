package signal

import (
	"fmt"
	"time"

	"github.com/bullarcdev/bullarc"
)

// Aggregate combines individual indicator signals into a single composite signal.
// The composite type is determined by weighted vote (confidence-weighted sum per type),
// and the confidence reflects how dominant the winning side was.
func Aggregate(symbol string, signals []bullarc.Signal) bullarc.Signal {
	if len(signals) == 0 {
		return bullarc.Signal{
			Type:        bullarc.SignalHold,
			Confidence:  50,
			Indicator:   "composite",
			Symbol:      symbol,
			Timestamp:   time.Now(),
			Explanation: "no indicator signals available",
		}
	}

	scores := map[bullarc.SignalType]float64{
		bullarc.SignalBuy:  0,
		bullarc.SignalSell: 0,
		bullarc.SignalHold: 0,
	}
	counts := map[bullarc.SignalType]int{}
	var latest time.Time

	for _, s := range signals {
		scores[s.Type] += s.Confidence
		counts[s.Type]++
		if s.Timestamp.After(latest) {
			latest = s.Timestamp
		}
	}

	winner := winningType(scores)

	totalScore := scores[bullarc.SignalBuy] + scores[bullarc.SignalSell] + scores[bullarc.SignalHold]
	confidence := 50.0
	if totalScore > 0 {
		confidence = scores[winner] / totalScore * 100
	}

	explanation := fmt.Sprintf(
		"%s: %d buy, %d sell, %d hold signals (confidence=%.0f%%)",
		winner,
		counts[bullarc.SignalBuy],
		counts[bullarc.SignalSell],
		counts[bullarc.SignalHold],
		confidence,
	)

	return bullarc.Signal{
		Type:        winner,
		Confidence:  confidence,
		Indicator:   "composite",
		Symbol:      symbol,
		Timestamp:   latest,
		Explanation: explanation,
	}
}

func winningType(scores map[bullarc.SignalType]float64) bullarc.SignalType {
	winner := bullarc.SignalHold
	best := scores[bullarc.SignalHold]
	for _, t := range []bullarc.SignalType{bullarc.SignalBuy, bullarc.SignalSell} {
		if scores[t] > best {
			best = scores[t]
			winner = t
		}
	}
	return winner
}
