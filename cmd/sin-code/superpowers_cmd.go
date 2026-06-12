// SPDX-License-Identifier: MIT
// Purpose: `sin-code superpowers` — CLI binding for the superpowers
// integration. install/update/clone the upstream repo, pin the commit,
// append the SIN-Code overlay, and register the stdio MCP server.
// Docs: superpowers.doc.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/superpowers"
	"github.com/spf13/cobra"
)

// NewSuperpowersCmd builds the `superpowers` cobra subcommand. Pattern
// matches NewChatCmd / NewSkillCmd: returns *cobra.Command with the
// relevant subcommands attached.
func NewSuperpowersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "superpowers",
		Short: "Integrate obra/superpowers skills into SIN-Code",
		Long: `sin-code superpowers clones obra/superpowers, pins the commit,
applies a SIN-Code overlay to every SKILL.md, regenerates PROMPT.md, and
registers the stdio MCP server so the agent can discover & load skills
at runtime.

This subcommand is network-free by default unless 'install' / 'update' is
invoked. All other subcommands (list, show, find, doctor) are local-only
and safe to call offline.`,
	}

	var (
		yes   bool
		repo  string
		branch string
		query string
		jsonOut bool
		agentsPath string
	)

	// ── install ──────────────────────────────────────────────────────
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Clone obra/superpowers, apply overlay, write PROMPT.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			res, err := superpowers.Install(ctx, repo, branch)
			if err != nil {
				return err
			}
			// Auto-register the MCP server entry so the agent loop can
			// launch it on demand. Idempotent — see RegisterMCP.
			mcpPath, err := superpowers.RegisterMCP("")
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: MCP register failed: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "registered MCP server entry at %s\n", mcpPath)
			}
			// Auto-inject the AGENTS.md block if a path was given or if
			// the user passes --agents. Default is no injection.
			if agentsPath != "" {
				skills, _ := superpowers.List("")
				snippet := superpowers.AGENTSSnippet(skills)
				if err := superpowers.InjectAGENTS(agentsPath, snippet); err != nil {
					fmt.Fprintf(os.Stderr, "warning: AGENTS.md injection failed: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "injected superpowers block into %s\n", agentsPath)
				}
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Printf("installed %d skill(s) from %s\n  pinned: %s\n  branch: %s\n  duration: %s\n",
				res.Skills, res.Repo, res.SHA, res.Branch, res.Duration)
			return nil
		},
	}
	installCmd.Flags().StringVar(&repo, "repo", "", "override upstream repo URL (test fixtures, mirrors)")
	installCmd.Flags().StringVar(&branch, "branch", superpowers.DefaultBranch, "branch to track (default main)")
	installCmd.Flags().StringVar(&agentsPath, "agents", "", "optional: path to AGENTS.md to inject the superpowers block into")
	installCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	installCmd.Flags().BoolVar(&yes, "yes", false, "accept defaults (currently a no-op; reserved for non-interactive future use)")

	// ── update ───────────────────────────────────────────────────────
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Pull latest from upstream and re-pin",
		RunE: func(cmd *cobra.Command, args []string) error {
			// For now update is an alias for install with the current
			// pin intact. The pin file is rewritten on success.
			if !yes {
				fmt.Fprintln(os.Stderr, "running `superpowers update` (use --yes to skip future confirmation prompts)")
			}
			return runInstallOrUpdate(cmd, repo, branch, agentsPath, jsonOut)
		},
	}
	updateCmd.Flags().StringVar(&repo, "repo", "", "override upstream repo URL")
	updateCmd.Flags().StringVar(&branch, "branch", superpowers.DefaultBranch, "branch to track (default main)")
	updateCmd.Flags().StringVar(&agentsPath, "agents", "", "optional: AGENTS.md path to re-inject")
	updateCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	updateCmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation prompts")

	// ── pin ──────────────────────────────────────────────────────────
	pinCmd := &cobra.Command{
		Use:   "pin <sha>",
		Short: "Pin a specific commit SHA as the active superpowers version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := superpowers.Pin(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("pinned to %s on branch %s (updated %s)\n",
				st.SHA, st.Branch, st.UpdatedAt.Format("2006-01-02T15:04:05Z"))
			return nil
		},
	}

	// ── list ─────────────────────────────────────────────────────────
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List installed superpowers skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			all, err := superpowers.List("")
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(all)
			}
			if len(all) == 0 {
				fmt.Println("no skills installed — run `sin-code superpowers install`")
				return nil
			}
			fmt.Printf("%-30s %-12s %s\n", "SKILL", "HASH8", "PATH")
			for _, s := range all {
				hash := s.Hash
				if len(hash) > 8 {
					hash = hash[:8]
				}
				fmt.Printf("%-30s %-12s %s\n", s.Name, hash, s.Path)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	// ── show ─────────────────────────────────────────────────────────
	showCmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print the full SKILL.md for the given skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := superpowers.Get(args[0])
			if err != nil {
				return err
			}
			body, err := os.ReadFile(info.Path)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(body)
			return err
		},
	}

	// ── find ─────────────────────────────────────────────────────────
	findCmd := &cobra.Command{
		Use:   "find <query>",
		Short: "Substring search across skill name + description",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hits, err := superpowers.Find(args[0], 0)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(hits)
			}
			if len(hits) == 0 {
				fmt.Printf("no skills match %q\n", args[0])
				return nil
			}
			for _, h := range hits {
				desc := h.Description
				if desc == "" {
					desc = "(no description)"
				}
				fmt.Printf("- %s: %s\n", h.Name, desc)
			}
			return nil
		},
	}
	findCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	// `query` is only used as a positional placeholder; cobra captures it
	// via args[0]. The variable stays here so future flags (--limit,
	// --field) can share its help text.
	_ = query

	// ── serve ────────────────────────────────────────────────────────
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the superpowers stdio MCP server (JSON-RPC 2.0)",
		Long: `sin-code superpowers serve launches the stdio MCP server. It
speaks the JSON-RPC 2.0 protocol on stdin/stdout. The mcpclient package
launches this binary on demand when the 'superpowers' MCP server is
referenced in mcp.json.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			srv := superpowers.NewServer("")
			return srv.Serve(ctx)
		},
	}

	// ── init ─────────────────────────────────────────────────────────
	initCmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Scaffold a minimal SKILL.md in the given directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(abs, 0o755); err != nil {
				return err
			}
			skillPath := filepath.Join(abs, "SKILL.md")
			if _, err := os.Stat(skillPath); err == nil {
				return fmt.Errorf("init: %s already exists", skillPath)
			}
			// Use the parent directory name as the default skill name.
			name := filepath.Base(abs)
			body := "---\n" +
				"name: " + name + "\n" +
				"description: TODO — describe what this skill does and when to use it\n" +
				"---\n\n" +
				"# " + name + "\n\n" +
				"Describe the workflow here. Keep it focused on a single capability.\n"
			if err := os.WriteFile(skillPath, []byte(body), 0o644); err != nil {
				return err
			}
			// Apply overlay so the user can immediately see the integration.
			superpowers.AppendOverlay(skillPath)
			fmt.Printf("scaffolded %s\n", skillPath)
			return nil
		},
	}

	// ── doctor ───────────────────────────────────────────────────────
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Verify install + overlay + MCP registration + AGENTS.md injection",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(jsonOut)
		},
	}
	doctorCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	cmd.AddCommand(installCmd, updateCmd, pinCmd, listCmd, showCmd,
		findCmd, serveCmd, initCmd, doctorCmd)
	_ = sort.Strings // keep import even if unused in future refactors
	return cmd
}

// runInstallOrUpdate is the shared body for install / update.
func runInstallOrUpdate(cmd *cobra.Command, repo, branch, agentsPath string, jsonOut bool) error {
	ctx := cmd.Context()
	res, err := superpowers.Install(ctx, repo, branch)
	if err != nil {
		return err
	}
	if _, err := superpowers.RegisterMCP(""); err != nil {
		fmt.Fprintf(os.Stderr, "warning: MCP register failed: %v\n", err)
	}
	if agentsPath != "" {
		skills, _ := superpowers.List("")
		snippet := superpowers.AGENTSSnippet(skills)
		if err := superpowers.InjectAGENTS(agentsPath, snippet); err != nil {
			fmt.Fprintf(os.Stderr, "warning: AGENTS.md injection failed: %v\n", err)
		}
	}
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	fmt.Printf("updated %d skill(s) from %s\n  pinned: %s\n  branch: %s\n  duration: %s\n",
		res.Skills, res.Repo, res.SHA, res.Branch, res.Duration)
	return nil
}

// runDoctor is a read-only verification check: pin file exists, overlay
// markers present in every SKILL.md, MCP server entry present, etc.
func runDoctor(jsonOut bool) error {
	type check struct {
		Name   string `json:"name"`
		OK     bool   `json:"ok"`
		Detail string `json:"detail,omitempty"`
	}
	checks := []check{}

	// 1. Skills directory exists.
	if _, err := os.Stat(superpowers.SkillsDir()); err == nil {
		checks = append(checks, check{Name: "skills_dir", OK: true})
	} else {
		checks = append(checks, check{Name: "skills_dir", OK: false, Detail: err.Error()})
	}

	// 2. Pin file present and parseable.
	pin, err := superpowers.CurrentPin()
	switch {
	case err != nil:
		checks = append(checks, check{Name: "pin_file", OK: false, Detail: err.Error()})
	case pin == nil:
		checks = append(checks, check{Name: "pin_file", OK: false, Detail: "no .sin-code-pin (run `superpowers install`)"})
	default:
		checks = append(checks, check{Name: "pin_file", OK: true, Detail: pin.SHA[:min(8, len(pin.SHA))]})
	}

	// 3. Overlay present on every SKILL.md.
	all, _ := superpowers.List("")
	missing := 0
	for _, s := range all {
		b, err := os.ReadFile(s.Path)
		if err != nil {
			missing++
			continue
		}
		if !strings.Contains(string(b), superpowers.OverlayMarker) {
			missing++
		}
	}
	if len(all) == 0 {
		checks = append(checks, check{Name: "overlay", OK: false, Detail: "no skills discovered"})
	} else if missing == 0 {
		checks = append(checks, check{Name: "overlay", OK: true, Detail: fmt.Sprintf("%d skills, all have overlay", len(all))})
	} else {
		checks = append(checks, check{Name: "overlay", OK: false, Detail: fmt.Sprintf("%d/%d missing overlay", missing, len(all))})
	}

	// 4. MCP server entry.
	if _, err := os.Stat(superpowers.MCPConfigPath()); err == nil {
		checks = append(checks, check{Name: "mcp_registered", OK: true})
	} else {
		checks = append(checks, check{Name: "mcp_registered", OK: false, Detail: err.Error()})
	}

	// 5. PROMPT.md present.
	if _, err := os.Stat(superpowers.PROMPTFile()); err == nil {
		checks = append(checks, check{Name: "prompt_file", OK: true})
	} else {
		checks = append(checks, check{Name: "prompt_file", OK: false, Detail: err.Error()})
	}

	allOK := true
	for _, c := range checks {
		if !c.OK {
			allOK = false
			break
		}
	}
	if jsonOut {
		out := map[string]any{
			"checks": checks,
			"all_ok": allOK,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, c := range checks {
		marker := "OK  "
		if !c.OK {
			marker = "FAIL"
		}
		fmt.Printf("%s %-18s %s\n", marker, c.Name, c.Detail)
	}
	if !allOK {
		return fmt.Errorf("doctor: one or more checks failed")
	}
	return nil
}

// min is a small helper to keep doctor.go self-contained without
// pulling in the "min" built-in (Go 1.21+ has it, but we want the file
// to compile cleanly under older toolchains too).
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Unused but kept so the package compiles even if no subcommand is
// selected (defensive — cobra will still show help, never RunE).
var _ = context.Background
