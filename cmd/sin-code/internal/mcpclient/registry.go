// SPDX-License-Identifier: MIT
// Purpose: built-in registry of the OpenSIN-Code ecosystem MCP servers
// (the 12 skill repos developed FOR SIN-Code). Entries activate only if
// their launcher binary exists on PATH or SIN_SKILLS_DIR points at local
// checkouts — unreachable servers are skipped by ConnectAll anyway.
package mcpclient

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var (
	// userHomeDirHook lets tests force the default skills-dir path to empty
	// so the PATH fallback branches in DefaultServers are exercised.
	userHomeDirHook = os.UserHomeDir

	// testSkillsDir lets coverage tests override the skills directory without
	// touching the real filesystem or environment variables.
	testSkillsDir *string

	// lookPathHook is swapped in tests to avoid depending on the real PATH.
	lookPathHook = exec.LookPath
)

// canonicalBinary maps a short server name to the upstream console-script
// binary that exposes the MCP server. These names differ from the naive
// "sin-<name>" fallback for historical reasons (e.g. goalmode ships as
// sin-goal-mode, mcpbuilder as sin-mcp-server-builder).
func canonicalBinary(name string) string {
	m := map[string]string{
		"goalmode":      "sin-goal-mode",
		"scheduler":     "sin-scheduler",
		"codocs":        "sin-codocs",
		"marketplace":   "sin-marketplace",
		"browser":       "sin-browser",
		"contextbridge": "sin-context-bridge",
		"honcho":        "sin-honcho-rollback",
		"frontend":      "sin-frontend-design",
		"mcpbuilder":    "sin-mcp-server-builder",
		"grillme":       "sin-grill-me",
	}
	if b, ok := m[name]; ok {
		return b
	}
	return "sin-" + name
}

// findOnPath returns the first candidate found on PATH, or empty.
func findOnPath(names ...string) string {
	for _, n := range names {
		if p, err := lookPathHook(n); err == nil {
			return p
		}
	}
	return ""
}

// pythonConfig returns a stdio ServerConfig for a discovered Python MCP
// entrypoint. If the script is at the repo root it is run directly; otherwise
// it is run as a module with PYTHONPATH set to the source root so relative
// imports (e.g. src/sin_scheduler/mcp_server.py) work.
func pythonConfig(repoRoot, script string) ServerConfig {
	if filepath.Dir(script) == repoRoot {
		return ServerConfig{Transport: "stdio", Command: "python3", Args: []string{script}}
	}

	rel, err := filepath.Rel(repoRoot, script)
	if err != nil {
		rel = script
	}
	parts := strings.Split(rel, string(os.PathSeparator))

	// Find the first directory in the relative path that is a Python package.
	// The source root is the directory containing that package.
	sourceRoot := repoRoot
	pkgIndex := 0
	for i := 0; i < len(parts)-1; i++ {
		d := filepath.Join(append([]string{repoRoot}, parts[:i+1]...)...)
		if _, err := os.Stat(filepath.Join(d, "__init__.py")); err == nil {
			pkgIndex = i
			break
		}
	}
	if pkgIndex > 0 {
		sourceRoot = filepath.Join(append([]string{repoRoot}, parts[:pkgIndex]...)...)
	}
	module := strings.Join(parts[pkgIndex:], ".")
	module = strings.TrimSuffix(module, ".py")

	cfg := ServerConfig{Transport: "stdio", Command: "python3", Args: []string{"-m", module}, Dir: repoRoot}
	cfg.Env = map[string]string{"PYTHONPATH": sourceRoot}
	return cfg
}

// findMcpServer searches a skill checkout for the actual MCP server module.
// It checks the canonical root-level script first, then common package
// locations (src, lib, top-level subdirectories). Test trees are skipped.
// The returned path is absolute. If nothing is found it returns "".
func findMcpServer(dir string) string {
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	root := filepath.Join(dir, "mcp_server.py")
	if _, err := os.Stat(root); err == nil {
		return root
	}

	patterns := []string{
		filepath.Join(dir, "src", "*", "mcp_server.py"),
		filepath.Join(dir, "src", "mcp_server.py"),
		filepath.Join(dir, "lib", "mcp_server.py"),
		filepath.Join(dir, "*", "mcp_server.py"),
	}
	var candidates []string
	for _, pat := range patterns {
		if matches, _ := filepath.Glob(pat); len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Strings(candidates)
	for _, c := range candidates {
		rel, _ := filepath.Rel(dir, c)
		if strings.HasPrefix(rel, "tests"+string(os.PathSeparator)) ||
			strings.HasPrefix(rel, "test"+string(os.PathSeparator)) {
			continue
		}
		return c
	}
	return ""
}

// DefaultServers returns the ecosystem registry. Server names double as
// tool-name prefixes ("websearch__search", "browser__navigate", ...), which
// the permission matrix gates via the "mcp" policy class.
func DefaultServers() []ServerConfig {
	skillsDir := skillsDirOrDefault()
	py := func(repo string) ServerConfig {
		name := shortName(repo)
		cfg := ServerConfig{Name: name, Transport: "stdio"}
		candidates := []string{
			canonicalBinary(name) + "-mcp",
			"sin-" + name + "-mcp",
			canonicalBinary(name),
			"sin-" + name,
			name,
		}
		if skillsDir != "" {
			localDir := filepath.Join(skillsDir, repo)
			// Prefer the canonical root-level mcp_server.py, then discover the
			// actual MCP server module inside the package tree. Repo-cloned skills
			// are the preferred source; PATH binaries are the fallback.
			if script := findMcpServer(localDir); script != "" {
				cfg = pythonConfig(localDir, script)
				cfg.Name = name
			} else {
				// No local checkout entrypoint: fall back to a console script on PATH.
				cfg.Command = findOnPath(candidates...)
				if cfg.Command == "" {
					cfg.Command = "sin-" + name
				}
			}
		} else {
			cfg.Command = findOnPath(candidates...)
			if cfg.Command == "" {
				cfg.Command = "sin-" + name
			}
		}
		return cfg
	}
	// goNative returns a ServerConfig for a Go-native skill. It prefers the
	// binary built inside SIN_SKILLS_DIR/<repo>/<binary> so that skillmgr
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

		// v3.22.0: sin-analyse-suite — multimodal preprocessing (image, video, PDF, logs, data, audio)
		goNative("sin-analyse-suite", "sin-analyse", "serve"),

		// v3.22.0 (issue #382): native_browser — pure-Go headless browser facade
		// (cmd/sin-code/internal/native_browser). Registered here so its tool
		// namespace native_browser__* is enumerated by the catalog + permission
		// matrix; the actual implementation runs in-process behind the Driver
		// seam and never spawns a subprocess. The optional sin-native-browser
		// binary is a future stdio shim — see issue #382 follow-up for the
		// release that promotes an MCP façade behind the same namespace.
		goNative("native_browser", "sin-native-browser", "serve"),

		// External MCP server (Python stdio) — autodev-cli v0.4.0 (Bridged-External, never vendored)
		{Name: "autodev", Transport: "stdio", Command: "autodev-mcp"},
	}
}

func shortName(repo string) string {
	m := map[string]string{
		"web_search_bundle":                 "websearch",
		"sin-analyse-suite":                 "analyse",
		"native_browser":                    "native_browser",
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

// skillsDirOrDefault returns the configured SIN_SKILLS_DIR or the default
// local share location used by skillmgr. This keeps the registry in sync
// with where skillmgr actually installs skills.
func skillsDirOrDefault() string {
	if testSkillsDir != nil {
		return *testSkillsDir
	}
	if d := os.Getenv("SIN_SKILLS_DIR"); d != "" {
		return d
	}
	home, err := userHomeDirHook()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share", "sin-code", "skills")
}
