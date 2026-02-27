package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicProvider_Complete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/messages", r.URL.Path)
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		assert.Equal(t, anthropicAPIVersion, r.Header.Get("anthropic-version"))
		assert.Equal(t, "application/json", r.Header.Get("content-type"))

		var body anthropicRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "claude-haiku-4-5-20251001", body.Model)
		assert.Equal(t, 256, body.MaxTokens)
		require.Len(t, body.Messages, 1)
		assert.Equal(t, "user", body.Messages[0].Role)
		assert.Equal(t, "explain this", body.Messages[0].Content)

		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			ID:    "msg_123",
			Model: "claude-haiku-4-5-20251001",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "The stock looks bullish."},
			},
			Usage: anthropicUsage{InputTokens: 10, OutputTokens: 6},
		})
	}))
	defer srv.Close()

	p := NewAnthropicProvider("test-key", "", WithAnthropicBaseURL(srv.URL))
	resp, err := p.Complete(context.Background(), bullarc.LLMRequest{Prompt: "explain this", MaxTokens: 256})
	require.NoError(t, err)
	assert.Equal(t, "The stock looks bullish.", resp.Text)
	assert.Equal(t, 16, resp.TokensUsed)
	assert.Equal(t, "claude-haiku-4-5-20251001", resp.Model)
}

func TestAnthropicProvider_Complete_DefaultMaxTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body anthropicRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, 1024, body.MaxTokens)

		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			Model:   body.Model,
			Content: []anthropicContentBlock{{Type: "text", Text: "ok"}},
		})
	}))
	defer srv.Close()

	p := NewAnthropicProvider("key", "", WithAnthropicBaseURL(srv.URL))
	resp, err := p.Complete(context.Background(), bullarc.LLMRequest{Prompt: "hi"})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Text)
}

func TestAnthropicProvider_Complete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(anthropicResponse{
			Error: &anthropicError{Type: "authentication_error", Message: "invalid api key"},
		})
	}))
	defer srv.Close()

	p := NewAnthropicProvider("bad-key", "", WithAnthropicBaseURL(srv.URL))
	_, err := p.Complete(context.Background(), bullarc.LLMRequest{Prompt: "hi"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid api key")
}

func TestAnthropicProvider_Complete_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// never respond
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewAnthropicProvider("key", "", WithAnthropicBaseURL(srv.URL))
	_, err := p.Complete(ctx, bullarc.LLMRequest{Prompt: "hi"})
	require.Error(t, err)
}

func TestAnthropicProvider_Name(t *testing.T) {
	p := NewAnthropicProvider("key", "")
	assert.Equal(t, "anthropic", p.Name())
}

func TestAnthropicProvider_DefaultModel(t *testing.T) {
	p := NewAnthropicProvider("key", "")
	assert.Equal(t, anthropicDefaultModel, p.model)
}
