package main

import (
	"github.com/bullarc/bullarc/internal/config"
)

// parseWatchlist parses a comma-separated string of symbols into a deduplicated slice.
func parseWatchlist(s string) []string {
	return resolveSymbols("", s)
}

// loadWatchlistFromKeystore reads the persisted watchlist from the credentials file.
// Returns nil (empty) on any error.
func loadWatchlistFromKeystore() []string {
	ksPath, err := config.DefaultKeystorePath()
	if err != nil {
		return nil
	}
	creds, err := config.LoadCredentials(ksPath)
	if err != nil {
		return nil
	}
	return creds.Watchlist
}
