package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

const protocolVersion = "2024-11-05"

// jsonrpcMsg is a JSON-RPC 2.0 message (request, response, or notification).
type jsonrpcMsg struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type jsonrpcOK struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

type jsonrpcErr struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   rpcError        `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolHandler processes a tool call and returns a text result.
type ToolHandler func(ctx context.Context, args map[string]any) (string, error)

// Tool describes a callable MCP tool.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     ToolHandler
}

// Server is an MCP server that communicates over stdio using JSON-RPC 2.0.
type Server struct {
	name    string
	version string
	tools   map[string]Tool
	mu      sync.Mutex
	out     io.Writer
}

// New creates a Server with the given name and version string.
func New(name, version string) *Server {
	return &Server{
		name:    name,
		version: version,
		tools:   make(map[string]Tool),
		out:     os.Stdout,
	}
}

// AddTool registers a tool with the server.
func (s *Server) AddTool(t Tool) {
	s.tools[t.Name] = t
}

// ToolCount returns the number of registered tools.
func (s *Server) ToolCount() int {
	return len(s.tools)
}

// Serve reads JSON-RPC messages from stdin and dispatches them until ctx is cancelled
// or stdin is closed. It uses a 4 MiB scanner buffer to support large messages.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for {
		select {
		case <-ctx.Done():
			slog.Info("mcp: server shutting down")
			return nil
		default:
		}

		if !scanner.Scan() {
			break
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		s.dispatch(ctx, line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp: stdin: %w", err)
	}
	return nil
}

func (s *Server) dispatch(ctx context.Context, data []byte) {
	var msg jsonrpcMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("mcp: malformed JSON-RPC message", "err", err)
		return
	}

	// Notifications carry no id; no response is expected.
	if msg.ID == nil {
		slog.Debug("mcp: notification received", "method", msg.Method)
		return
	}

	id := *msg.ID

	switch msg.Method {
	case "initialize":
		s.writeOK(id, s.resultInitialize())
	case "ping":
		s.writeOK(id, map[string]any{})
	case "tools/list":
		s.writeOK(id, s.resultToolsList())
	case "tools/call":
		result, err := s.handleToolsCall(ctx, msg.Params)
		if err != nil {
			s.writeErr(id, -32603, err.Error())
		} else {
			s.writeOK(id, result)
		}
	case "resources/list":
		s.writeOK(id, map[string]any{"resources": []any{}})
	case "prompts/list":
		s.writeOK(id, map[string]any{"prompts": []any{}})
	default:
		s.writeErr(id, -32601, "method not found: "+msg.Method)
	}
}

func (s *Server) resultInitialize() map[string]any {
	return map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": s.name, "version": s.version},
	}
}

func (s *Server) resultToolsList() map[string]any {
	tools := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		tools = append(tools, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		})
	}
	return map[string]any{"tools": tools}
}

func (s *Server) handleToolsCall(ctx context.Context, params json.RawMessage) (any, error) {
	var p struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, fmt.Errorf("invalid tools/call params: %w", err)
	}
	t, ok := s.tools[p.Name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", p.Name)
	}
	text, err := t.Handler(ctx, p.Arguments)
	if err != nil {
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": "error: " + err.Error()}},
			"isError": true,
		}, nil
	}
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": false,
	}, nil
}

func (s *Server) writeOK(id json.RawMessage, result any) {
	s.write(jsonrpcOK{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) writeErr(id json.RawMessage, code int, message string) {
	s.write(jsonrpcErr{JSONRPC: "2.0", ID: id, Error: rpcError{Code: code, Message: message}})
}

func (s *Server) write(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("mcp: marshal response failed", "err", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = fmt.Fprintf(s.out, "%s\n", data)
}
