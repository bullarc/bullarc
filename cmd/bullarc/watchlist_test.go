package main

import (
	"path/filepath"
	"testing"

	"github.com/bullarc/bullarc/internal/config"
)

func TestParseWatchlist_CommaSeparated(t *testing.T) {
	got := parseWatchlist("AAPL,MSFT,BTC/USD")
	want := []string{"AAPL", "MSFT", "BTC/USD"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i, sym := range want {
		if got[i] != sym {
			t.Errorf("index %d: expected %q, got %q", i, sym, got[i])
		}
	}
}

func TestParseWatchlist_Deduplicates(t *testing.T) {
	got := parseWatchlist("AAPL,MSFT,AAPL")
	if len(got) != 2 {
		t.Fatalf("expected 2 symbols after deduplication, got %v", got)
	}
}

func TestParseWatchlist_TrimsSpaces(t *testing.T) {
	got := parseWatchlist(" AAPL , MSFT ")
	want := []string{"AAPL", "MSFT"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i, sym := range want {
		if got[i] != sym {
			t.Errorf("index %d: expected %q, got %q", i, sym, got[i])
		}
	}
}

func TestParseWatchlist_EmptyString(t *testing.T) {
	got := parseWatchlist("")
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

func TestLoadWatchlistFromKeystore_ReturnsWatchlist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	ksPath := filepath.Join(tmpDir, "bullarc", "credentials")
	err := config.SaveCredentials(ksPath, config.Credentials{
		Watchlist: []string{"AAPL", "MSFT", "BTC/USD"},
	})
	if err != nil {
		t.Fatalf("save credentials: %v", err)
	}

	got := loadWatchlistFromKeystore()
	want := []string{"AAPL", "MSFT", "BTC/USD"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i, sym := range want {
		if got[i] != sym {
			t.Errorf("index %d: expected %q, got %q", i, sym, got[i])
		}
	}
}

func TestLoadWatchlistFromKeystore_ReturnsNilWhenEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	ksPath := filepath.Join(tmpDir, "bullarc", "credentials")
	err := config.SaveCredentials(ksPath, config.Credentials{LLMAPIKey: "key"})
	if err != nil {
		t.Fatalf("save credentials: %v", err)
	}

	got := loadWatchlistFromKeystore()
	if len(got) != 0 {
		t.Fatalf("expected empty watchlist, got %v", got)
	}
}

func TestLoadWatchlistFromKeystore_ReturnsNilWhenNoFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	got := loadWatchlistFromKeystore()
	if len(got) != 0 {
		t.Fatalf("expected nil when no credentials file, got %v", got)
	}
}
