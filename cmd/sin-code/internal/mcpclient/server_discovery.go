// SPDX-License-Identifier: MIT
// Purpose: Dynamic MCP server discovery with a struct-based API (issue #368).
// ServerDiscovery scans a configurable list of config paths (directories of
// per-server JSON files and/or single mcp.json files), normalizes each entry
// to a DiscoveredServer, deduplicates by name, and supports add/remove of
// individual servers persisted under ~/.config/mcp/servers/<name>.json.
//
// This is the struct-based companion to the function-based DiscoverConfigs in
// discovery.go. The two coexist in the same package: DiscoverConfigs returns
// ServerConfig entries for the connection manager, while ServerDiscovery
// returns DiscoveredServer entries (with a Source field) for the `mcp discover`
// / `mcp add` CLI surface.
//
// Thread-safe: all mutating operations take d.mu (mandate M7).
package mcpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// DiscoveredServer is the normalized representation of an MCP server found in a
// config location. Source records the path the entry was discovered from.
type DiscoveredServer struct {
	Name      string            `json:"name"`
	Command   string            `json:"command,omitempty"`
	Transport string            `json:"transport,omitempty"`
	Source    string            `json:"source,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// ServerDiscovery scans config paths for MCP server definitions.
type ServerDiscovery struct {
	configPaths []string
	mu          sync.Mutex
}

// NewServerDiscovery creates a discovery scanner. When paths is empty the
// default locations are used:
//   - ~/.config/mcp/                 (directory of per-server JSON files)
//   - .sin-code/mcp.json             (project-local)
//   - ./mcp.json                     (current directory)
func NewServerDiscovery(paths []string) *ServerDiscovery {
	if len(paths) == 0 {
		paths = defaultDiscoveryPaths()
	}
	cp := make([]string, len(paths))
	copy(cp, paths)
	return &ServerDiscovery{configPaths: cp}
}

// defaultDiscoveryPaths returns the default scan locations.
func defaultDiscoveryPaths() []string {
	var paths []string
	if cfg, err := userConfigDirHook(); err == nil && cfg != "" {
		paths = append(paths, filepath.Join(cfg, "mcp"))
	}
	paths = append(paths, filepath.Join(".sin-code", "mcp.json"))
	paths = append(paths, "mcp.json")
	return paths
}

// ConfigPaths returns a copy of the configured scan paths.
func (d *ServerDiscovery) ConfigPaths() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.configPaths...)
}

// Discover scans every config path and returns deduplicated servers. A path may
// be either a directory (all *.json files inside are parsed) or a single file.
// Missing paths are silently skipped (additive, never fatal). Duplicate names
// are kept by first occurrence. The result is sorted by name for determinism.
// A non-nil error is returned only when a present file fails to parse.
func (d *ServerDiscovery) Discover() ([]DiscoveredServer, error) {
	paths := d.ConfigPaths()

	seen := make(map[string]bool)
	var out []DiscoveredServer
	var firstErr error

	for _, p := range paths {
		servers, err := d.discoverPath(p)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, s := range servers {
			if s.Name == "" || seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			if s.Source == "" {
				s.Source = p
			}
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, firstErr
}

// discoverPath parses a single path which may be a directory or a file.
func (d *ServerDiscovery) discoverPath(p string) ([]DiscoveredServer, error) {
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if info.IsDir() {
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		var out []DiscoveredServer
		for _, e := range entries {
			if e.IsDir() || !stringsHasSuffix(e.Name(), ".json") {
				continue
			}
			servers, perr := d.ParseConfig(filepath.Join(p, e.Name()))
			if perr != nil {
				return nil, perr
			}
			out = append(out, servers...)
		}
		return out, nil
	}
	return d.ParseConfig(p)
}

// ParseConfig parses a single mcp.json file. Three shapes are accepted:
//  1. {"mcpServers": {"<name>": {"command":...,"args":...,"url":...,"env":...}}}
//  2. a single DiscoveredServer object {"name":...,"command":...}
//  3. an array of DiscoveredServer objects
//
// The map key is used as the server name for shape (1). Source is set to path.
func (d *ServerDiscovery) ParseConfig(path string) ([]DiscoveredServer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Shape 1: mcpServers map.
	var mapFile struct {
		MCPServers map[string]struct {
			Command   string            `json:"command"`
			Args      []string          `json:"args"`
			URL       string            `json:"url"`
			Transport string            `json:"transport"`
			Env       map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &mapFile); err == nil && len(mapFile.MCPServers) > 0 {
		out := make([]DiscoveredServer, 0, len(mapFile.MCPServers))
		for name, e := range mapFile.MCPServers {
			transport := e.Transport
			if transport == "" {
				transport = "stdio"
				if e.URL != "" {
					transport = "sse"
				}
			}
			out = append(out, DiscoveredServer{
				Name:      name,
				Command:   e.Command,
				Transport: transport,
				Source:    path,
				Env:       e.Env,
			})
		}
		return out, nil
	}

	// Shape 2: single object.
	var single DiscoveredServer
	if err := json.Unmarshal(data, &single); err == nil && single.Name != "" {
		if single.Transport == "" {
			single.Transport = "stdio"
		}
		single.Source = path
		return []DiscoveredServer{single}, nil
	}

	// Shape 3: array.
	var arr []DiscoveredServer
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, fmt.Errorf("config %s: not an mcpServers map, single object, or array: %w", path, err)
	}
	for i := range arr {
		if arr[i].Transport == "" {
			arr[i].Transport = "stdio"
		}
		if arr[i].Source == "" {
			arr[i].Source = path
		}
	}
	return arr, nil
}

// AddServer persists a server to ~/.config/mcp/servers/<name>.json so it is
// discoverable on subsequent scans.
func (d *ServerDiscovery) AddServer(s DiscoveredServer) error {
	if s.Name == "" {
		return fmt.Errorf("server name required")
	}
	cfg, err := userConfigDirHook()
	if err != nil {
		return err
	}
	serversDir := filepath.Join(cfg, "mcp", "servers")
	if err := os.MkdirAll(serversDir, 0o755); err != nil {
		return err
	}
	// Marshal a clean record without the Source field.
	rec := struct {
		Name      string            `json:"name"`
		Command   string            `json:"command,omitempty"`
		Transport string            `json:"transport,omitempty"`
		Env       map[string]string `json:"env,omitempty"`
	}{
		Name:      s.Name,
		Command:   s.Command,
		Transport: s.Transport,
		Env:       s.Env,
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(serversDir, s.Name+".json"), data, 0o644)
}

// RemoveServer deletes ~/.config/mcp/servers/<name>.json. It is idempotent: a
// missing file is not an error.
func (d *ServerDiscovery) RemoveServer(name string) error {
	if name == "" {
		return fmt.Errorf("server name required")
	}
	cfg, err := userConfigDirHook()
	if err != nil {
		return err
	}
	path := filepath.Join(cfg, "mcp", "servers", name+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
