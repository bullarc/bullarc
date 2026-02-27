package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bullarc/bullarc"
	"github.com/bullarc/bullarc/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubBackend is a test double implementing the mcp.Backend interface.
type stubBackend struct {
	analyzeFunc func(ctx context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error)
}

func (s *stubBackend) Analyze(ctx context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
	if s.analyzeFunc != nil {
		return s.analyzeFunc(ctx, req)
	}
	return bullarc.AnalysisResult{Symbol: req.Symbol, Timestamp: time.Now()}, nil
}

func (s *stubBackend) BacktestCSV(_ context.Context, _, _ string, _ []string) (bullarc.BacktestResult, error) {
	return bullarc.BacktestResult{}, nil
}

func (s *stubBackend) ListIndicators() []bullarc.IndicatorMeta { return nil }

// callGetSignals is a helper that invokes the get_signals tool registered on a server
// built with the given backend.
func callGetSignals(t *testing.T, b mcp.Backend, args map[string]any) (string, bool) {
	t.Helper()
	srv := mcp.New("test", "0.0.0")
	mcp.RegisterTools(srv, b)

	// Find the get_signals tool by looking it up on the server and calling its handler.
	// We exercise the handler via the exported RegisterTools + a direct lookup helper.
	// Since Handler is embedded in the Tool struct, we need to go via RegisterTools.
	// Use ExposedHandlerForTest if available — instead we call via JSON-RPC dispatch.
	return invokeToolViaServer(t, srv, "get_signals", args)
}

// invokeToolViaServer calls a named tool on srv by driving its JSON-RPC dispatch loop
// with a single tools/call message.
func invokeToolViaServer(t *testing.T, srv *mcp.Server, toolName string, args map[string]any) (string, bool) {
	t.Helper()

	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": json.RawMessage(argsJSON),
		},
	}
	reqJSON, err := json.Marshal(req)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(reqJSON, &result))

	// Drive the server via its exported DispatchForTest — since that isn't available we
	// instead access the result through the backend adapter pattern used in RegisterTools.
	// Invoke the tool handler directly through the server's in-process dispatch helper.
	return dispatchAndExtract(t, srv, reqJSON)
}

// dispatchAndExtract exercises the server using a pipe-based JSON-RPC invocation.
func dispatchAndExtract(t *testing.T, srv *mcp.Server, reqJSON []byte) (string, bool) {
	t.Helper()
	return mcp.DispatchForTest(t, srv, reqJSON)
}

// TestGetSignals_MissingSymbols verifies that omitting the symbols argument returns an error.
func TestGetSignals_MissingSymbols(t *testing.T) {
	b := &stubBackend{}
	text, isError := callGetSignals(t, b, map[string]any{})
	assert.True(t, isError, "expected isError=true for missing symbols")
	assert.Contains(t, text, "symbols is required")
}

// TestGetSignals_EmptySymbolsArray verifies that passing an empty symbols array returns an error.
func TestGetSignals_EmptySymbolsArray(t *testing.T) {
	b := &stubBackend{}
	text, isError := callGetSignals(t, b, map[string]any{"symbols": []any{}})
	assert.True(t, isError, "expected isError=true for empty symbols")
	assert.Contains(t, text, "symbols is required")
}

// TestGetSignals_EmptyStringSymbol verifies that a symbol that is an empty string returns an error.
func TestGetSignals_EmptyStringSymbol(t *testing.T) {
	b := &stubBackend{}
	text, isError := callGetSignals(t, b, map[string]any{"symbols": []any{""}})
	assert.True(t, isError, "expected isError=true for empty-string symbol")
	assert.Contains(t, text, "non-empty string")
}

// TestGetSignals_ValidSymbol verifies that a valid symbol with signals returns correct output.
func TestGetSignals_ValidSymbol(t *testing.T) {
	now := time.Date(2026, 2, 27, 10, 0, 0, 0, time.UTC)
	b := &stubBackend{
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			return bullarc.AnalysisResult{
				Symbol:    req.Symbol,
				Timestamp: now,
				Signals: []bullarc.Signal{
					{
						Type:        bullarc.SignalBuy,
						Confidence:  72.5,
						Indicator:   "composite",
						Symbol:      req.Symbol,
						Timestamp:   now,
						Explanation: "strong uptrend",
					},
				},
			}, nil
		},
	}

	text, isError := callGetSignals(t, b, map[string]any{"symbols": []any{"AAPL"}})
	require.False(t, isError, "expected no error for valid symbol, got: %s", text)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &results))
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "AAPL", r["symbol"])
	assert.Equal(t, "BUY", r["signal"])
	assert.InDelta(t, 72.5, r["confidence"], 0.01)
	assert.Equal(t, now.Format(time.RFC3339), r["timestamp"])
	assert.Equal(t, "strong uptrend", r["explanation"])
	assert.Nil(t, r["error"], "no error field expected on success")
}

// TestGetSignals_MultipleSymbols verifies that multiple symbols are all analyzed and returned.
func TestGetSignals_MultipleSymbols(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	b := &stubBackend{
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			sig := bullarc.SignalHold
			if req.Symbol == "BULL" {
				sig = bullarc.SignalBuy
			}
			return bullarc.AnalysisResult{
				Symbol:    req.Symbol,
				Timestamp: now,
				Signals: []bullarc.Signal{
					{Type: sig, Confidence: 50.0, Indicator: "composite", Symbol: req.Symbol},
				},
			}, nil
		},
	}

	text, isError := callGetSignals(t, b, map[string]any{"symbols": []any{"BULL", "BEAR", "FLAT"}})
	require.False(t, isError, "expected no error for multiple symbols, got: %s", text)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &results))
	require.Len(t, results, 3)

	symbols := make([]string, len(results))
	for i, r := range results {
		symbols[i] = r["symbol"].(string)
	}
	assert.Equal(t, []string{"BULL", "BEAR", "FLAT"}, symbols)
	assert.Equal(t, "BUY", results[0]["signal"])
	assert.Equal(t, "HOLD", results[1]["signal"])
}

// TestGetSignals_AnalysisError verifies that an analysis error is reported per-symbol
// and does not cause the overall call to fail.
func TestGetSignals_AnalysisError(t *testing.T) {
	b := &stubBackend{
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			if req.Symbol == "BAD" {
				return bullarc.AnalysisResult{}, fmt.Errorf("data source unavailable for BAD")
			}
			return bullarc.AnalysisResult{
				Symbol:    req.Symbol,
				Timestamp: time.Now(),
				Signals: []bullarc.Signal{
					{Type: bullarc.SignalHold, Confidence: 50, Indicator: "composite", Symbol: req.Symbol},
				},
			}, nil
		},
	}

	text, isError := callGetSignals(t, b, map[string]any{"symbols": []any{"AAPL", "BAD"}})
	require.False(t, isError, "per-symbol errors must not surface as top-level errors, got: %s", text)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &results))
	require.Len(t, results, 2)

	// First symbol succeeds.
	assert.Equal(t, "AAPL", results[0]["symbol"])
	assert.Equal(t, "HOLD", results[0]["signal"])
	assert.Nil(t, results[0]["error"])

	// Second symbol carries error message.
	assert.Equal(t, "BAD", results[1]["symbol"])
	assert.NotEmpty(t, results[1]["error"])
	assert.Contains(t, results[1]["error"].(string), "data source unavailable for BAD")
}

// TestGetSignals_NoSignalsProduced verifies that a symbol producing no signals carries an informative error.
func TestGetSignals_NoSignalsProduced(t *testing.T) {
	b := &stubBackend{
		analyzeFunc: func(_ context.Context, req bullarc.AnalysisRequest) (bullarc.AnalysisResult, error) {
			return bullarc.AnalysisResult{Symbol: req.Symbol, Timestamp: time.Now()}, nil
		},
	}

	text, isError := callGetSignals(t, b, map[string]any{"symbols": []any{"EMPTY"}})
	require.False(t, isError, "no-signals result must not surface as a top-level error, got: %s", text)

	var results []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &results))
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, "EMPTY", r["symbol"])
	assert.NotEmpty(t, r["error"])
	assert.Contains(t, r["error"].(string), "no signals produced")
}
