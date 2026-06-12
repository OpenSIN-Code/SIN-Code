// SPDX-License-Identifier: MIT
// Purpose: minimal JSON-RPC 2.0 stdio MCP server for superpowers.
// Exposes three tools (list_skills / find_skill / use_skill) so the
// SIN-Code agent can discover & load obra/superpowers skills at runtime
// over the standard MCP wire protocol. Stdlib-only: no third-party deps.
// Docs: superpowers.doc.md
package superpowers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Server is the stdio MCP server. Construct with NewServer, then call
// Serve(ctx) which blocks until ctx is cancelled or stdin reaches EOF.
type Server struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	mu     sync.Mutex // protects mcp.json writes
	cfgDir string
}

// NewServer returns a Server bound to the global stdin/stdout. The
// optional cfgDir overrides MCPConfigPath() resolution; pass "" for the
// production default.
func NewServer(cfgDir string) *Server {
	if cfgDir == "" {
		cfgDir = Home()
	}
	return &Server{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, cfgDir: cfgDir}
}

// Serve runs the JSON-RPC 2.0 read loop. The protocol is line-delimited
// JSON: one request per line, one response per line. We deliberately
// avoid HTTP-style framing to keep the implementation small and easy to
// test.
func (s *Server) Serve(ctx context.Context) error {
	dec := json.NewDecoder(s.stdin)
	enc := json.NewEncoder(s.stdout)
	for {
		// Honor context cancellation between messages.
		if err := ctx.Err(); err != nil {
			return err
		}
		var req jsonRPCRequest
		if err := dec.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			// A malformed line is not fatal — we just emit a parse-error
			// reply and keep reading. This matches the spirit of MCP's
			// "lenient" stdio transport.
			fmt.Fprintf(s.stderr, "superpowers-mcp: decode error: %v\n", err)
			continue
		}
		resp := s.handle(ctx, &req)
		if resp == nil {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
}

// handle dispatches one request. Returns nil for notifications (no reply
// required by JSON-RPC 2.0).
func (s *Server) handle(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	// Notifications have no "id". We don't reply.
	if req.ID == nil {
		return nil
	}
	switch req.Method {
	case "initialize":
		return s.result(req, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]string{
				"name":    "sin-code-superpowers",
				"version": "v3.7.0",
			},
			"capabilities": map[string]any{"tools": map[string]string{}},
		})
	case "tools/list":
		return s.result(req, map[string]any{"tools": s.toolList()})
	case "tools/call":
		var p toolCallParams
		_ = json.Unmarshal(req.Params, &p)
		return s.callTool(ctx, req, &p)
	case "ping":
		return s.result(req, map[string]string{"pong": "ok"})
	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    -32601,
				Message: "method not found: " + req.Method,
			},
		}
	}
}

func (s *Server) result(req *jsonRPCRequest, v any) *jsonRPCResponse {
	data, err := json.Marshal(v)
	if err != nil {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32603, Message: err.Error()},
		}
	}
	return &jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: data}
}

func (s *Server) callTool(ctx context.Context, req *jsonRPCRequest, p *toolCallParams) *jsonRPCResponse {
	switch p.Name {
	case "superpowers_list_skills":
		all, err := List("")
		if err != nil {
			return s.errResult(req, err)
		}
		return s.result(req, toolResult{Content: marshalText(all)})
	case "superpowers_find_skill":
		var args struct {
			Query      string `json:"query"`
			MaxResults int    `json:"max_results"`
		}
		_ = json.Unmarshal(p.Arguments, &args)
		hits, err := Find(args.Query, args.MaxResults)
		if err != nil {
			return s.errResult(req, err)
		}
		return s.result(req, toolResult{Content: marshalText(hits)})
	case "superpowers_use_skill":
		var args struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(p.Arguments, &args)
		info, err := Get(args.Name)
		if err != nil {
			return s.errResult(req, err)
		}
		body, rerr := os.ReadFile(info.Path)
		if rerr != nil {
			return s.errResult(req, rerr)
		}
		return s.result(req, toolResult{Content: marshalTextRaw(string(body))})
	default:
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &jsonRPCError{
				Code:    -32602,
				Message: "unknown tool: " + p.Name,
			},
		}
	}
}

func (s *Server) errResult(req *jsonRPCRequest, err error) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error:   &jsonRPCError{Code: -32000, Message: err.Error()},
	}
}

func (s *Server) toolList() []toolSpec {
	return []toolSpec{
		{
			Name:        "superpowers_list_skills",
			Description: "List all installed obra/superpowers skills discovered on disk.",
			InputSchema: emptySchema(),
		},
		{
			Name:        "superpowers_find_skill",
			Description: "Substring search across skill name + description.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":       map[string]string{"type": "string"},
					"max_results": map[string]string{"type": "integer"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "superpowers_use_skill",
			Description: "Load the full SKILL.md body for the named skill (post-overlay).",
			InputSchema: map[string]any{
				"type":     "object",
				"properties": map[string]any{"name": map[string]string{"type": "string"}},
				"required": []string{"name"},
			},
		},
	}
}

// RegisterMCP appends the superpowers server entry to the given mcp.json
// (or to MCPConfigPath() if empty). Idempotent: if the entry is already
// present with the same command, it is left untouched. Returns the path
// actually written.
func RegisterMCP(mcpPath string) (string, error) {
	if mcpPath == "" {
		mcpPath = MCPConfigPath()
	}
	// Load existing config (or start with an empty shell).
	cfg := map[string]any{}
	if b, err := os.ReadFile(mcpPath); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	existing, _ := servers["superpowers"].(map[string]any)
	entry := map[string]any{
		"command": "sin-code",
		"args":    []string{"superpowers", "serve"},
	}
	if existing != nil {
		// Idempotency: keep existing entry if command + args match.
		if existing["command"] == entry["command"] {
			return mcpPath, nil
		}
	}
	servers["superpowers"] = entry
	cfg["mcpServers"] = servers
	return mcpPath, WriteJSON(mcpPath, cfg)
}

// ── JSON-RPC plumbing ─────────────────────────────────────────────────

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *jsonRPCError    `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type toolSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolResult struct {
	Content []toolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func emptySchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func marshalText(v any) []toolContent {
	b, _ := json.MarshalIndent(v, "", "  ")
	return []toolContent{{Type: "text", Text: string(b)}}
}

func marshalTextRaw(s string) []toolContent {
	return []toolContent{{Type: "text", Text: s}}
}

// ── Bufio helpers (used by tests for hermetic Serve) ──────────────────

// NewServerWithIO is a test-friendly constructor: pass arbitrary
// readers/writers and the resulting Server won't touch the real stdio.
func NewServerWithIO(in io.Reader, out, err io.Writer, cfgDir string) *Server {
	return &Server{stdin: in, stdout: out, stderr: err, cfgDir: cfgDir}
}

// PromptFromReader reads one line from r, ignoring errors. Used by the
// stdio harness in tests to drain a prompt after a request.
func PromptFromReader(r io.Reader) string {
	sc := bufio.NewScanner(r)
	if sc.Scan() {
		return strings.TrimRight(sc.Text(), "\r\n")
	}
	return ""
}
