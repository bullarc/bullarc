package signal

import (
	"fmt"
	"time"

	"github.com/bullarc/bullarc"
)

// ScoredArticle carries the sentiment classification needed to generate a
// news sentiment trading signal. The engine populates this from the LLM
// scorer output before passing it into NewsSentimentSignal.
type ScoredArticle struct {
	Sentiment  string // "bullish", "bearish", or "neutral"
	Confidence int    // 0–100
}

// NewsSentimentSignal produces a BUY, SELL, or HOLD signal from a slice of
// sentiment-scored news articles. Articles should already be filtered to the
// desired time window (e.g. last 24h) before being passed here.
//
// Returns (zero Signal, false) when articles is empty so that callers can
// omit the signal from aggregation entirely rather than injecting a HOLD.
//
// Confidence reflects sentiment agreement: unanimous direction = high
// confidence, evenly split = low confidence. It is computed as the fraction
// of the total confidence-weighted score that belongs to the winning side,
// expressed as a percentage (0–100).
func NewsSentimentSignal(symbol string, articles []ScoredArticle) (bullarc.Signal, bool) {
	if len(articles) == 0 {
		return bullarc.Signal{}, false
	}

	var buyScore, sellScore, holdScore float64
	var buyCount, sellCount, holdCount int

	for _, a := range articles {
		conf := float64(a.Confidence)
		switch a.Sentiment {
		case "bullish":
			buyScore += conf
			buyCount++
		case "bearish":
			sellScore += conf
			sellCount++
		default: // "neutral" or unrecognised
			holdScore += conf
			holdCount++
		}
	}

	// Determine the winning direction by confidence-weighted score.
	winner := bullarc.SignalHold
	winnerScore := holdScore
	if buyScore > winnerScore {
		winnerScore = buyScore
		winner = bullarc.SignalBuy
	}
	if sellScore > winnerScore {
		winnerScore = sellScore
		winner = bullarc.SignalSell
	}

	totalScore := buyScore + sellScore + holdScore
	confidence := 50.0
	if totalScore > 0 {
		confidence = winnerScore / totalScore * 100
	}

	explanation := fmt.Sprintf(
		"NewsSentiment: %d bullish, %d bearish, %d neutral articles",
		buyCount, sellCount, holdCount,
	)

	return bullarc.Signal{
		Type:        winner,
		Confidence:  confidence,
		Indicator:   "NewsSentiment",
		Symbol:      symbol,
		Timestamp:   time.Now(),
		Explanation: explanation,
	}, true
}
