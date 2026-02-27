package main

import (
	"path/filepath"
	"testing"

	"github.com/bullarc/bullarc/internal/config"
)

func TestResolveSymbols_SingleFlag(t *testing.T) {
	got := resolveSymbols("AAPL", "")
	if len(got) != 1 || got[0] != "AAPL" {
		t.Fatalf("expected [AAPL], got %v", got)
	}
}

func TestResolveSymbols_MultiFlag(t *testing.T) {
	got := resolveSymbols("", "AAPL,MSFT")
	if len(got) != 2 {
		t.Fatalf("expected 2 symbols, got %v", got)
	}
	if got[0] != "AAPL" || got[1] != "MSFT" {
		t.Errorf("unexpected order: %v", got)
	}
}

func TestResolveSymbols_DeduplicatesAcrossFlags(t *testing.T) {
	got := resolveSymbols("AAPL", "AAPL,MSFT")
	if len(got) != 2 {
		t.Fatalf("expected 2 symbols after dedup, got %v", got)
	}
}

func TestResolveSymbols_EmptyReturnEmpty(t *testing.T) {
	got := resolveSymbols("", "")
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestAnalyze_WatchlistFallback_UsesStoredSymbols(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	ksPath := filepath.Join(tmpDir, "bullarc", "credentials")
	err := config.SaveCredentials(ksPath, config.Credentials{
		Watchlist: []string{"AAPL", "MSFT"},
	})
	if err != nil {
		t.Fatalf("save credentials: %v", err)
	}

	// When no symbols are provided, watchlist is loaded from keystore.
	syms := resolveSymbols("", "")
	if len(syms) != 0 {
		t.Fatalf("resolveSymbols should return empty without flags, got %v", syms)
	}

	// loadWatchlistFromKeystore should return the saved symbols.
	got := loadWatchlistFromKeystore()
	if len(got) != 2 || got[0] != "AAPL" || got[1] != "MSFT" {
		t.Fatalf("expected [AAPL MSFT] from watchlist, got %v", got)
	}
}

func TestAnalyze_CLISymbolsOverrideWatchlist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	ksPath := filepath.Join(tmpDir, "bullarc", "credentials")
	err := config.SaveCredentials(ksPath, config.Credentials{
		Watchlist: []string{"AAPL", "MSFT"},
	})
	if err != nil {
		t.Fatalf("save credentials: %v", err)
	}

	// When a symbol is provided on the CLI, it should be used directly (watchlist is not consulted).
	syms := resolveSymbols("TSLA", "")
	if len(syms) != 1 || syms[0] != "TSLA" {
		t.Fatalf("expected [TSLA], got %v", syms)
	}
	// Since resolveSymbols returned non-empty, the watchlist should not be loaded.
}
