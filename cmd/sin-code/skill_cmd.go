// SPDX-License-Identifier: MIT
// Purpose: `sin-code skill` — manage ecosystem skill installations AND
// distribute bundled Skill artifacts to external agent families.
//
// Two surfaces share this command tree on purpose:
//
//   - Ecosystem (skillmgr, v3.5.0): clones upstream skill repos such as
//     `SIN-Code-Websearch` and verifies their MCP entrypoints. Triggered by
//     `sin-code skill install <name>` or `sin-code skill install all`.
//
//   - Distribution (skilldist, issue #169): takes a bundled SKILL.md (one of
//     the 34 embedded skills under skills/<cat>-skills/<name>/) and writes
//     it into one of eight supported agent families (Claude Code, Codex,
//     Gemini, opencode, Cursor, Windsurf, Cline, GitHub Copilot) using a
//     marker-fenced block so a re-run replaces the block in place. Triggered
//     by `sin-code skill install <name> --agent <target>` or `--agent all`.
//
// The two surfaces are mutually exclusive: passing `--agent` switches into
// distribution mode; without `--agent` the legacy ecosystem behaviour is
// preserved 1:1 (including the `all` positional shortcut).
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skilldist"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/OpenSIN-Code/SIN-Code/skills"
)

// agentFlagAll is the magic value for `--agent` that means "try every
// registered target". We keep it as a string constant rather than a bool
// so future flags (`--agent <target1>,<target2>`) can extend without a
// breaking change.
const agentFlagAll = "all"

// reservedAgentNames mirrors skilldist.TargetNames() with the addition of
// `all`. Keeping this here lets cobra's `--agent` completion help text
// show the full choice set without needing to import skilldist in two
// places.
func reservedAgentNames() []string {
	out := []string{agentFlagAll}
	out = append(out, skilldist.TargetNames()...)
	return out
}

// bundledSkillFS is overridden by tests so they can point at a TempDir.
//
// We do NOT cache the FS at package level because the underlying fs.FS
// is immutable once constructed; the override pattern keeps test
// isolation trivial.
var bundledSkillFS = func() (fs.FS, error) { return skills.ListFS() }

// resolveHome picks the home directory for install paths. Order of
// precedence: $SIN_CODE_HOME > $HOME > os.UserHomeDir().
func resolveHome() (string, error) {
	if v := os.Getenv("SIN_CODE_HOME"); v != "" {
		return v, nil
	}
	if v := os.Getenv("HOME"); v != "" {
		return v, nil
	}
	return os.UserHomeDir()
}

// extractSkillFromFS reads a single SKILL.md (and optional sidecar files)
// from the bundled skills FS into a real on-disk directory under dstRoot.
// skilldist.FormatDir installs from a `SrcRoot`, so a real path is required
// even though skillsmith would happily read from fs.FS.
func extractSkillFromFS(src fs.FS, dstRoot, skill string) error {
	out := filepath.Join(dstRoot, skill)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	skillMD, err := fs.ReadFile(src, filepath.Join(skill, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("extractSkillFromFS(%q): read SKILL.md: %w", skill, err)
	}
	if err := os.WriteFile(filepath.Join(out, "SKILL.md"), skillMD, 0o644); err != nil {
		return err
	}
	for _, sub := range []string{"context", "frameworks", "tasks", "templates"} {
		entries, err := fs.ReadDir(src, filepath.Join(skill, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			data, err := fs.ReadFile(src, filepath.Join(skill, sub, e.Name()))
			if err != nil {
				return err
			}
			subDir := filepath.Join(out, sub)
			if err := os.MkdirAll(subDir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(subDir, e.Name()), data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// readSkillBody returns the SKILL.md body ready for marker-fence embedding.
// The YAML frontmatter is stripped and the body is LF-normalised so the
// host agent sees a portable representation.
func readSkillBody(src fs.FS, skill string) (string, error) {
	raw, err := fs.ReadFile(src, filepath.Join(skill, "SKILL.md"))
	if err != nil {
		return "", fmt.Errorf("readSkillBody(%q): %w", skill, err)
	}
	body := skilldist.StripFrontmatter(string(raw))
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("readSkillBody(%q): empty body after stripping frontmatter", skill)
	}
	return body, nil
}

// resolveAgentFlag walks the agent flag through the precedence rules and
// returns either the literal `all` or a Target from skilldist.Targets.
// Any other value (including the empty string) is rejected with a
// cobra-friendly error message.
func resolveAgentFlag(agentFlag string) ([]string, error) {
	if agentFlag == "" || agentFlag == agentFlagAll {
		return skilldist.TargetNames(), nil
	}
	if _, ok := skilldist.Targets[agentFlag]; !ok {
		return nil, fmt.Errorf("unknown agent %q (supported: %s, %s)",
			agentFlag, agentFlagAll, strings.Join(skilldist.TargetNames(), ", "))
	}
	return []string{agentFlag}, nil
}

// NewSkillCmd builds the `sin-code skill` command tree.
//
// Two helper commands shadow each other but coexist:
//
//	`sin-code skill status`           ecosystem-installer state (legacy).
//	`sin-code skill list [--agent X]` the new --installed matrix view.
//
// Both are useful: status talks to the upstream repos, list talks to
// the on-disk install of bundled skills. They use different storage and
// must not be merged.
func NewSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Install and manage ecosystem MCP skills",
		Long: `The skill subcommand has two surfaces.

Ecosystem install (default, v3.5.0):
  sin-code skill install <name>... | all
  sin-code skill status

Bundle distribution (issue #169, --agent flag):
  sin-code skill install <name> --agent <target>
  sin-code skill install <name> --agent all
  sin-code skill uninstall <name> --agent <target>
  sin-code skill list [--installed] [--agent <target>]

Distribution uses marker-fenced installs so a re-run is idempotent: the
block between <!-- SIN-CODE-SKILL-START: <name> --> and
<!-- SIN-CODE-SKILL-END:   <name> --> is replaced in place.`,
	}

	var jsonOut bool
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show install + runnable state of all known ecosystem skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			sts := skillmgr.Status(cmd.Context())
			sort.Slice(sts, func(i, j int) bool { return sts[i].Name < sts[j].Name })
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(sts)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-15s %-10s %-9s %s\n", "SKILL", "INSTALLED", "RUNNABLE", "DETAIL")
			for _, s := range sts {
				fmt.Fprintf(cmd.OutOrStdout(), "%-15s %-10v %-9v %s\n", s.Name, s.Installed, s.Runnable, s.Detail)
			}
			return nil
		},
	}
	statusCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	var agentFlag string
	installCmd := &cobra.Command{
		Use:   "install <name>... | all",
		Short: "Clone/update skill repos OR --agent <target> distribute a bundled skill",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Distribution mode: --agent was set.
			if agentFlag != "" || os.Getenv("SIN_CODE_AGENT") != "" {
				if agentFlag == "" {
					agentFlag = os.Getenv("SIN_CODE_AGENT")
				}
				return runSkillInstallDistribute(cmd, args, agentFlag)
			}
			// Legacy ecosystem install mode.
			return runSkillInstallEcosystem(cmd, args)
		},
	}
	installCmd.Flags().StringVar(&agentFlag, "agent", "",
		"target agent (claude-code|codex|gemini|opencode|cursor|windsurf|cline|copilot|all); "+
			"or $SIN_CODE_AGENT. Empty = ecosystem install mode.")

	// list shows the install status of bundled skills against each
	// registered agent family.
	var installedOnly bool
	listCmd := &cobra.Command{
		Use:   "list [--installed] [--agent <target>]",
		Short: "List bundled skills and their distribution status per agent",
		Long: `Without flags, lists every bundled skill and reports which
agent families currently have it installed. --installed filters out
bundled skills with zero installs across all targets. --agent <target>
filters the report to one target (use "all" for the unfiltered view).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillList(cmd, agentFlag, installedOnly, jsonOut)
		},
	}
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	listCmd.Flags().BoolVar(&installedOnly, "installed", false,
		"only show bundled skills with at least one installed copy")
	listCmd.Flags().StringVar(&agentFlag, "agent", "",
		"target agent (default: all)")

	uninstallCmd := &cobra.Command{
		Use:   "uninstall <name> --agent <target>",
		Short: "Remove a bundled skill from a target agent family",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if agentFlag == "" {
				agentFlag = os.Getenv("SIN_CODE_AGENT")
			}
			if agentFlag == "" {
				return fmt.Errorf("--agent <target> is required for uninstall (also accepts $SIN_CODE_AGENT)")
			}
			return runSkillUninstall(cmd, args, agentFlag)
		},
	}
	uninstallCmd.Flags().StringVar(&agentFlag, "agent", "",
		"target agent (required)")

	cmd.AddCommand(statusCmd, installCmd, listCmd, uninstallCmd)
	return cmd
}

// runSkillInstallEcosystem is the pre-existing v3.5.0 behaviour: clone
// (or pull) an upstream skill repo and verify its MCP entrypoint.
//
// Kept as a helper so the parent install command stays readable.
func runSkillInstallEcosystem(cmd *cobra.Command, args []string) error {
	names := args
	if len(args) == 1 && args[0] == "all" {
		names = names[:0]
		for n := range skillmgr.KnownSkills() {
			names = append(names, n)
		}
		sort.Strings(names)
	}
	failed := 0
	for _, n := range names {
		st, err := skillmgr.Install(cmd.Context(), n)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", n, err)
			failed++
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "OK   %s (runnable=%v, %s)\n", st.Name, st.Runnable, st.Detail)
	}
	if failed > 0 {
		return fmt.Errorf("%d skill(s) failed to install", failed)
	}
	return nil
}

// runSkillInstallDistribute writes each bundled skill name into the
// resolved set of agent families via skilldist.Install.
//
// The body is sourced from the embedded skills.SkillsFS via a one-shot
// extraction to t.TempDir()-style scratch. Re-running is idempotent — the
// marker-fence replacement guarantees no duplicated blocks.
func runSkillInstallDistribute(cmd *cobra.Command, args []string, agentFlag string) error {
	targets, err := resolveAgentFlag(agentFlag)
	if err != nil {
		return err
	}
	home, err := resolveHome()
	if err != nil {
		return err
	}
	src, err := bundledSkillFS()
	if err != nil {
		return fmt.Errorf("open bundled skills FS: %w", err)
	}
	scratch, err := os.MkdirTemp("", "sin-code-skilldist-")
	if err != nil {
		return fmt.Errorf("create scratch dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	failed := 0
	for _, skill := range args {
		body, err := readSkillBody(src, skill)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", skill, err)
			failed++
			continue
		}
		if err := extractSkillFromFS(src, scratch, skill); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: extract: %v\n", skill, err)
			failed++
			continue
		}
		for _, agentName := range targets {
			tgt := skilldist.Targets[agentName]
			err := skilldist.Install(skill, tgt, skilldist.InstallOptions{
				SrcRoot: scratch,
				Home:    home,
				Body:    body,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL %s → %s: %v\n", skill, tgt.DisplayName, err)
				failed++
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK   %s → %s\n", skill, tgt.DisplayName)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d install(s) failed", failed)
	}
	return nil
}

// runSkillList renders the install matrix. The default view is every
// bundled skill × every agent family; --installed filters out rows with
// zero installs; --agent filters the agent columns to one target.
type listRow struct {
	Skill     string          `json:"skill"`
	Lifecycle string          `json:"lifecycle,omitempty"` // issue #139
	Targets   map[string]bool `json:"targets"`
	HasAny    bool            `json:"has_any"`
}

func runSkillList(cmd *cobra.Command, agentFlag string, installedOnly, jsonOut bool) error {
	home, err := resolveHome()
	if err != nil {
		return err
	}
	src, err := bundledSkillFS()
	if err != nil {
		return fmt.Errorf("open bundled skills FS: %w", err)
	}

	targets := skilldist.TargetNames()
	if agentFlag != "" && agentFlag != agentFlagAll {
		if _, ok := skilldist.Targets[agentFlag]; !ok {
			return fmt.Errorf("unknown agent %q (supported: all, %s)",
				agentFlag, strings.Join(skilldist.TargetNames(), ", "))
		}
		targets = []string{agentFlag}
	}

	skillNames := bundledSkillNames(src)
	rows := make([]listRow, 0, len(skillNames))
	for _, sk := range skillNames {
		row := listRow{Skill: sk, Targets: make(map[string]bool, len(targets))}
		// Read the lifecycle from the embedded SKILL.md frontmatter
		// (issue #139). Bundled skills are content-addressed; the
		// lifecycle field is part of the manifest. If missing
		// (legacy skills before the migration), the field is
		// empty and the CLI shows a `[unknown]` marker.
		if sm, err := fs.ReadFile(src, sk+"/SKILL.md"); err == nil {
			row.Lifecycle = parseLifecycleFromFrontmatter(string(sm))
		}
		for _, ag := range targets {
			tgt := skilldist.Targets[ag]
			ok, err := skilldist.IsInstalled(tgt, sk, home)
			if err != nil {
				row.Targets[ag] = false
				continue
			}
			row.Targets[ag] = ok
			if ok {
				row.HasAny = true
			}
		}
		if installedOnly && !row.HasAny {
			continue
		}
		rows = append(rows, row)
	}

	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	// Pretty table.
	header := fmt.Sprintf("%-32s %-12s", "SKILL", "LIFECYCLE")
	for _, ag := range targets {
		tgt := skilldist.Targets[ag]
		header += fmt.Sprintf(" %-12s", tgt.DisplayName)
	}
	fmt.Fprintln(cmd.OutOrStdout(), header)
	for _, r := range rows {
		lc := r.Lifecycle
		if lc == "" {
			lc = "unknown"
		}
		row := fmt.Sprintf("%-32s [%-8s]", r.Skill, lc)
		for _, ag := range targets {
			if r.Targets[ag] {
				row += " ✓           "
			} else {
				row += " —           "
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), row)
	}
	return nil
}

// runSkillUninstall reverses a previous --agent install. Each (skill, target)
// pair is removed via skilldist.Uninstall.
func runSkillUninstall(cmd *cobra.Command, args []string, agentFlag string) error {
	targets, err := resolveAgentFlag(agentFlag)
	if err != nil {
		return err
	}
	home, err := resolveHome()
	if err != nil {
		return err
	}
	failed := 0
	for _, skill := range args {
		for _, agentName := range targets {
			tgt := skilldist.Targets[agentName]
			if err := skilldist.Uninstall(tgt, skill, home); err != nil {
				fmt.Fprintf(os.Stderr, "FAIL %s → %s: %v\n", skill, tgt.DisplayName, err)
				failed++
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK   %s ← %s removed\n", skill, tgt.DisplayName)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d uninstall(s) failed", failed)
	}
	return nil
}

// bundledSkillNames walks the flattened skills FS and returns every skill
// directory (leaf containing SKILL.md) in alphabetical order. The flat
// FS exposes each skill at path "<skill>/SKILL.md"; skillsmith would do
// the same lookup but we want discovery without depending on skillsmith's
// walking helper here.
func bundledSkillNames(src fs.FS) []string {
	var out []string
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Confirm the directory has a SKILL.md before listing it.
		if _, err := fs.Stat(src, filepath.Join(e.Name(), "SKILL.md")); err == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// parseLifecycleFromFrontmatter extracts the `lifecycle:` value from
// a SKILL.md's YAML frontmatter. The format is intentionally narrow:
// we do not pull in a yaml dep just for this; a regex is enough.
//
// Returns "" if the field is missing or malformed. The caller treats
// "" as `[unknown]` so the operator notices skills that have not been
// migrated yet (run scripts/sync_lifecycle.py --apply).
func parseLifecycleFromFrontmatter(s string) string {
	const openDelim = "---"
	if !strings.HasPrefix(s, openDelim) {
		return ""
	}
	rest := strings.TrimPrefix(s, openDelim)
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return ""
	}
	fm := rest[:idx]
	// Look for `lifecycle: <value>` (allow leading whitespace).
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "lifecycle:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "lifecycle:"))
			// Strip surrounding quotes if present.
			if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"') {
				val = val[1 : len(val)-1]
			}
			return val
		}
	}
	return ""
}
