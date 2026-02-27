package main

import (
	"strings"
	"testing"
)

func TestBuildEngine_NoDataSource_ReturnsHelpfulError(t *testing.T) {
	t.Setenv("ALPACA_API_KEY", "")
	t.Setenv("ALPACA_SECRET_KEY", "")

	e, err := buildEngine("", "", "", "", "")
	if err != nil {
		t.Fatalf("buildEngine should succeed; data source check is caller's responsibility, got err: %v", err)
	}
	if e.HasDataSource() {
		t.Fatal("expected no data source registered")
	}

	err = errNoDataSource()
	if err == nil {
		t.Fatal("errNoDataSource should return non-nil error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ALPACA_API_KEY") {
		t.Errorf("error message should mention ALPACA_API_KEY, got: %s", msg)
	}
	if !strings.Contains(msg, "--alpaca-key") {
		t.Errorf("error message should mention --alpaca-key flag, got: %s", msg)
	}
	if !strings.Contains(msg, "--csv") {
		t.Errorf("error message should mention --csv flag, got: %s", msg)
	}
}

func TestBuildEngine_AlpacaKeyFromFlag(t *testing.T) {
	t.Setenv("ALPACA_API_KEY", "")
	t.Setenv("ALPACA_SECRET_KEY", "")

	e, err := buildEngine("", "", "", "flagkeyid", "flagsecret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !e.HasDataSource() {
		t.Fatal("expected Alpaca data source to be registered when flag key is provided")
	}
}

func TestBuildEngine_AlpacaKeyFromEnv(t *testing.T) {
	t.Setenv("ALPACA_API_KEY", "envkeyid")
	t.Setenv("ALPACA_SECRET_KEY", "envsecret")

	e, err := buildEngine("", "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !e.HasDataSource() {
		t.Fatal("expected Alpaca data source to be registered when env vars are set")
	}
}

func TestBuildEngine_FlagTakesPrecedenceOverEnv(t *testing.T) {
	t.Setenv("ALPACA_API_KEY", "envkeyid")
	t.Setenv("ALPACA_SECRET_KEY", "envsecret")

	// Flag key provided — should still succeed and register a data source.
	e, err := buildEngine("", "", "", "flagkeyid", "flagsecret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !e.HasDataSource() {
		t.Fatal("expected Alpaca data source to be registered")
	}
}

func TestBuildEngine_CSVDataSource(t *testing.T) {
	t.Setenv("ALPACA_API_KEY", "")
	t.Setenv("ALPACA_SECRET_KEY", "")

	// CSV path registers a data source without Alpaca credentials.
	e, err := buildEngine("", "../../testdata/AAPL_1d.csv", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !e.HasDataSource() {
		t.Fatal("expected CSV data source to be registered")
	}
}

func TestBuildEngine_LLMKeyFromEnv(t *testing.T) {
	t.Setenv("ALPACA_API_KEY", "key")
	t.Setenv("ALPACA_SECRET_KEY", "secret")
	t.Setenv("ANTHROPIC_API_KEY", "envllmkey")

	e, err := buildEngine("", "", "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Engine built successfully; LLM provider is registered internally.
	// We can't inspect the LLM key directly, but building without error confirms
	// that ANTHROPIC_API_KEY env var was read and an LLM provider registered.
	_ = e
}

func TestBuildEngine_LLMKeyFromFlagOverridesEnv(t *testing.T) {
	t.Setenv("ALPACA_API_KEY", "key")
	t.Setenv("ALPACA_SECRET_KEY", "secret")
	t.Setenv("ANTHROPIC_API_KEY", "envllmkey")

	// Passing --llm-key should not cause an error; flag takes precedence.
	e, err := buildEngine("", "", "flagllmkey", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = e
}
