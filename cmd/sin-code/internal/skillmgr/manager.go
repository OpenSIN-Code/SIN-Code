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

type SkillStatus struct {
	Name      string `json:"name"`
	Repo      string `json:"repo"`
	Installed bool   `json:"installed"`
	Runnable  bool   `json:"runnable"`
	Detail    string `json:"detail,omitempty"`
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

// KnownSkills maps server names to their org repos — MUST stay in sync
// with mcpclient.DefaultServers (ecosystem-sync CI enforces it).
func KnownSkills() map[string]string {
	return map[string]string{
		"websearch":     "web_search_bundle",
		"scheduler":     "SIN-Code-Scheduler-Skill",
		"goalmode":      "SIN-Code-Goal-Mode-Skill",
		"grillme":       "SIN-Code-Grill-Me-Skill",
		"marketplace":   "SIN-Code-Marketplace-Skill",
		"codocs":        "SIN-Code-Doc-Coauthoring-Skill",
		"contextbridge": "SIN-Code-Context-Bridge-Skill",
		"honcho":        "SIN-Code-Honcho-Rollback-Skill",
		"frontend":      "SIN-Code-Frontend-Design-Skill",
		"mcpbuilder":    "SIN-Code-MCP-Server-Builder-Skill",
		"browser":       "SIN-Browser-Tools",
		"simone":        "Simone-MCP",
		"symfonylens":   "SIN-Code-Symfony-Lens",
		// Shop / commerce (issue #142 fusion). The bundled skills
		// document the canonical implementation; the source repos
		// listed below are the install targets for
		// `sin-code skill install <name>`. The skill name and
		// repo are 1:1 unless a single repo covers multiple
		// bundled skills (see skillmgr.InstallBatch).
		// v3.22.0: sin-analyse-suite — Go-native multimodal preprocessing.
		"analyse": "sin-analyse-suite",

		"shop-cj-dropshipping": "cj-dropshipping-skill",
		"shop-stripe":          "SIN-Stripe-Bundle",
		"shop-tiktok":          "SIN-eCommerce-Scraper-Bundle",
	}
}

// Install clones (or pulls) a skill repo and verifies its entrypoint.
func Install(ctx context.Context, name string) (*SkillStatus, error) {
	repo, ok := KnownSkills()[name]
	if !ok {
		return nil, fmt.Errorf("unknown skill %q (see `sin-code skill list`)", name)
	}
	dir := filepath.Join(SkillsDir(), repo)
	st := &SkillStatus{Name: name, Repo: repo}

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
		cmd := _execCommandContext(cctx, "git", "clone", "--depth", "1", "--quiet", orgURL+repo, dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return st, fmt.Errorf("git clone: %w\n%s", err, out)
		}
	}
	st.Installed = true
	st.Runnable, st.Detail = verifyEntrypoint(cctx, dir, repo)
	return st, nil
}

// Status reports install + runnable state for every known skill.
func Status(ctx context.Context) []SkillStatus {
	var out []SkillStatus
	for name, repo := range KnownSkills() {
		st := SkillStatus{Name: name, Repo: repo}
		dir := filepath.Join(SkillsDir(), repo)
		if _, err := _osStat(dir); err == nil {
			st.Installed = true
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			st.Runnable, st.Detail = verifyEntrypoint(cctx, dir, repo)
			cancel()
		} else {
			// Even if the repo is not cloned, a system-installed console script
			// on PATH makes the skill runnable.
			if bin := findSkillBinary(name); bin != "" {
				st.Runnable = true
				st.Detail = "available on PATH: " + bin
			}
		}
		out = append(out, st)
	}
	return out
}

// verifyEntrypoint finds and smoke-tests the MCP entrypoint.
func verifyEntrypoint(ctx context.Context, dir, repo string) (bool, string) {
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
	if bin := findSkillBinary(repoNameFromRepo(repo)); bin != "" {
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
	if _, err := _osStat(filepath.Join(dir, "go.mod")); err == nil {
		binary := goBinaryName(repo)
		binPath := filepath.Join(dir, binary)
		// If the binary is already built, skip the expensive rebuild in status.
		if _, err := _osStat(binPath); err == nil {
			return true, "go binary present: " + binary
		}
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

// repoNameFromRepo returns the short skill name for a repo by stripping the
// org-specific prefix/suffix. This is only used for PATH fallback lookups.
func repoNameFromRepo(repo string) string {
	// The short names are already the keys in KnownSkills; for a single-repo
	// lookup we reverse-engineer the common patterns.
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
	case "sin-analyse-suite":
		return "sin-analyse"
	case "native_browser":
		return "sin-native-browser"
	}
	return "sin-" + strings.ReplaceAll(repo, "_", "-")
}
