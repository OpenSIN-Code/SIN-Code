// SPDX-License-Identifier: MIT
// Purpose: skill lifecycle manager — install ecosystem skills from the
// OpenSIN-Code org (git clone/pull), verify their MCP entrypoints, and
// keep SIN_SKILLS_DIR in sync with the registry. Closes the gap between
// "registered in registry.go" and "actually runnable on this machine".
package skillmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Package-level hooks for testing without real git/python/go/network calls.
// Production defaults are the standard library functions.
var (
	_osStat             = os.Stat
	_osMkdirAll         = os.MkdirAll
	_execCommandContext = exec.CommandContext
	_filepathGlob       = filepath.Glob
	_execLookPath       = exec.LookPath
)

const orgURL = "https://github.com/OpenSIN-Code/"

// SkillInfo is the canonical metadata for an ecosystem skill. It augments the
// shortname->repo mapping with deprecation state so the CLI can skip stale
// skills during `install all` while still surfacing them in `skill status`.
type SkillInfo struct {
	Name             string `json:"name"`
	Repo             string `json:"repo"`
	Deprecated       bool   `json:"deprecated,omitempty"`
	DeprecatedReason string `json:"deprecated_reason,omitempty"`
	SkipInInstallAll bool   `json:"skip_in_install_all,omitempty"`
}

type SkillStatus struct {
	Name             string `json:"name"`
	Repo             string `json:"repo"`
	Installed        bool   `json:"installed"`
	Runnable         bool   `json:"runnable"`
	Detail           string `json:"detail,omitempty"`
	Deprecated       bool   `json:"deprecated,omitempty"`
	DeprecatedReason string `json:"deprecated_reason,omitempty"`
}

// SkillsDir resolves the local skills checkout directory.
func SkillsDir() string {
	if d := os.Getenv("SIN_SKILLS_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "sin-code", "skills")
}

// canonicalBinary maps a short skill name to the upstream console-script
// binary that exposes the MCP server. Mirrors mcpclient.canonicalBinary.
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

// findSkillBinary returns the first candidate found on PATH, or "".
func findSkillBinary(name string) string {
	candidates := []string{
		canonicalBinary(name) + "-mcp",
		"sin-" + name + "-mcp",
		canonicalBinary(name),
		"sin-" + name,
		name,
	}
	for _, c := range candidates {
		if p, err := _execLookPath(c); err == nil {
			return p
		}
	}
	return ""
}

// findPythonCliEntrypoint searches a skill checkout for a Python CLI wrapper
// script that exposes the MCP server via a "serve" subcommand (e.g.
// scripts/sin_context_bridge.py). If found it returns the absolute script path.
func findPythonCliEntrypoint(dir, name string) string {
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
	for _, c := range candidates {
		if _, err := _osStat(c); err == nil {
			return c
		}
	}
	return ""
}

// findMcpServer searches a skill checkout for the actual MCP server module.
// It checks the canonical root-level script first, then common package
// locations (src, lib, top-level subdirectories). Test trees are skipped.
// The returned path is absolute. If nothing is found it returns "".
func findMcpServer(dir string) string {
	root := filepath.Join(dir, "mcp_server.py")
	if _, err := _osStat(root); err == nil {
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
		if matches, _ := _filepathGlob(pat); len(matches) > 0 {
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

// KnownSkillsInfo returns the canonical metadata for every ecosystem skill.
// It is the single source of truth for the registry; KnownSkills() provides a
// backward-compatible shortname->repo map.
func KnownSkillsInfo() []SkillInfo {
	return []SkillInfo{
		{Name: "websearch", Repo: "web_search_bundle"},
		{Name: "scheduler", Repo: "SIN-Code-Scheduler-Skill"},
		{Name: "goalmode", Repo: "SIN-Code-Goal-Mode-Skill"},
		{Name: "grillme", Repo: "SIN-Code-Grill-Me-Skill"},
		{Name: "marketplace", Repo: "SIN-Code-Marketplace-Skill"},
		{Name: "codocs", Repo: "SIN-Code-Doc-Coauthoring-Skill"},
		{Name: "contextbridge", Repo: "SIN-Code-Context-Bridge-Skill"},
		{Name: "honcho", Repo: "SIN-Code-Honcho-Rollback-Skill"},
		{Name: "frontend", Repo: "SIN-Code-Frontend-Design-Skill"},
		{Name: "mcpbuilder", Repo: "SIN-Code-MCP-Server-Builder-Skill"},
		{Name: "browser", Repo: "SIN-Browser-Tools"},
		{Name: "simone", Repo: "Simone-MCP"},
		{Name: "symfonylens", Repo: "SIN-Code-Symfony-Lens"},
		// v3.22.0: SIN-Analyse-Suite — Go-native multimodal preprocessing.
		{Name: "analyse", Repo: "SIN-Analyse-Suite"},

		// Shop / commerce (issue #142 fusion). The bundled skills
		// document the canonical implementation; the upstream repos
		// below are no longer maintained and are deprecated.
		{
			Name:             "shop-cj-dropshipping",
			Repo:             "cj-dropshipping-skill",
			Deprecated:       true,
			DeprecatedReason: "upstream repo SIN-Shop-Center/cj-dropshipping-skill is not maintained and no runnable MCP entrypoint exists",
			SkipInInstallAll: true,
		},
		{
			Name:             "shop-stripe",
			Repo:             "SIN-Stripe-Bundle",
			Deprecated:       true,
			DeprecatedReason: "upstream repo SIN-Shop-Center/SIN-Stripe-Bundle is not maintained and no runnable MCP entrypoint exists",
			SkipInInstallAll: true,
		},
		{
			Name:             "shop-tiktok",
			Repo:             "SIN-eCommerce-Scraper-Bundle",
			Deprecated:       true,
			DeprecatedReason: "upstream repo SIN-Shop-Center/SIN-eCommerce-Scraper-Bundle is not maintained and no runnable MCP entrypoint exists",
			SkipInInstallAll: true,
		},
	}
}

// KnownSkills maps server names to their org repos — MUST stay in sync
// with mcpclient.DefaultServers (ecosystem-sync CI enforces it).
func KnownSkills() map[string]string {
	info := KnownSkillsInfo()
	m := make(map[string]string, len(info))
	for _, i := range info {
		m[i.Name] = i.Repo
	}
	return m
}

// LookupSkillInfo returns the canonical SkillInfo for a known ecosystem skill,
// or nil if the name is not registered.
func LookupSkillInfo(name string) *SkillInfo {
	for _, i := range KnownSkillsInfo() {
		if i.Name == name {
			// Return a copy to prevent callers from mutating the registry.
			cp := i
			return &cp
		}
	}
	return nil
}

// InstallAll installs every non-deprecated ecosystem skill. It is the
// implementation of `sin-code skill install all`. Deprecated skills are
// skipped (but still reported as Installed=false with their deprecation reason
// in Detail) so the batch command never fails because of stale repos.
func InstallAll(ctx context.Context) ([]SkillStatus, error) {
	var out []SkillStatus
	failed := 0
	for _, info := range KnownSkillsInfo() {
		if info.SkipInInstallAll {
			out = append(out, SkillStatus{
				Name:             info.Name,
				Repo:             info.Repo,
				Installed:        false,
				Runnable:         false,
				Detail:           "deprecated: " + info.DeprecatedReason,
				Deprecated:       info.Deprecated,
				DeprecatedReason: info.DeprecatedReason,
			})
			continue
		}
		st, err := Install(ctx, info.Name)
		if err != nil {
			failed++
			out = append(out, SkillStatus{
				Name:      info.Name,
				Repo:      info.Repo,
				Installed: false,
				Runnable:  false,
				Detail:    err.Error(),
			})
			continue
		}
		out = append(out, *st)
	}
	if failed > 0 {
		return out, fmt.Errorf("%d skill(s) failed to install", failed)
	}
	return out, nil
}

// Install clones (or pulls) a skill repo and verifies its entrypoint.
func Install(ctx context.Context, name string) (*SkillStatus, error) {
	info := LookupSkillInfo(name)
	if info == nil {
		return nil, fmt.Errorf("unknown skill %q (see `sin-code skill list`)", name)
	}
	dir := filepath.Join(SkillsDir(), info.Repo)
	st := &SkillStatus{
		Name:             name,
		Repo:             info.Repo,
		Deprecated:       info.Deprecated,
		DeprecatedReason: info.DeprecatedReason,
	}

	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if _, err := _osStat(filepath.Join(dir, ".git")); err == nil {
		cmd := _execCommandContext(cctx, "git", "-C", dir, "pull", "--ff-only", "--quiet")
		if out, err := cmd.CombinedOutput(); err != nil {
			return st, fmt.Errorf("git pull: %w\n%s", err, out)
		}
	} else {
		if err := _osMkdirAll(SkillsDir(), 0o755); err != nil {
			return st, err
		}
		cmd := _execCommandContext(cctx, "git", "clone", "--depth", "1", "--quiet", orgURL+info.Repo, dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return st, fmt.Errorf("git clone: %w\n%s", err, out)
		}
	}
	st.Installed = true
	st.Runnable, st.Detail = verifyEntrypoint(cctx, dir, info.Repo)
	return st, nil
}

// checkSkillStatus returns the install + runnable state for a single skill.
func checkSkillStatus(ctx context.Context, info SkillInfo) SkillStatus {
	st := SkillStatus{
		Name:             info.Name,
		Repo:             info.Repo,
		Deprecated:       info.Deprecated,
		DeprecatedReason: info.DeprecatedReason,
	}
	dir := filepath.Join(SkillsDir(), info.Repo)
	if _, err := _osStat(dir); err == nil {
		st.Installed = true
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		st.Runnable, st.Detail = verifyEntrypoint(cctx, dir, info.Repo)
		cancel()
	} else {
		// Even if the repo is not cloned, a system-installed console script
		// on PATH makes the skill installed and runnable.
		if bin := findSkillBinary(info.Name); bin != "" {
			st.Installed = true
			if info.Name == "honcho" {
				cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				if err := checkHonchoServer(cctx); err != nil {
					st.Runnable = false
					st.Detail = fmt.Sprintf("available on PATH: %s but Honcho server unreachable at %s: %v", bin, honchoServerURL(), err)
				} else {
					st.Runnable = true
					st.Detail = "available on PATH: " + bin
				}
				cancel()
			} else {
				st.Runnable = true
				st.Detail = "available on PATH: " + bin
			}
		}
	}
	return st
}

// Status reports install + runnable state for every known skill.
func Status(ctx context.Context) []SkillStatus {
	var out []SkillStatus
	for _, info := range KnownSkillsInfo() {
		out = append(out, checkSkillStatus(ctx, info))
	}
	return out
}

// Doctor checks every known ecosystem skill and reports why it is not runnable.
// It returns a SkillStatus for each known skill; non-runnable skills have a
// Detail field explaining the failure (not installed, missing entrypoint,
// dependency unreachable, etc.).
func Doctor(ctx context.Context) []SkillStatus {
	var out []SkillStatus
	for _, info := range KnownSkillsInfo() {
		st := checkSkillStatus(ctx, info)
		if !st.Installed && st.Detail == "" {
			st.Detail = "not installed: " + filepath.Join(SkillsDir(), info.Repo)
		}
		out = append(out, st)
	}
	return out
}

// verifyEntrypoint finds and smoke-tests the MCP entrypoint.
func verifyEntrypoint(ctx context.Context, dir, repo string) (bool, string) {
	name := repoNameFromRepo(repo)

	// Simone-MCP exposes the MCP server through src/cli.py (the src/mcp_server.py
	// file is just a re-export of public symbols). Prefer the CLI entrypoint
	// and fall back to the simone-cli console script on PATH.
	if repo == "Simone-MCP" {
		if _, err := _osStat(filepath.Join(dir, "src", "cli.py")); err == nil {
			return true, "simone CLI entrypoint: src/cli.py"
		}
		if bin := findSkillBinary("simone"); bin != "" {
			return true, "available on PATH: " + bin
		}
		return false, "no recognized MCP entrypoint"
	}

	// SIN-Code-Symfony-Lens exposes the MCP server as the symfony_lens.server
	// Python module. Fall back to the symfony-lens console script on PATH.
	if repo == "SIN-Code-Symfony-Lens" {
		if _, err := _osStat(filepath.Join(dir, "symfony_lens", "server.py")); err == nil {
			return true, "python module: symfony_lens.server"
		}
		if bin := findSkillBinary("symfonylens"); bin != "" {
			return true, "available on PATH: " + bin
		}
		return false, "no recognized MCP entrypoint"
	}

	// SIN-Code-Honcho-Rollback-Skill requires the external Honcho server. If we
	// find a local script or a PATH binary, verify the server is reachable before
	// reporting the skill as runnable.
	if repo == "SIN-Code-Honcho-Rollback-Skill" {
		return verifyHonchoEntrypoint(ctx, dir)
	}

	// Python skills that expose the MCP server through a CLI wrapper script
	// (e.g. sin-context-bridge's scripts/sin_context_bridge.py serve) must be
	// detected before the generic module entrypoint, because the module itself
	// may only define the server without running it.
	if cliScript := findPythonCliEntrypoint(dir, name); cliScript != "" {
		cmd := _execCommandContext(ctx, "python3", cliScript, "serve", "--list-tools")
		out, err := cmd.CombinedOutput()
		if err == nil {
			var probe struct {
				Tools []json.RawMessage `json:"tools"`
			}
			if json.Unmarshal(out, &probe) == nil && len(probe.Tools) > 0 {
				return true, fmt.Sprintf("%d tools", len(probe.Tools))
			}
			return true, "python CLI entrypoint responds"
		}
		return true, "python CLI entrypoint: " + cliScript
	}

	// Python MCP server: prefer the canonical root mcp_server.py, then
	// discover the actual module inside the package tree.
	if script := findMcpServer(dir); script != "" {
		if filepath.Base(filepath.Dir(script)) == filepath.Base(dir) {
			// Root-level mcp_server.py: try the non-standard --list-tools probe
			// for extra detail, but do not fail the whole verification if the
			// server does not honour it.
			cmd := _execCommandContext(ctx, "python3", script, "--list-tools")
			out, err := cmd.CombinedOutput()
			if err == nil {
				var probe struct {
					Tools []json.RawMessage `json:"tools"`
				}
				if json.Unmarshal(out, &probe) == nil && len(probe.Tools) > 0 {
					return true, fmt.Sprintf("%d tools", len(probe.Tools))
				}
				return true, "entrypoint responds (tool list format unknown)"
			}
			return true, "root mcp_server.py present"
		}
		return true, "python entrypoint: " + script
	}

	// If the repo has no mcp_server.py module, it may still be runnable via a
	// console script installed on PATH (e.g. sin-goal-mode, sin-marketplace).
	if bin := findSkillBinary(name); bin != "" {
		return true, "available on PATH: " + bin
	}

	// Fallback to a runnable Python module entrypoint.
	if matches, _ := _filepathGlob(filepath.Join(dir, "src", "*", "__main__.py")); len(matches) > 0 {
		sort.Strings(matches)
		return true, "python module entrypoint: " + matches[0]
	}

	if _, err := _osStat(filepath.Join(dir, "package.json")); err == nil {
		return true, "node entrypoint (package.json)"
	}

	// Go-native skill: a pre-built binary in the repo root is enough.
	binary := goBinaryName(repo)
	binPath := filepath.Join(dir, binary)
	if _, err := _osStat(binPath); err == nil {
		return true, "go binary present: " + binary
	}
	if _, err := _osStat(filepath.Join(dir, "go.mod")); err == nil {
		// Go-native skill: build the binary into the repo root so the MCP
		// registry can use the full path (SIN_SKILLS_DIR/<repo>/<binary>).
		cmd := _execCommandContext(ctx, "go", "build", "-o", binary, "./cmd/"+binary)
		cmd.Dir = dir
		if _, err := cmd.CombinedOutput(); err != nil {
			return false, fmt.Sprintf("go entrypoint exists but build failed: %v", err)
		}
		return true, "go entrypoint builds: " + binary
	}
	return false, "no recognized MCP entrypoint"
}

// honchoServerURL returns the configured Honcho server endpoint for the
// SIN-Code-Honcho-Rollback-Skill probe.
func honchoServerURL() string {
	if u := os.Getenv("HONCHO_SERVER_URL"); u != "" {
		return u
	}
	return "http://localhost:8000"
}

// verifyHonchoEntrypoint checks that the Honcho rollback skill has an
// entrypoint (local script or PATH binary) and that the external Honcho server
// is reachable. It returns the runnable flag and a detail string.
func verifyHonchoEntrypoint(ctx context.Context, dir string) (bool, string) {
	var detail string
	if _, err := _osStat(filepath.Join(dir, "scripts", "sin_honcho_rollback.py")); err == nil {
		detail = "python CLI entrypoint: scripts/sin_honcho_rollback.py"
	} else if bin := findSkillBinary("honcho"); bin != "" {
		detail = "available on PATH: " + bin
	} else {
		return false, "no recognized MCP entrypoint"
	}
	if err := checkHonchoServer(ctx); err != nil {
		return false, fmt.Sprintf("%s but Honcho server unreachable at %s: %v", detail, honchoServerURL(), err)
	}
	return true, detail
}

// checkHonchoServer probes whether the external Honcho server is reachable.
// It returns nil when the server responds, otherwise the connection error.
func checkHonchoServer(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, honchoServerURL()+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("honcho server returned %s", resp.Status)
	}
	return nil
}

// repoNameFromRepo returns the short skill name for a repo by stripping the
// org-specific prefix/suffix. This is only used for PATH fallback lookups.
func repoNameFromRepo(repo string) string {
	// The short names are already the keys in KnownSkills; for a single-repo
	// lookup we reverse-engineer the common patterns.
	if repo == "SIN-Analyse-Suite" || repo == "sin-analyse-suite" {
		return "analyse"
	}
	repo = strings.TrimPrefix(repo, "SIN-Code-")
	repo = strings.TrimSuffix(repo, "-Skill")
	repo = strings.TrimSuffix(repo, "-Tools")
	repo = strings.TrimPrefix(repo, "SIN-")
	repo = strings.TrimPrefix(repo, "Simone-")
	repo = strings.ToLower(repo)
	repo = strings.ReplaceAll(repo, "_", "-")
	return repo
}

// goBinaryName maps a Go-native repo to the binary name the MCP registry expects.
func goBinaryName(repo string) string {
	switch repo {
	case "web_search_bundle":
		return "sin-websearch"
	case "sin-analyse-suite", "SIN-Analyse-Suite":
		return "sin-analyse"
	case "native_browser":
		return "sin-native-browser"
	}
	return "sin-" + strings.ReplaceAll(repo, "_", "-")
}
