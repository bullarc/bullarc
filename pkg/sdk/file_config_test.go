package sdk_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/engine"
	"github.com/bullarc/bullarc/pkg/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- FileConfig round-trip serialization ---

func TestFileConfig_RoundTrip_SymbolsIndicatorsInterval(t *testing.T) {
	fc := sdk.FileConfig{
		Symbols:    []string{"AAPL", "TSLA"},
		Indicators: []string{"SMA_14", "RSI_14"},
		Interval:   "1Day",
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	require.NoError(t, sdk.SaveFileConfig(path, fc))

	got, err := sdk.LoadFileConfig(path)
	require.NoError(t, err)
	assert.Equal(t, fc, got)
}

func TestFileConfig_RoundTrip_DataSource(t *testing.T) {
	fc := sdk.FileConfig{
		DataSource: sdk.FileDataSource{
			Type:   "alpaca",
			APIKey: "key123",
			Secret: "sec456",
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	require.NoError(t, sdk.SaveFileConfig(path, fc))

	got, err := sdk.LoadFileConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "alpaca", got.DataSource.Type)
	assert.Equal(t, "key123", got.DataSource.APIKey)
	assert.Equal(t, "sec456", got.DataSource.Secret)
}

func TestFileConfig_RoundTrip_LLM(t *testing.T) {
	fc := sdk.FileConfig{
		LLM: sdk.FileLLM{
			Type:   "anthropic",
			APIKey: "sk-ant-xyz",
			Model:  "claude-haiku-4-5-20251001",
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	require.NoError(t, sdk.SaveFileConfig(path, fc))

	got, err := sdk.LoadFileConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", got.LLM.Type)
	assert.Equal(t, "sk-ant-xyz", got.LLM.APIKey)
	assert.Equal(t, "claude-haiku-4-5-20251001", got.LLM.Model)
}

func TestFileConfig_RoundTrip_EmptyConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	require.NoError(t, sdk.SaveFileConfig(path, sdk.FileConfig{}))

	got, err := sdk.LoadFileConfig(path)
	require.NoError(t, err)
	assert.Equal(t, sdk.FileConfig{}, got)
}

func TestFileConfig_JSONIsReadable(t *testing.T) {
	fc := sdk.FileConfig{
		Symbols:  []string{"AAPL"},
		Interval: "1Day",
		DataSource: sdk.FileDataSource{
			Type:   "alpaca",
			APIKey: "key",
			Secret: "sec",
		},
		LLM: sdk.FileLLM{
			Type:   "anthropic",
			APIKey: "llm-key",
		},
	}
	data, err := json.MarshalIndent(fc, "", "  ")
	require.NoError(t, err)

	var got sdk.FileConfig
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, fc, got)
}

// --- Validate ---

func TestFileConfig_Validate_UnknownDataSourceType(t *testing.T) {
	fc := sdk.FileConfig{
		DataSource: sdk.FileDataSource{Type: "unknown_source"},
	}
	err := fc.Validate()
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestFileConfig_Validate_UnknownLLMType(t *testing.T) {
	fc := sdk.FileConfig{
		LLM: sdk.FileLLM{Type: "gpt4"},
	}
	err := fc.Validate()
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestFileConfig_Validate_EmptyTypesAreAccepted(t *testing.T) {
	fc := sdk.FileConfig{
		Symbols:    []string{"AAPL"},
		DataSource: sdk.FileDataSource{},
		LLM:        sdk.FileLLM{},
	}
	assert.NoError(t, fc.Validate())
}

// --- SaveFileConfig errors ---

func TestSaveFileConfig_InvalidTypeReturnsError(t *testing.T) {
	fc := sdk.FileConfig{
		DataSource: sdk.FileDataSource{Type: "bad_source"},
	}
	dir := t.TempDir()
	err := sdk.SaveFileConfig(filepath.Join(dir, "cfg.json"), fc)
	require.Error(t, err)
	var bErr *bullarc.Error
	assert.True(t, errors.As(err, &bErr))
}

// --- LoadFileConfig errors ---

func TestLoadFileConfig_NotFound(t *testing.T) {
	_, err := sdk.LoadFileConfig("/nonexistent/path/config.json")
	require.Error(t, err)
}

func TestLoadFileConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0o600))
	_, err := sdk.LoadFileConfig(path)
	require.Error(t, err)
}

func TestLoadFileConfig_UnknownTypeInFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"data_source":{"type":"mystery"}}`), 0o600))
	_, err := sdk.LoadFileConfig(path)
	require.Error(t, err)
}

// --- FromFileConfig ---

func TestFromFileConfig_BasicFields(t *testing.T) {
	fc := sdk.FileConfig{
		Symbols:    []string{"MSFT"},
		Indicators: []string{"SMA_14"},
		Interval:   "1Hour",
	}
	opts, err := sdk.FromFileConfig(fc)
	require.NoError(t, err)
	require.NotEmpty(t, opts)

	e := engine.New()
	client, err := sdk.NewWithOptions(e, opts...)
	require.NoError(t, err)

	cfg := client.Config()
	assert.Equal(t, []string{"MSFT"}, cfg.Symbols)
	assert.Equal(t, []string{"SMA_14"}, cfg.Indicators)
	assert.Equal(t, "1Hour", cfg.Interval)
}

func TestFromFileConfig_InvalidIndicatorFailsAtApply(t *testing.T) {
	fc := sdk.FileConfig{
		Indicators: []string{"BOGUS_XYZ_99"},
	}
	opts, err := sdk.FromFileConfig(fc)
	require.NoError(t, err)

	e := engine.New()
	_, err = sdk.NewWithOptions(e, opts...)
	require.Error(t, err)
}

func TestFromFileConfig_InvalidIntervalFailsAtApply(t *testing.T) {
	fc := sdk.FileConfig{
		Interval: "5Years",
	}
	opts, err := sdk.FromFileConfig(fc)
	require.NoError(t, err)

	e := engine.New()
	_, err = sdk.NewWithOptions(e, opts...)
	require.Error(t, err)
}

func TestFromFileConfig_UnknownTypeReturnsError(t *testing.T) {
	fc := sdk.FileConfig{
		DataSource: sdk.FileDataSource{Type: "csv"},
	}
	_, err := sdk.FromFileConfig(fc)
	require.Error(t, err)
}

func TestFromFileConfig_AlpacaEmptyKeyFailsAtApply(t *testing.T) {
	// Alpaca type with empty API key: FromFileConfig succeeds (validation passes),
	// but applying the option returns an error.
	fc := sdk.FileConfig{
		DataSource: sdk.FileDataSource{Type: "alpaca", APIKey: ""},
	}
	opts, err := sdk.FromFileConfig(fc)
	require.NoError(t, err)

	e := engine.New()
	_, err = sdk.NewWithOptions(e, opts...)
	require.Error(t, err)
}

func TestFromFileConfig_SetsDataSourceName(t *testing.T) {
	fc := sdk.FileConfig{
		DataSource: sdk.FileDataSource{
			Type:   "alpaca",
			APIKey: "key123",
			Secret: "sec456",
		},
	}
	opts, err := sdk.FromFileConfig(fc)
	require.NoError(t, err)

	e := engine.New()
	client, err := sdk.NewWithOptions(e, opts...)
	require.NoError(t, err)
	assert.Equal(t, "alpaca", client.Config().DataSourceName)
	assert.NotNil(t, client.Config().DataSource)
}

func TestFromFileConfig_SetsLLMProviderName(t *testing.T) {
	fc := sdk.FileConfig{
		LLM: sdk.FileLLM{
			Type:   "anthropic",
			APIKey: "sk-ant-key",
		},
	}
	opts, err := sdk.FromFileConfig(fc)
	require.NoError(t, err)

	e := engine.New()
	client, err := sdk.NewWithOptions(e, opts...)
	require.NoError(t, err)
	assert.Equal(t, "anthropic", client.Config().LLMProviderName)
	assert.NotNil(t, client.Config().LLMProvider)
}

// --- Round-trip via engine: engine initializes with all settings applied ---

func TestFileConfig_EngineInitializesWithAllSettings(t *testing.T) {
	fc := sdk.FileConfig{
		Symbols:    []string{"AAPL", "MSFT"},
		Indicators: []string{"SMA_14", "RSI_14"},
		Interval:   "1Day",
	}
	opts, err := sdk.FromFileConfig(fc)
	require.NoError(t, err)

	e := engine.New()
	for _, ind := range engine.DefaultIndicators() {
		e.RegisterIndicator(ind)
	}
	client, err := sdk.NewWithOptions(e, opts...)
	require.NoError(t, err)

	cfg := client.Config()
	assert.Equal(t, []string{"AAPL", "MSFT"}, cfg.Symbols)
	assert.Equal(t, []string{"SMA_14", "RSI_14"}, cfg.Indicators)
	assert.Equal(t, "1Day", cfg.Interval)
}

// --- Conflict detection ---

func TestWithAlpacaDataSource_ConflictsWithCustomDataSource(t *testing.T) {
	ds := &trackingDataSource{}
	e := engine.New()
	// Custom data source first, then Alpaca → conflict
	_, err := sdk.NewWithOptions(e,
		sdk.WithDataSource(ds),
		sdk.WithAlpacaDataSource("key123", "secret456"),
	)
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestWithDataSource_ConflictsWithAlpacaDataSource(t *testing.T) {
	ds := &trackingDataSource{}
	e := engine.New()
	// Alpaca first, then custom data source → conflict
	_, err := sdk.NewWithOptions(e,
		sdk.WithAlpacaDataSource("key123", "secret456"),
		sdk.WithDataSource(ds),
	)
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestWithAlpacaDataSource_EmptyKeyReturnsError(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e, sdk.WithAlpacaDataSource("", "secret"))
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestWithAlpacaDataSource_SetsDataSourceAndName(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithAlpacaDataSource("key123", "secret456"))
	require.NoError(t, err)
	assert.Equal(t, "alpaca", client.Config().DataSourceName)
	assert.NotNil(t, client.Config().DataSource)
}

func TestWithAlpacaDataSource_CanBeReplacedByAnotherAlpaca(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e,
		sdk.WithAlpacaDataSource("key1", "sec1"),
	)
	require.NoError(t, err)

	// Reconfigure with different Alpaca creds; should succeed (named → named is fine)
	err = client.Configure(sdk.WithAlpacaDataSource("key2", "sec2"))
	require.NoError(t, err)
	assert.Equal(t, "alpaca", client.Config().DataSourceName)
}

func TestWithAnthropicProvider_ConflictsWithCustomLLMProvider(t *testing.T) {
	llmp := &trackingLLMProvider{name: "stub"}
	e := engine.New()
	// Custom LLM first, then Anthropic → conflict
	_, err := sdk.NewWithOptions(e,
		sdk.WithLLMProvider(llmp),
		sdk.WithAnthropicProvider("sk-ant-key", ""),
	)
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestWithLLMProvider_ConflictsWithAnthropicProvider(t *testing.T) {
	llmp := &trackingLLMProvider{name: "stub"}
	e := engine.New()
	// Anthropic first, then custom LLM → conflict
	_, err := sdk.NewWithOptions(e,
		sdk.WithAnthropicProvider("sk-ant-key", ""),
		sdk.WithLLMProvider(llmp),
	)
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestWithAnthropicProvider_EmptyKeyReturnsError(t *testing.T) {
	e := engine.New()
	_, err := sdk.NewWithOptions(e, sdk.WithAnthropicProvider("", ""))
	require.Error(t, err)
	var bErr *bullarc.Error
	require.True(t, errors.As(err, &bErr))
	assert.Equal(t, "INVALID_PARAMETER", bErr.Code)
}

func TestWithAnthropicProvider_SetsLLMProviderAndName(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithAnthropicProvider("sk-ant-key", ""))
	require.NoError(t, err)
	assert.Equal(t, "anthropic", client.Config().LLMProviderName)
	assert.NotNil(t, client.Config().LLMProvider)
}

func TestWithAnthropicProvider_CanBeReplacedByAnotherAnthropic(t *testing.T) {
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithAnthropicProvider("sk-ant-key1", ""))
	require.NoError(t, err)

	// Reconfigure with different key; should succeed
	err = client.Configure(sdk.WithAnthropicProvider("sk-ant-key2", ""))
	require.NoError(t, err)
	assert.Equal(t, "anthropic", client.Config().LLMProviderName)
}

// --- Configure rollback includes new fields ---

func TestConfigure_ConflictRollsBackDataSourceName(t *testing.T) {
	ds := &trackingDataSource{}
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithAlpacaDataSource("key", "sec"))
	require.NoError(t, err)
	before := client.Config()

	// Trying to add a custom data source should fail and leave config unchanged.
	err = client.Configure(sdk.WithDataSource(ds))
	require.Error(t, err)
	assert.Equal(t, before.DataSourceName, client.Config().DataSourceName)
	assert.Equal(t, before.DataSource, client.Config().DataSource)
}

func TestConfigure_ConflictRollsBackLLMProviderName(t *testing.T) {
	llmp := &trackingLLMProvider{name: "stub"}
	e := engine.New()
	client, err := sdk.NewWithOptions(e, sdk.WithAnthropicProvider("sk-ant-key", ""))
	require.NoError(t, err)
	before := client.Config()

	// Trying to add a custom LLM should fail and leave config unchanged.
	err = client.Configure(sdk.WithLLMProvider(llmp))
	require.Error(t, err)
	assert.Equal(t, before.LLMProviderName, client.Config().LLMProviderName)
	assert.Equal(t, before.LLMProvider, client.Config().LLMProvider)
}
