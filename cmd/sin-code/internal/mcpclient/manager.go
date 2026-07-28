// SPDX-License-Identifier: MIT
// Purpose: consume EXTERNAL MCP servers (Simone-MCP, Browser-Tools, skills,
// Orchestration) and merge their tools into the agent router with
// "server__tool" namespacing (mandate C5, AGENTS.md §8).
package mcpclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/logger"
)

var (
	// connectTransportHook lets tests inject an in-memory MCP transport so the
	// connection success path can be exercised without spawning subprocesses.
	connectTransportHook func(ctx context.Context, cfg ServerConfig) (sdk.Transport, error)

	// listToolsHook lets tests inject a ListTools error without building a
	// custom transport/connection implementation.
	listToolsHook func(ctx context.Context, sess *sdk.ClientSession) (*sdk.ListToolsResult, error)

	// testConnectHook lets coverage tests bypass the real transport and SDK
	// entirely by returning a fake session and a tool list directly.
	testConnectHook func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error)

	// testTransportProvider lets coverage tests inject an in-memory transport
	// for the real SDK connection path.
	testTransportProvider func(cfg ServerConfig) (sdk.Transport, error)

	// testListToolsErr lets coverage tests force the ListTools step to fail.
	testListToolsErr error
)

type ServerConfig struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Dir       string            `json:"dir,omitempty"`
}

type Tool struct {
	Server      string
	Name        string
	Qualified   string
	Description string
	InputSchema map[string]any
}

type session interface {
	CallTool(ctx context.Context, params *sdk.CallToolParams) (*sdk.CallToolResult, error)
	Close() error
}

type realSession struct {
	*sdk.ClientSession
}

func (r *realSession) Close() error {
	if r.ClientSession == nil {
		return nil
	}
	return r.ClientSession.Close()
}

type Manager struct {
	configs        []ServerConfig
	connectTimeout time.Duration
	mu             sync.RWMutex
	sessions       map[string]session
	tools          []Tool
	warnedMu       sync.Mutex
	warnedServers  map[string]bool
	// Quiet suppresses per-server warnings and the connection summary
	// line on stderr. Set to true in headless mode when the operator has
	// not passed --verbose so the output stays clean.
	Quiet bool
}

func NewManager(configs []ServerConfig) *Manager {
	return &Manager{
		configs:        configs,
		connectTimeout: 10 * time.Second,
		sessions:       map[string]session{},
		warnedServers:  map[string]bool{},
	}
}

// SetConnectTimeout overrides the per-server connection timeout (default 3s).
func (m *Manager) SetConnectTimeout(d time.Duration) {
	if d > 0 {
		m.connectTimeout = d
	}
}

// ConnectAll connects to every configured server in parallel. Each server
// gets a per-server connection timeout (default 3s, configurable via
// SetConnectTimeout / mcp.connect_timeout). A single failing server is
// logged and skipped — external tools are additive, never fatal.
// Warnings are deduplicated: each server name is warned about at most once.
func (m *Manager) ConnectAll(ctx context.Context) error {
	if len(m.configs) == 0 {
		return nil
	}

	timeout := m.connectTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}

	type result struct {
		name string
		err  error
	}

	results := make([]result, len(m.configs))
	var wg sync.WaitGroup
	client := sdk.NewClient(&sdk.Implementation{Name: "sin-code", Version: "3.1.0"}, nil)

	for i, cfg := range m.configs {
		wg.Add(1)
		go func(i int, cfg ServerConfig) {
			defer wg.Done()
			serverCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			results[i] = result{
				name: cfg.Name,
				err:  m.connect(serverCtx, client, cfg),
			}
		}(i, cfg)
	}

	wg.Wait()

	var connected, failed, newFailures int
	var failedNames []string
	for _, r := range results {
		if r.err != nil {
			failed++
			failedNames = append(failedNames, r.name)
			m.warnedMu.Lock()
			if !m.warnedServers[r.name] {
				m.warnedServers[r.name] = true
				newFailures++
				m.warnedMu.Unlock()
				if !m.Quiet {
					logger.Warn("mcp server unavailable", map[string]any{
						"server": r.name,
						"error":  r.err.Error(),
					})
				}
			} else {
				m.warnedMu.Unlock()
			}
		} else {
			connected++
		}
	}

	if !m.Quiet && newFailures > 0 && failed > 0 {
		fmt.Fprintf(os.Stderr, "MCP: %d/%d servers connected (%d skipped: %s)\n",
			connected, len(m.configs), failed, strings.Join(failedNames, ", "))
	}

	return nil
}

func (m *Manager) connect(ctx context.Context, client *sdk.Client, cfg ServerConfig) error {
	if testConnectHook != nil {
		sess, tools, err := testConnectHook(ctx, client, cfg)
		if err != nil {
			return err
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		m.sessions[cfg.Name] = sess
		m.tools = append(m.tools, tools...)
		return nil
	}

	var transport sdk.Transport
	if testTransportProvider != nil {
		t, err := testTransportProvider(cfg)
		if err != nil {
			return err
		}
		transport = t
	} else {
		switch cfg.Transport {
		case "stdio":
			cmdCtx, cmdCancel := context.WithCancel(context.Background())
			_ = cmdCancel // lifetime tied to the process; cleaned up on Close()
			cmd := exec.CommandContext(cmdCtx, cfg.Command, cfg.Args...)
			cmd.Env = os.Environ()
			for k, v := range cfg.Env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
			if cfg.Dir != "" {
				cmd.Dir = cfg.Dir
			}
			// Simone-MCP (Python SDK) sends capabilities.extensions as a JSON
			// array instead of a map, which the go-sdk cannot unmarshal. Use
			// a custom transport that normalises the field at the raw-JSON
			// level before the SDK parser sees it.
			if cfg.Name == "simone" {
				transport = newExtensionsFixTransport(cmd)
			} else {
				transport = &sdk.CommandTransport{Command: cmd}
			}
		case "http":
			transport = &sdk.StreamableClientTransport{Endpoint: cfg.URL}
		default:
			return fmt.Errorf("unknown transport %q", cfg.Transport)
		}
	}

	if connectTransportHook != nil {
		if t, err := connectTransportHook(ctx, cfg); err != nil {
			return err
		} else if t != nil {
			transport = t
		}
	}

	sess, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return err
	}

	if testListToolsErr != nil {
		_ = sess.Close()
		return testListToolsErr
	}

	var res *sdk.ListToolsResult
	if listToolsHook != nil {
		res, err = listToolsHook(ctx, sess)
	} else {
		res, err = sess.ListTools(ctx, nil)
	}
	if err != nil {
		_ = sess.Close()
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[cfg.Name] = &realSession{ClientSession: sess}
	for _, t := range res.Tools {
		m.tools = append(m.tools, Tool{
			Server:      cfg.Name,
			Name:        t.Name,
			Qualified:   cfg.Name + "__" + t.Name,
			Description: t.Description,
		})
	}
	return nil
}

func (m *Manager) Tools() []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Tool(nil), m.tools...)
}

// Call routes a qualified name ("server__tool") to the owning server.
func (m *Manager) Call(ctx context.Context, qualified string, args map[string]any) (string, error) {
	server, tool, ok := strings.Cut(qualified, "__")
	if !ok {
		return "", fmt.Errorf("not an external tool: %q", qualified)
	}
	m.mu.RLock()
	sess, found := m.sessions[server]
	m.mu.RUnlock()
	if !found {
		return "", fmt.Errorf("no MCP session for server %q", server)
	}
	res, err := sess.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return "", err
	}
	var out strings.Builder
	for _, c := range res.Content {
		if tc, isText := c.(*sdk.TextContent); isText {
			out.WriteString(tc.Text)
		}
	}
	if res.IsError {
		return out.String(), fmt.Errorf("tool %s returned an error", qualified)
	}
	return out.String(), nil
}

// IsExternal reports whether a tool name belongs to an external server.
func (m *Manager) IsExternal(name string) bool {
	return strings.Contains(name, "__")
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		_ = s.Close()
	}
	m.sessions = map[string]session{}
}
