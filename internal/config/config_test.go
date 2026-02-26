package config_test

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/bullarc/bullarc/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", name)
}

func TestLoad_ValidYAML(t *testing.T) {
	cfg, err := config.Load(testdataPath("config_test.yaml"))
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "1Hour", cfg.Engine.DefaultInterval)
	assert.Equal(t, 300, cfg.Engine.MaxBars)

	assert.ElementsMatch(t, []string{"RSI_14", "MACD_12_26_9", "VWAP"}, cfg.Indicators.Enabled)

	assert.True(t, cfg.Webhooks.Enabled)
	assert.Equal(t, []string{"http://localhost:9000/hook"}, cfg.Webhooks.URLs)
	assert.Equal(t, 5*time.Second, cfg.Webhooks.Timeout)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load(testdataPath("does_not_exist.yaml"))
	require.Error(t, err)
}

func TestLoad_EmptyConfig(t *testing.T) {
	cfg, err := config.Load(testdataPath("config_empty.yaml"))
	require.NoError(t, err)
	assert.Empty(t, cfg.Indicators.Enabled)
	assert.False(t, cfg.Webhooks.Enabled)
}
