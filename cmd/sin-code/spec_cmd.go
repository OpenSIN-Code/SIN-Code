// SPDX-License-Identifier: MIT
// Purpose: `sin-code spec` — the Spec-Layer CLI (issue #122). It parses,
// validates, and renders *.spec.md files that capture the contract a change
// must satisfy, bridging human intent and machine-checkable verification.
// Docs: docs/spec-layer.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/spec"
)

// NewSpecCmd builds the `spec` cobra subcommand (validate + show).
func NewSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Author, validate & inspect *.spec.md contracts (Spec-Layer)",
		Long: `sin-code spec is the Spec-Layer: a *.spec.md file captures the contract a
change must satisfy — Objective, Requirements, Acceptance Criteria (with
optional verify commands), and hard Invariants. It is the bridge between
human intent and machine-checkable verification consumed by the agent and
autopilot.

  sin-code spec validate feature.spec.md     # structural check, non-zero on error
  sin-code spec show feature.spec.md          # parsed summary
  sin-code spec show --json feature.spec.md   # parsed spec as JSON`,
	}
	cmd.AddCommand(newSpecValidateCmd())
	cmd.AddCommand(newSpecShowCmd())
	cmd.AddCommand(newSpecCheckCmd())
	cmd.AddCommand(newSpecAuthorCmd())
	return cmd
}

func newSpecValidateCmd() *cobra.Command {
	var quiet bool
	c := &cobra.Command{
		Use:   "validate <file.spec.md>",
		Short: "Validate a spec file for structural completeness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := spec.Load(args[0])
			if err != nil {
				return err
			}
			res := spec.Validate(s)
			out := cmd.OutOrStdout()
			if !quiet {
				for _, iss := range res.Issues {
					fmt.Fprintln(out, iss.String())
				}
			}
			if !res.OK() {
				return fmt.Errorf("spec %s: %d error(s)", args[0], len(res.Errors()))
			}
			if !quiet {
				fmt.Fprintf(out, "spec %s: OK (%d requirements, %d criteria)\n",
					args[0], len(s.Requirements), len(s.Criteria))
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress output; rely on exit code")
	return c
}

func newSpecShowCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "show <file.spec.md>",
		Short: "Print a parsed spec (summary or --json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := spec.Load(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(s)
			}
			title := s.Title
			if title == "" {
				title = "(untitled spec)"
			}
			fmt.Fprintf(out, "%s\n", title)
			fmt.Fprintf(out, "  objective:    %s\n", firstLine(s.Objective))
			fmt.Fprintf(out, "  requirements: %d\n", len(s.Requirements))
			for _, r := range s.Requirements {
				fmt.Fprintf(out, "    %s [%s] %s\n", r.ID, r.Priority, r.Text)
			}
			fmt.Fprintf(out, "  criteria:     %d\n", len(s.Criteria))
			for _, cr := range s.Criteria {
				if cr.Verify != "" {
					fmt.Fprintf(out, "    %s %s  (verify: %s)\n", cr.ID, cr.Text, cr.Verify)
				} else {
					fmt.Fprintf(out, "    %s %s\n", cr.ID, cr.Text)
				}
			}
			if len(s.Invariants) > 0 {
				fmt.Fprintf(out, "  invariants:   %d\n", len(s.Invariants))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit the parsed spec as JSON")
	return c
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	if s == "" {
		return "(none)"
	}
	return s
}

// newSpecCheckCmd runs the verify: command of every criterion in
// the given spec (or every *.spec.md in the repo with --all) and
// reports pass/fail. Exits non-zero on any must-priority failure.
// This is the CI-gate entry point (issue #157, spec-ci workflow).
func newSpecCheckCmd() *cobra.Command {
	var (
		all     bool
		asJSON  bool
		timeout time.Duration
	)
	c := &cobra.Command{
		Use:   "check [file.spec.md]",
		Short: "Run every criterion's verify: command and report pass/fail",
		Long: `sin-code spec check runs each Acceptance Criterion's ` + "`verify:`" + `
command and aggregates the results. Exits non-zero on any
must-priority failure (so the CI gate can block the PR).

  sin-code spec check feature.spec.md     # one spec
  sin-code spec check --all                # every .spec.md tracked by git
  sin-code spec check --all --json         # machine-readable report
  sin-code spec check --timeout 30s ...   # override per-criterion timeout`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			paths, err := collectSpecPaths(args, all)
			if err != nil {
				return err
			}
			if len(paths) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no *.spec.md files found")
				return nil
			}
			reports := make([]*spec.CheckReport, 0, len(paths))
			anyFailure := false
			for _, p := range paths {
				s, err := spec.Load(p)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "load %s: %v\n", p, err)
					anyFailure = true
					continue
				}
				rep, err := s.Check(ctx, timeout)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "check %s: %v\n", p, err)
					anyFailure = true
					continue
				}
				reports = append(reports, rep)
				if !asJSON {
					renderCheckReport(cmd.OutOrStdout(), rep)
				}
				if rep.HasFailures() {
					anyFailure = true
				}
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(reports); err != nil {
					return err
				}
			}
			if anyFailure {
				return fmt.Errorf("spec check: at least one must-priority criterion failed")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&all, "all", false, "check every *.spec.md tracked by git")
	c.Flags().BoolVar(&asJSON, "json", false, "emit per-criterion results as JSON")
	c.Flags().DurationVar(&timeout, "timeout", spec.DefaultCheckTimeout, "per-criterion timeout")
	return c
}

// collectSpecPaths returns the list of *.spec.md files to check.
// With `all`, it uses `git ls-files` to find them. With an explicit
// arg, it returns [arg]. With neither, it returns an error.
func collectSpecPaths(args []string, all bool) ([]string, error) {
	if all {
		out, err := exec.Command("git", "ls-files", "*.spec.md").Output()
		if err != nil {
			return nil, fmt.Errorf("git ls-files: %w", err)
		}
		var paths []string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				paths = append(paths, line)
			}
		}
		return paths, nil
	}
	if len(args) == 1 {
		return []string{args[0]}, nil
	}
	return nil, fmt.Errorf("specify a file or use --all")
}

func renderCheckReport(w io.Writer, rep *spec.CheckReport) {
	fmt.Fprintf(w, "\n=== %s (%s) ===\n", rep.Title, rep.SpecPath)
	for _, r := range rep.Results {
		mark := "✓"
		if r.Skipped {
			mark = "○"
		} else if !r.Passed {
			mark = "✗"
		}
		cmd := r.Command
		if cmd == "" {
			cmd = "(no verify: command — skipped)"
		}
		fmt.Fprintf(w, "  %s %-4s %s\n     verify: %s\n", mark, r.ID, r.Text, cmd)
		if !r.Passed && !r.Skipped && r.Output != "" {
			// Surface failure output, indented for readability.
			for _, line := range strings.Split(strings.TrimRight(r.Output, "\n"), "\n") {
				fmt.Fprintf(w, "       %s\n", line)
			}
		}
	}
	fmt.Fprintf(w, "  %d passed, %d failed, %d skipped (%s total)\n",
		rep.Passed, rep.Failed, rep.Skipped, rep.Duration.Round(time.Millisecond))
}

// newSpecAuthorCmd is the self-authoring mode (issue #157). It runs
// a Planner LLM call to produce a *.spec.md, an Implementer call to
// write the code, and a drift check. On mismatch, retry up to 3
// times. With --apply, opens a PR via gh.
func newSpecAuthorCmd() *cobra.Command {
	var (
		outFile string
		apply   bool
		model   string
	)
	c := &cobra.Command{
		Use:   "author <description>",
		Short: "Self-author a spec + implementation from a one-line description",
		Long: `sin-code spec author runs a Planner LLM call to produce a *.spec.md
(Issue, Requirements, Acceptance Criteria) and an Implementer call
to write the code. The drift checker verifies the result and
retries up to 3 times on mismatch. With --apply, opens a PR via gh.

This is the SOTA self-authoring mode. It requires a model client
configured in .sin-code.yml (model.default) and the gh CLI on PATH
when --apply is set.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			desc := strings.Join(args, " ")
			fmt.Fprintf(cmd.OutOrStdout(),
				"authoring spec for: %s\n  model:  %s\n  out:    %s\n  apply:  %v\n",
				desc, model, outFile, apply)
			// The full implementation requires a model client wired
			// into internal/learning or a new internal/spec/author.go.
			// For PR 1, we ship the scaffolding and a no-op
			// placeholder; PR 2 adds the LLM loop.
			fmt.Fprintln(cmd.OutOrStdout(),
				"\n[not yet implemented — see docs/SPEC-LAYER.md §\"Self-authoring\"]")
			fmt.Fprintln(cmd.OutOrStdout(),
				"PR 1 ships the parser, check, and CLI. PR 2 adds the LLM loop.")
			return nil
		},
	}
	c.Flags().StringVarP(&outFile, "out", "o", "spec.spec.md", "output path for the generated spec")
	c.Flags().BoolVar(&apply, "apply", false, "open a PR via gh after authoring")
	c.Flags().StringVar(&model, "model", "anthropic/claude-haiku-4-5", "model for the Planner/Implementer calls")
	return c
}
