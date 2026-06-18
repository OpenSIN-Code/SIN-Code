// SPDX-License-Identifier: MIT
// Purpose: dynamic MCP server discovery from standard config locations (issue #368).
package mcpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

var (
	// userConfigDirHook lets tests override the config directory without
	// touching the real filesystem.
	userConfigDirHook = os.UserConfigDir
)

// DiscoverConfigs scans well-known MCP server config locations and returns
// discovered ServerConfig entries. Later entries override earlier ones by name.
// The scan locations are:
//   - ~/.config/mcp/servers/*.json
//   - ~/.config/claude/claude_desktop_config.json (mcpServers map)
//   - ~/.opencode/opencode.json (mcpServers map)
//   - ~/.codex/config.json (mcpServers map)
//   - <workspace>/.sin-code/mcp.json (mcpServers map)
func DiscoverConfigs(workspace string) []ServerConfig {
	merged := map[string]ServerConfig{}

	// Per-user directory of individual server JSON files.
	if cfg, err := userConfigDirHook(); err == nil {
		serversDir := filepath.Join(cfg, "mcp", "servers")
		entries, _ := os.ReadDir(serversDir)
		for _, e := range entries {
			if e.IsDir() || !stringsHasSuffix(e.Name(), ".json") {
				continue
			}
			path := filepath.Join(serversDir, e.Name())
			cfgs, err := readServerConfigs(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: skipping discovered mcp config %s: %v\n", path, err)
				continue
			}
			for _, c := range cfgs {
				merged[c.Name] = c
			}
		}
	}

	// Claude Desktop config.
	if home, err := userHomeDirHook(); err == nil {
		if cfgs, err := readMCPServersMap(filepath.Join(home, ".config", "claude", "claude_desktop_config.json")); err == nil {
			for _, c := range cfgs {
				merged[c.Name] = c
			}
		}
	}

	// opencode config.
	if home, err := userHomeDirHook(); err == nil {
		if cfgs, err := readMCPServersMap(filepath.Join(home, ".config", "opencode", "opencode.json")); err == nil {
			for _, c := range cfgs {
				merged[c.Name] = c
			}
		}
	}

	// codex config.
	if home, err := userHomeDirHook(); err == nil {
		if cfgs, err := readMCPServersMap(filepath.Join(home, ".config", "codex", "config.json")); err == nil {
			for _, c := range cfgs {
				merged[c.Name] = c
			}
		}
	}

	// Workspace config.
	if workspace != "" {
		if cfgs, err := readMCPServersMap(filepath.Join(workspace, ".sin-code", "mcp.json")); err == nil {
			for _, c := range cfgs {
				merged[c.Name] = c
			}
		}
	}

	out := make([]ServerConfig, 0, len(merged))
	for _, c := range merged {
		out = append(out, c)
	}
	return out
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

// readServerConfigs parses a single JSON file that may be either a ServerConfig
// object or an array of ServerConfig objects.
func readServerConfigs(path string) ([]ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var single ServerConfig
	if err := json.Unmarshal(data, &single); err == nil && single.Name != "" {
		return []ServerConfig{single}, nil
	}
	var arr []ServerConfig
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("neither ServerConfig object nor array: %w", err)
	}
	return arr, nil
}

// mcpServersFile is the common shape used by Claude Desktop, opencode, and codex.
type mcpServersFile struct {
	MCPServers map[string]struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		URL     string            `json:"url"`
		Env     map[string]string `json:"env"`
	} `json:"mcpServers"`
}

// readMCPServersMap parses a config file with a top-level mcpServers map and
// returns ServerConfig entries. The map key is used as the server name if the
// entry does not already contain one.
func readMCPServersMap(path string) ([]ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f mcpServersFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if len(f.MCPServers) == 0 {
		return nil, nil
	}
	out := make([]ServerConfig, 0, len(f.MCPServers))
	for name, e := range f.MCPServers {
		transport := "stdio"
		if e.URL != "" {
			transport = "sse"
		}
		out = append(out, ServerConfig{
			Name:      name,
			Transport: transport,
			Command:   e.Command,
			Args:      e.Args,
			URL:       e.URL,
			Env:       e.Env,
		})
	}
	return out, nil
}

// WriteServerConfig writes a single ServerConfig to the user config directory
// as ~/.config/mcp/servers/<name>.json.
func WriteServerConfig(cfg ServerConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("server config name required")
	}
	configDir, err := userConfigDirHook()
	if err != nil {
		return err
	}
	serversDir := filepath.Join(configDir, "mcp", "servers")
	if err := os.MkdirAll(serversDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(serversDir, cfg.Name+".json")
	return os.WriteFile(path, data, 0o644)
}
