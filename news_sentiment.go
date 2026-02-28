package bullarc

import "time"

// ScoredNewsHeadline is a news headline paired with its LLM-derived sentiment score.
// It is produced by Engine.GetNewsSentiment and consumed by the get_news_sentiment MCP tool.
type ScoredNewsHeadline struct {
	ArticleID   string    `json:"article_id"`
	Headline    string    `json:"headline"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"published_at"`
	// Sentiment is one of "bullish", "neutral", or "bearish".
	Sentiment  string `json:"sentiment"`
	Confidence int    `json:"confidence"` // 0-100
	Reasoning  string `json:"reasoning"`
}

// NewsSentimentSummary aggregates sentiment-scored headlines for a symbol over a time window.
type NewsSentimentSummary struct {
	Symbol    string               `json:"symbol"`
	Headlines []ScoredNewsHeadline `json:"headlines"`
	// AggregateSentiment is the dominant sentiment: "bullish", "neutral", or "bearish".
	AggregateSentiment string `json:"aggregate_sentiment"`
	// AggregateScore is the weighted-average confidence score (0-100) for the aggregate direction.
	AggregateScore float64 `json:"aggregate_score"`
}
