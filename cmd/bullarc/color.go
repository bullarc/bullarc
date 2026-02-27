package main

import (
	"io"
	"os"

	"github.com/bullarc/bullarc"
)

const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
)

// isTTY reports whether w is a terminal.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// applyColor wraps the signal type in bold ANSI color codes.
// BUY is green, SELL is red, HOLD is yellow.
func applyColor(sig bullarc.SignalType) string {
	s := string(sig)
	switch sig {
	case bullarc.SignalBuy:
		return ansiBold + ansiGreen + s + ansiReset
	case bullarc.SignalSell:
		return ansiBold + ansiRed + s + ansiReset
	default: // SignalHold
		return ansiBold + ansiYellow + s + ansiReset
	}
}

// colorSignal returns the signal type as a bold, color-coded string when w is a
// terminal, or as plain text otherwise. BUY is green, SELL is red, HOLD is yellow.
func colorSignal(w io.Writer, sig bullarc.SignalType) string {
	if !isTTY(w) {
		return string(sig)
	}
	return applyColor(sig)
}
