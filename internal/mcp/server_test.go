package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/bullarc/bullarc/internal/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildRPCRequest constructs a JSON-RPC 2.0 request message.
func buildRPCRequest(id int, method string, params any) []byte {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	data, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return data
}

// buildRPCNotification constructs a JSON-RPC 2.0 notification (no id).
func buildRPCNotification(method string, params any) []byte {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	data, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	return data
}

// TestServer_Initialize verifies that the initialize handshake returns the
// correct protocol version, server info, and tool capabilities.
func TestServer_Initialize(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")

	req := buildRPCRequest(1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test-client", "version": "1.0.0"},
	})

	resp := mcp.DispatchRawForTest(t, srv, req)

	assert.Equal(t, "2.0", resp["jsonrpc"], "jsonrpc field must be 2.0")
	assert.Nil(t, resp["error"], "initialize must not return an error")

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "result must be an object")

	assert.Equal(t, "2024-11-05", result["protocolVersion"], "protocol version must match")

	caps, ok := result["capabilities"].(map[string]any)
	require.True(t, ok, "capabilities must be an object")
	assert.NotNil(t, caps["tools"], "capabilities must declare tools")

	serverInfo, ok := result["serverInfo"].(map[string]any)
	require.True(t, ok, "serverInfo must be an object")
	assert.Equal(t, "bullarc", serverInfo["name"])
	assert.Equal(t, "0.1.0", serverInfo["version"])
}

// TestServer_Initialize_PreservesID verifies that the response id matches the
// request id, as required by JSON-RPC 2.0.
func TestServer_Initialize_PreservesID(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")

	req := buildRPCRequest(42, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mcp-inspector", "version": "0.0.1"},
	})

	resp := mcp.DispatchRawForTest(t, srv, req)
	// JSON numbers decode to float64 by default.
	assert.EqualValues(t, 42, resp["id"], "response id must match request id")
}

// TestServer_Ping verifies that the ping method returns an empty result object.
func TestServer_Ping(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")

	req := buildRPCRequest(2, "ping", nil)
	resp := mcp.DispatchRawForTest(t, srv, req)

	assert.Equal(t, "2.0", resp["jsonrpc"])
	assert.Nil(t, resp["error"], "ping must not return an error")

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "ping result must be an object")
	assert.Empty(t, result, "ping result must be an empty object")
}

// TestServer_ToolsList verifies that tools/list returns all registered tools
// with the required fields for MCP clients to discover them.
func TestServer_ToolsList(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")
	mcp.RegisterTools(srv, &stubBackend{})

	req := buildRPCRequest(3, "tools/list", nil)
	resp := mcp.DispatchRawForTest(t, srv, req)

	assert.Nil(t, resp["error"], "tools/list must not return an error")

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok)

	tools, ok := result["tools"].([]any)
	require.True(t, ok, "result.tools must be an array")
	assert.Len(t, tools, 5, "server must expose get_signals, backtest_strategy, list_indicators, explain_signal, stream_signals")

	// Build a name→tool map for assertion.
	byName := make(map[string]map[string]any, len(tools))
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		require.True(t, ok, "each tool must be an object")
		name, _ := tool["name"].(string)
		require.NotEmpty(t, name, "tool name must not be empty")
		byName[name] = tool
	}

	expectedTools := []string{"get_signals", "backtest_strategy", "list_indicators", "explain_signal", "stream_signals"}
	for _, name := range expectedTools {
		tool, exists := byName[name]
		require.True(t, exists, "tool %q must be present", name)
		assert.NotEmpty(t, tool["description"], "tool %q must have a description", name)

		schema, ok := tool["inputSchema"].(map[string]any)
		require.True(t, ok, "tool %q must have an inputSchema object", name)
		assert.Equal(t, "object", schema["type"], "tool %q inputSchema.type must be 'object'", name)
	}
}

// TestServer_ToolsList_SchemaValidity verifies that each tool's inputSchema is
// a valid MCP/JSON Schema shape with the required fields.
func TestServer_ToolsList_SchemaValidity(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")
	mcp.RegisterTools(srv, &stubBackend{})

	req := buildRPCRequest(4, "tools/list", nil)
	resp := mcp.DispatchRawForTest(t, srv, req)

	result := resp["result"].(map[string]any)
	tools := result["tools"].([]any)

	for _, raw := range tools {
		tool := raw.(map[string]any)
		name := tool["name"].(string)
		schema := tool["inputSchema"].(map[string]any)

		// JSON Schema: type must be "object".
		assert.Equal(t, "object", schema["type"], "tool %q: inputSchema.type must be 'object'", name)

		// JSON Schema: properties must be a map (can be empty for no-arg tools).
		props, ok := schema["properties"].(map[string]any)
		assert.True(t, ok, "tool %q: inputSchema.properties must be an object", name)

		// JSON Schema: required fields must reference existing properties.
		if reqFields, ok := schema["required"].([]any); ok {
			for _, f := range reqFields {
				field, _ := f.(string)
				_, found := props[field]
				assert.True(t, found, "tool %q: required field %q must be in properties", name, field)
			}
		}
	}
}

// TestServer_Notification_InitializedIgnored verifies that the
// notifications/initialized notification (sent by clients after the init
// handshake) produces no response, as required by MCP protocol.
func TestServer_Notification_InitializedIgnored(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")

	notif := buildRPCNotification("notifications/initialized", nil)

	// DispatchRawForTest requires a non-empty response, but notifications
	// must produce no output. We use the raw bytes helper directly.
	// Since no response is written, the buffer will be empty.
	raw := mcp.DispatchBytesForTest(t, srv, notif)
	assert.Empty(t, raw, "notifications/initialized must produce no response")
}

// TestServer_Notification_CancelledIgnored verifies that
// notifications/cancelled does not produce a response.
func TestServer_Notification_CancelledIgnored(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")

	notif := buildRPCNotification("notifications/cancelled", map[string]any{
		"requestId": 1,
		"reason":    "user cancelled",
	})

	raw := mcp.DispatchBytesForTest(t, srv, notif)
	assert.Empty(t, raw, "notifications/cancelled must produce no response")
}

// TestServer_UnknownMethod verifies that unsupported methods return a
// JSON-RPC "method not found" error (-32601).
func TestServer_UnknownMethod(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")

	req := buildRPCRequest(5, "unsupported/method", nil)
	resp := mcp.DispatchRawForTest(t, srv, req)

	rpcErr, ok := resp["error"].(map[string]any)
	require.True(t, ok, "unknown method must return an error object")
	assert.EqualValues(t, -32601, rpcErr["code"], "method not found code must be -32601")
	assert.Nil(t, resp["result"], "error response must not contain a result")
}

// TestServer_ResourcesList verifies that resources/list returns an empty
// resources array so generic MCP clients do not receive an error.
func TestServer_ResourcesList(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")

	req := buildRPCRequest(6, "resources/list", nil)
	resp := mcp.DispatchRawForTest(t, srv, req)

	assert.Nil(t, resp["error"], "resources/list must not return an error")

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "resources/list result must be an object")

	resources, ok := result["resources"].([]any)
	require.True(t, ok, "result.resources must be an array")
	assert.Empty(t, resources, "resources array must be empty when no resources are registered")
}

// TestServer_PromptsList verifies that prompts/list returns an empty prompts
// array so generic MCP clients do not receive an error.
func TestServer_PromptsList(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")

	req := buildRPCRequest(7, "prompts/list", nil)
	resp := mcp.DispatchRawForTest(t, srv, req)

	assert.Nil(t, resp["error"], "prompts/list must not return an error")

	result, ok := resp["result"].(map[string]any)
	require.True(t, ok, "prompts/list result must be an object")

	prompts, ok := result["prompts"].([]any)
	require.True(t, ok, "result.prompts must be an array")
	assert.Empty(t, prompts, "prompts array must be empty when no prompts are registered")
}

// TestServer_MalformedJSON verifies that a malformed JSON message does not
// crash the server or produce a response.
func TestServer_MalformedJSON(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")

	malformed := []byte(`{this is not valid json`)
	raw := mcp.DispatchBytesForTest(t, srv, malformed)
	assert.Empty(t, raw, "malformed JSON must produce no response")
}

// TestServer_ToolsCall_UnknownTool verifies that calling an unknown tool
// returns an internal error, not a panic.
func TestServer_ToolsCall_UnknownTool(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")
	mcp.RegisterTools(srv, &stubBackend{})

	req := buildRPCRequest(8, "tools/call", map[string]any{
		"name":      "nonexistent_tool",
		"arguments": map[string]any{},
	})
	resp := mcp.DispatchRawForTest(t, srv, req)

	// Unknown tool is an internal error (-32603) returned by handleToolsCall.
	rpcErr, ok := resp["error"].(map[string]any)
	require.True(t, ok, "unknown tool must return an error")
	assert.EqualValues(t, -32603, rpcErr["code"])
	assert.Contains(t, rpcErr["message"].(string), "nonexistent_tool")
}

// TestServer_FullProtocolFlow simulates a complete MCP client handshake:
// initialize → notifications/initialized → tools/list → tools/call.
func TestServer_FullProtocolFlow(t *testing.T) {
	srv := mcp.New("bullarc", "0.1.0")
	mcp.RegisterTools(srv, &stubBackend{})

	// Step 1: initialize.
	initResp := mcp.DispatchRawForTest(t, srv, buildRPCRequest(1, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "cursor", "version": "1.0.0"},
	}))
	assert.Nil(t, initResp["error"], "initialize must succeed")
	result := initResp["result"].(map[string]any)
	assert.Equal(t, "2024-11-05", result["protocolVersion"])

	// Step 2: notifications/initialized — must produce no response.
	raw := mcp.DispatchBytesForTest(t, srv, buildRPCNotification("notifications/initialized", nil))
	assert.Empty(t, raw, "notifications/initialized must be silent")

	// Step 3: tools/list — all tools must be present.
	listResp := mcp.DispatchRawForTest(t, srv, buildRPCRequest(2, "tools/list", nil))
	assert.Nil(t, listResp["error"])
	tools := listResp["result"].(map[string]any)["tools"].([]any)
	assert.Len(t, tools, 5)

	// Step 4: tools/call list_indicators — must return a result.
	callResp := mcp.DispatchRawForTest(t, srv, buildRPCRequest(3, "tools/call", map[string]any{
		"name":      "list_indicators",
		"arguments": map[string]any{},
	}))
	assert.Nil(t, callResp["error"], "list_indicators call must not return a JSON-RPC error")
	callResult := callResp["result"].(map[string]any)
	content := callResult["content"].([]any)
	require.Len(t, content, 1)
	entry := content[0].(map[string]any)
	assert.Equal(t, "text", entry["type"])
}
