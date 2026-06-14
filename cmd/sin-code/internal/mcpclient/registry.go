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
	return []ServerConfig{
		// web_search_bundle is the Go-native successor to SIN-Code-Websearch-Skill.
		{Name: "websearch", Transport: "stdio", Command: "sin-websearch", Args: []string{"serve"}},
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
