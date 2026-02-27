package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bullarc/bullarc"
)

const (
	// minWidth is the minimum terminal width required for a usable display.
	minWidth     = 60
	defaultWidth = 80
	labelWidth   = 10 // "timestamp:" is 10 chars
)

// getTerminalWidth is the width resolver used by PrintResult and PrintTable.
// Replaced in tests to simulate narrow or wide terminals.
var getTerminalWidth = func() int {
	if cols, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && cols > 0 {
		return cols
	}
	return defaultWidth
}

// truncate returns s truncated to max visible runes.
// The last rune is replaced with '…' when the string exceeds max.
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// wordWrap wraps s so that no text segment exceeds maxTextWidth runes per line.
// Continuation lines are prefixed with indent spaces.
func wordWrap(s string, indent, maxTextWidth int) string {
	if maxTextWidth <= 0 {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}

	var sb strings.Builder
	lineUsed := 0
	for _, word := range words {
		wlen := utf8.RuneCountInString(word)
		if lineUsed == 0 {
			sb.WriteString(word)
			lineUsed = wlen
		} else if lineUsed+1+wlen > maxTextWidth {
			sb.WriteByte('\n')
			sb.WriteString(strings.Repeat(" ", indent))
			sb.WriteString(word)
			lineUsed = wlen
		} else {
			sb.WriteByte(' ')
			sb.WriteString(word)
			lineUsed += 1 + wlen
		}
	}
	return sb.String()
}

// visibleLen returns the number of visible runes in s, excluding ANSI escape sequences.
func visibleLen(s string) int {
	inEscape := false
	count := 0
	for _, r := range s {
		switch {
		case r == '\033':
			inEscape = true
		case inEscape:
			if r == 'm' {
				inEscape = false
			}
		default:
			count++
		}
	}
	return count
}

// padRight pads s to width visible characters by appending spaces.
func padRight(s string, width int) string {
	n := visibleLen(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// PrintResult writes a human-readable formatted analysis result to w.
func PrintResult(w io.Writer, result bullarc.AnalysisResult) {
	width := getTerminalWidth()
	if width < minWidth {
		fmt.Fprintf(w, "terminal too narrow (need %d cols, got %d)\n", minWidth, width)
		return
	}

	valueWidth := width - labelWidth - 1
	if valueWidth < 10 {
		valueWidth = 10
	}

	fmt.Fprintf(w, "%-*s %s\n", labelWidth, "symbol:", result.Symbol)
	fmt.Fprintf(w, "%-*s %s\n", labelWidth, "timestamp:", result.Timestamp.Format(time.RFC3339))

	if len(result.Signals) == 0 {
		fmt.Fprintln(w, "no signals (insufficient data)")
		return
	}

	composite := result.Signals[0]
	sig := colorSignal(w, composite.Type)
	fmt.Fprintf(w, "%-*s %s (confidence=%.0f%%)\n", labelWidth, "signal:", sig, composite.Confidence)

	wrapped := wordWrap(composite.Explanation, labelWidth+1, valueWidth)
	fmt.Fprintf(w, "%-*s %s\n", labelWidth, "summary:", wrapped)

	if result.LLMAnalysis != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "explanation:")
		fmt.Fprintln(w, wordWrap(result.LLMAnalysis, 0, width))
	}
}

// PrintTable writes a table of analysis results aligned in columns to w.
func PrintTable(w io.Writer, results []bullarc.AnalysisResult) {
	if len(results) == 0 {
		return
	}

	width := getTerminalWidth()
	if width < minWidth {
		fmt.Fprintf(w, "terminal too narrow (need %d cols, got %d)\n", minWidth, width)
		return
	}

	const (
		colPad  = 2
		signalW = 4 // max len of BUY/SELL/HOLD
		confW   = 4 // "100%"
	)

	symW := len("SYMBOL")
	for _, r := range results {
		if n := utf8.RuneCountInString(r.Symbol); n > symW {
			symW = n
		}
	}

	// fixed: symW + colPad + signalW + colPad + confW + colPad
	fixed := symW + colPad + signalW + colPad + confW + colPad
	summaryW := width - fixed
	if summaryW < 10 {
		summaryW = 10
	}

	// Header
	fmt.Fprintln(w, padRight("SYMBOL", symW)+"  "+padRight("SIGN", signalW)+"  "+padRight("CONF", confW)+"  "+"SUMMARY")

	// Separator
	sep := strings.Repeat("─", symW) + "  " +
		strings.Repeat("─", signalW) + "  " +
		strings.Repeat("─", confW) + "  " +
		strings.Repeat("─", summaryW)
	fmt.Fprintln(w, sep)

	// Rows
	for _, r := range results {
		sym := padRight(r.Symbol, symW)
		sig := padRight("─", signalW)
		conf := padRight("─", confW)
		summary := "no signals"
		if len(r.Signals) > 0 {
			s := r.Signals[0]
			sig = padRight(colorSignal(w, s.Type), signalW)
			conf = padRight(fmt.Sprintf("%.0f%%", s.Confidence), confW)
			summary = truncate(s.Explanation, summaryW)
		}
		fmt.Fprintf(w, "%s  %s  %s  %s\n", sym, sig, conf, summary)
	}
}
