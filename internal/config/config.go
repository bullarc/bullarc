package config

import "time"

// Config is the top-level application configuration.
type Config struct {
	Engine      EngineConfig      `json:"engine" yaml:"engine"`
	DataSources DataSourcesConfig `json:"data_sources" yaml:"data_sources"`
	Indicators  IndicatorsConfig  `json:"indicators" yaml:"indicators"`
	LLM         LLMConfig         `json:"llm" yaml:"llm"`
	MCP         MCPConfig         `json:"mcp" yaml:"mcp"`
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
	Default string           `json:"default" yaml:"default"`
	Polygon DataSourceConfig `json:"polygon" yaml:"polygon"`
	Yahoo   DataSourceConfig `json:"yahoo" yaml:"yahoo"`
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
