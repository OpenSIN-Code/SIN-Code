// SPDX-License-Identifier: MIT
// Purpose: `sin-code autopr` subcommand — auto-fix trivial regressions
// (issue #158). Three subcommands:
//   - run  : classify + render a PR plan (dry-run by default)
//   - show : print the most recent plan from a JSON file
//   - plan : render the plan only, do not emit commands
//
// The actual PR creation goes through the gh-bridge (M4 ask-classified).
// The verify gate (M3) must be green before any PR is opened.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autopr"
)

func NewAutoPRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autopr",
		Short: "Self-healing pipeline: auto-fix trivial regressions and open a PR",
		Long: `sin-code autopr runs the post-task.complete pipeline (issue #158):

  1. Read every .spec.md + the verify-gate report
  2. Classify each regression as trivial | mechanical | non_trivial
  3. Build a deterministic PR body (no LLM)
  4. Emit the plan; the actual gh pr create is ` + "`ask`" + `-classified (M4)

All subcommands are pure (no I/O beyond the report file). The pipeline
is opt-in via .sin-code.yml's ` + "`autopr.enabled: true`" + ` policy key.`,
	}
	cmd.AddCommand(
		newAutoPRRunCmd(),
		newAutoPRPlanCmd(),
	)
	return cmd
}

func newAutoPRRunCmd() *cobra.Command {
	var workspace string
	var jsonOut bool
	var inFile string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the autopr pipeline and render a PR plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			issues, err := loadIssues(inFile)
			if err != nil {
				return err
			}
			rep := autopr.NewReport(workspace, issues)
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rep)
			}
			if !rep.WouldCreatePR {
				fmt.Fprintln(cmd.OutOrStdout(), "no auto-fixable issues found; nothing to do")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "PR plan:")
			fmt.Fprintf(cmd.OutOrStdout(), "  title: %s\n", rep.PRTitle)
			fmt.Fprintf(cmd.OutOrStdout(), "  auto-fixable: %d\n", len(rep.AutoFixable))
			fmt.Fprintf(cmd.OutOrStdout(), "  requires human: %d\n", len(rep.RequiresHuman))
			fmt.Fprintln(cmd.OutOrStdout(), "  commands to run:")
			for _, c := range rep.CommandsToRun {
				fmt.Fprintf(cmd.OutOrStdout(), "    %s\n", c)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "  PR body (preview):")
			for _, line := range splitLines(rep.PRBody) {
				fmt.Fprintf(cmd.OutOrStdout(), "    %s\n", line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the report as JSON")
	cmd.Flags().StringVar(&inFile, "issues", "", "JSON file with classified issues (default: empty = empty plan)")
	return cmd
}

func newAutoPRPlanCmd() *cobra.Command {
	var workspace string
	var inFile string
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Render the PR plan only; do not emit any commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			issues, err := loadIssues(inFile)
			if err != nil {
				return err
			}
			rep := autopr.NewReport(workspace, issues)
			if !rep.WouldCreatePR {
				fmt.Fprintln(cmd.OutOrStdout(), "no auto-fixable issues")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), rep.PRBody)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", ".", "workspace root")
	cmd.Flags().StringVar(&inFile, "issues", "", "JSON file with classified issues")
	return cmd
}

// loadIssues reads a JSON file of []autopr.Issue. Empty path is OK
// (returns nil, nil) — the caller can still build an empty plan.
func loadIssues(path string) ([]autopr.Issue, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("autopr: read %s: %w", path, err)
	}
	var out []autopr.Issue
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("autopr: parse %s: %w", path, err)
	}
	return out, nil
}

// splitLines is a tiny helper to keep the run-command's PR-body
// preview readable. Local to the file to avoid an import for one
// use site.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
