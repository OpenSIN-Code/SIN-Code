// SPDX-License-Identifier: MIT
// Purpose: `sin-code profile` — render the single-source-of-truth
// project profile (docs/agent-profiles/sin-profile.md, issue #175)
// into the per-agent mirror files (Claude Code, Codex, opencode,
// Gemini CLI, Cursor, Windsurf, Cline, GitHub Copilot) and verify
// the mirrors stay byte-stable against the source.
//
// Subcommands:
//
//	profile show                    # print the source markdown
//	profile list                    # print the per-agent target table
//	profile render <target|all>     # write one or all mirrors
//	profile render --dry-run        # preview bytes without writing
//	profile verify                  # gate CI on mirror SHA drift
//
// All writers go through `internal/profile`, which keeps the
// renderer pure and the verify-gate reproducible (M3).
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/profile"
)

// NewProfileCmd builds the `profile` cobra subcommand, complete with
// render / show / list / verify sub-actions.
func NewProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Render & verify the single-source-of-truth agent profile (issue #175)",
		Long: `sin-code profile renders docs/agent-profiles/sin-profile.md — the
single-source-of-truth project profile — into the per-agent mirror files
SIN-Code installs into every supported host agent: Claude Code,
opencode, Gemini CLI, Codex, Cursor, Windsurf, Cline, and GitHub
Copilot. Edit the source markdown, run "sin-code profile render all",
and the bytes stable across every host agent.

CI integrations should call "sin-code profile verify" — it refuses to
pass whenever any per-agent mirror drifts off the source.`,
	}

	cmd.AddCommand(newProfileShowCmd())
	cmd.AddCommand(newProfileListCmd())
	cmd.AddCommand(newProfileRenderCmd())
	cmd.AddCommand(newProfileVerifyCmd())

	return cmd
}

// newProfileShowCmd prints the source markdown to stdout.
func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the source profile markdown",
		RunE: func(_ *cobra.Command, _ []string) error {
			base := resolveRepoRoot()
			body, err := profile.LoadSource(base)
			if err != nil {
				return err
			}
			fmt.Print(body)
			return nil
		},
	}
}

// newProfileListCmd prints the per-agent target table.
func newProfileListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every supported host-agent target",
		RunE: func(_ *cobra.Command, _ []string) error {
			tab := profile.ListTable()
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(tab)
			}
			fmt.Printf("%-13s %-13s %-9s %s\n", "NAME", "FORMAT", "PATH-TPL", "INSTALL")
			for _, e := range tab {
				fmt.Printf("%-13s %-13s %-9s %s\n",
					e.Name, e.Format, "<skill>", e.InstallPath)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

// newProfileRenderCmd writes one or all mirrors to disk.
//
//	`render <name>` (Claude Code, codex…) — write a single mirror
//	`render all`                            — write every mirror
//	`render --dry-run <name|all>`           — preview to stdout, no IO
func newProfileRenderCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "render <target|all>",
		Short: "Write one or all per-agent mirrors",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			base := resolveRepoRoot()

			// Dry-run path: render but do not write.
			if dryRun {
				body, err := profile.LoadSource(base)
				if err != nil {
					return err
				}
				if args[0] == "all" {
					return renderDryRunAll(body, base)
				}
				return renderDryRunOne(args[0], body, base)
			}

			body, err := profile.LoadSource(base)
			if err != nil {
				return err
			}

			written, err := profile.WriteSelected(base, body, args[0])
			if err != nil {
				return err
			}
			for _, p := range written {
				fmt.Printf("WROTE %s\n", p)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"preview the rendered bytes to stdout; do not write to disk")
	return cmd
}

// newProfileVerifyCmd is the CI gate. It reads every per-agent mirror
// on disk and refuses to succeed if any of them is missing or drifted
// from the source render. Exits 0 on full match, non-zero with a
// Markdown-table error on drift.
//
//	--json emits a JSON array suitable for CI parsing.
func newProfileVerifyCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify per-agent mirrors match the source SHA (CI gate)",
		RunE: func(_ *cobra.Command, _ []string) error {
			base := resolveRepoRoot()
			body, err := profile.LoadSource(base)
			if err != nil {
				return err
			}
			res, err := profile.Verify(base, body)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(res)
			} else {
				writeVerifyTable(res)
			}
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "profile: verify OK (all mirrors match source SHA)")
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

// renderDryRunAll prints every per-target byte sequence to stdout,
// each prefixed by the target name and a 12-char SHA digest. Useful
// for diffing without modifying the working tree.
func renderDryRunAll(body, base string) error {
	rendered, keys, err := profile.RenderAll(body)
	if err != nil {
		return err
	}
	for _, name := range keys {
		h, err := profile.HashSource(profile.Targets[name], body)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "──────── %s (sha256:%s) ────────\n", name, short(h, 12))
		resolved, _ := profile.Resolve(profile.Targets[name], base)
		fmt.Fprintf(os.Stdout, "→ %s\n\n", resolved)
		fmt.Println(rendered[name])
		fmt.Println()
	}
	return nil
}

// renderDryRunOne prints a single per-target render.
func renderDryRunOne(name, body, _ string) error {
	tgt, ok := profile.Targets[name]
	if !ok {
		return fmt.Errorf("profile: unknown target %q (registered: %v)",
			name, profile.TargetNames())
	}
	h, err := profile.HashSource(tgt, body)
	if err != nil {
		return err
	}
	rendered, err := profile.Render(tgt, body)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "──────── %s (sha256:%s) ────────\n", name, short(h, 12))
	fmt.Println(rendered)
	return nil
}

// writeVerifyTable emits a Markdown-style table the human reads at
// the terminal. JSON mode is opt-in.
func writeVerifyTable(res []profile.Result) {
	fmt.Printf("%-13s %-9s %-8s %s\n", "TARGET", "FOUND", "MATCH", "PATH")
	for _, r := range res {
		fmt.Printf("%-13s %-9v %-8v %s\n",
			r.Target.Name, r.Found, r.Match, r.Path)
	}
}

// short returns the first n chars of a hex string. Used in
// human-readable output; full hex is still in --json output.
func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// resolveRepoRoot returns the directory writers should treat as the
// repository root. Defaults to "." (writer's cwd). The CLI never
// chdirs; the source path is relative.
func resolveRepoRoot() string {
	return "."
}
