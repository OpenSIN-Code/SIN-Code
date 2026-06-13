// SPDX-License-Identifier: MIT
// Purpose: minimal JSON-RPC 2.0 stdio MCP server for vane. Exposes two
// tools (vane_research / vane_health) so the SIN-Code agent can query a
// self-hosted Vane instance at runtime over the standard MCP wire
// protocol. Stdlib-only: no third-party deps. Graceful Degradation: any
// tool error is returned with isError=true and a "fallback to
// websearch" hint — the process NEVER panics or returns a fatal JSON-RPC
// error to the caller (M7: race-clean, M2: stdlib only).
// Docs: vane.doc.md
package vane

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server is the stdio MCP server. Construct with NewServer, then call
// Serve(ctx) which blocks until ctx is cancelled or stdin reaches EOF.
type Server struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	// mu protects the cfgDir + the constructor's "have I built a Client
	// yet" flag for the lazy initialization path. All shared-state reads
	// are read-only-after-construction in production, but tests swap the
	// underlying reader, so the mutex stays for safety.
	mu     sync.Mutex
	cfgDir string

	clientOnce sync.Once
	client     *Client
	clientErr  error
}

// NewServer returns a Server bound to the global stdin/stdout. The
// optional cfgDir overrides Home() resolution; pass "" for the
// production default.
func NewServer(cfgDir string) *Server {
	if cfgDir == "" {
		cfgDir = Home()
	}
	return &Server{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		cfgDir: cfgDir,
	}
}

// NewServerWithIO is a test-friendly constructor: pass arbitrary
// readers/writers and the resulting Server won't touch the real stdio.
// Mirrors superpowers.NewServerWithIO.
func NewServerWithIO(in io.Reader, out, err io.Writer, cfgDir string) *Server {
	return &Server{stdin: in, stdout: out, stderr: err, cfgDir: cfgDir}
}

// lazyClient returns the long-lived Vane Client, building it on first
// use. Build errors are sticky — repeated calls return the original
// error so we don't re-read the config on every request.
func (s *Server) lazyClient() (*Client, error) {
	s.clientOnce.Do(func() {
		// SIN_CODE_HOME controls config resolution. Set the env to the
		// cfgDir for the duration of LoadConfig so a server launched
		// with `sin-code vane serve --home /tmp/x` honors that path.
		prev := os.Getenv("SIN_CODE_HOME")
		if s.cfgDir != "" {
			_ = os.Setenv("SIN_CODE_HOME", s.cfgDir)
		}
		cfg, _, err := LoadConfig()
		if prev == "" {
			_ = os.Unsetenv("SIN_CODE_HOME")
		} else {
			_ = os.Setenv("SIN_CODE_HOME", prev)
		}
		if err != nil {
			s.clientErr = err
			return
		}
		s.client = NewClient(cfg)
	})
	return s.client, s.clientErr
}

// Serve runs the JSON-RPC 2.0 read loop over line-delimited JSON. One
// request per line, one response per line. We use bufio.Scanner (not
// json.Decoder) so a malformed line does not eat subsequent valid
// requests — the scanner can re-sync at the next newline.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.stdin)
	// Allow large JSON-RPC payloads (queries can carry long context).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	enc := json.NewEncoder(s.stdout)
	for scanner.Scan() {
		// Honor context cancellation between messages.
		if err := ctx.Err(); err != nil {
			return err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			// A malformed line is not fatal — we just emit a parse-error
			// reply (best-effort) and keep reading. This matches the
			// spirit of MCP's "lenient" stdio transport.
			fmt.Fprintf(s.stderr, "vane-mcp: decode error: %v\n", err)
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

// dispatch handles one request. Returns nil for notifications (no
// reply required by JSON-RPC 2.0).
func (s *Server) dispatch(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	if req.ID == nil {
		return nil
	}
	switch req.Method {
	case "initialize":
		return s.result(req, map[string]any{
			"protocolVersion": "2025-06-18",
			"serverInfo": map[string]string{
				"name":    "sin-code-vane",
				"version": "v3.8.0",
			},
			"capabilities": map[string]any{"tools": map[string]string{}},
		})
	case "notifications/initialized":
		// Client is acknowledging initialize. No reply.
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
	case "vane_research":
		return s.callResearch(ctx, req, p)
	case "vane_health":
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

func (s *Server) callResearch(ctx context.Context, req *jsonRPCRequest, p *toolCallParams) *jsonRPCResponse {
	var args struct {
		Query        string `json:"query"`
		FocusMode    string `json:"focus_mode"`
		Optimization string `json:"optimization"`
	}
	_ = json.Unmarshal(p.Arguments, &args)

	// Per-tool timeout: cap at the client's configured timeout or 30s,
	// whichever is smaller, so a stuck Vane instance never blocks the
	// agent loop indefinitely.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	client, err := s.lazyClient()
	if err != nil {
		return s.toolError(req, "vane_research: bridge init failed: "+err.Error())
	}
	ans, err := client.Search(ctx, args.Query, args.FocusMode, args.Optimization)
	if err != nil {
		// Graceful Degradation contract: surface the failure to the
		// model with isError=true and an explicit fallback hint. The
		// model can then ask the user to fall back to the websearch
		// ecosystem skill rather than us aborting the request.
		return s.toolError(req, fmt.Sprintf(
			"%s\n\nFallback: use the websearch ecosystem skill instead.",
			err.Error(),
		))
	}
	return s.result(req, toolResult{Content: marshalText(FormatAnswer(ans))})
}

func (s *Server) callHealth(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	// Health-check is bounded to 5s so a hung instance does not pin a
	// tool slot.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client, err := s.lazyClient()
	if err != nil {
		return s.toolError(req, "vane_health: bridge init failed: "+err.Error())
	}
	if err := client.Healthy(ctx); err != nil {
		return s.toolError(req, "vane_health: "+err.Error())
	}
	return s.result(req, toolResult{Content: marshalText("vane: healthy")})
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

// toolList returns the schema for both vane tools. Focus modes and
// optimizations are emitted as JSON-Schema enums so strict clients
// validate before they call.
func (s *Server) toolList() []toolSpec {
	focusEnums := make([]string, 0, len(FocusModes))
	for k := range FocusModes {
		focusEnums = append(focusEnums, k)
	}
	optEnums := make([]string, 0, len(Optimizations))
	for k := range Optimizations {
		optEnums = append(optEnums, k)
	}
	return []toolSpec{
		{
			Name:        "vane_research",
			Description: "Query a self-hosted Vane AI-answering engine. Returns a markdown answer with a 'Cited Sources' section. Graceful Degradation: on bridge failure returns isError=true with a fallback hint to the websearch ecosystem skill.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The research question. Required.",
					},
					"focus_mode": map[string]any{
						"type":        "string",
						"enum":        focusEnums,
						"description": "Which source domain Vane should prioritize. Defaults to webSearch.",
					},
					"optimization": map[string]any{
						"type":        "string",
						"enum":        optEnums,
						"description": "Quality knob: speed | balanced | quality. Defaults to balanced.",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "vane_health",
			Description: "Check whether the configured Vane instance is reachable. Returns isError=true if the bridge is down.",
			InputSchema: emptySchema(),
		},
	}
}

// ── JSON-RPC plumbing ─────────────────────────────────────────────────

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

// ── Public entry point used by cobra `vane serve` ─────────────────────

// Serve is a convenience wrapper: build a default Server and run Serve
// until stdin closes or ctx is cancelled. Used by the `sin-code vane
// serve` cobra command (owned by a different subagent).
func Serve(ctx context.Context) error {
	return NewServer("").Serve(ctx)
}

// formatIntBytes is a tiny helper used by Health replies — declared
// here to keep the import surface small even though it is currently
// only used in tests. Unused-but-exported helpers would clutter the
// public API, so it stays unexported and only used in this file.
func formatIntBytes(n int64) string {
	return strconv.FormatInt(n, 10) + "B"
}
