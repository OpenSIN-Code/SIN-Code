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
)

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
	sessions map[string]*sdk.ClientSession
	tools    []Tool
}

func NewManager(configs []ServerConfig) *Manager {
	return &Manager{configs: configs, sessions: map[string]*sdk.ClientSession{}}
}

// ConnectAll connects to every configured server. A single failing server is
// logged and skipped — external tools are additive, never fatal.
func (m *Manager) ConnectAll(ctx context.Context) error {
	client := sdk.NewClient(&sdk.Implementation{Name: "sin-code", Version: "3.1.0"}, nil)
	for _, cfg := range m.configs {
		if err := m.connect(ctx, client, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warn: mcp server %q unavailable: %v\n", cfg.Name, err)
		}
	}
	return nil
}

func (m *Manager) connect(ctx context.Context, client *sdk.Client, cfg ServerConfig) error {
	var transport sdk.Transport
	switch cfg.Transport {
	case "stdio":
		cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		transport = &sdk.CommandTransport{Command: cmd}
	case "http":
		transport = &sdk.StreamableClientTransport{Endpoint: cfg.URL}
	default:
		return fmt.Errorf("unknown transport %q", cfg.Transport)
	}

	sess, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return err
	}
	res, err := sess.ListTools(ctx, nil)
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
	m.sessions = map[string]*sdk.ClientSession{}
}
