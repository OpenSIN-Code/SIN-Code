// SPDX-License-Identifier: MIT
// Purpose: `sin-code dox` — CLI binding for the agent0ai/dox (MIT)
// self-maintaining AGENTS.md hierarchy protocol. Provides init / new /
// check / tree subcommands, each backed by cmd/sin-code/internal/dox.
// Docs: dox.doc.md
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dox"
	"github.com/spf13/cobra"
)

// NewDoxCmd builds the `dox` cobra subcommand. Pattern matches
// NewSuperpowersCmd: returns *cobra.Command with four subcommands
// (init, new, check, tree) attached.
func NewDoxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dox",
		Short: "Self-maintaining AGENTS.md hierarchy (agent0ai/dox protocol)",
		Long: `sin-code dox integrates the agent0ai/dox (MIT) self-maintaining
AGENTS.md hierarchy protocol. It uses marker-based injection
(<!-- SIN-Code dox:begin/end -->) that coexists with the
SIN-Code superpowers block in the same AGENTS.md, validates the
tree for broken links and orphan nodes, and can scaffold new child
nodes with automatic parent-INDEX registration.

All subcommands are local-only and safe to call offline.`,
	}

	var (
		jsonOut bool
		force   bool
	)

	// ── init ─────────────────────────────────────────────────────────
	initCmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Create a root AGENTS.md and inject the dox block",
		Long: `init scaffolds a root AGENTS.md at the given path (default: current
directory) and injects the dox-managed block. The block is delimited by
<!-- SIN-Code dox:begin --> and <!-- SIN-Code dox:end -->, so it can
coexist with other managed blocks (e.g. the SIN-Code superpowers block)
in the same file.`,
		Args: cobra.MaximumNArgs(1),
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
			agentsPath := filepath.Join(abs, dox.AgentsFileName)
			if _, err := os.Stat(agentsPath); err != nil && !force {
				// Seed a minimal root AGENTS.md.
				seed := "---\n" +
					"title: " + filepath.Base(abs) + "\n" +
					"---\n\n" +
					"# " + filepath.Base(abs) + "\n\n" +
					"Root of the dox-managed AGENTS.md tree.\n"
				if err := os.WriteFile(agentsPath, []byte(seed), 0o644); err != nil {
					return err
				}
			}
			body := "## Dox-managed regions\n\n" +
				"This block is owned by `sin-code dox`. Do not edit by hand.\n" +
				"Re-run `sin-code dox init` to refresh.\n"
			if err := dox.InjectRoot(agentsPath, body); err != nil {
				return err
			}
			if jsonOut {
				out := map[string]any{
					"agents_path": agentsPath,
					"marker":      dox.BeginMarker,
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			fmt.Printf("initialized %s\n", agentsPath)
			return nil
		},
	}
	initCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	initCmd.Flags().BoolVar(&force, "force", false, "overwrite an existing root AGENTS.md")

	// ── new ──────────────────────────────────────────────────────────
	newCmd := &cobra.Command{
		Use:   "new <name> [--title <title>]",
		Short: "Scaffold a new child node under the given parent",
		Long: `new creates a new child directory with an INDEX.md (or AGENTS.md at
the root) and registers the child in the parent's index. The child is
immediately discoverable by ` + "`sin-code dox check`" + `.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			parent, _ := cmd.Flags().GetString("parent")
			title, _ := cmd.Flags().GetString("title")
			if parent == "" {
				parent = "."
			}
			abs, err := filepath.Abs(parent)
			if err != nil {
				return err
			}
			child, err := dox.Scaffold(abs, name, title)
			if err != nil {
				return err
			}
			if jsonOut {
				out := map[string]any{
					"path":  child,
					"name":  name,
					"title": title,
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			fmt.Printf("scaffolded %s\n", child)
			return nil
		},
	}
	newCmd.Flags().String("parent", ".", "parent directory (defaults to current)")
	newCmd.Flags().String("title", "", "human title for the new node (defaults to name)")
	newCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	// ── check ────────────────────────────────────────────────────────
	checkCmd := &cobra.Command{
		Use:   "check [root]",
		Short: "Validate the dox tree (broken links, orphans, TODOs, missing index)",
		Long: `check walks the tree starting at <root> (default: current directory)
and reports every structural problem. Exits non-zero if any error-level
finding is reported; warn-level findings (e.g. TODO sentinels) do not
fail the check.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			findings, err := dox.Check(root)
			if err != nil {
				return err
			}
			// Stable order so JSON output is diff-friendly.
			sortFindings(findings)
			if jsonOut {
				out := map[string]any{
					"findings": findings,
					"healthy":  !hasErrors(findings),
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					return err
				}
			} else {
				if len(findings) == 0 {
					fmt.Println("healthy: no findings")
				} else {
					for _, f := range findings {
						tag := "WARN"
						if f.Severity == "error" {
							tag = "ERR "
						}
						fmt.Printf("%s %-12s %s — %s\n", tag, f.Kind, f.Path, f.Message)
					}
				}
			}
			if hasErrors(findings) {
				return fmt.Errorf("dox check: %d error(s) found", countErrors(findings))
			}
			return nil
		},
	}
	checkCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	// ── tree ─────────────────────────────────────────────────────────
	treeCmd := &cobra.Command{
		Use:   "tree [root]",
		Short: "Print a human-readable tree of the dox hierarchy",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			out, err := dox.RenderTree(root)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]string{"tree": out})
			}
			fmt.Print(out)
			return nil
		},
	}
	treeCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON (returns tree as a single string field)")

	cmd.AddCommand(initCmd, newCmd, checkCmd, treeCmd)
	return cmd
}

func sortFindings(fs []dox.Finding) {
	// stable-ish: by severity (error first), then path, then kind.
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0; j-- {
			if lessFinding(fs[j], fs[j-1]) {
				fs[j-1], fs[j] = fs[j], fs[j-1]
			} else {
				break
			}
		}
	}
}

func lessFinding(a, b dox.Finding) bool {
	if a.Severity != b.Severity {
		// "error" sorts before "warn"
		return a.Severity == "error"
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.Kind < b.Kind
}

func hasErrors(fs []dox.Finding) bool {
	for _, f := range fs {
		if f.Severity == "error" {
			return true
		}
	}
	return false
}

func countErrors(fs []dox.Finding) int {
	n := 0
	for _, f := range fs {
		if f.Severity == "error" {
			n++
		}
	}
	return n
}
