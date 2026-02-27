package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bullarc/bullarc/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCredentials_FileNotExist(t *testing.T) {
	creds, err := config.LoadCredentials(filepath.Join(t.TempDir(), "credentials"))
	require.NoError(t, err)
	assert.Empty(t, creds.LLMAPIKey)
	assert.Empty(t, creds.AlpacaKeyID)
	assert.Empty(t, creds.AlpacaSecretKey)
}

func TestSaveAndLoadCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "credentials")

	in := config.Credentials{
		LLMAPIKey:       "llm-test-key",
		AlpacaKeyID:     "alpaca-key-id",
		AlpacaSecretKey: "alpaca-secret",
	}
	require.NoError(t, config.SaveCredentials(path, in))

	out, err := config.LoadCredentials(path)
	require.NoError(t, err)
	assert.Equal(t, in.LLMAPIKey, out.LLMAPIKey)
	assert.Equal(t, in.AlpacaKeyID, out.AlpacaKeyID)
	assert.Equal(t, in.AlpacaSecretKey, out.AlpacaSecretKey)
}

func TestSaveCredentials_RestrictedPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")

	require.NoError(t, config.SaveCredentials(path, config.Credentials{LLMAPIKey: "key"}))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestSaveCredentials_CreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "credentials")
	require.NoError(t, config.SaveCredentials(path, config.Credentials{LLMAPIKey: "key"}))

	_, err := os.Stat(path)
	require.NoError(t, err)
}

func TestDefaultKeystorePath_ReturnsNonEmpty(t *testing.T) {
	p, err := config.DefaultKeystorePath()
	require.NoError(t, err)
	assert.NotEmpty(t, p)
	assert.Contains(t, p, "bullarc")
	assert.Contains(t, p, "credentials")
}

func TestDefaultKeystorePath_RespectsXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	p, err := config.DefaultKeystorePath()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "bullarc", "credentials"), p)
}

func TestLoadCredentials_InvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	require.NoError(t, os.WriteFile(path, []byte("not-json"), 0o600))

	_, err := config.LoadCredentials(path)
	require.Error(t, err)
}

func TestSaveCredentials_OverwritesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")

	require.NoError(t, config.SaveCredentials(path, config.Credentials{LLMAPIKey: "old-key"}))
	require.NoError(t, config.SaveCredentials(path, config.Credentials{LLMAPIKey: "new-key"}))

	out, err := config.LoadCredentials(path)
	require.NoError(t, err)
	assert.Equal(t, "new-key", out.LLMAPIKey)
}
