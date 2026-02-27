package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level application configuration.
type Config struct {
	Engine       EngineConfig       `json:"engine" yaml:"engine"`
	DataSources  DataSourcesConfig  `json:"data_sources" yaml:"data_sources"`
	Indicators   IndicatorsConfig   `json:"indicators" yaml:"indicators"`
	LLM          LLMConfig          `json:"llm" yaml:"llm"`
	MCP          MCPConfig          `json:"mcp" yaml:"mcp"`
	Webhooks     WebhookConfig      `json:"webhooks" yaml:"webhooks"`
	Social       SocialConfig       `json:"social" yaml:"social"`
	PaperTrading PaperTradingConfig `json:"paper_trading" yaml:"paper_trading"`
}

// EngineConfig configures the analysis engine.
type EngineConfig struct {
	DefaultSymbol   string        `json:"default_symbol" yaml:"default_symbol"`
	DefaultInterval string        `json:"default_interval" yaml:"default_interval"`
	Timeout         time.Duration `json:"timeout" yaml:"timeout"`
	MaxBars         int           `json:"max_bars" yaml:"max_bars"`
}

// DataSourcesConfig configures data source providers.
type DataSourcesConfig struct {
	Default string                 `json:"default" yaml:"default"`
	Alpaca  AlpacaDataSourceConfig `json:"alpaca" yaml:"alpaca"`
	Massive DataSourceConfig       `json:"massive" yaml:"massive"`
	Yahoo   DataSourceConfig       `json:"yahoo" yaml:"yahoo"`
}

// AlpacaDataSourceConfig configures the Alpaca Markets data source.
type AlpacaDataSourceConfig struct {
	Enabled   bool   `json:"enabled" yaml:"enabled"`
	KeyID     string `json:"key_id" yaml:"key_id"`
	SecretKey string `json:"secret_key" yaml:"secret_key"`
	BaseURL   string `json:"base_url" yaml:"base_url"`
}

// DataSourceConfig configures a single data source.
type DataSourceConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	APIKey  string `json:"api_key" yaml:"api_key"`
	BaseURL string `json:"base_url" yaml:"base_url"`
}

// IndicatorsConfig configures which indicators are active and their defaults.
type IndicatorsConfig struct {
	Enabled    []string       `json:"enabled" yaml:"enabled"`
	Parameters map[string]any `json:"parameters" yaml:"parameters"`
}

// LLMConfig configures LLM provider settings.
type LLMConfig struct {
	Provider    string  `json:"provider" yaml:"provider"`
	Model       string  `json:"model" yaml:"model"`
	APIKey      string  `json:"api_key" yaml:"api_key"`
	MaxTokens   int     `json:"max_tokens" yaml:"max_tokens"`
	Temperature float64 `json:"temperature" yaml:"temperature"`
}

// MCPConfig configures the MCP server.
type MCPConfig struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Address string `json:"address" yaml:"address"`
}

// WebhookConfig configures outbound webhook delivery for signals.
type WebhookConfig struct {
	Enabled bool          `json:"enabled" yaml:"enabled"`
	URLs    []string      `json:"urls" yaml:"urls"`
	Timeout time.Duration `json:"timeout" yaml:"timeout"`
}

// SocialConfig configures Reddit mention tracking.
type SocialConfig struct {
	Enabled        bool          `json:"enabled" yaml:"enabled"`
	Provider       string        `json:"provider" yaml:"provider"`               // "tradestie" (default) or "apewisdom"
	PollInterval   time.Duration `json:"poll_interval" yaml:"poll_interval"`     // default: 15 minutes
	SpikeThreshold float64       `json:"spike_threshold" yaml:"spike_threshold"` // default: 3.0
}

// PaperTradingConfig configures Alpaca paper trading mode.
type PaperTradingConfig struct {
	Enabled             bool    `json:"enabled" yaml:"enabled"`
	KeyID               string  `json:"key_id" yaml:"key_id"`
	SecretKey           string  `json:"secret_key" yaml:"secret_key"`
	BaseURL             string  `json:"base_url" yaml:"base_url"`
	AutoExecute         bool    `json:"auto_execute" yaml:"auto_execute"`
	ConfidenceThreshold float64 `json:"confidence_threshold" yaml:"confidence_threshold"` // default: 70
}

// Load reads a YAML (or JSON) config file at path and returns the parsed Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return &cfg, nil
}
