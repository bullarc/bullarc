package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bullarc/bullarc"
)

const (
	anthropicDefaultBaseURL = "https://api.anthropic.com"
	anthropicAPIVersion     = "2023-06-01"
	anthropicDefaultModel   = "claude-haiku-4-5-20251001"
)

// AnthropicProvider implements bullarc.LLMProvider using the Anthropic Messages API.
type AnthropicProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// AnthropicOption is a functional option for AnthropicProvider.
type AnthropicOption func(*AnthropicProvider)

// WithAnthropicBaseURL overrides the Anthropic API base URL (useful for testing).
func WithAnthropicBaseURL(u string) AnthropicOption {
	return func(p *AnthropicProvider) { p.baseURL = u }
}

// WithAnthropicHTTPClient sets a custom HTTP client on the AnthropicProvider.
func WithAnthropicHTTPClient(c *http.Client) AnthropicOption {
	return func(p *AnthropicProvider) { p.client = c }
}

// NewAnthropicProvider creates an AnthropicProvider with the given API key and model.
// If model is empty, it defaults to claude-haiku-4-5-20251001.
func NewAnthropicProvider(apiKey, model string, opts ...AnthropicOption) *AnthropicProvider {
	if model == "" {
		model = anthropicDefaultModel
	}
	p := &AnthropicProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: anthropicDefaultBaseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name returns the provider name.
func (p *AnthropicProvider) Name() string { return "anthropic" }

// anthropicMessage is a single message in the Anthropic API request.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicRequest is the JSON body for the Anthropic Messages API.
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

// anthropicContentBlock is a single content block in the Anthropic response.
type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// anthropicUsage holds token usage from the Anthropic response.
type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// anthropicResponse is the JSON body returned by the Anthropic Messages API.
type anthropicResponse struct {
	ID      string                  `json:"id"`
	Model   string                  `json:"model"`
	Content []anthropicContentBlock `json:"content"`
	Usage   anthropicUsage          `json:"usage"`
	Error   *anthropicError         `json:"error,omitempty"`
}

// anthropicError is the error body returned by the Anthropic API on failure.
type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Complete sends a prompt to the Anthropic Messages API and returns the response.
func (p *AnthropicProvider) Complete(ctx context.Context, req bullarc.LLMRequest) (bullarc.LLMResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	body := anthropicRequest{
		Model:     p.model,
		MaxTokens: maxTokens,
		Messages:  []anthropicMessage{{Role: "user", Content: req.Prompt}},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return bullarc.LLMResponse{}, bullarc.ErrLLMUnavailable.Wrap(fmt.Errorf("marshal request: %w", err))
	}

	endpoint := p.baseURL + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return bullarc.LLMResponse{}, bullarc.ErrLLMUnavailable.Wrap(fmt.Errorf("build request: %w", err))
	}
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return bullarc.LLMResponse{}, bullarc.ErrTimeout.Wrap(err)
		}
		return bullarc.LLMResponse{}, bullarc.ErrLLMUnavailable.Wrap(fmt.Errorf("http request: %w", err))
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return bullarc.LLMResponse{}, bullarc.ErrLLMUnavailable.Wrap(fmt.Errorf("read response: %w", err))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return bullarc.LLMResponse{}, bullarc.ErrLLMUnavailable.Wrap(fmt.Errorf("decode response: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("status %d", resp.StatusCode)
		if apiResp.Error != nil {
			msg = apiResp.Error.Message
		}
		return bullarc.LLMResponse{}, bullarc.ErrLLMUnavailable.Wrap(fmt.Errorf("api error: %s", msg))
	}

	text := extractText(apiResp.Content)
	return bullarc.LLMResponse{
		Text:       text,
		TokensUsed: apiResp.Usage.InputTokens + apiResp.Usage.OutputTokens,
		Model:      apiResp.Model,
	}, nil
}

// extractText returns the concatenated text from all text content blocks.
func extractText(blocks []anthropicContentBlock) string {
	var buf bytes.Buffer
	for _, b := range blocks {
		if b.Type == "text" {
			buf.WriteString(b.Text)
		}
	}
	return buf.String()
}
