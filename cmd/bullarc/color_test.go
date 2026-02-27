package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bullarc/bullarc"
)

func TestColorSignal_NonTTY(t *testing.T) {
	// bytes.Buffer is not a *os.File, so isTTY returns false.
	w := &bytes.Buffer{}
	cases := []struct {
		sig  bullarc.SignalType
		want string
	}{
		{bullarc.SignalBuy, "BUY"},
		{bullarc.SignalSell, "SELL"},
		{bullarc.SignalHold, "HOLD"},
	}
	for _, tc := range cases {
		got := colorSignal(w, tc.sig)
		if got != tc.want {
			t.Errorf("colorSignal(%s) = %q, want %q", tc.sig, got, tc.want)
		}
	}
}

func TestColorSignal_NonTTY_NoEscapeCodes(t *testing.T) {
	// Non-TTY output must not contain ANSI escape codes.
	w := &bytes.Buffer{}
	for _, sig := range []bullarc.SignalType{bullarc.SignalBuy, bullarc.SignalSell, bullarc.SignalHold} {
		got := colorSignal(w, sig)
		if strings.Contains(got, "\033[") {
			t.Errorf("colorSignal(%s) on non-TTY contains ANSI escape codes: %q", sig, got)
		}
	}
}

func TestApplyColor_ContainsSignalText(t *testing.T) {
	for _, sig := range []bullarc.SignalType{bullarc.SignalBuy, bullarc.SignalSell, bullarc.SignalHold} {
		got := applyColor(sig)
		if !strings.Contains(got, string(sig)) {
			t.Errorf("applyColor(%s) = %q, does not contain signal text", sig, got)
		}
	}
}

func TestApplyColor_BUY_IsGreen(t *testing.T) {
	got := applyColor(bullarc.SignalBuy)
	if !strings.Contains(got, ansiGreen) {
		t.Errorf("applyColor(BUY) = %q, want green ANSI code %q", got, ansiGreen)
	}
	if !strings.Contains(got, ansiBold) {
		t.Errorf("applyColor(BUY) = %q, want bold ANSI code %q", got, ansiBold)
	}
	if !strings.Contains(got, ansiReset) {
		t.Errorf("applyColor(BUY) = %q, want reset ANSI code %q", got, ansiReset)
	}
}

func TestApplyColor_SELL_IsRed(t *testing.T) {
	got := applyColor(bullarc.SignalSell)
	if !strings.Contains(got, ansiRed) {
		t.Errorf("applyColor(SELL) = %q, want red ANSI code %q", got, ansiRed)
	}
	if !strings.Contains(got, ansiBold) {
		t.Errorf("applyColor(SELL) = %q, want bold ANSI code %q", got, ansiBold)
	}
	if !strings.Contains(got, ansiReset) {
		t.Errorf("applyColor(SELL) = %q, want reset ANSI code %q", got, ansiReset)
	}
}

func TestApplyColor_HOLD_IsYellow(t *testing.T) {
	got := applyColor(bullarc.SignalHold)
	if !strings.Contains(got, ansiYellow) {
		t.Errorf("applyColor(HOLD) = %q, want yellow ANSI code %q", got, ansiYellow)
	}
	if !strings.Contains(got, ansiBold) {
		t.Errorf("applyColor(HOLD) = %q, want bold ANSI code %q", got, ansiBold)
	}
	if !strings.Contains(got, ansiReset) {
		t.Errorf("applyColor(HOLD) = %q, want reset ANSI code %q", got, ansiReset)
	}
}

func TestApplyColor_BUY_NotRedOrYellow(t *testing.T) {
	got := applyColor(bullarc.SignalBuy)
	if strings.Contains(got, ansiRed) {
		t.Errorf("applyColor(BUY) = %q, must not contain red ANSI code", got)
	}
	if strings.Contains(got, ansiYellow) {
		t.Errorf("applyColor(BUY) = %q, must not contain yellow ANSI code", got)
	}
}

func TestApplyColor_SELL_NotGreenOrYellow(t *testing.T) {
	got := applyColor(bullarc.SignalSell)
	if strings.Contains(got, ansiGreen) {
		t.Errorf("applyColor(SELL) = %q, must not contain green ANSI code", got)
	}
	if strings.Contains(got, ansiYellow) {
		t.Errorf("applyColor(SELL) = %q, must not contain yellow ANSI code", got)
	}
}
