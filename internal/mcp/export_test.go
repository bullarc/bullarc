package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// DispatchForTest drives a single JSON-RPC request through the server and
// returns the text content and isError flag from the tool response.
// It replaces the server's output writer with an in-memory buffer so nothing
// is written to stdout during tests.
func DispatchForTest(t testing.TB, srv *Server, reqJSON []byte) (string, bool) {
	t.Helper()

	var buf bytes.Buffer
	srv.out = &buf

	srv.dispatch(context.Background(), reqJSON)

	var resp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp), "server response: %s", buf.String())

	if resp.Error != nil {
		return resp.Error.Message, true
	}
	if len(resp.Result.Content) == 0 {
		return "", resp.Result.IsError
	}
	text := resp.Result.Content[0].Text
	// Strip leading "error: " prefix added by handleToolsCall when isError=true.
	if resp.Result.IsError && len(text) > 7 && text[:7] == "error: " {
		text = text[7:]
	}
	return text, resp.Result.IsError
}

// DispatchRawForTest drives a single JSON-RPC request through the server and
// returns the raw JSON-decoded response map. It is used by protocol compliance
// tests that need to inspect the full response shape rather than just tool
// content.
func DispatchRawForTest(t testing.TB, srv *Server, reqJSON []byte) map[string]any {
	t.Helper()

	var buf bytes.Buffer
	srv.out = &buf

	srv.dispatch(context.Background(), reqJSON)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &resp), "server response: %s", buf.String())
	return resp
}

// DispatchBytesForTest drives a single message through the server and returns
// the raw bytes written to the output. For notifications (no id), the server
// must produce no output, so the returned slice will be empty.
func DispatchBytesForTest(t testing.TB, srv *Server, msgJSON []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	srv.out = &buf

	srv.dispatch(context.Background(), msgJSON)

	return buf.Bytes()
}
