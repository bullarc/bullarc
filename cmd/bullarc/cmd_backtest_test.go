package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
)

// ---- parseIndicatorList -----------------------------------------------------

func TestParseIndicatorList_Empty(t *testing.T) {
	got := parseIndicatorList("")
	if got != nil {
		t.Errorf("parseIndicatorList(\"\") = %v, want nil", got)
	}
}

func TestParseIndicatorList_Single(t *testing.T) {
	got := parseIndicatorList("SMA_14")
	if len(got) != 1 || got[0] != "SMA_14" {
		t.Errorf("parseIndicatorList(\"SMA_14\") = %v, want [SMA_14]", got)
	}
}

func TestParseIndicatorList_Multiple(t *testing.T) {
	got := parseIndicatorList("SMA_14,RSI_14,MACD")
	if len(got) != 3 {
		t.Errorf("parseIndicatorList: got %v, want 3 elements", got)
	}
}

func TestParseIndicatorList_TrimsSpaces(t *testing.T) {
	got := parseIndicatorList(" SMA_14 , RSI_14 ")
	for _, s := range got {
		if strings.ContainsAny(s, " ") {
			t.Errorf("parseIndicatorList: entry has whitespace: %q", s)
		}
	}
}

func TestParseIndicatorList_SkipsEmptySegments(t *testing.T) {
	got := parseIndicatorList("SMA_14,,RSI_14")
	if len(got) != 2 {
		t.Errorf("parseIndicatorList empty segments: got %v, want 2 elements", got)
	}
}

// ---- PrintBacktestResult ----------------------------------------------------

func makeBacktestResult(symbol string, total, buy, sell, hold int, simReturn, drawdown, winRate float64) bullarc.BacktestResult {
	return bullarc.BacktestResult{
		Symbol:    symbol,
		Timestamp: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
		Summary: bullarc.BacktestSummary{
			TotalSignals: total,
			BuyCount:     buy,
			SellCount:    sell,
			HoldCount:    hold,
			SimReturn:    simReturn,
			MaxDrawdown:  drawdown,
			WinRate:      winRate,
		},
	}
}

func TestPrintBacktestResult_ContainsSymbol(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	PrintBacktestResult(&buf, makeBacktestResult("AAPL", 10, 4, 3, 3, 5.5, 2.1, 66.7))
	out := buf.String()
	if !strings.Contains(out, "AAPL") {
		t.Errorf("PrintBacktestResult: expected AAPL in output, got:\n%s", out)
	}
}

func TestPrintBacktestResult_ContainsSignalCounts(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	PrintBacktestResult(&buf, makeBacktestResult("TSLA", 9, 4, 3, 2, 8.0, 3.0, 75.0))
	out := buf.String()

	for _, want := range []string{"9", "4", "3", "2"} {
		if !strings.Contains(out, want) {
			t.Errorf("PrintBacktestResult: expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestPrintBacktestResult_ContainsReturnMetrics(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	PrintBacktestResult(&buf, makeBacktestResult("MSFT", 5, 2, 2, 1, 12.34, 5.67, 50.0))
	out := buf.String()

	if !strings.Contains(out, "12.34%") {
		t.Errorf("PrintBacktestResult: expected return 12.34%% in output, got:\n%s", out)
	}
	if !strings.Contains(out, "5.67%") {
		t.Errorf("PrintBacktestResult: expected drawdown 5.67%% in output, got:\n%s", out)
	}
	if !strings.Contains(out, "50.0%") {
		t.Errorf("PrintBacktestResult: expected win rate 50.0%% in output, got:\n%s", out)
	}
}

func TestPrintBacktestResult_NoSignals(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	PrintBacktestResult(&buf, makeBacktestResult("GOOG", 0, 0, 0, 0, 0, 0, 0))
	out := buf.String()
	if !strings.Contains(out, "no signals") {
		t.Errorf("PrintBacktestResult no-signals: expected 'no signals', got:\n%s", out)
	}
}

func TestPrintBacktestResult_NarrowTerminal(t *testing.T) {
	restore := overrideWidth(40)
	defer restore()

	var buf bytes.Buffer
	PrintBacktestResult(&buf, makeBacktestResult("AAPL", 10, 4, 3, 3, 5.5, 2.1, 66.7))
	out := buf.String()
	if !strings.Contains(out, "too narrow") {
		t.Errorf("PrintBacktestResult narrow: expected 'too narrow' message, got:\n%s", out)
	}
}

func TestPrintBacktestResult_ContainsTimestamp(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	PrintBacktestResult(&buf, makeBacktestResult("AAPL", 10, 4, 3, 3, 5.5, 2.1, 66.7))
	out := buf.String()
	if !strings.Contains(out, "2024-06-01") {
		t.Errorf("PrintBacktestResult: expected date 2024-06-01 in output, got:\n%s", out)
	}
}

func TestPrintBacktestResult_ContainsLabels(t *testing.T) {
	restore := overrideWidth(80)
	defer restore()

	var buf bytes.Buffer
	PrintBacktestResult(&buf, makeBacktestResult("AAPL", 5, 2, 2, 1, 3.0, 1.0, 50.0))
	out := buf.String()

	for _, label := range []string{"signals:", "buy:", "sell:", "hold:", "return:", "drawdown:", "win rate:"} {
		if !strings.Contains(out, label) {
			t.Errorf("PrintBacktestResult: expected label %q in output, got:\n%s", label, out)
		}
	}
}
