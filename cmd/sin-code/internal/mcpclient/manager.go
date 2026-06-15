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

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/logger"
)

var (
	warningOnce   sync.Once
	warnedServers = make(map[string]bool)
	warnedMu      sync.Mutex

	// testConnectHook bypasses the real transport/Connect/ListTools dance and
	// returns a fake session + tools directly. Used by coverage tests.
	testConnectHook func(ctx context.Context, client *sdk.Client, cfg ServerConfig) (session, []Tool, error)

	// testTransportProvider overrides stdio transport creation for the real
	// Connect path (e.g. in-memory transports).
	testTransportProvider func(cfg ServerConfig) (sdk.Transport, error)

	// testListToolsErr injects a ListTools error in the real Connect path.
	testListToolsErr error
)

// session is the minimal surface Manager needs from an MCP client session.
// realSession wraps *sdk.ClientSession; tests supply fakeSession.
type session interface {
	CallTool(ctx context.Context, params *sdk.CallToolParams) (*sdk.CallToolResult, error)
	Close() error
}

// realSession adapts *sdk.ClientSession to the session interface.
type realSession struct {
	sess *sdk.ClientSession
}

func (r *realSession) CallTool(ctx context.Context, params *sdk.CallToolParams) (*sdk.CallToolResult, error) {
	return r.sess.CallTool(ctx, params)
}

func (r *realSession) Close() error {
	if r.sess == nil {
		return nil
	}
	return r.sess.Close()
}

type ServerConfig struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

type Tool struct {
	Server      string
	Name        string
	Qualified   string
	Description string
	InputSchema map[string]any
}

type Manager struct {
	configs  []ServerConfig
	mu       sync.RWMutex
	sessions map[string]session
	tools    []Tool
}

func NewManager(configs []ServerConfig) *Manager {
	return &Manager{configs: configs, sessions: map[string]session{}}
}

// ConnectAll connects to every configured server. A single failing server is
// logged and skipped — external tools are additive, never fatal.
// Warnings are deduplicated: each server name is warned about at most once.
func (m *Manager) ConnectAll(ctx context.Context) error {
	client := sdk.NewClient(&sdk.Implementation{Name: "sin-code", Version: "3.1.0"}, nil)
	for _, cfg := range m.configs {
		if err := m.connect(ctx, client, cfg); err != nil {
			warnedMu.Lock()
			if !warnedServers[cfg.Name] {
				warnedServers[cfg.Name] = true
				warnedMu.Unlock()
				logger.Warn("mcp server unavailable", map[string]any{
					"server": cfg.Name,
					"error":  err.Error(),
				})
			} else {
				warnedMu.Unlock()
			}
		}
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
	switch cfg.Transport {
	case "stdio":
		if testTransportProvider != nil {
			tr, err := testTransportProvider(cfg)
			if err != nil {
				return err
			}
			transport = tr
		} else {
			cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
			cmd.Env = os.Environ()
			for k, v := range cfg.Env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
			transport = &sdk.CommandTransport{Command: cmd}
		}
	case "http":
		transport = &sdk.StreamableClientTransport{Endpoint: cfg.URL}
	default:
		return fmt.Errorf("unknown transport %q", cfg.Transport)
	}

	sdkSess, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return err
	}
	sess := &realSession{sess: sdkSess}

	res, err := sdkSess.ListTools(ctx, nil)
	if testListToolsErr != nil {
		err = testListToolsErr
	}
	if err != nil {
		_ = sess.Close()
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[cfg.Name] = sess
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
