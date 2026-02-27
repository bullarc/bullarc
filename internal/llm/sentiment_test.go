package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubLLMProvider is a test double for bullarc.LLMProvider.
type stubLLMProvider struct {
	responses []bullarc.LLMResponse
	errs      []error
	calls     int
}

func (s *stubLLMProvider) Name() string { return "stub" }

func (s *stubLLMProvider) Complete(_ context.Context, _ bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return bullarc.LLMResponse{}, s.errs[i]
	}
	if i < len(s.responses) {
		return s.responses[i], nil
	}
	return bullarc.LLMResponse{Text: `{"sentiment":"neutral","confidence":0,"reasoning":""}`}, nil
}

func makeArticle(id, headline, summary string) bullarc.NewsArticle {
	return bullarc.NewsArticle{
		ID:          id,
		Headline:    headline,
		Summary:     summary,
		Source:      "test",
		Symbols:     []string{"AAPL"},
		PublishedAt: time.Now(),
	}
}

func TestSentimentScorer_ScoreArticles_Bullish(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"sentiment":"bullish","confidence":85,"reasoning":"Strong earnings beat."}`},
		},
	}
	scorer := NewSentimentScorer(provider)
	articles := []bullarc.NewsArticle{makeArticle("art1", "Apple earnings beat estimates", "")}

	results, err := scorer.ScoreArticles(context.Background(), articles)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "art1", results[0].ArticleID)
	assert.Equal(t, SentimentBullish, results[0].Sentiment)
	assert.Equal(t, 85, results[0].Confidence)
	assert.Equal(t, "Strong earnings beat.", results[0].Reasoning)
}

func TestSentimentScorer_ScoreArticles_Bearish(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"sentiment":"bearish","confidence":72,"reasoning":"Guidance cut signals weakness."}`},
		},
	}
	scorer := NewSentimentScorer(provider)
	articles := []bullarc.NewsArticle{makeArticle("art2", "Apple cuts revenue guidance", "")}

	results, err := scorer.ScoreArticles(context.Background(), articles)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, SentimentBearish, results[0].Sentiment)
	assert.Equal(t, 72, results[0].Confidence)
}

func TestSentimentScorer_ScoreArticles_Neutral(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"sentiment":"neutral","confidence":50,"reasoning":"Routine update."}`},
		},
	}
	scorer := NewSentimentScorer(provider)
	articles := []bullarc.NewsArticle{makeArticle("art3", "Apple updates privacy policy", "")}

	results, err := scorer.ScoreArticles(context.Background(), articles)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, SentimentNeutral, results[0].Sentiment)
	assert.Equal(t, 50, results[0].Confidence)
}

func TestSentimentScorer_ScoreArticles_CacheHit(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"sentiment":"bullish","confidence":90,"reasoning":"Good news."}`},
		},
	}
	scorer := NewSentimentScorer(provider)
	article := makeArticle("cached-id", "Apple new product launch", "")

	// First call: scores via LLM.
	results1, err := scorer.ScoreArticles(context.Background(), []bullarc.NewsArticle{article})
	require.NoError(t, err)
	require.Len(t, results1, 1)
	assert.Equal(t, SentimentBullish, results1[0].Sentiment)
	assert.Equal(t, 1, provider.calls, "should call LLM once")

	// Second call with same article ID: should return cached result without calling LLM.
	results2, err := scorer.ScoreArticles(context.Background(), []bullarc.NewsArticle{article})
	require.NoError(t, err)
	require.Len(t, results2, 1)
	assert.Equal(t, SentimentBullish, results2[0].Sentiment)
	assert.Equal(t, 1, provider.calls, "LLM must NOT be called again for cached article")
}

func TestSentimentScorer_ScoreArticles_NoProvider(t *testing.T) {
	scorer := NewSentimentScorer(nil)
	articles := []bullarc.NewsArticle{makeArticle("art4", "Some headline", "")}

	_, err := scorer.ScoreArticles(context.Background(), articles)
	require.Error(t, err)

	var bullarcErr *bullarc.Error
	require.True(t, errors.As(err, &bullarcErr), "error must be bullarc.Error")
	assert.Equal(t, "LLM_UNAVAILABLE", bullarcErr.Code)
}

func TestSentimentScorer_ScoreArticles_InvalidJSON(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `not valid json at all`},
		},
	}
	scorer := NewSentimentScorer(provider)
	articles := []bullarc.NewsArticle{makeArticle("art5", "Some headline", "")}

	results, err := scorer.ScoreArticles(context.Background(), articles)
	require.NoError(t, err, "invalid JSON should not return error; should default to neutral")
	require.Len(t, results, 1)
	assert.Equal(t, SentimentNeutral, results[0].Sentiment)
	assert.Equal(t, 0, results[0].Confidence)
}

func TestSentimentScorer_ScoreArticles_InvalidSentimentClass(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"sentiment":"unknown","confidence":60,"reasoning":"test"}`},
		},
	}
	scorer := NewSentimentScorer(provider)
	articles := []bullarc.NewsArticle{makeArticle("art6", "Some headline", "")}

	results, err := scorer.ScoreArticles(context.Background(), articles)
	require.NoError(t, err, "unknown sentiment class should not return error; should default to neutral")
	require.Len(t, results, 1)
	assert.Equal(t, SentimentNeutral, results[0].Sentiment)
	assert.Equal(t, 0, results[0].Confidence)
}

func TestSentimentScorer_ScoreArticles_JSONWithPreamble(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `Here is the analysis: {"sentiment":"bearish","confidence":65,"reasoning":"Negative outlook."} Hope that helps.`},
		},
	}
	scorer := NewSentimentScorer(provider)
	articles := []bullarc.NewsArticle{makeArticle("art7", "Apple faces lawsuit", "")}

	results, err := scorer.ScoreArticles(context.Background(), articles)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, SentimentBearish, results[0].Sentiment)
	assert.Equal(t, 65, results[0].Confidence)
}

func TestSentimentScorer_ScoreArticles_EmptyArticles(t *testing.T) {
	provider := &stubLLMProvider{}
	scorer := NewSentimentScorer(provider)

	results, err := scorer.ScoreArticles(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.Equal(t, 0, provider.calls)
}

func TestSentimentScorer_ScoreArticles_MultipleArticles(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"sentiment":"bullish","confidence":80,"reasoning":"Good news."}`},
			{Text: `{"sentiment":"bearish","confidence":70,"reasoning":"Bad news."}`},
			{Text: `{"sentiment":"neutral","confidence":50,"reasoning":"Meh."}`},
		},
	}
	scorer := NewSentimentScorer(provider)
	articles := []bullarc.NewsArticle{
		makeArticle("m1", "Headline 1", ""),
		makeArticle("m2", "Headline 2", ""),
		makeArticle("m3", "Headline 3", ""),
	}

	results, err := scorer.ScoreArticles(context.Background(), articles)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, SentimentBullish, results[0].Sentiment)
	assert.Equal(t, SentimentBearish, results[1].Sentiment)
	assert.Equal(t, SentimentNeutral, results[2].Sentiment)
	assert.Equal(t, 3, provider.calls)
}

func TestSentimentScorer_ScoreArticles_LLMError(t *testing.T) {
	provider := &stubLLMProvider{
		errs: []error{bullarc.ErrLLMUnavailable},
	}
	scorer := NewSentimentScorer(provider)
	articles := []bullarc.NewsArticle{makeArticle("art8", "Some headline", "")}

	_, err := scorer.ScoreArticles(context.Background(), articles)
	require.Error(t, err)
}

func TestSentimentScorer_ScoreArticles_MixedCacheAndFresh(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"sentiment":"bullish","confidence":88,"reasoning":"First."}`},
			{Text: `{"sentiment":"bearish","confidence":55,"reasoning":"Third."}`},
		},
	}
	scorer := NewSentimentScorer(provider)

	// Score article 1 to put it in cache.
	first := makeArticle("first", "First article", "")
	_, err := scorer.ScoreArticles(context.Background(), []bullarc.NewsArticle{first})
	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)

	// Now score: cached "first", new "second", new "third".
	second := makeArticle("second", "Second article", "")
	third := makeArticle("third", "Third article", "")

	// Override responses for the next two calls.
	provider.responses = append(provider.responses[:1], bullarc.LLMResponse{
		Text: `{"sentiment":"neutral","confidence":40,"reasoning":"Second."}`,
	}, bullarc.LLMResponse{
		Text: `{"sentiment":"bearish","confidence":55,"reasoning":"Third."}`,
	})

	results, err := scorer.ScoreArticles(context.Background(), []bullarc.NewsArticle{first, second, third})
	require.NoError(t, err)
	require.Len(t, results, 3)

	// first comes from cache (bullish, 88).
	assert.Equal(t, SentimentBullish, results[0].Sentiment)
	assert.Equal(t, 88, results[0].Confidence)
	// second and third are fresh.
	assert.Equal(t, "first", results[0].ArticleID)
	assert.Equal(t, "second", results[1].ArticleID)
	assert.Equal(t, "third", results[2].ArticleID)
}

func TestSentimentScorer_ConfidenceClamped(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"sentiment":"bullish","confidence":150,"reasoning":"Over 100."}`},
		},
	}
	scorer := NewSentimentScorer(provider)
	results, err := scorer.ScoreArticles(context.Background(), []bullarc.NewsArticle{makeArticle("clamp", "Headline", "")})
	require.NoError(t, err)
	assert.Equal(t, 100, results[0].Confidence)
}
