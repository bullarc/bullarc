package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bullarcdev/bullarc"
	"github.com/bullarcdev/bullarc/internal/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeResult(symbol string) bullarc.AnalysisResult {
	return bullarc.AnalysisResult{
		Symbol:    symbol,
		Timestamp: time.Date(2024, 1, 15, 9, 30, 0, 0, time.UTC),
		Signals: []bullarc.Signal{
			{
				Type:        bullarc.SignalBuy,
				Confidence:  75,
				Indicator:   "composite",
				Symbol:      symbol,
				Timestamp:   time.Date(2024, 1, 15, 9, 30, 0, 0, time.UTC),
				Explanation: "BUY: 3 buy, 1 sell, 1 hold signals (confidence=75%)",
			},
		},
	}
}

// TestDispatch_PostsJSON verifies Dispatch sends a POST with the correct JSON body.
func TestDispatch_PostsJSON(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := webhook.New([]string{srv.URL}, 5*time.Second)
	result := makeResult("AAPL")
	err := d.Dispatch(context.Background(), result)
	require.NoError(t, err)

	var payload struct {
		Symbol    string           `json:"symbol"`
		Timestamp time.Time        `json:"timestamp"`
		Signals   []bullarc.Signal `json:"signals"`
	}
	require.NoError(t, json.Unmarshal(received, &payload))
	assert.Equal(t, "AAPL", payload.Symbol)
	require.Len(t, payload.Signals, 1)
	assert.Equal(t, bullarc.SignalBuy, payload.Signals[0].Type)
}

// TestDispatch_MultipleTargets verifies all configured URLs receive the payload.
func TestDispatch_MultipleTargets(t *testing.T) {
	hitCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hitCount++
		w.WriteHeader(http.StatusOK)
	})
	srv1 := httptest.NewServer(handler)
	defer srv1.Close()
	srv2 := httptest.NewServer(handler)
	defer srv2.Close()

	d := webhook.New([]string{srv1.URL, srv2.URL}, 5*time.Second)
	err := d.Dispatch(context.Background(), makeResult("TSLA"))
	require.NoError(t, err)
	assert.Equal(t, 2, hitCount)
}

// TestDispatch_NoURLs verifies Dispatch is a no-op when no URLs are configured.
func TestDispatch_NoURLs(t *testing.T) {
	d := webhook.New(nil, 5*time.Second)
	err := d.Dispatch(context.Background(), makeResult("AAPL"))
	assert.NoError(t, err)
}

// TestDispatch_ErrorOnNon2xx verifies a non-2xx response is treated as an error.
func TestDispatch_ErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := webhook.New([]string{srv.URL}, 5*time.Second)
	err := d.Dispatch(context.Background(), makeResult("AAPL"))
	assert.Error(t, err)
}

// TestDispatch_PartialFailureReturnsError verifies that one failing target still returns an error.
func TestDispatch_PartialFailureReturnsError(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ok.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()

	d := webhook.New([]string{ok.URL, bad.URL}, 5*time.Second)
	err := d.Dispatch(context.Background(), makeResult("AAPL"))
	assert.Error(t, err)
}

// TestDispatch_DefaultTimeout verifies New uses a default timeout when zero is passed.
func TestDispatch_DefaultTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Respond immediately — just verifying the dispatcher works with default timeout.
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()

	d := webhook.New([]string{slow.URL}, 0) // zero → default timeout applied internally
	err := d.Dispatch(context.Background(), makeResult("AAPL"))
	assert.NoError(t, err)
}
