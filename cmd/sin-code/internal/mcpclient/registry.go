// SPDX-License-Identifier: MIT
// Purpose: built-in registry of the OpenSIN-Code ecosystem MCP servers
// (the 12 skill repos developed FOR SIN-Code). Entries activate only if
// their launcher binary exists on PATH or SIN_SKILLS_DIR points at local
// checkouts — unreachable servers are skipped by ConnectAll anyway.
package mcpclient

import (
	"os"
	"path/filepath"
)

// DefaultServers returns the ecosystem registry. Server names double as
// tool-name prefixes ("websearch__search", "browser__navigate", ...), which
// the permission matrix gates via the "mcp" policy class.
func DefaultServers() []ServerConfig {
	skillsDir := os.Getenv("SIN_SKILLS_DIR")
	py := func(repo string) ServerConfig {
		name := shortName(repo)
		cfg := ServerConfig{Name: name, Transport: "stdio"}
		if skillsDir != "" {
			cfg.Command = "python3"
			cfg.Args = []string{filepath.Join(skillsDir, repo, "mcp_server.py")}
		} else {
			cfg.Command = "sin-" + name
		}
		return cfg
	}
	// goNative returns a ServerConfig for a Go-native skill. It prefers the
	// binary built inside SIN_SKILLS_DIR/<repo>/sin-websearch so that skillmgr
	// can install and run the skill without requiring the user to put the binary
	// on PATH. Falls back to the binary name on PATH if no local checkout exists.
	goNative := func(repo, binary string, args ...string) ServerConfig {
		name := shortName(repo)
		cfg := ServerConfig{Name: name, Transport: "stdio", Args: args}
		if skillsDir != "" {
			localBin := filepath.Join(skillsDir, repo, binary)
			if _, err := os.Stat(localBin); err == nil {
				cfg.Command = localBin
			} else {
				cfg.Command = binary
			}
		} else {
			cfg.Command = binary
		}
		return cfg
	}
	return []ServerConfig{
		// web_search_bundle is the Go-native successor to SIN-Code-Websearch-Skill.
		goNative("web_search_bundle", "sin-websearch", "serve"),
		py("SIN-Code-Scheduler-Skill"),
		py("SIN-Code-Goal-Mode-Skill"),
		py("SIN-Code-Grill-Me-Skill"),
		py("SIN-Code-Marketplace-Skill"),
		py("SIN-Code-Doc-Coauthoring-Skill"),
		py("SIN-Code-Context-Bridge-Skill"),
		py("SIN-Code-Honcho-Rollback-Skill"),
		py("SIN-Code-Frontend-Design-Skill"),
		py("SIN-Code-MCP-Server-Builder-Skill"),
		py("SIN-Browser-Tools"),
		py("Simone-MCP"),
		py("SIN-Code-Symfony-Lens"),
	}
}

func shortName(repo string) string {
	m := map[string]string{
		"web_search_bundle":                 "websearch",
		"SIN-Code-Websearch-Skill":          "websearch",
		"SIN-Code-Scheduler-Skill":          "scheduler",
		"SIN-Code-Goal-Mode-Skill":          "goalmode",
		"SIN-Code-Grill-Me-Skill":           "grillme",
		"SIN-Code-Marketplace-Skill":        "marketplace",
		"SIN-Code-Doc-Coauthoring-Skill":    "codocs",
		"SIN-Code-Context-Bridge-Skill":     "contextbridge",
		"SIN-Code-Honcho-Rollback-Skill":    "honcho",
		"SIN-Code-Frontend-Design-Skill":    "frontend",
		"SIN-Code-MCP-Server-Builder-Skill": "mcpbuilder",
		"SIN-Browser-Tools":                 "browser",
		"Simone-MCP":                        "simone",
		"SIN-Code-Symfony-Lens":             "symfonylens",
	}
	if s, ok := m[repo]; ok {
		return s
	}
	return repo
}
