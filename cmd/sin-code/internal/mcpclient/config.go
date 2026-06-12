// SPDX-License-Identifier: MIT
// Purpose: discover and load external MCP server configs (mandate C5).
// Merge order (later wins by Name): built-in defaults -> user config
// (~/.config/sin-code/mcp.json) -> workspace (.sin-code/mcp.json).
// A server entry with "disabled": true removes it from the final set.
package mcpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type fileEntry struct {
	ServerConfig
	Disabled bool `json:"disabled,omitempty"`
}

type configFile struct {
	MCPServers map[string]fileEntry `json:"mcpServers"`
}

// LoadConfigs returns the effective server list for a workspace.
// Missing files are fine; broken files are logged to stderr and skipped
// (additive, never fatal — same guarantee as ConnectAll).
func LoadConfigs(workspace string) []ServerConfig {
	merged := map[string]fileEntry{}
	for _, e := range DefaultServers() {
		merged[e.Name] = fileEntry{ServerConfig: e}
	}
	for _, path := range configPaths(workspace) {
		entries, err := readConfigFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "warn: skipping invalid mcp config %s: %v\n", path, err)
			}
			continue
		}
		for _, e := range entries {
			merged[e.Name] = e
		}
	}
	out := make([]ServerConfig, 0, len(merged))
	for _, e := range merged {
		if e.Disabled {
			continue
		}
		e.ServerConfig.Command = os.ExpandEnv(e.ServerConfig.Command)
		e.ServerConfig.URL = os.ExpandEnv(e.ServerConfig.URL)
		for i, a := range e.ServerConfig.Args {
			e.ServerConfig.Args[i] = os.ExpandEnv(a)
		}
		out = append(out, e.ServerConfig)
	}
	return out
}

func configPaths(workspace string) []string {
	var paths []string
	if cfg, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfg, "sin-code", "mcp.json"))
	}
	if workspace != "" {
		paths = append(paths, filepath.Join(workspace, ".sin-code", "mcp.json"))
	}
	return paths
}

func readConfigFile(path string) ([]fileEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cf configFile
	if err := json.Unmarshal(data, &cf); err == nil && len(cf.MCPServers) > 0 {
		out := make([]fileEntry, 0, len(cf.MCPServers))
		for name, e := range cf.MCPServers {
			if e.Name == "" {
				e.Name = name
			}
			out = append(out, e)
		}
		return out, nil
	}
	var arr []fileEntry
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("neither {\"mcpServers\":{}} map nor array: %w", err)
	}
	return arr, nil
}
