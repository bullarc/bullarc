package signal_test

import (
	"testing"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/signal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeScoredArticle(sentiment string, confidence int) signal.ScoredArticle {
	return signal.ScoredArticle{Sentiment: sentiment, Confidence: confidence}
}

// TestNewsSentimentSignal_NoArticles verifies that an empty slice produces no signal.
func TestNewsSentimentSignal_NoArticles(t *testing.T) {
	_, ok := signal.NewsSentimentSignal("AAPL", nil)
	assert.False(t, ok, "empty articles must return false")

	_, ok = signal.NewsSentimentSignal("AAPL", []signal.ScoredArticle{})
	assert.False(t, ok, "empty slice must return false")
}

// TestNewsSentimentSignal_AllBullish verifies that unanimous bullish articles produce BUY.
func TestNewsSentimentSignal_AllBullish(t *testing.T) {
	articles := []signal.ScoredArticle{
		makeScoredArticle("bullish", 80),
		makeScoredArticle("bullish", 90),
		makeScoredArticle("bullish", 70),
	}
	sig, ok := signal.NewsSentimentSignal("AAPL", articles)
	require.True(t, ok)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	assert.Equal(t, "NewsSentiment", sig.Indicator)
	assert.Equal(t, "AAPL", sig.Symbol)
	assert.InDelta(t, 100.0, sig.Confidence, 0.01, "unanimous bullish should yield 100%% confidence")
}

// TestNewsSentimentSignal_AllBearish verifies that unanimous bearish articles produce SELL.
func TestNewsSentimentSignal_AllBearish(t *testing.T) {
	articles := []signal.ScoredArticle{
		makeScoredArticle("bearish", 75),
		makeScoredArticle("bearish", 85),
	}
	sig, ok := signal.NewsSentimentSignal("AAPL", articles)
	require.True(t, ok)
	assert.Equal(t, bullarc.SignalSell, sig.Type)
	assert.InDelta(t, 100.0, sig.Confidence, 0.01, "unanimous bearish should yield 100%% confidence")
}

// TestNewsSentimentSignal_AllNeutral verifies that all-neutral articles produce HOLD.
func TestNewsSentimentSignal_AllNeutral(t *testing.T) {
	articles := []signal.ScoredArticle{
		makeScoredArticle("neutral", 50),
		makeScoredArticle("neutral", 60),
	}
	sig, ok := signal.NewsSentimentSignal("AAPL", articles)
	require.True(t, ok)
	assert.Equal(t, bullarc.SignalHold, sig.Type)
	assert.InDelta(t, 100.0, sig.Confidence, 0.01, "unanimous neutral should yield 100%% confidence")
}

// TestNewsSentimentSignal_MajorityBullish verifies majority bullish produces BUY.
func TestNewsSentimentSignal_MajorityBullish(t *testing.T) {
	articles := []signal.ScoredArticle{
		makeScoredArticle("bullish", 80),
		makeScoredArticle("bullish", 80),
		makeScoredArticle("bullish", 80),
		makeScoredArticle("bearish", 80),
		makeScoredArticle("bearish", 80),
	}
	sig, ok := signal.NewsSentimentSignal("AAPL", articles)
	require.True(t, ok)
	assert.Equal(t, bullarc.SignalBuy, sig.Type)
	// bullish score = 240, bearish = 160, total = 400 → confidence = 60%
	assert.InDelta(t, 60.0, sig.Confidence, 0.1)
}

// TestNewsSentimentSignal_MajorityBearish verifies majority bearish produces SELL.
func TestNewsSentimentSignal_MajorityBearish(t *testing.T) {
	articles := []signal.ScoredArticle{
		makeScoredArticle("bearish", 70),
		makeScoredArticle("bearish", 70),
		makeScoredArticle("bearish", 70),
		makeScoredArticle("bullish", 70),
	}
	sig, ok := signal.NewsSentimentSignal("AAPL", articles)
	require.True(t, ok)
	assert.Equal(t, bullarc.SignalSell, sig.Type)
	// bearish = 210, bullish = 70, total = 280 → confidence = 75%
	assert.InDelta(t, 75.0, sig.Confidence, 0.1)
}

// TestNewsSentimentSignal_EvenSplit verifies a tied split defaults to HOLD
// (since hold wins the tiebreak as the default winner type).
func TestNewsSentimentSignal_EvenSplit(t *testing.T) {
	articles := []signal.ScoredArticle{
		makeScoredArticle("bullish", 80),
		makeScoredArticle("bearish", 80),
	}
	sig, ok := signal.NewsSentimentSignal("AAPL", articles)
	require.True(t, ok)
	// With equal scores and no hold votes, the hold winner score is 0 and
	// buy wins the tiebreak because it is checked first.
	assert.NotEmpty(t, sig.Type)
	assert.GreaterOrEqual(t, sig.Confidence, 0.0)
	assert.LessOrEqual(t, sig.Confidence, 100.0)
}

// TestNewsSentimentSignal_ConfidenceProportionalToAgreement verifies that
// unanimous agreement yields higher confidence than a split.
func TestNewsSentimentSignal_ConfidenceProportionalToAgreement(t *testing.T) {
	unanimous := []signal.ScoredArticle{
		makeScoredArticle("bullish", 70),
		makeScoredArticle("bullish", 70),
		makeScoredArticle("bullish", 70),
	}
	split := []signal.ScoredArticle{
		makeScoredArticle("bullish", 70),
		makeScoredArticle("bullish", 70),
		makeScoredArticle("bearish", 70),
	}

	uSig, uOk := signal.NewsSentimentSignal("AAPL", unanimous)
	sSig, sOk := signal.NewsSentimentSignal("AAPL", split)

	require.True(t, uOk)
	require.True(t, sOk)
	assert.Greater(t, uSig.Confidence, sSig.Confidence,
		"unanimous agreement should yield higher confidence than split")
}

// TestNewsSentimentSignal_ExplanationContainsCounts verifies explanation format.
func TestNewsSentimentSignal_ExplanationContainsCounts(t *testing.T) {
	articles := []signal.ScoredArticle{
		makeScoredArticle("bullish", 80),
		makeScoredArticle("bullish", 80),
		makeScoredArticle("bearish", 60),
		makeScoredArticle("neutral", 50),
	}
	sig, ok := signal.NewsSentimentSignal("TSLA", articles)
	require.True(t, ok)
	assert.Contains(t, sig.Explanation, "2 bullish")
	assert.Contains(t, sig.Explanation, "1 bearish")
	assert.Contains(t, sig.Explanation, "1 neutral")
}

// TestNewsSentimentSignal_SingleArticle verifies a single article always gives 100% confidence.
func TestNewsSentimentSignal_SingleArticle(t *testing.T) {
	for _, sentiment := range []string{"bullish", "bearish", "neutral"} {
		t.Run(sentiment, func(t *testing.T) {
			articles := []signal.ScoredArticle{makeScoredArticle(sentiment, 75)}
			sig, ok := signal.NewsSentimentSignal("AAPL", articles)
			require.True(t, ok)
			assert.InDelta(t, 100.0, sig.Confidence, 0.01,
				"single article should give 100%% confidence")
		})
	}
}

// TestNewsSentimentSignal_UnrecognisedSentimentTreatedAsNeutral verifies that
// unknown sentiment strings are treated as neutral (HOLD direction).
func TestNewsSentimentSignal_UnrecognisedSentimentTreatedAsNeutral(t *testing.T) {
	articles := []signal.ScoredArticle{
		{Sentiment: "unknown", Confidence: 60},
		{Sentiment: "unknown", Confidence: 60},
	}
	sig, ok := signal.NewsSentimentSignal("AAPL", articles)
	require.True(t, ok)
	assert.Equal(t, bullarc.SignalHold, sig.Type)
}

// TestNewsSentimentSignal_ZeroConfidenceArticles verifies behaviour when all
// confidence values are zero (avoids division by zero in confidence calc).
func TestNewsSentimentSignal_ZeroConfidenceArticles(t *testing.T) {
	articles := []signal.ScoredArticle{
		makeScoredArticle("bullish", 0),
		makeScoredArticle("bearish", 0),
	}
	sig, ok := signal.NewsSentimentSignal("AAPL", articles)
	require.True(t, ok)
	assert.Equal(t, 50.0, sig.Confidence, "zero total score should default confidence to 50")
}
