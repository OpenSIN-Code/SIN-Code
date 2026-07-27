// SPDX-License-Identifier: MIT
// Purpose: discover and load external MCP server configs (mandate C5) and
// the built-in registry of the OpenSIN-Code ecosystem MCP servers (the 12
// skill repos developed FOR SIN-Code). Merge order (later wins by Name):
// built-in defaults -> user config (~/.config/sin-code/mcp.json) ->
// workspace (.sin-code/mcp.json). A server entry with "disabled": true
// removes it from the final set. Registry entries activate only if their
// launcher binary exists on PATH or SIN_SKILLS_DIR points at local
// checkouts — unreachable servers are skipped by ConnectAll anyway.
package mcpclient

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
// Merge order: built-in defaults -> discovered configs -> ~/.config/sin-code/mcp.json -> workspace/.sin-code/mcp.json.
func LoadConfigs(workspace string) []ServerConfig {
	merged := map[string]fileEntry{}
	for _, e := range DefaultServers() {
		merged[e.Name] = fileEntry{ServerConfig: e}
	}
	for _, c := range DiscoverConfigs(workspace) {
		merged[c.Name] = fileEntry{ServerConfig: c}
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

// ── Built-in ecosystem registry ──────────────────────────────────────────

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
		"simone":        "simone-cli",
		"symfonylens":   "symfony-lens",
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

// pythonCliEntrypoint returns a stdio ServerConfig for Python skills that
// expose their MCP server through a CLI wrapper script (e.g. sin-context-bridge
// ships scripts/sin_context_bridge.py with a "serve" subcommand). The script is
// run from the repo root so relative imports (e.g. lib.*) resolve.
func pythonCliEntrypoint(repo, dir, name string) (ServerConfig, bool) {
	bin := canonicalBinary(name)
	binBase := strings.TrimPrefix(bin, "sin-")
	underscored := "sin_" + strings.ReplaceAll(binBase, "-", "_")
	base := strings.ReplaceAll(name, "_", "-")
	candidates := []string{
		filepath.Join(dir, "scripts", underscored+".py"),
		filepath.Join(dir, "scripts", "sin_"+name+".py"),
		filepath.Join(dir, "scripts", "sin-"+base+".py"),
		filepath.Join(dir, "scripts", name+".py"),
		filepath.Join(dir, "scripts", base+".py"),
		filepath.Join(dir, "scripts", "mcp_server.py"),
	}
	for _, script := range candidates {
		if _, err := os.Stat(script); err == nil {
			cfg := ServerConfig{
				Name:      name,
				Transport: "stdio",
				Command:   "python3",
				Args:      []string{script, "serve"},
				Dir:       dir,
				Env:       map[string]string{"PYTHONPATH": dir},
			}
			return cfg, true
		}
	}
	return ServerConfig{}, false
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

// simoneConfig returns the stdio ServerConfig for the Simone-MCP skill.
// The upstream repo is Python (src/cli.py) and also ships the simone-cli
// console script when installed via pip.
func simoneConfig(skillsDir string) ServerConfig {
	name := "simone"
	if skillsDir != "" {
		dir := filepath.Join(skillsDir, "Simone-MCP")
		cli := filepath.Join(dir, "src", "cli.py")
		if _, err := os.Stat(cli); err == nil {
			return ServerConfig{Name: name, Transport: "stdio", Command: "python3", Args: []string{cli, "serve-mcp"}, Dir: dir}
		}
	}
	candidates := []string{
		canonicalBinary(name) + "-mcp",
		"sin-" + name + "-mcp",
		canonicalBinary(name),
		"sin-" + name,
		name,
	}
	if p := findOnPath(candidates...); p != "" {
		return ServerConfig{Name: name, Transport: "stdio", Command: p, Args: []string{"serve-mcp"}}
	}
	return ServerConfig{Name: name, Transport: "stdio", Command: canonicalBinary(name), Args: []string{"serve-mcp"}}
}

// symfonyLensConfig returns the stdio ServerConfig for the Symfony-Lens skill.
// The upstream repo exposes the MCP server as the Python module
// symfony_lens.server and also ships the symfony-lens console script.
func symfonyLensConfig(skillsDir string) ServerConfig {
	name := "symfonylens"
	if skillsDir != "" {
		dir := filepath.Join(skillsDir, "SIN-Code-Symfony-Lens")
		if _, err := os.Stat(filepath.Join(dir, "symfony_lens", "server.py")); err == nil {
			cfg := ServerConfig{Name: name, Transport: "stdio", Command: "python3", Args: []string{"-m", "symfony_lens.server"}, Dir: dir}
			cfg.Env = map[string]string{"PYTHONPATH": dir}
			return cfg
		}
	}
	candidates := []string{
		canonicalBinary(name) + "-mcp",
		"sin-" + name + "-mcp",
		canonicalBinary(name),
		"sin-" + name,
		name,
	}
	if p := findOnPath(candidates...); p != "" {
		return ServerConfig{Name: name, Transport: "stdio", Command: p}
	}
	return ServerConfig{Name: name, Transport: "stdio", Command: canonicalBinary(name)}
}

// notionConfig returns the stdio ServerConfig for the vibe-notion MCP bridge.
// The bridge is a Python script that wraps the globally-installed `vibe-notion`
// npm CLI as a subprocess (Bridged-External pattern, M6). It resolves the
// bridge script path in this order:
//  1. $SIN_NOTION_MCP_PATH env var (absolute path to mcp_server.py)
//  2. ~/skills/vibe-notion-mcp/mcp_server.py (default install location)
//  3. <skillsDir>/vibe-notion-mcp/mcp_server.py (skills-dir relative)
//
// The Python interpreter is resolved from the bridge's venv if present,
// otherwise falls back to system python3. The vibe-notion binary must be
// on PATH (installed via `npm install -g vibe-notion`).
func notionConfig(skillsDir string) ServerConfig {
	name := "notion"
	scriptPath := notionMCPPath(skillsDir)

	// Prefer the venv python if it exists alongside the script.
	venvPython := filepath.Join(filepath.Dir(scriptPath), ".venv", "bin", "python3")
	pythonCmd := "python3"
	if _, err := os.Stat(venvPython); err == nil {
		pythonCmd = venvPython
	}

	cfg := ServerConfig{
		Name:      name,
		Transport: "stdio",
		Command:   pythonCmd,
		Args:      []string{scriptPath},
		Env: map[string]string{
			"VIBE_NOTION_BIN":     "vibe-notion",
			"VIBE_NOTION_TIMEOUT": "60",
		},
	}
	return cfg
}

// notionMCPPath resolves the path to the vibe-notion MCP bridge script.
func notionMCPPath(skillsDir string) string {
	if p := os.Getenv("SIN_NOTION_MCP_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	home, err := userHomeDirHook()
	if err == nil && home != "" {
		candidate := filepath.Join(home, "skills", "vibe-notion-mcp", "mcp_server.py")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if skillsDir != "" {
		candidate := filepath.Join(skillsDir, "vibe-notion-mcp", "mcp_server.py")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join("vibe-notion-mcp", "mcp_server.py")
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
			// Some Python skills expose the MCP server via a CLI wrapper script
			// (e.g. sin-context-bridge's scripts/sin_context_bridge.py serve).
			// Prefer that over the module entrypoint when present.
			if cliCfg, ok := pythonCliEntrypoint(repo, localDir, name); ok {
				cfg = cliCfg
			} else if script := findMcpServer(localDir); script != "" {
				cfg = pythonConfig(localDir, script)
				cfg.Name = name
			} else {
				// No local checkout entrypoint: fall back to a console script on PATH.
				found := findOnPath(candidates...)
				if found != "" {
					cfg.Command = found
				} else {
					cfg.Command = "sin-" + name
				}
			}
		} else {
			found := findOnPath(candidates...)
			if found != "" {
				cfg.Command = found
			} else {
				cfg.Command = "sin-" + name
			}
		}
		return cfg
	}
	// bundledPython returns a ServerConfig for an MCP server shipped in the
	// SIN-Code Python companion package. Bundled modules never depend on a
	// separately cloned skill repository or a PATH shim.
	bundledPython := func(name, module string) ServerConfig {
		return ServerConfig{
			Name:      name,
			Transport: "stdio",
			Command:   "python3",
			Args:      []string{"-m", module},
		}
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
	// Build the server list. Some entries are conditional on the binary
	// existing — this avoids "executable not found" and EOF errors for
	// servers whose binaries are not yet built or whose Python modules
	// are not installed.
	servers := []ServerConfig{
		// web_search_bundle is the Go-native successor to SIN-Code-Websearch-Skill.
		goNative("web_search_bundle", "sin-websearch", "serve"),
		py("SIN-Code-Scheduler-Skill"),
		py("SIN-Code-Goal-Mode-Skill"),
		py("SIN-Code-Grill-Me-Skill"),
		bundledPython("marketplace", "sin_code_bundle.tools.marketplace.server"),
		py("SIN-Code-Context-Bridge-Skill"),
		py("SIN-Code-Honcho-Rollback-Skill"),
		py("SIN-Code-Frontend-Design-Skill"),
		bundledPython("mcpbuilder", "sin_code_bundle.tools.mcp_server_builder.mcp_server"),
		py("SIN-Browser-Tools"),
		simoneConfig(skillsDir),
		symfonyLensConfig(skillsDir),

		// v3.22.0: SIN-Analyse-Suite — multimodal preprocessing (image, video, PDF, logs, data, audio)
		goNative("SIN-Analyse-Suite", "sin-analyse", "serve"),

		// v3.27.0: vibe-notion — Bridged-External MCP wrapper around the
		// vibe-notion npm CLI (full Notion access: pages, databases, blocks,
		// comments, users, workspaces). Act-as-user via token_v2 from browser
		// session, or bot mode via NOTION_TOKEN. 17 tools (10 read, 6 write,
		// 1 raw). Read tools auto-allowed, write tools gated (M4).
		notionConfig(skillsDir),
	}

	// native_browser (issue #382): the actual implementation runs in-process
	// behind the Driver seam. The optional sin-native-browser binary is a
	// future stdio shim — only register if the binary exists (local or PATH)
	// so the MCP client doesn't attempt to connect to a non-existent process.
	nbCfg := goNative("native_browser", "sin-native-browser", "serve")
	if binaryAvailable(skillsDir, "native_browser", "sin-native-browser") {
		servers = append(servers, nbCfg)
	}

	// autodev (Bridged-External): only register if the autodev-mcp binary is
	// on PATH and the autodev Python module is importable. The binary is a
	// console script that crashes with ModuleNotFoundError when the package
	// is not installed — checking PATH alone is insufficient because the
	// script exists even when the module doesn't.
	if autodevAvailable() {
		servers = append(servers, ServerConfig{Name: "autodev", Transport: "stdio", Command: "autodev-mcp"})
	}

	// youtube-for-ai-agents — Node.js MCP server, 9 tools (search, transcript,
	// video info, channel videos/info, playlist, download, clip, highlight reel).
	// No YouTube Data API key needed — uses youtubei.js InnerTube client.
	// Optional cookie login for age-restricted/personalized content.
	// Repo: https://github.com/JCodesMore/youtube-for-ai-agents
	servers = append(servers, ServerConfig{Name: "youtube", Transport: "stdio", Command: "node", Args: []string{youtubeMCPPath()}})

	return servers
}

func shortName(repo string) string {
	m := map[string]string{
		"web_search_bundle":              "websearch",
		"sin-analyse-suite":              "analyse",
		"SIN-Analyse-Suite":              "analyse",
		"native_browser":                 "native_browser",
		"SIN-Code-Scheduler-Skill":       "scheduler",
		"SIN-Code-Goal-Mode-Skill":       "goalmode",
		"SIN-Code-Grill-Me-Skill":        "grillme",
		"SIN-Code-Doc-Coauthoring-Skill": "codocs",
		"SIN-Code-Context-Bridge-Skill":  "contextbridge",
		"SIN-Code-Honcho-Rollback-Skill": "honcho",
		"SIN-Code-Frontend-Design-Skill": "frontend",
		"SIN-Browser-Tools":              "browser",
		"Simone-MCP":                     "simone",
		"SIN-Code-Symfony-Lens":          "symfonylens",
		"youtube":                        "youtube",
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

// youtubeMCPPath resolves the path to the youtube-for-ai-agents MCP server.
// Checks (in order): $SIN_YOUTUBE_MCP_PATH env, ~/dev/youtube-for-ai-agents/dist/index.js,
// and falls back to npx execution via "npx".
func youtubeMCPPath() string {
	if p := os.Getenv("SIN_YOUTUBE_MCP_PATH"); p != "" {
		return p
	}
	home, err := userHomeDirHook()
	if err == nil && home != "" {
		candidate := filepath.Join(home, "dev", "youtube-for-ai-agents", "dist", "index.js")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "dist/index.js"
}

// binaryAvailable checks whether a Go-native skill binary exists either in
// the local skills directory or on PATH. Returns false when neither is
// present, so the caller can skip registering a stdio server that would
// immediately fail with "executable file not found".
func binaryAvailable(skillsDir, repo, binary string) bool {
	if skillsDir != "" {
		localBin := filepath.Join(skillsDir, repo, binary)
		if _, err := os.Stat(localBin); err == nil {
			return true
		}
	}
	if p, err := lookPathHook(binary); err == nil && p != "" {
		return true
	}
	return false
}

// autodevAvailable checks whether the autodev-mcp binary is on PATH AND the
// autodev Python module is importable. The console script autodev-mcp is
// installed by autodev-cli but crashes with ModuleNotFoundError when the
// package is missing or the wrong version is installed. We probe by running
// `python3 -c "import autodev.cli_mcp"` with a short timeout.
func autodevAvailable() bool {
	if _, err := lookPathHook("autodev-mcp"); err != nil {
		return false
	}
	cmd := exec.Command("python3", "-c", "import autodev.cli_mcp")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}
