package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bullarc/bullarc"
)

// ScoredHeadline pairs a news headline with its LLM-derived sentiment classification.
// It is the input to the news thesis step of the multi-step analysis chain.
type ScoredHeadline struct {
	Headline   string
	Sentiment  string
	Confidence int
}

// technicalThesisResponse is the expected JSON schema from the technical thesis call.
type technicalThesisResponse struct {
	Signal     string `json:"signal"`
	Confidence int    `json:"confidence"`
	Direction  string `json:"direction"`
	KeyLevels  string `json:"key_levels"`
	Confluence string `json:"confluence"`
	Thesis     string `json:"thesis"`
}

// newsThesisResponse is the expected JSON schema from the news thesis call.
type newsThesisResponse struct {
	SentimentTrend string `json:"sentiment_trend"`
	Catalysts      string `json:"catalysts"`
	Risks          string `json:"risks"`
	Thesis         string `json:"thesis"`
}

// synthesisResponse is the expected JSON schema from the synthesis call.
type synthesisResponse struct {
	Signal     string `json:"signal"`
	Confidence int    `json:"confidence"`
	Reasoning  string `json:"reasoning"`
}

// TechnicalThesisPrompt builds the step-1 prompt: a structured technical assessment
// of indicator values, key levels, confluence, and a preliminary signal direction.
func TechnicalThesisPrompt(
	symbol string,
	indicatorValues map[string][]bullarc.IndicatorValue,
	composite bullarc.Signal,
	currentPrice float64,
) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(
		"You are a quantitative financial analyst. Produce a technical thesis for %s based on the following indicator data.\n\n",
		symbol,
	))
	b.WriteString(fmt.Sprintf("Current price: %.4f\n\n", currentPrice))

	b.WriteString("=== Indicator Values (latest) ===\n")
	for name, values := range indicatorValues {
		if len(values) == 0 {
			continue
		}
		latest := values[len(values)-1]
		if len(latest.Extra) > 0 {
			b.WriteString(fmt.Sprintf("  %s: %.4f", name, latest.Value))
			for k, v := range latest.Extra {
				b.WriteString(fmt.Sprintf(", %s=%.4f", k, v))
			}
			b.WriteString("\n")
		} else {
			b.WriteString(fmt.Sprintf("  %s: %.4f\n", name, latest.Value))
		}
	}

	b.WriteString("\n=== Preliminary Composite Signal ===\n")
	b.WriteString(fmt.Sprintf("Direction: %s\n", composite.Type))
	b.WriteString(fmt.Sprintf("Confidence: %.0f%%\n", composite.Confidence))

	b.WriteString(`
Assess the technical picture and respond with ONLY a JSON object in this exact format:
{"signal":"BUY|SELL|HOLD","confidence":0-100,"direction":"bullish|bearish|neutral","key_levels":"brief description of support/resistance","confluence":"which indicators agree and disagree","thesis":"2-3 sentence technical assessment"}

- signal: "BUY", "SELL", or "HOLD"
- confidence: integer 0-100
- direction: overall technical direction
- key_levels: key support or resistance levels observed
- confluence: summary of indicator agreement or divergence
- thesis: 2-3 sentence plain-English technical assessment`)

	return b.String()
}

// NewsThesisPrompt builds the step-2 prompt: a news and fundamental assessment
// based on scored news headlines and their sentiment classifications.
func NewsThesisPrompt(symbol string, headlines []ScoredHeadline) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(
		"You are a financial news analyst. Assess the fundamental and news-driven outlook for %s based on recent scored headlines.\n\n",
		symbol,
	))

	b.WriteString("=== Recent News Headlines (with LLM sentiment scores) ===\n")
	for i, h := range headlines {
		b.WriteString(fmt.Sprintf("  %d. [%s, confidence=%d%%] %s\n",
			i+1, h.Sentiment, h.Confidence, h.Headline))
	}

	b.WriteString(`
Assess the news-driven and fundamental picture and respond with ONLY a JSON object in this exact format:
{"sentiment_trend":"bullish|bearish|neutral","catalysts":"brief description of positive catalysts","risks":"brief description of risks or negative factors","thesis":"2-3 sentence news and fundamental assessment"}

- sentiment_trend: overall news sentiment direction
- catalysts: key bullish catalysts from the news
- risks: key bearish risks or concerns from the news
- thesis: 2-3 sentence plain-English news and fundamental assessment`)

	return b.String()
}

// SynthesisPrompt builds the step-3 prompt: a final combined signal synthesizing
// both the technical thesis and the news thesis into a single trading decision.
func SynthesisPrompt(symbol, technicalThesis, newsThesis string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(
		"You are a senior portfolio manager synthesizing technical and fundamental analysis for %s into a final trading signal.\n\n",
		symbol,
	))

	b.WriteString("=== Technical Thesis ===\n")
	b.WriteString(technicalThesis)
	b.WriteString("\n\n")

	if newsThesis != "" {
		b.WriteString("=== News and Fundamental Thesis ===\n")
		b.WriteString(newsThesis)
		b.WriteString("\n\n")
	} else {
		b.WriteString("=== News and Fundamental Thesis ===\n")
		b.WriteString("No recent news data available. Base the synthesis on technical factors only.\n\n")
	}

	b.WriteString(`Synthesize both theses into a final trading signal and respond with ONLY a JSON object in this exact format:
{"signal":"BUY|SELL|HOLD","confidence":0-100,"reasoning":"3-5 sentence explanation referencing both technical and news factors"}

- signal: "BUY", "SELL", or "HOLD"
- confidence: integer 0-100 reflecting overall conviction
- reasoning: 3-5 sentences that explicitly reference both technical indicators and news/fundamental factors`)

	return b.String()
}

// parseTechnicalThesisResponse parses the LLM JSON response from the technical thesis call.
// Returns (zero, false) on invalid JSON or unrecognised signal type.
func parseTechnicalThesisResponse(symbol, text string) (technicalThesisResponse, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end > start {
		text = text[start : end+1]
	}

	var raw technicalThesisResponse
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		slog.Warn("failed to parse technical thesis response", "symbol", symbol, "err", err)
		return technicalThesisResponse{}, false
	}

	sigType := bullarc.SignalType(raw.Signal)
	switch sigType {
	case bullarc.SignalBuy, bullarc.SignalSell, bullarc.SignalHold:
	default:
		slog.Warn("invalid signal type in technical thesis response", "symbol", symbol, "signal", raw.Signal)
		return technicalThesisResponse{}, false
	}

	return raw, true
}

// parseNewsThesisResponse parses the LLM JSON response from the news thesis call.
// Returns (zero, false) on invalid JSON.
func parseNewsThesisResponse(symbol, text string) (newsThesisResponse, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end > start {
		text = text[start : end+1]
	}

	var raw newsThesisResponse
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		slog.Warn("failed to parse news thesis response", "symbol", symbol, "err", err)
		return newsThesisResponse{}, false
	}
	return raw, true
}

// parseSynthesisResponse parses the LLM JSON response from the synthesis call.
// Returns (zero, false) on invalid JSON or unrecognised signal type.
func parseSynthesisResponse(symbol, text string) (synthesisResponse, bool) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end > start {
		text = text[start : end+1]
	}

	var raw synthesisResponse
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		slog.Warn("failed to parse synthesis response", "symbol", symbol, "err", err)
		return synthesisResponse{}, false
	}

	sigType := bullarc.SignalType(raw.Signal)
	switch sigType {
	case bullarc.SignalBuy, bullarc.SignalSell, bullarc.SignalHold:
	default:
		slog.Warn("invalid signal type in synthesis response", "symbol", symbol, "signal", raw.Signal)
		return synthesisResponse{}, false
	}

	return raw, true
}

// RunMultiStepChain executes the three-step LLM analysis chain:
//
//  1. Technical thesis: indicator values → structured technical assessment.
//  2. News thesis (skipped if no headlines): scored headlines → news assessment.
//  3. Synthesis: both theses → final combined signal.
//
// Returns (signal, reasoning, true) on success. Fallback behaviour:
//   - If step 3 fails: falls back to the step-1 signal and thesis as reasoning.
//   - If step 1 fails: returns (zero Signal, "", false).
func RunMultiStepChain(
	ctx context.Context,
	symbol string,
	indicatorValues map[string][]bullarc.IndicatorValue,
	composite bullarc.Signal,
	currentPrice float64,
	headlines []ScoredHeadline,
	provider bullarc.LLMProvider,
) (bullarc.Signal, string, bool) {
	// ── Step 1: Technical thesis ──────────────────────────────────────────────
	techPrompt := TechnicalThesisPrompt(symbol, indicatorValues, composite, currentPrice)
	techResp, err := provider.Complete(ctx, bullarc.LLMRequest{
		Prompt:    techPrompt,
		MaxTokens: 512,
	})
	if err != nil {
		slog.Warn("multi-step chain step 1 failed", "symbol", symbol, "err", err)
		return bullarc.Signal{}, "", false
	}

	techThesis, ok := parseTechnicalThesisResponse(symbol, techResp.Text)
	if !ok {
		slog.Warn("multi-step chain step 1 parse failed, omitting LLM analysis", "symbol", symbol)
		return bullarc.Signal{}, "", false
	}
	slog.Info("multi-step chain step 1 complete",
		"symbol", symbol, "direction", techThesis.Direction, "signal", techThesis.Signal)

	// Build the step-1 fallback signal (used if synthesis fails).
	step1Signal := technicalThesisToSignal(symbol, techThesis)

	// ── Step 2: News thesis (optional) ───────────────────────────────────────
	newsThesisText := ""
	if len(headlines) > 0 {
		newsPrompt := NewsThesisPrompt(symbol, headlines)
		newsResp, err := provider.Complete(ctx, bullarc.LLMRequest{
			Prompt:    newsPrompt,
			MaxTokens: 512,
		})
		if err != nil {
			slog.Warn("multi-step chain step 2 failed, proceeding without news thesis",
				"symbol", symbol, "err", err)
		} else {
			if news, ok := parseNewsThesisResponse(symbol, newsResp.Text); ok {
				newsThesisText = news.Thesis
				slog.Info("multi-step chain step 2 complete",
					"symbol", symbol, "sentiment_trend", news.SentimentTrend)
			} else {
				slog.Warn("multi-step chain step 2 parse failed, proceeding without news thesis",
					"symbol", symbol)
			}
		}
	} else {
		slog.Info("multi-step chain step 2 skipped: no headlines available", "symbol", symbol)
	}

	// ── Step 3: Synthesis ─────────────────────────────────────────────────────
	synthPrompt := SynthesisPrompt(symbol, techThesis.Thesis, newsThesisText)
	synthResp, err := provider.Complete(ctx, bullarc.LLMRequest{
		Prompt:    synthPrompt,
		MaxTokens: 768,
	})
	if err != nil {
		slog.Warn("multi-step chain step 3 failed, falling back to technical thesis",
			"symbol", symbol, "err", err)
		return step1Signal, techThesis.Thesis, true
	}

	synthesis, ok := parseSynthesisResponse(symbol, synthResp.Text)
	if !ok {
		slog.Warn("multi-step chain step 3 parse failed, falling back to technical thesis",
			"symbol", symbol)
		return step1Signal, techThesis.Thesis, true
	}

	confidence := float64(synthesis.Confidence)
	if confidence < 0 {
		confidence = 0
	} else if confidence > 100 {
		confidence = 100
	}

	sig := bullarc.Signal{
		Type:        bullarc.SignalType(synthesis.Signal),
		Confidence:  confidence,
		Indicator:   "LLMMultiStep",
		Symbol:      symbol,
		Timestamp:   time.Now(),
		Explanation: synthesis.Reasoning,
	}

	slog.Info("multi-step chain synthesis complete",
		"symbol", symbol, "signal", sig.Type, "confidence", sig.Confidence)

	return sig, synthesis.Reasoning, true
}

// technicalThesisToSignal converts the parsed technical thesis JSON into a bullarc.Signal.
// It is used as the fallback when step 3 (synthesis) fails.
func technicalThesisToSignal(symbol string, thesis technicalThesisResponse) bullarc.Signal {
	confidence := float64(thesis.Confidence)
	if confidence < 0 {
		confidence = 0
	} else if confidence > 100 {
		confidence = 100
	}
	return bullarc.Signal{
		Type:        bullarc.SignalType(thesis.Signal),
		Confidence:  confidence,
		Indicator:   "LLMMultiStep",
		Symbol:      symbol,
		Timestamp:   time.Now(),
		Explanation: thesis.Thesis,
	}
}
