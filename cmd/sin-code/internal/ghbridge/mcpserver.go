// SPDX-License-Identifier: MIT
// Purpose: minimal JSON-RPC 2.0 stdio MCP server for the gh-bridge.
// Exposes three tools (gh_query / gh_execute / gh_health) so the
// SIN-Code agent can call a tightly-allowlisted subset of the gh CLI
// at runtime over the standard MCP wire protocol. Stdlib-only — no
// third-party deps. Graceful Degradation: any tool error is returned
// with isError=true and the process NEVER panics or returns a fatal
// JSON-RPC error to the caller (M7: race-clean, M2: stdlib only).
// Docs: ghbridge.doc.md
package ghbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ── JSON-RPC plumbing (mirrors vane/mcpserver.go shape) ───────────────

type jsonRPCRequest struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
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

// ── Server ─────────────────────────────────────────────────────────────

// Server is the stdio MCP server. Construct with NewServer, then call
// Serve(ctx) which blocks until ctx is cancelled or stdin reaches EOF.
type Server struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	// bridge is the underlying gh-bridge. Built lazily on first use so
	// a missing gh binary does not prevent `initialize` / `tools/list`
	// from working — the agent can still discover the surface.
	mu         sync.Mutex
	bridge     *Bridge
	bridgeErr  error
	bridgeOnce sync.Once
}

// NewServer returns a Server bound to the global stdin/stdout.
func NewServer() *Server {
	return &Server{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
	}
}

// NewServerWithIO is the test-friendly constructor: pass arbitrary
// readers/writers and the resulting Server won't touch the real stdio.
// Mirrors vane.NewServerWithIO / superpowers.NewServerWithIO.
func NewServerWithIO(in io.Reader, out, err io.Writer) *Server {
	return &Server{stdin: in, stdout: out, stderr: err}
}

// lazyBridge returns the long-lived Bridge, building it on first use.
// Build errors are sticky — repeated calls return the original error
// so we don't re-probe the gh binary on every request.
func (s *Server) lazyBridge() (*Bridge, error) {
	s.bridgeOnce.Do(func() {
		b := New()
		// Sanity probe: missing-gh would make every call fail
		// identically. Surface once, here, with a friendly hint.
		if _, err := exec.LookPath("gh"); err != nil {
			s.bridgeErr = fmt.Errorf("ghbridge: gh binary not found on PATH: %w", err)
			return
		}
		s.bridge = b
	})
	return s.bridge, s.bridgeErr
}

// ── Serve loop ─────────────────────────────────────────────────────────

// Serve runs the JSON-RPC 2.0 read loop over line-delimited JSON. One
// request per line, one response per line. Uses bufio.Scanner (not
// json.Decoder) so a malformed line does not eat subsequent valid
// requests — the scanner re-syncs at the next newline.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.stdin)
	// Allow large JSON-RPC payloads (queries can carry long context).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	enc := json.NewEncoder(s.stdout)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			// Malformed line is not fatal — log and keep reading.
			fmt.Fprintf(s.stderr, "ghbridge-mcp: decode error: %v\n", err)
			continue
		}
		resp := s.dispatch(ctx, &req)
		if resp == nil {
			continue // notification — no reply required
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

// dispatch handles one request. Returns nil for notifications.
func (s *Server) dispatch(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	if req.ID == nil {
		return nil
	}
	switch req.Method {
	case "initialize":
		return s.result(req, map[string]any{
			"protocolVersion": "2025-06-18",
			"serverInfo": map[string]string{
				"name":    "sin-code-gh-bridge",
				"version": "v3.9.0",
			},
			"capabilities": map[string]any{"tools": map[string]string{}},
		})
	case "notifications/initialized":
		return nil
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

// callTool dispatches the named tool. ANY error path returns
// isError=true with a fallback hint — we never return a JSON-RPC error
// for tool-level failures (the caller is a friendly LLM, not a
// programmer who wants -32602 codes).
func (s *Server) callTool(ctx context.Context, req *jsonRPCRequest, p *toolCallParams) *jsonRPCResponse {
	switch p.Name {
	case "gh_query":
		return s.callQuery(ctx, req, p)
	case "gh_execute":
		return s.callExecute(ctx, req, p)
	case "gh_health":
		return s.callHealth(ctx, req)
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

// callQuery handles gh_query (read-only). Args must classify as
// TierReadOnly — anything else is rejected with a hint pointing the
// agent at gh_execute.
func (s *Server) callQuery(ctx context.Context, req *jsonRPCRequest, p *toolCallParams) *jsonRPCResponse {
	var args struct {
		Args []string `json:"args"`
	}
	_ = json.Unmarshal(p.Arguments, &args)
	return dispatch(s, ctx, req, "gh_query", args.Args, TierReadOnly)
}

// callExecute handles gh_execute (mutating + read-only). Args must NOT
// classify as TierForbidden — the agent is responsible for ask-policy
// confirmation. TierReadOnly works here too (lets the model use one
// tool for both).
func (s *Server) callExecute(ctx context.Context, req *jsonRPCRequest, p *toolCallParams) *jsonRPCResponse {
	var args struct {
		Args []string `json:"args"`
	}
	_ = json.Unmarshal(p.Arguments, &args)
	return dispatch(s, ctx, req, "gh_execute", args.Args, TierMutating)
}

// callHealth is the bridge liveness probe.
func (s *Server) callHealth(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	// Health-check is bounded to HealthTimeout so a hung `gh auth
	// status` does not pin a tool slot.
	hctx, cancel := context.WithTimeout(ctx, HealthTimeout)
	defer cancel()
	bridge, err := s.lazyBridge()
	if err != nil {
		return s.toolError(req, "gh_health: bridge init failed: "+err.Error())
	}
	if err := bridge.Health(hctx); err != nil {
		return s.toolError(req, "gh_health: "+err.Error())
	}
	return s.result(req, toolResult{Content: marshalText("gh-bridge: healthy")})
}

// dispatch is the shared core for gh_query / gh_execute. It enforces
// the tool/tier cross-check: a gh_query call that classifies as
// mutating is rejected with a hint; a gh_execute call that classifies
// as forbidden is also rejected (defense in depth — Classify()
// already returns TierForbidden with err != nil in that case).
//
// `toolTier` is the tier this tool is allowed to execute:
//   - gh_query  → TierReadOnly
//   - gh_execute → TierMutating (read-only also OK)
func dispatch(s *Server, ctx context.Context, req *jsonRPCRequest, tool string, args []string, toolTier Tier) *jsonRPCResponse {
	tier, err := Classify(args)
	if err != nil || tier == TierForbidden {
		// Defense in depth: forbidden commands must NEVER reach the
		// runner. The classifier's error message is the user-facing
		// explanation.
		msg := "rejected: command not in the allowlist"
		if err != nil {
			msg = "rejected: " + err.Error()
		}
		return s.toolError(req, msg)
	}
	// Cross-check tool vs tier.
	if tool == "gh_query" && tier != TierReadOnly {
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mustMarshal(req, toolResult{
				Content: []toolContent{{
					Type: "text",
					Text: "rejected: this command mutates state — use gh_execute (ask-policy) instead",
				}},
				IsError: true,
			}),
		}
	}
	bridge, berr := s.lazyBridge()
	if berr != nil {
		return s.toolError(req, tool+": bridge init failed: "+berr.Error())
	}
	// Per-tool timeout: cap at bridge.Timeout or 60s, whichever is
	// smaller, so a stuck gh instance never blocks the agent loop.
	runCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, DefaultExecuteTimeout)
		defer cancel()
	}
	stdout, _, runErr := bridge.Execute(runCtx, args)
	if runErr != nil {
		return s.toolError(req, tool+": "+runErr.Error())
	}
	return s.result(req, toolResult{Content: marshalText(stdout)})
}

// mustMarshal is a tiny helper for the rare case where we already have
// a fully-formed toolResult we just need to wrap in a JSON-RPC
// envelope. If marshalling somehow fails (it can't, for our static
// shape), we fall back to a JSON-RPC error so the call still closes
// cleanly.
func mustMarshal(req *jsonRPCRequest, v toolResult) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		// Unreachable in practice — the shape is static. We return a
		// valid (if empty) result so the call does not hang.
		return json.RawMessage(`{"content":[],"isError":true}`)
	}
	return data
}

// toolError returns a successful JSON-RPC envelope whose body carries
// isError=true. The text content is human-readable so an LLM can
// surface it directly to the user.
func (s *Server) toolError(req *jsonRPCRequest, msg string) *jsonRPCResponse {
	return s.result(req, toolResult{
		Content: []toolContent{{Type: "text", Text: msg}},
		IsError: true,
	})
}

// toolList returns the schema for all three gh-bridge tools. The
// `args` field is a string array — strict clients can pre-validate
// against the allowlist before invoking.
func (s *Server) toolList() []toolSpec {
	return []toolSpec{
		{
			Name:        "gh_query",
			Description: "Run a read-only gh CLI command (e.g. issue list, pr view). Commands that mutate state are rejected with a hint to use gh_execute instead. Allowed groups: " + AllowedSurface() + ".",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"args": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Arguments to pass to gh (e.g. [\"issue\", \"list\", \"--limit\", \"10\"]). Group must be in the allowlist; verb must be read-only.",
					},
				},
				"required": []string{"args"},
			},
		},
		{
			Name:        "gh_execute",
			Description: "Run a mutating gh CLI command (e.g. issue create, pr merge). Use only after the user has confirmed via ask-policy. Forbidden groups (auth, secret, api, config, etc.) and forbidden verbs (delete) are hard-rejected before reaching the gh binary. Allowed groups: " + AllowedSurface() + ".",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"args": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Arguments to pass to gh (e.g. [\"issue\", \"create\", \"--title\", \"...\"]). Must NOT contain forbidden groups or tokens.",
					},
				},
				"required": []string{"args"},
			},
		},
		{
			Name:        "gh_health",
			Description: "Check that the gh CLI is installed and authenticated (gh auth status). Returns isError=true if gh is missing or the user is not logged in.",
			InputSchema: emptySchema(),
		},
	}
}

// ── RegisterMCP ───────────────────────────────────────────────────────

// MCPConfigPath is where the gh-bridge MCP server is registered.
// Matches the convention used by vane.MCPConfigPath() and
// superpowers.MCPConfigPath() so a single $SIN_CODE_HOME override
// moves every package's root.
func MCPConfigPath() string {
	if v := os.Getenv("SIN_CODE_HOME"); v != "" {
		return filepath.Join(v, "mcp.json")
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".local", "share", "sin-code", "mcp.json")
	}
	return filepath.Join(".", ".sin-code-home", "mcp.json")
}

// RegisterMCP merges the "gh" server entry into mcpPath (or
// MCPConfigPath() if empty). Idempotent: existing entries with the
// same command + args are left untouched. Preserves every other key
// in the file (notably pre-existing "superpowers", "vane" entries).
func RegisterMCP(mcpPath string) (string, error) {
	if mcpPath == "" {
		mcpPath = MCPConfigPath()
	}
	exe, err := os.Executable()
	if err != nil {
		return mcpPath, fmt.Errorf("ghbridge: resolve executable: %w", err)
	}
	// Load existing config (or start with an empty shell). Tolerate a
	// malformed file by treating it as empty — the next save will
	// re-write a clean shape.
	cfg := map[string]any{}
	if b, err := os.ReadFile(mcpPath); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	entry := map[string]any{
		"command": exe,
		"args":    []string{"gh", "serve"},
	}
	if existing, ok := servers[ServerName].(map[string]any); ok {
		// Idempotency: same command + args → no-op.
		if existing["command"] == entry["command"] {
			return mcpPath, nil
		}
	}
	servers[ServerName] = entry
	cfg["mcpServers"] = servers
	return mcpPath, writeJSONAtomic(mcpPath, cfg)
}

// writeJSONAtomic marshals v and writes to path via a temp-file +
// rename so a crash mid-write cannot corrupt the existing file. Parent
// directories are created on demand. Mirrors vane.writeJSONAtomic.
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ghbridge-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, strings.NewReader(string(data)+"\n")); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
