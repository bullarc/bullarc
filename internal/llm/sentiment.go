package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bullarc/bullarc"
)

// SentimentClass is the sentiment classification for a news article.
type SentimentClass string

const (
	SentimentBullish SentimentClass = "bullish"
	SentimentNeutral SentimentClass = "neutral"
	SentimentBearish SentimentClass = "bearish"
)

// SentimentResult holds the sentiment analysis result for a single article.
type SentimentResult struct {
	ArticleID  string         `json:"article_id"`
	Sentiment  SentimentClass `json:"sentiment"`
	Confidence int            `json:"confidence"`
	Reasoning  string         `json:"reasoning"`
}

// SentimentScorer scores news article headlines using an LLM provider.
type SentimentScorer struct {
	provider bullarc.LLMProvider
	mu       sync.RWMutex
	cache    map[string]SentimentResult
}

// NewSentimentScorer creates a SentimentScorer backed by the given LLM provider.
// If provider is nil, ScoreArticles returns ErrLLMUnavailable.
func NewSentimentScorer(provider bullarc.LLMProvider) *SentimentScorer {
	return &SentimentScorer{
		provider: provider,
		cache:    make(map[string]SentimentResult),
	}
}

// ScoreArticles scores each article's headline using the LLM provider and returns
// a SentimentResult per article. Previously scored article IDs return cached results
// without calling the LLM again.
func (s *SentimentScorer) ScoreArticles(ctx context.Context, articles []bullarc.NewsArticle) ([]SentimentResult, error) {
	if s.provider == nil {
		return nil, bullarc.ErrLLMUnavailable.Wrap(fmt.Errorf("no LLM provider configured: an LLM key is required for sentiment scoring"))
	}

	results := make([]SentimentResult, 0, len(articles))
	for _, article := range articles {
		s.mu.RLock()
		cached, hit := s.cache[article.ID]
		s.mu.RUnlock()

		if hit {
			results = append(results, cached)
			continue
		}

		result, err := s.scoreOne(ctx, article)
		if err != nil {
			return nil, err
		}

		s.mu.Lock()
		s.cache[article.ID] = result
		s.mu.Unlock()

		results = append(results, result)
	}
	return results, nil
}

// scoreOne calls the LLM to score a single article headline.
func (s *SentimentScorer) scoreOne(ctx context.Context, article bullarc.NewsArticle) (SentimentResult, error) {
	prompt := buildSentimentPrompt(article)
	resp, err := s.provider.Complete(ctx, bullarc.LLMRequest{
		Prompt:    prompt,
		MaxTokens: 256,
	})
	if err != nil {
		return SentimentResult{}, err
	}
	return parseSentimentResponse(article.ID, resp.Text), nil
}

// sentimentLLMResponse is the expected JSON schema from the LLM.
type sentimentLLMResponse struct {
	Sentiment  string `json:"sentiment"`
	Confidence int    `json:"confidence"`
	Reasoning  string `json:"reasoning"`
}

// parseSentimentResponse parses the LLM JSON response into a SentimentResult.
// On invalid JSON or unrecognised sentiment class, it returns neutral with
// confidence 0 and logs a warning.
func parseSentimentResponse(articleID, text string) SentimentResult {
	// Extract JSON object from surrounding text (LLMs often add preamble).
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end > start {
		text = text[start : end+1]
	}

	var raw sentimentLLMResponse
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		slog.Warn("failed to parse LLM sentiment response, defaulting to neutral",
			"article_id", articleID, "err", err)
		return SentimentResult{ArticleID: articleID, Sentiment: SentimentNeutral, Confidence: 0}
	}

	sentiment := SentimentClass(raw.Sentiment)
	switch sentiment {
	case SentimentBullish, SentimentNeutral, SentimentBearish:
	default:
		slog.Warn("invalid sentiment class in LLM response, defaulting to neutral",
			"article_id", articleID, "sentiment", raw.Sentiment)
		return SentimentResult{ArticleID: articleID, Sentiment: SentimentNeutral, Confidence: 0}
	}

	confidence := raw.Confidence
	if confidence < 0 {
		confidence = 0
	} else if confidence > 100 {
		confidence = 100
	}

	return SentimentResult{
		ArticleID:  articleID,
		Sentiment:  sentiment,
		Confidence: confidence,
		Reasoning:  raw.Reasoning,
	}
}

// buildSentimentPrompt builds the LLM prompt for a single article.
func buildSentimentPrompt(article bullarc.NewsArticle) string {
	return fmt.Sprintf(
		`You are a financial sentiment analyst. Analyze the sentiment of the following news headline for trading purposes.

Headline: %s
Summary: %s

Respond with ONLY a JSON object in this exact format:
{"sentiment": "bullish|neutral|bearish", "confidence": 0-100, "reasoning": "brief explanation"}

- sentiment: "bullish" if positive for the stock, "bearish" if negative, "neutral" if unclear
- confidence: integer 0-100 indicating your confidence level
- reasoning: 1-2 sentence explanation`,
		article.Headline,
		article.Summary,
	)
}
