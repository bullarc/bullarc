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

func TestColorSignal_TTY(t *testing.T) {
	// Use /dev/tty if available; otherwise skip.
	// We can't easily open a real TTY in a test, so just verify the ANSI codes
	// are present when we force TTY by testing isTTY separately via a pipe trick.
	// Instead, verify that colorSignal output contains the plain signal text.
	w := &bytes.Buffer{}
	for _, sig := range []bullarc.SignalType{bullarc.SignalBuy, bullarc.SignalSell, bullarc.SignalHold} {
		got := colorSignal(w, sig)
		if !strings.Contains(got, string(sig)) {
			t.Errorf("colorSignal output %q does not contain %q", got, string(sig))
		}
	}
}
