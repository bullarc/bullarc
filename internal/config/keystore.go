package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	keystoreDir     = "bullarc"
	keystoreFile    = "credentials"
	keystorePerm    = 0o600
	keystoreDirPerm = 0o700
)

// Credentials holds persistently stored API keys and preferences.
type Credentials struct {
	LLMAPIKey       string   `json:"llm_api_key,omitempty"`
	AlpacaKeyID     string   `json:"alpaca_key_id,omitempty"`
	AlpacaSecretKey string   `json:"alpaca_secret_key,omitempty"`
	Watchlist       []string `json:"watchlist,omitempty"`
}

// DefaultKeystorePath returns the default path for the credentials file.
// On Unix/macOS this is $XDG_CONFIG_HOME/bullarc/credentials or ~/.config/bullarc/credentials.
func DefaultKeystorePath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("keystore: resolve home dir: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, keystoreDir, keystoreFile), nil
}

// LoadCredentials reads the credentials file at path.
// If the file does not exist, an empty Credentials is returned without error.
func LoadCredentials(path string) (Credentials, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Credentials{}, nil
		}
		return Credentials{}, fmt.Errorf("keystore: read %s: %w", path, err)
	}
	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("keystore: parse %s: %w", path, err)
	}
	return creds, nil
}

// SaveCredentials writes creds to path with restricted file permissions (0600).
// The parent directory is created with 0700 if it does not exist.
func SaveCredentials(path string, creds Credentials) error {
	if err := os.MkdirAll(filepath.Dir(path), keystoreDirPerm); err != nil {
		return fmt.Errorf("keystore: create dir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("keystore: marshal credentials: %w", err)
	}
	if err := os.WriteFile(path, data, keystorePerm); err != nil {
		return fmt.Errorf("keystore: write %s: %w", path, err)
	}
	return nil
}
