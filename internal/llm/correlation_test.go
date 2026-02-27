package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/bullarc/bullarc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCorrelationCheckPrompt_ContainsRequiredFields verifies the prompt includes
// the target symbol, all portfolio symbols, and the expected JSON schema.
func TestCorrelationCheckPrompt_ContainsRequiredFields(t *testing.T) {
	prompt := CorrelationCheckPrompt("NVDA", []string{"AMD", "INTC", "TSM"})

	assert.Contains(t, prompt, "NVDA")
	assert.Contains(t, prompt, "AMD")
	assert.Contains(t, prompt, "INTC")
	assert.Contains(t, prompt, "TSM")
	assert.Contains(t, prompt, "correlated")
	assert.Contains(t, prompt, "high|medium|low")
}

// TestCorrelationCheckPrompt_SinglePortfolioSymbol verifies the prompt is built
// correctly with a single portfolio entry.
func TestCorrelationCheckPrompt_SinglePortfolioSymbol(t *testing.T) {
	prompt := CorrelationCheckPrompt("AAPL", []string{"MSFT"})

	assert.Contains(t, prompt, "AAPL")
	assert.Contains(t, prompt, "MSFT")
}

// TestParseCorrelationResponse_HighOverlapCorrelated verifies parsing of a
// highly correlated response.
func TestParseCorrelationResponse_HighOverlapCorrelated(t *testing.T) {
	text := `{"correlated": true, "overlap": "high", "reasoning": "Both are semiconductor companies."}`

	correlated, overlap, ok := parseCorrelationResponse(text)

	require.True(t, ok)
	assert.True(t, correlated)
	assert.Equal(t, "high", overlap)
}

// TestParseCorrelationResponse_MediumOverlapNotCorrelated verifies parsing of a
// medium overlap response where correlated is false.
func TestParseCorrelationResponse_MediumOverlapNotCorrelated(t *testing.T) {
	text := `{"correlated": false, "overlap": "medium", "reasoning": "Same sector but different sub-industries."}`

	correlated, overlap, ok := parseCorrelationResponse(text)

	require.True(t, ok)
	assert.False(t, correlated)
	assert.Equal(t, "medium", overlap)
}

// TestParseCorrelationResponse_LowOverlapDiversified verifies parsing of a
// low overlap response indicating good diversification.
func TestParseCorrelationResponse_LowOverlapDiversified(t *testing.T) {
	text := `{"correlated": false, "overlap": "low", "reasoning": "Utilities vs tech — largely independent."}`

	correlated, overlap, ok := parseCorrelationResponse(text)

	require.True(t, ok)
	assert.False(t, correlated)
	assert.Equal(t, "low", overlap)
}

// TestParseCorrelationResponse_WithPreamble verifies JSON extraction from
// surrounding text added by the LLM.
func TestParseCorrelationResponse_WithPreamble(t *testing.T) {
	text := `Based on my analysis: {"correlated": true, "overlap": "high", "reasoning": "Very similar."} End.`

	correlated, overlap, ok := parseCorrelationResponse(text)

	require.True(t, ok)
	assert.True(t, correlated)
	assert.Equal(t, "high", overlap)
}

// TestParseCorrelationResponse_InvalidJSON returns false on malformed JSON.
func TestParseCorrelationResponse_InvalidJSON(t *testing.T) {
	correlated, overlap, ok := parseCorrelationResponse("not json at all")

	assert.False(t, ok)
	assert.False(t, correlated)
	assert.Empty(t, overlap)
}

// TestParseCorrelationResponse_UnrecognisedOverlap returns false for unknown
// overlap values.
func TestParseCorrelationResponse_UnrecognisedOverlap(t *testing.T) {
	text := `{"correlated": true, "overlap": "extreme", "reasoning": "Very high."}`

	correlated, overlap, ok := parseCorrelationResponse(text)

	assert.False(t, ok)
	assert.False(t, correlated)
	assert.Empty(t, overlap)
}

// TestParseCorrelationResponse_EmptyString returns false on empty input.
func TestParseCorrelationResponse_EmptyString(t *testing.T) {
	correlated, overlap, ok := parseCorrelationResponse("")

	assert.False(t, ok)
	assert.False(t, correlated)
	assert.Empty(t, overlap)
}

// TestCheckCorrelation_Success verifies that a valid provider response is parsed correctly.
func TestCheckCorrelation_Success(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"correlated": true, "overlap": "high", "reasoning": "Both are large-cap tech stocks."}`},
		},
	}

	correlated, overlap, ok := CheckCorrelation(context.Background(), "NVDA", []string{"AMD", "INTC"}, provider)

	require.True(t, ok)
	assert.True(t, correlated)
	assert.Equal(t, "high", overlap)
	assert.Equal(t, 1, provider.calls)
}

// TestCheckCorrelation_LowOverlapResponse verifies parsing of low overlap.
func TestCheckCorrelation_LowOverlapResponse(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `{"correlated": false, "overlap": "low", "reasoning": "Unrelated sectors."}`},
		},
	}

	correlated, overlap, ok := CheckCorrelation(context.Background(), "XOM", []string{"AAPL", "MSFT"}, provider)

	require.True(t, ok)
	assert.False(t, correlated)
	assert.Equal(t, "low", overlap)
}

// TestCheckCorrelation_LLMError returns (false, "", false) on provider error.
func TestCheckCorrelation_LLMError(t *testing.T) {
	provider := &stubLLMProvider{
		errs: []error{errors.New("connection timeout")},
	}

	correlated, overlap, ok := CheckCorrelation(context.Background(), "AAPL", []string{"MSFT"}, provider)

	assert.False(t, ok)
	assert.False(t, correlated)
	assert.Empty(t, overlap)
}

// TestCheckCorrelation_InvalidResponse returns (false, "", false) on unparseable response.
func TestCheckCorrelation_InvalidResponse(t *testing.T) {
	provider := &stubLLMProvider{
		responses: []bullarc.LLMResponse{
			{Text: `not valid json`},
		},
	}

	correlated, overlap, ok := CheckCorrelation(context.Background(), "AAPL", []string{"MSFT"}, provider)

	assert.False(t, ok)
	assert.False(t, correlated)
	assert.Empty(t, overlap)
}

// TestCheckCorrelation_AllOverlapLevels verifies that all three overlap levels
// are parsed correctly.
func TestCheckCorrelation_AllOverlapLevels(t *testing.T) {
	tests := []struct {
		overlap  string
		response string
	}{
		{"high", `{"correlated": true, "overlap": "high", "reasoning": "High overlap."}`},
		{"medium", `{"correlated": false, "overlap": "medium", "reasoning": "Medium overlap."}`},
		{"low", `{"correlated": false, "overlap": "low", "reasoning": "Low overlap."}`},
	}

	for _, tc := range tests {
		t.Run(tc.overlap, func(t *testing.T) {
			provider := &stubLLMProvider{
				responses: []bullarc.LLMResponse{{Text: tc.response}},
			}
			_, overlap, ok := CheckCorrelation(context.Background(), "TEST", []string{"OTHER"}, provider)
			require.True(t, ok)
			assert.Equal(t, tc.overlap, overlap)
		})
	}
}
