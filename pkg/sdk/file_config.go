package sdk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bullarc/bullarc"
)

// FileConfig is a JSON-serializable SDK configuration for persistence and reuse.
// It captures symbols, indicators, interval, and named provider credentials so that
// a complete SDK setup can be saved to disk and restored in a later session.
type FileConfig struct {
	Symbols    []string       `json:"symbols,omitempty"`
	Indicators []string       `json:"indicators,omitempty"`
	Interval   string         `json:"interval,omitempty"`
	DataSource FileDataSource `json:"data_source,omitempty"`
	LLM        FileLLM        `json:"llm,omitempty"`
}

// FileDataSource holds the named data source type and its credentials.
type FileDataSource struct {
	// Type identifies the data source. Supported value: "alpaca".
	Type    string `json:"type,omitempty"`
	APIKey  string `json:"api_key,omitempty"`
	Secret  string `json:"secret,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

// FileLLM holds the named LLM provider type and its configuration.
type FileLLM struct {
	// Type identifies the LLM provider. Supported value: "anthropic".
	Type   string `json:"type,omitempty"`
	APIKey string `json:"api_key,omitempty"`
	Model  string `json:"model,omitempty"`
}

// Validate checks the FileConfig for unsupported provider types.
func (fc FileConfig) Validate() error {
	if fc.DataSource.Type != "" && fc.DataSource.Type != "alpaca" {
		return bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("unsupported data_source.type %q: supported values are \"alpaca\"", fc.DataSource.Type),
		)
	}
	if fc.LLM.Type != "" && fc.LLM.Type != "anthropic" {
		return bullarc.ErrInvalidParameter.Wrap(
			fmt.Errorf("unsupported llm.type %q: supported values are \"anthropic\"", fc.LLM.Type),
		)
	}
	return nil
}

// SaveFileConfig marshals fc to JSON and writes it to path with mode 0600.
// The parent directory is created if it does not exist.
func SaveFileConfig(path string, fc FileConfig) error {
	if err := fc.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return fmt.Errorf("sdk: marshal file config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("sdk: create config dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("sdk: write file config %s: %w", path, err)
	}
	return nil
}

// LoadFileConfig reads, parses, and validates a FileConfig from path.
func LoadFileConfig(path string) (FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, fmt.Errorf("sdk: read file config %s: %w", path, err)
	}
	var fc FileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return FileConfig{}, fmt.Errorf("sdk: parse file config %s: %w", path, err)
	}
	if err := fc.Validate(); err != nil {
		return FileConfig{}, err
	}
	return fc, nil
}

// FromFileConfig converts fc into a slice of Options for use with NewWithOptions or Configure.
// Returns an error if any field value is invalid (e.g. an unrecognised indicator name or interval).
// The options are applied in order: symbols, indicators, interval, data source, LLM.
func FromFileConfig(fc FileConfig) ([]Option, error) {
	if err := fc.Validate(); err != nil {
		return nil, err
	}
	var opts []Option
	if len(fc.Symbols) > 0 {
		opts = append(opts, WithSymbols(fc.Symbols...))
	}
	if len(fc.Indicators) > 0 {
		opts = append(opts, WithIndicators(fc.Indicators...))
	}
	if fc.Interval != "" {
		opts = append(opts, WithInterval(fc.Interval))
	}
	if fc.DataSource.Type == "alpaca" {
		opts = append(opts, WithAlpacaDataSource(fc.DataSource.APIKey, fc.DataSource.Secret))
	}
	if fc.LLM.Type == "anthropic" {
		opts = append(opts, WithAnthropicProvider(fc.LLM.APIKey, fc.LLM.Model))
	}
	return opts, nil
}
