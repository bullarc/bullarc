package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubNewsSource is an in-memory NewsSource used in engine news sentiment tests.
type stubNewsSource struct {
	articles []bullarc.NewsArticle
	err      error
}

func (s *stubNewsSource) FetchNews(_ context.Context, _ []string, _ time.Time) ([]bullarc.NewsArticle, error) {
	return s.articles, s.err
}

// stubSentimentLLM is a minimal LLMProvider that returns fixed JSON responses.
type stubSentimentLLM struct {
	responses []bullarc.LLMResponse
	calls     int
}

func (s *stubSentimentLLM) Name() string { return "stub-sentiment" }

func (s *stubSentimentLLM) Complete(_ context.Context, _ bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	i := s.calls
	s.calls++
	if i < len(s.responses) {
		return s.responses[i], nil
	}
	return bullarc.LLMResponse{Text: `{"sentiment":"neutral","confidence":50,"reasoning":""}`}, nil
}

func makeNewsArticle(id string) bullarc.NewsArticle {
	return bullarc.NewsArticle{
		ID:          id,
		Headline:    "Headline " + id,
		Summary:     "",
		Source:      "test",
		Symbols:     []string{"AAPL"},
		PublishedAt: time.Now().Add(-1 * time.Hour),
	}
}

// TestAnalyze_NewsSentimentSignalIncluded verifies that when a news source and
// scorer are registered, a NewsSentiment signal appears in the result.
func TestAnalyze_NewsSentimentSignalIncluded(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	// Register a news source returning three bullish articles.
	articles := []bullarc.NewsArticle{
		makeNewsArticle("a1"),
		makeNewsArticle("a2"),
		makeNewsArticle("a3"),
	}
	provider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{
			sentimentJSON("bullish", 80),
			sentimentJSON("bullish", 85),
			sentimentJSON("bullish", 90),
		},
	}
	scorer := llm.NewSentimentScorer(provider)
	e.RegisterNewsSource(&stubNewsSource{articles: articles})
	e.RegisterSentimentScorer(scorer)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	var newsSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "NewsSentiment" {
			newsSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, newsSig, "NewsSentiment signal must be present in result")
	assert.Equal(t, "AAPL", newsSig.Symbol)
	assert.Equal(t, bullarc.SignalBuy, newsSig.Type, "all-bullish articles should yield BUY")
	assert.InDelta(t, 100.0, newsSig.Confidence, 0.01, "unanimous bullish should give 100%% confidence")
}

// TestAnalyze_NewsSentimentSignalOmittedWhenNoNews verifies that with no recent
// news the signal is absent — it must not default to HOLD and dilute confidence.
func TestAnalyze_NewsSentimentSignalOmittedWhenNoNews(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubSentimentLLM{}
	scorer := llm.NewSentimentScorer(provider)
	e.RegisterNewsSource(&stubNewsSource{articles: nil}) // no articles
	e.RegisterSentimentScorer(scorer)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	for _, sig := range result.Signals {
		assert.NotEqual(t, "NewsSentiment", sig.Indicator,
			"NewsSentiment signal must not appear when there is no news")
	}
}

// TestAnalyze_NewsSentimentSignalOmittedWithoutRegistration verifies that the
// news signal is absent when neither a news source nor scorer is registered.
func TestAnalyze_NewsSentimentSignalOmittedWithoutRegistration(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	for _, sig := range result.Signals {
		assert.NotEqual(t, "NewsSentiment", sig.Indicator)
	}
}

// TestAnalyze_NewsSentimentBearishProducesSell verifies bearish majority → SELL.
func TestAnalyze_NewsSentimentBearishProducesSell(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	articles := []bullarc.NewsArticle{
		makeNewsArticle("b1"),
		makeNewsArticle("b2"),
	}
	provider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{
			sentimentJSON("bearish", 75),
			sentimentJSON("bearish", 80),
		},
	}
	scorer := llm.NewSentimentScorer(provider)
	e.RegisterNewsSource(&stubNewsSource{articles: articles})
	e.RegisterSentimentScorer(scorer)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	var newsSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "NewsSentiment" {
			newsSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, newsSig)
	assert.Equal(t, bullarc.SignalSell, newsSig.Type)
}

// TestAnalyze_NewsSentimentWeight verifies that the configurable weight
// scales the confidence of the news sentiment signal.
func TestAnalyze_NewsSentimentWeight(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	articles := []bullarc.NewsArticle{makeNewsArticle("w1")}
	provider := &stubSentimentLLM{
		responses: []bullarc.LLMResponse{sentimentJSON("bullish", 100)},
	}
	scorer := llm.NewSentimentScorer(provider)
	e.RegisterNewsSource(&stubNewsSource{articles: articles})
	e.RegisterSentimentScorer(scorer)
	e.SetNewsSentimentWeight(0.5) // halve the confidence

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err)

	var newsSig *bullarc.Signal
	for i := range result.Signals {
		if result.Signals[i].Indicator == "NewsSentiment" {
			newsSig = &result.Signals[i]
			break
		}
	}
	require.NotNil(t, newsSig)
	// Original confidence = 100, weight = 0.5 → scaled confidence = 50.
	assert.InDelta(t, 50.0, newsSig.Confidence, 0.01)
}

// TestAnalyze_NewsSentimentFetchErrorSkipsSignal verifies that a news source
// fetch error causes the signal to be omitted without failing Analyze.
func TestAnalyze_NewsSentimentFetchErrorSkipsSignal(t *testing.T) {
	bars := trendingBars(100, 100, 0.5)
	e := newEngineWithBars(bars)

	provider := &stubSentimentLLM{}
	scorer := llm.NewSentimentScorer(provider)
	e.RegisterNewsSource(&stubNewsSource{err: bullarc.ErrDataSourceUnavailable})
	e.RegisterSentimentScorer(scorer)

	result, err := e.Analyze(context.Background(), bullarc.AnalysisRequest{Symbol: "AAPL"})
	require.NoError(t, err, "news fetch error must not propagate from Analyze")

	for _, sig := range result.Signals {
		assert.NotEqual(t, "NewsSentiment", sig.Indicator)
	}
}

// sentimentJSON builds a fixed JSON LLM response for a given sentiment and confidence.
func sentimentJSON(sentiment string, confidence int) bullarc.LLMResponse {
	return bullarc.LLMResponse{
		Text: `{"sentiment":"` + sentiment + `","confidence":` + itoa(confidence) + `,"reasoning":"test"}`,
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 3)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
