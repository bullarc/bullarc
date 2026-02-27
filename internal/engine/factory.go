package engine

import (
	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/datasource"
	"github.com/bullarc/bullarc/internal/llm"
)

// NewAlpacaDataSource creates a DataSource backed by the Alpaca Markets API.
// It is exposed here so that pkg/sdk can build named data sources without
// importing internal/datasource directly.
func NewAlpacaDataSource(keyID, secretKey string) bullarc.DataSource {
	return datasource.NewAlpacaSource(keyID, secretKey)
}

// NewAnthropicProvider creates an LLMProvider backed by the Anthropic Messages API.
// If model is empty the default model is used.
// It is exposed here so that pkg/sdk can build named LLM providers without
// importing internal/llm directly.
func NewAnthropicProvider(apiKey, model string) bullarc.LLMProvider {
	return llm.NewAnthropicProvider(apiKey, model)
}
