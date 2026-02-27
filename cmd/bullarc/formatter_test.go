package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
)

// ---- truncate ---------------------------------------------------------------

func TestTruncate_ShortString(t *testing.T) {
	got := truncate("hello", 10)
	if got != "hello" {
		t.Errorf("truncate(%q, 10) = %q, want %q", "hello", got, "hello")
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	got := truncate("hello", 5)
	if got != "hello" {
		t.Errorf("truncate(%q, 5) = %q, want %q", "hello", got, "hello")
	}
}

func TestTruncate_TooLong(t *testing.T) {
	got := truncate("hello world", 8)
	if got != "hello w…" {
		t.Errorf("truncate(%q, 8) = %q, want %q", "hello world", got, "hello w…")
	}
}

func TestTruncate_MaxOne(t *testing.T) {
	got := truncate("hello", 1)
	if got != "…" {
		t.Errorf("truncate(%q, 1) = %q, want %q", "hello", got, "…")
	}
}

func TestTruncate_MaxZero(t *testing.T) {
	got := truncate("hello", 0)
	if got != "" {
		t.Errorf("truncate(%q, 0) = %q, want empty string", "hello", got)
	}
}

func TestTruncate_Unicode(t *testing.T) {
	// "héllo" is 5 runes, truncate to 4 → "hél…"
	got := truncate("héllo", 4)
	if got != "hél…" {
		t.Errorf("truncate(\"héllo\", 4) = %q, want %q", got, "hél…")
	}
}

// ---- wordWrap ---------------------------------------------------------------

func TestWordWrap_ShortLine(t *testing.T) {
	got := wordWrap("hello world", 0, 80)
	if got != "hello world" {
		t.Errorf("wordWrap: got %q, want %q", got, "hello world")
	}
}

func TestWordWrap_Wraps(t *testing.T) {
	// "one two three" with maxTextWidth=7 → "one two\nthree"
	got := wordWrap("one two three", 0, 7)
	want := "one two\nthree"
	if got != want {
		t.Errorf("wordWrap: got %q, want %q", got, want)
	}
}

func TestWordWrap_IndentedContinuation(t *testing.T) {
	got := wordWrap("one two three four", 4, 8)
	// first line: "one two" (7 chars, fits in 8)
	// next word "three" (5) + 1 space: 7+1+5=13 > 8 → wrap with 4-space indent
	// "one two\n    three four"
	if !strings.HasPrefix(got, "one two\n    ") {
		t.Errorf("wordWrap continuation not indented: %q", got)
	}
}

func TestWordWrap_EmptyString(t *testing.T) {
	got := wordWrap("", 0, 80)
	if got != "" {
		t.Errorf("wordWrap empty: got %q", got)
	}
}

func TestWordWrap_ZeroWidth(t *testing.T) {
	s := "hello world"
	got := wordWrap(s, 0, 0)
	if got != s {
		t.Errorf("wordWrap(0 width): got %q, want passthrough %q", got, s)
	}
}

// ---- visibleLen -------------------------------------------------------------

func TestVisibleLen_PlainText(t *testing.T) {
	if n := visibleLen("hello"); n != 5 {
		t.Errorf("visibleLen(\"hello\") = %d, want 5", n)
	}
}

func TestVisibleLen_WithANSI(t *testing.T) {
	colored := ansiBold + ansiGreen + "BUY" + ansiReset
	if n := visibleLen(colored); n != 3 {
		t.Errorf("visibleLen(colored \"BUY\") = %d, want 3", n)
	}
}

func TestVisibleLen_Empty(t *testing.T) {
	if n := visibleLen(""); n != 0 {
		t.Errorf("visibleLen(\"\") = %d, want 0", n)
	}
}

// ---- padRight ---------------------------------------------------------------

func TestPadRight_Shorter(t *testing.T) {
	got := padRight("abc", 6)
	if got != "abc   " {
		t.Errorf("padRight: got %q, want %q", got, "abc   ")
	}
}

func TestPadRight_Exact(t *testing.T) {
	got := padRight("abcd", 4)
	if got != "abcd" {
		t.Errorf("padRight exact: got %q", got)
	}
}

func TestPadRight_Longer(t *testing.T) {
	got := padRight("abcdefg", 4)
	if got != "abcdefg" {
		t.Errorf("padRight longer: got %q", got)
	}
}

func TestPadRight_ANSIColored(t *testing.T) {
	colored := ansiBold + ansiGreen + "BUY" + ansiReset // 3 visible chars
	got := padRight(colored, 6)
	// should append 3 spaces to reach visible width 6
	if visibleLen(got) != 6 {
		t.Errorf("padRight(colored, 6) visible len = %d, want 6", visibleLen(got))
	}
}

// ---- PrintResult ------------------------------------------------------------

func makeResult(symbol string, sigType bullarc.SignalType, conf float64, explanation, llm string) bullarc.AnalysisResult {
	result := bullarc.AnalysisResult{
		Symbol:      symbol,
		Timestamp:   time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		LLMAnalysis: llm,
	}
	if sigType != "" {
		result.Signals = []bullarc.Signal{
			{
				Type:        sigType,
				Confidence:  conf,
				Explanation: explanation,
			},
		}
	}
	return result
}

func TestPrintResult_ContainsSymbol(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	PrintResult(&buf, makeResult("AAPL", bullarc.SignalBuy, 85, "strong momentum", ""))
	out := buf.String()
	if !strings.Contains(out, "AAPL") {
		t.Errorf("PrintResult: expected AAPL in output, got:\n%s", out)
	}
}

func TestPrintResult_ContainsSignal(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	PrintResult(&buf, makeResult("AAPL", bullarc.SignalBuy, 85, "strong momentum", ""))
	out := buf.String()
	if !strings.Contains(out, "BUY") {
		t.Errorf("PrintResult: expected BUY in output, got:\n%s", out)
	}
	if !strings.Contains(out, "85%") {
		t.Errorf("PrintResult: expected 85%% in output, got:\n%s", out)
	}
}

func TestPrintResult_NoSignals(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	result := bullarc.AnalysisResult{Symbol: "TSLA", Timestamp: time.Now()}
	PrintResult(&buf, result)
	out := buf.String()
	if !strings.Contains(out, "no signals") {
		t.Errorf("PrintResult no-signals: expected 'no signals', got:\n%s", out)
	}
}

func TestPrintResult_LLMAnalysis(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	PrintResult(&buf, makeResult("MSFT", bullarc.SignalHold, 60, "mixed signals", "The market is uncertain."))
	out := buf.String()
	if !strings.Contains(out, "explanation:") {
		t.Errorf("PrintResult: expected 'explanation:' section, got:\n%s", out)
	}
	if !strings.Contains(out, "The market is uncertain.") {
		t.Errorf("PrintResult: expected LLM text, got:\n%s", out)
	}
}

func TestPrintResult_NarrowTerminal(t *testing.T) {
	restore := overrideWidth(40)
	defer restore()

	var buf bytes.Buffer
	PrintResult(&buf, makeResult("AAPL", bullarc.SignalBuy, 85, "strong", ""))
	out := buf.String()
	if !strings.Contains(out, "too narrow") {
		t.Errorf("PrintResult narrow: expected 'too narrow' message, got:\n%s", out)
	}
}

func TestPrintResult_LongExplanationWraps(t *testing.T) {
	restore := overrideWidth(60)
	defer restore()

	long := "The RSI is currently at 72 indicating overbought conditions with strong momentum persisting across multiple timeframes."
	var buf bytes.Buffer
	PrintResult(&buf, makeResult("AAPL", bullarc.SignalBuy, 85, long, ""))
	out := buf.String()

	// Output should contain a newline in the summary section (word wrap happened)
	lines := strings.Split(out, "\n")
	// At width=60, labelWidth+1=11, so valueWidth=49; the explanation is >49 chars → should wrap
	summaryLines := 0
	for _, l := range lines {
		if strings.HasPrefix(l, strings.Repeat(" ", labelWidth+1)) {
			summaryLines++
		}
	}
	if summaryLines == 0 && !strings.Contains(out, "\n"+strings.Repeat(" ", labelWidth+1)) {
		// Just check the output has multiple non-empty lines
		nonEmpty := 0
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				nonEmpty++
			}
		}
		if nonEmpty < 3 {
			t.Errorf("PrintResult long explanation: expected wrapped output, got:\n%s", out)
		}
	}
}

// ---- PrintTable -------------------------------------------------------------

func TestPrintTable_Empty(t *testing.T) {
	restore := overrideWidth(120)
	defer restore()

	var buf bytes.Buffer
	PrintTable(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("PrintTable empty: expected no output, got %q", buf.String())
	}
}

func TestPrintTable_ContainsHeaders(t *testing.T) {
	restore := overrideWidth(120)
	defer restore()

	var buf bytes.Buffer
	PrintTable(&buf, []bullarc.AnalysisResult{
		makeResult("AAPL", bullarc.SignalBuy, 85, "strong momentum", ""),
	})
	out := buf.String()
	if !strings.Contains(out, "SYMBOL") {
		t.Errorf("PrintTable: missing SYMBOL header in:\n%s", out)
	}
	if !strings.Contains(out, "SUMMARY") {
		t.Errorf("PrintTable: missing SUMMARY header in:\n%s", out)
	}
}

func TestPrintTable_MultipleSymbols_Aligned(t *testing.T) {
	restore := overrideWidth(120)
	defer restore()

	results := []bullarc.AnalysisResult{
		makeResult("AAPL", bullarc.SignalBuy, 85, "strong momentum", ""),
		makeResult("TSLA", bullarc.SignalSell, 72, "RSI overbought", ""),
		makeResult("MSFT", bullarc.SignalHold, 60, "mixed signals", ""),
	}
	var buf bytes.Buffer
	PrintTable(&buf, results)
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// header + separator + 3 rows = 5 lines
	if len(lines) < 5 {
		t.Errorf("PrintTable: expected >= 5 lines, got %d:\n%s", len(lines), out)
	}
	// Each data line should contain the symbol
	for _, sym := range []string{"AAPL", "TSLA", "MSFT"} {
		if !strings.Contains(out, sym) {
			t.Errorf("PrintTable: missing symbol %s in:\n%s", sym, out)
		}
	}
}

func TestPrintTable_LongSymbol(t *testing.T) {
	restore := overrideWidth(120)
	defer restore()

	results := []bullarc.AnalysisResult{
		makeResult("VERYLONGSYMBOLNAME", bullarc.SignalBuy, 90, "momentum", ""),
		makeResult("A", bullarc.SignalSell, 50, "weak", ""),
	}
	var buf bytes.Buffer
	PrintTable(&buf, results)
	out := buf.String()

	if !strings.Contains(out, "VERYLONGSYMBOLNAME") {
		t.Errorf("PrintTable: long symbol truncated unexpectedly: %s", out)
	}
}

func TestPrintTable_NarrowTerminal(t *testing.T) {
	restore := overrideWidth(40)
	defer restore()

	var buf bytes.Buffer
	PrintTable(&buf, []bullarc.AnalysisResult{
		makeResult("AAPL", bullarc.SignalBuy, 85, "strong", ""),
	})
	out := buf.String()
	if !strings.Contains(out, "too narrow") {
		t.Errorf("PrintTable narrow: expected 'too narrow' message, got:\n%s", out)
	}
}

func TestPrintTable_NoSignals(t *testing.T) {
	restore := overrideWidth(120)
	defer restore()

	result := bullarc.AnalysisResult{Symbol: "GOOG", Timestamp: time.Now()}
	var buf bytes.Buffer
	PrintTable(&buf, []bullarc.AnalysisResult{result})
	out := buf.String()
	if !strings.Contains(out, "no signals") {
		t.Errorf("PrintTable no-signals: expected 'no signals' in output, got:\n%s", out)
	}
}

func TestPrintTable_LongSummaryTruncated(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	long := "This is a very long explanation that should definitely be truncated in a narrow table view."
	var buf bytes.Buffer
	PrintTable(&buf, []bullarc.AnalysisResult{
		makeResult("AAPL", bullarc.SignalBuy, 85, long, ""),
	})
	out := buf.String()
	if !strings.Contains(out, "…") {
		t.Errorf("PrintTable: long summary should be truncated with ellipsis:\n%s", out)
	}
}

// ---- resolveSymbols ---------------------------------------------------------

func TestResolveSymbols_SingleOnly(t *testing.T) {
	got := resolveSymbols("AAPL", "")
	if len(got) != 1 || got[0] != "AAPL" {
		t.Errorf("resolveSymbols single: got %v", got)
	}
}

func TestResolveSymbols_MultiOnly(t *testing.T) {
	got := resolveSymbols("", "AAPL,TSLA,MSFT")
	if len(got) != 3 {
		t.Errorf("resolveSymbols multi: got %v", got)
	}
}

func TestResolveSymbols_Both_NoDuplicates(t *testing.T) {
	got := resolveSymbols("AAPL", "AAPL,TSLA")
	if len(got) != 2 {
		t.Errorf("resolveSymbols dedup: got %v, want [AAPL TSLA]", got)
	}
}

func TestResolveSymbols_Empty(t *testing.T) {
	got := resolveSymbols("", "")
	if len(got) != 0 {
		t.Errorf("resolveSymbols empty: got %v", got)
	}
}

func TestResolveSymbols_Whitespace(t *testing.T) {
	got := resolveSymbols("", " AAPL , TSLA ")
	if len(got) != 2 {
		t.Errorf("resolveSymbols whitespace: got %v", got)
	}
	for _, s := range got {
		if strings.ContainsAny(s, " ") {
			t.Errorf("resolveSymbols: symbol has whitespace: %q", s)
		}
	}
}

// ---- helpers ----------------------------------------------------------------

// overrideWidth temporarily replaces getTerminalWidth and returns a restore func.
func overrideWidth(w int) func() {
	orig := getTerminalWidth
	getTerminalWidth = func() int { return w }
	return func() { getTerminalWidth = orig }
}
