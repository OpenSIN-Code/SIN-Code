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
	"time"
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
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		cmd := exec.CommandContext(cctx, "git", "-C", dir, "pull", "--ff-only", "--quiet")
		if out, err := cmd.CombinedOutput(); err != nil {
			return st, fmt.Errorf("git pull: %w\n%s", err, out)
		}
	} else {
		if err := os.MkdirAll(SkillsDir(), 0o755); err != nil {
			return st, err
		}
		cmd := exec.CommandContext(cctx, "git", "clone", "--depth", "1", "--quiet", orgURL+repo, dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			return st, fmt.Errorf("git clone: %w\n%s", err, out)
		}
	}
	st.Installed = true
	st.Runnable, st.Detail = verifyEntrypoint(cctx, dir)
	return st, nil
}

// Status reports install + runnable state for every known skill.
func Status(ctx context.Context) []SkillStatus {
	var out []SkillStatus
	for name, repo := range KnownSkills() {
		st := SkillStatus{Name: name, Repo: repo}
		dir := filepath.Join(SkillsDir(), repo)
		if _, err := os.Stat(dir); err == nil {
			st.Installed = true
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			st.Runnable, st.Detail = verifyEntrypoint(cctx, dir)
			cancel()
		}
		out = append(out, st)
	}
	return out
}

// verifyEntrypoint finds and smoke-tests the MCP entrypoint.
func verifyEntrypoint(ctx context.Context, dir string) (bool, string) {
	entry := filepath.Join(dir, "mcp_server.py")
	if _, err := os.Stat(entry); err == nil {
		cmd := exec.CommandContext(ctx, "python3", entry, "--list-tools")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return false, fmt.Sprintf("entrypoint exists but smoke test failed: %v", err)
		}
		var probe struct {
			Tools []json.RawMessage `json:"tools"`
		}
		if json.Unmarshal(out, &probe) == nil && len(probe.Tools) > 0 {
			return true, fmt.Sprintf("%d tools", len(probe.Tools))
		}
		return true, "entrypoint responds (tool list format unknown)"
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "src", "*", "__main__.py")); len(matches) > 0 {
		return true, "python module entrypoint: " + matches[0]
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		return true, "node entrypoint (package.json)"
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		// Go-native skill: build the binary into the repo root so the MCP
		// registry can use the full path (SIN_SKILLS_DIR/<repo>/<binary>).
		cmd := exec.CommandContext(ctx, "go", "build", "-o", "sin-websearch", "./cmd/sin-websearch")
		cmd.Dir = dir
		if _, err := cmd.CombinedOutput(); err != nil {
			return false, fmt.Sprintf("go entrypoint exists but build failed: %v", err)
		}
		return true, "go entrypoint builds"
	}
	return false, "no recognized MCP entrypoint"
}
