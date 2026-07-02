// SPDX-License-Identifier: MIT
// Package mcpinstall provides MCP server discovery, installation, and removal
// from a built-in registry of known MCP servers (issue #490).
package mcpinstall

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// MCPServerInfo describes an installable MCP server.
type MCPServerInfo struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name"`
	Description string      `json:"description"`
	Category    string      `json:"category"`
	Package     MCPPackage  `json:"package"`
	Tags        []string    `json:"tags"`
	Homepage    string      `json:"homepage"`
}

// MCPPackage describes how to install and run the MCP server.
type MCPPackage struct {
	Type    string            `json:"type"`    // "npm", "pip", "go", "binary"
	Name    string            `json:"name"`    // package name
	Command string            `json:"command"` // command to run
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// Registry is a catalog of known MCP servers.
type Registry struct {
	mu      sync.RWMutex
	servers map[string]MCPServerInfo
}

// NewRegistry returns a Registry pre-seeded with known MCP servers.
func NewRegistry() *Registry {
	r := &Registry{servers: make(map[string]MCPServerInfo)}
	r.seed()
	return r
}

func (r *Registry) seed() {
	known := []MCPServerInfo{
		{
			Name:        "youtube",
			DisplayName: "YouTube for AI Agents",
			Description: "9 YouTube tools — search, transcript, download, clip",
			Category:    "media",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@anthropic/youtube-mcp",
				Command: "node",
				Args:    []string{"dist/index.js"},
			},
			Tags: []string{"video", "transcript", "media"},
		},
		{
			Name:        "websearch",
			DisplayName: "Web Search",
			Description: "Multi-provider web search (DuckDuckGo, Tavily, SerpAPI, Brave)",
			Category:    "research",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@anthropic/websearch-mcp",
				Command: "node",
				Args:    []string{"dist/index.js"},
			},
			Tags: []string{"search", "web", "research"},
		},
		{
			Name:        "browser",
			DisplayName: "Browser Tools",
			Description: "106 Playwright-based browser automation tools (SIN-Browser-Tools)",
			Category:    "browser",
			Package: MCPPackage{
				Type:    "pip",
				Name:    "sin-browser-tools-library",
				Command: "sin-browser-mcp",
			},
			Tags: []string{"browser", "automation", "playwright"},
		},
		{
			Name:        "filesystem",
			DisplayName: "Filesystem",
			Description: "File system access for MCP clients",
			Category:    "system",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@modelcontextprotocol/server-filesystem",
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-filesystem", "/tmp"},
			},
			Tags: []string{"files", "system"},
		},
		{
			Name:        "github",
			DisplayName: "GitHub",
			Description: "GitHub API integration",
			Category:    "dev",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@modelcontextprotocol/server-github",
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-github"},
			},
			Tags: []string{"github", "git"},
		},
		{
			Name:        "postgres",
			DisplayName: "PostgreSQL",
			Description: "PostgreSQL database access",
			Category:    "data",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@modelcontextprotocol/server-postgres",
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-postgres"},
			},
			Tags: []string{"database", "sql"},
		},
		{
			Name:        "sqlite",
			DisplayName: "SQLite",
			Description: "SQLite database access",
			Category:    "data",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@modelcontextprotocol/server-sqlite",
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-sqlite"},
			},
			Tags: []string{"database", "sql"},
		},
		{
			Name:        "brave-search",
			DisplayName: "Brave Search",
			Description: "Brave Search API",
			Category:    "research",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@modelcontextprotocol/server-brave-search",
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-brave-search"},
			},
			Tags: []string{"search", "web"},
		},
		{
			Name:        "puppeteer",
			DisplayName: "Puppeteer",
			Description: "Browser automation via Puppeteer",
			Category:    "browser",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@modelcontextprotocol/server-puppeteer",
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-puppeteer"},
			},
			Tags: []string{"browser", "automation"},
		},
		{
			Name:        "memory",
			DisplayName: "Memory",
			Description: "Persistent memory storage",
			Category:    "memory",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@modelcontextprotocol/server-memory",
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-memory"},
			},
			Tags: []string{"memory", "storage"},
		},
		{
			Name:        "time",
			DisplayName: "Time",
			Description: "Time and timezone tools",
			Category:    "utility",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@modelcontextprotocol/server-time",
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-time"},
			},
			Tags: []string{"time", "timezone"},
		},
		{
			Name:        "fetch",
			DisplayName: "Fetch",
			Description: "URL fetching and content extraction",
			Category:    "utility",
			Package: MCPPackage{
				Type:    "npm",
				Name:    "@modelcontextprotocol/server-fetch",
				Command: "npx",
				Args:    []string{"@modelcontextprotocol/server-fetch"},
			},
			Tags: []string{"fetch", "url", "web"},
		},
	}
	for _, s := range known {
		r.servers[s.Name] = s
	}
}

// List returns all registered MCP servers sorted by name.
func (r *Registry) List() []MCPServerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]MCPServerInfo, 0, len(r.servers))
	for _, s := range r.servers {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get returns the server info for the given name.
func (r *Registry) Get(name string) (MCPServerInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.servers[name]
	return s, ok
}

// Search returns servers whose name, description, or category contains query.
func (r *Registry) Search(query string) []MCPServerInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	q := strings.ToLower(query)
	var out []MCPServerInfo
	for _, s := range r.servers {
		if strings.Contains(strings.ToLower(s.Name), q) ||
			strings.Contains(strings.ToLower(s.Description), q) ||
			strings.Contains(strings.ToLower(s.Category), q) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Categories returns the distinct category list sorted alphabetically.
func (r *Registry) Categories() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := make(map[string]bool)
	var out []string
	for _, s := range r.servers {
		if !seen[s.Category] {
			seen[s.Category] = true
			out = append(out, s.Category)
		}
	}
	sort.Strings(out)
	return out
}

// FetchRemote fetches MCP server info from a remote registry URL.
func FetchRemote(url string) ([]MCPServerInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	var servers []MCPServerInfo
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return servers, nil
}
