// internal/headroom/mcp.go
package headroom

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MCPClient speaks the Model Context Protocol to a headroom MCP server over
// stdio using JSON-RPC 2.0. It is dependency-free: it spawns the
// "headroom mcp" subprocess and exchanges newline-delimited JSON-RPC frames.
//
// The implementation covers the subset of MCP needed for compression:
//   - initialize
//   - tools/call (compress, learn)
type MCPClient struct {
	config Config

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	nextID  int64
	started bool
}

// jsonRPCRequest is a JSON-RPC 2.0 request frame.
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response frame.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// mcpToolCallParams is the params shape for an MCP tools/call request.
type mcpToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// mcpToolResult is the (simplified) result of an MCP tools/call.
type mcpToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError,omitempty"`
}

// NewMCPClient creates an MCP client (not yet started).
func NewMCPClient(cfg Config) *MCPClient {
	return &MCPClient{config: cfg}
}

// Start spawns the headroom MCP server subprocess and performs the MCP
// initialize handshake.
func (m *MCPClient) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return nil
	}

	cmd := exec.CommandContext(ctx, "headroom", "mcp")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting headroom mcp: %w", err)
	}

	m.cmd = cmd
	m.stdin = stdin
	m.stdout = bufio.NewReader(stdout)

	// Perform the initialize handshake.
	if _, err := m.call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "sin-code",
			"version": "1.0.0",
		},
	}); err != nil {
		_ = m.closeLocked()
		return fmt.Errorf("mcp initialize failed: %w", err)
	}

	m.started = true
	return nil
}

// Compress calls the headroom "compress" MCP tool.
func (m *MCPClient) Compress(ctx context.Context, content string) (*CompressionResult, error) {
	start := time.Now()
	res, err := m.callTool(ctx, "compress", map[string]interface{}{
		"content": content,
		"level":   m.config.CompressionLevel,
	})
	if err != nil {
		return nil, err
	}

	compressed := strings.TrimSpace(res)
	if compressed == "" {
		compressed = content
	}

	originalTokens := len(content) / 4
	compressedTokens := len(compressed) / 4
	savings := 0.0
	if originalTokens > 0 {
		savings = (1 - float64(compressedTokens)/float64(originalTokens)) * 100
	}

	return &CompressionResult{
		OriginalContent:   content,
		CompressedContent: compressed,
		OriginalTokens:    originalTokens,
		CompressedTokens:  compressedTokens,
		SavingsPercent:    savings,
		Algorithm:         "mcp-compress",
		DurationMs:        time.Since(start).Milliseconds(),
	}, nil
}

// Learn calls the headroom "learn" MCP tool with a session log.
func (m *MCPClient) Learn(ctx context.Context, sessionLog string) error {
	_, err := m.callTool(ctx, "learn", map[string]interface{}{
		"session": sessionLog,
	})
	return err
}

// callTool issues a tools/call request and returns the concatenated text result.
func (m *MCPClient) callTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	raw, err := m.call(ctx, "tools/call", mcpToolCallParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}
	var result mcpToolResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decoding tool result: %w", err)
	}
	if result.IsError {
		return "", fmt.Errorf("mcp tool %q returned an error", name)
	}
	var sb strings.Builder
	for _, c := range result.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String(), nil
}

// call performs a single JSON-RPC request/response round trip over stdio.
func (m *MCPClient) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	if m.stdin == nil || m.stdout == nil {
		return nil, fmt.Errorf("mcp client not started")
	}

	id := atomic.AddInt64(&m.nextID, 1)
	req := jsonRPCRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}

	// Write the request as a newline-delimited frame.
	if _, err := m.stdin.Write(append(payload, '\n')); err != nil {
		return nil, fmt.Errorf("writing request: %w", err)
	}

	// Read responses until we find the matching ID (notifications are skipped).
	type readResult struct {
		resp *jsonRPCResponse
		err  error
	}
	ch := make(chan readResult, 1)
	go func() {
		for {
			line, err := m.stdout.ReadBytes('\n')
			if err != nil {
				ch <- readResult{err: err}
				return
			}
			line = []byte(strings.TrimSpace(string(line)))
			if len(line) == 0 {
				continue
			}
			var resp jsonRPCResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				continue // skip non-response frames
			}
			if resp.ID == id {
				ch <- readResult{resp: &resp}
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("reading response: %w", r.err)
		}
		if r.resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", r.resp.Error.Code, r.resp.Error.Message)
		}
		return r.resp.Result, nil
	}
}

// Close terminates the MCP subprocess.
func (m *MCPClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closeLocked()
}

func (m *MCPClient) closeLocked() error {
	if m.stdin != nil {
		_ = m.stdin.Close()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
		_ = m.cmd.Wait()
	}
	m.started = false
	return nil
}
