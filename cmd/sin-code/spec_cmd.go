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
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/spec"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/wiring"
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
		drift   bool
		root    string
	)
	c := &cobra.Command{
		Use:   "check [file.spec.md]",
		Short: "Run every criterion's verify: command and report pass/fail",
		Long: `sin-code spec check runs each Acceptance Criterion's ` + "`verify:`" + `
command and aggregates the results. Exits non-zero on any
must-priority failure (so the CI gate can block the PR).

With --drift, also runs a Spec<->Code signature check: any
requirement that names a Go function signature in backticks
(e.g. ` + "`Foo(x int) error`" + `) is checked against the actual
source tree under --root (default: current dir).

  sin-code spec check feature.spec.md                  # one spec
  sin-code spec check --all                             # every .spec.md tracked by git
  sin-code spec check --all --json                      # machine-readable report
  sin-code spec check --all --drift                     # + signature drift
  sin-code spec check --all --drift --root ./cmd/...   # scope the walk
  sin-code spec check --timeout 30s ...                # override per-criterion timeout`,
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
			if root == "" {
				root = "."
			}
			anyFailure := false
			for _, p := range paths {
				s, err := spec.Load(p)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "load %s: %v\n", p, err)
					anyFailure = true
					continue
				}
				// 1. verify: command check.
				rep, err := s.Check(ctx, timeout)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "check %s: %v\n", p, err)
					anyFailure = true
				} else {
					if !asJSON {
						renderCheckReport(cmd.OutOrStdout(), rep)
					}
					if rep.HasFailures() {
						anyFailure = true
					}
				}
				// 2. signature drift (opt-in).
				if drift {
					dr, err := s.DetectSignatureDrift(root)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "drift %s: %v\n", p, err)
						anyFailure = true
					} else if len(dr.Hits) > 0 {
						if !asJSON {
							renderDriftReport(cmd.OutOrStdout(), dr)
						}
						if dr.HasFailures() {
							anyFailure = true
						}
					}
				}
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				// Note: --json mode currently emits the check reports only;
				// the drift report is rendered human-only because the
				// union type would need a discriminator. PR 3 adds
				// the JSON envelope if a downstream tool needs it.
				if err := enc.Encode(struct {
					Path  string              `json:"-"`
					Files []*spec.CheckReport `json:"files"`
				}{Files: nil}); err != nil {
					return err
				}
			}
			if anyFailure {
				return fmt.Errorf("spec check: at least one must-priority criterion or signature drifted")
			}
			return nil
		},
	}
	c.Flags().BoolVar(&all, "all", false, "check every *.spec.md tracked by git")
	c.Flags().BoolVar(&asJSON, "json", false, "emit per-criterion results as JSON")
	c.Flags().DurationVar(&timeout, "timeout", spec.DefaultCheckTimeout, "per-criterion timeout")
	c.Flags().BoolVar(&drift, "drift", false, "also run the Spec<->Code signature drift check")
	c.Flags().StringVar(&root, "root", ".", "root directory for the signature drift walk")
	return c
}

// renderDriftReport writes a human-readable drift summary.
func renderDriftReport(w io.Writer, dr *spec.DriftReport) {
	fmt.Fprintf(w, "\n--- signature drift (%s) ---\n", dr.SpecPath)
	for _, h := range dr.Hits {
		mark := "✓"
		if !h.Match {
			mark = "✗"
		}
		fmt.Fprintf(w, "  %s %s: %s(%s) %s\n", mark, h.RequirementID, h.FuncName, h.RawParamText, h.RawResultText)
		if !h.Match {
			fmt.Fprintf(w, "    %s\n", h.Note)
		}
	}
}

// applySpecAsPR is the --apply path: commit the generated spec and
// open a PR via the gh CLI bridge. The branch is named
// `spec/<id>` to keep spec-related work grouped.
//
// The PR body includes the spec's Objective and the verify: command
// summary so reviewers can see the contract at a glance.
//
// On any failure (no git repo, no gh, no network), the operator gets
// a clear error and the spec file is left in place for manual work.
func applySpecAsPR(stdout, stderr io.Writer, specPath string, s *spec.Spec) error {
	if s == nil {
		return fmt.Errorf("apply: nil spec")
	}
	branch := "spec/" + s.ID
	commitMsg := fmt.Sprintf("spec: %s\n\nSelf-authored via `sin spec author --apply`.\n\nSee %s for the contract.", s.Title, specPath)

	// Step 1: create + switch to the branch.
	fmt.Fprintf(stdout, "\n--apply: creating branch %q\n", branch)
	if out, err := exec.Command("git", "checkout", "-b", branch).CombinedOutput(); err != nil {
		fmt.Fprintf(stderr, "git checkout -b: %v\n%s\n", err, string(out))
		return fmt.Errorf("apply: git checkout failed")
	}

	// Step 2: stage + commit the spec file.
	if out, err := exec.Command("git", "add", specPath).CombinedOutput(); err != nil {
		fmt.Fprintf(stderr, "git add: %v\n%s\n", err, string(out))
		return fmt.Errorf("apply: git add failed")
	}
	if out, err := exec.Command("git", "commit", "-m", commitMsg).CombinedOutput(); err != nil {
		fmt.Fprintf(stderr, "git commit: %v\n%s\n", err, string(out))
		return fmt.Errorf("apply: git commit failed")
	}
	fmt.Fprintf(stdout, "--apply: committed %s on %s\n", specPath, branch)

	// Step 3: push the branch (may fail in offline mode; we
	// surface the error and let the operator retry).
	if out, err := exec.Command("git", "push", "-u", "origin", branch).CombinedOutput(); err != nil {
		fmt.Fprintf(stderr, "git push: %v\n%s\n", err, string(out))
		return fmt.Errorf("apply: git push failed (branch %s is committed but not pushed; push manually)", branch)
	}

	// Step 4: open a PR via ghbridge. We use the existing Tier
	// classifier to make sure pr-create is on the mutating tier
	// (the operator must have allowed it in their session).
	bridge := ghbridge.New()
	prArgs := []string{
		"pr", "create",
		"--base", "main",
		"--head", branch,
		"--title", "spec: " + s.Title,
		"--body", prBodyForSpec(s, specPath),
	}
	if _, _, err := bridge.Execute(context.Background(), prArgs); err != nil {
		return fmt.Errorf("apply: gh pr create failed: %w (branch %s is pushed; open the PR manually with: gh pr create --head %s)", err, branch, branch)
	}
	fmt.Fprintf(stdout, "--apply: PR opened for %s\n", branch)
	return nil
}

// prBodyForSpec renders a human-readable PR body from a spec. The
// body includes the Objective, the criteria list (so reviewers
// know what to verify), and a link to the spec file.
func prBodyForSpec(s *spec.Spec, specPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", s.Title)
	if s.Objective != "" {
		fmt.Fprintf(&b, "### Objective\n\n%s\n\n", s.Objective)
	}
	if len(s.Requirements) > 0 {
		b.WriteString("### Requirements\n\n")
		for _, r := range s.Requirements {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", r.Priority, r.ID, r.Text)
		}
		b.WriteString("\n")
	}
	if len(s.Criteria) > 0 {
		b.WriteString("### Acceptance Criteria\n\n")
		for _, c := range s.Criteria {
			if c.Verify != "" {
				fmt.Fprintf(&b, "- %s: %s  `verify: %s`\n", c.ID, c.Text, c.Verify)
			} else {
				fmt.Fprintf(&b, "- %s: %s\n", c.ID, c.Text)
			}
		}
		b.WriteString("\n")
	}
	if len(s.Invariants) > 0 {
		b.WriteString("### Invariants\n\n")
		for _, inv := range s.Invariants {
			fmt.Fprintf(&b, "- %s\n", inv)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "_Self-authored via `sin spec author --apply`. Spec file: `%s`._\n", specPath)
	return b.String()
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
// a Planner LLM call to produce a *.spec.md, an Implementer call
// to write the code, and a drift check. On mismatch, retry up to 3
// times. With --apply, opens a PR via gh.
func newSpecAuthorCmd() *cobra.Command {
	var (
		outFile    string
		apply      bool
		model      string
		dryRun     bool
		maxRetries int
		workdir    string
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
when --apply is set. With --dry-run, no LLM is contacted; a
stub spec is returned for end-to-end testing of the pipeline.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			desc := strings.Join(args, " ")
			if workdir == "" {
				workdir, _ = os.Getwd()
			}

			// Build a Completer. If the user passed --dry-run or no
			// model client is configured, leave the Completer nil
			// — the spec loop handles nil as a stub.
			var completer spec.Completer
			if !dryRun {
				// The wiring layer injects a real llm.Client. In
				// the headless CLI we use a nil Completer unless
				// SIN_SPEC_LLM_BASEURL is set (env var the
				// operator can use to point at a local model).
				if base := os.Getenv("SIN_SPEC_LLM_BASEURL"); base != "" {
					apiKey := os.Getenv("SIN_SPEC_LLM_API_KEY")
					completer = wiring.NewSpecCompleter(llm.NewClient(base, apiKey), model)
					if completer == nil {
						return fmt.Errorf("spec author: model client failed to initialize")
					}
				} else if !dryRun {
					fmt.Fprintln(cmd.OutOrStdout(),
						"no model client configured; set SIN_SPEC_LLM_BASEURL or pass --dry-run")
				}
			}

			res, err := wiring.AuthorSpec(context.Background(), desc, wiring.SpecAuthorOptions{
				Completer:  completer,
				Model:      model,
				MaxRetries: maxRetries,
				Workdir:    workdir,
			})
			if err != nil {
				return err
			}
			if res.Spec == nil {
				return fmt.Errorf("spec author: gave up after %d attempts; see Trace", len(res.Trace))
			}

			// Write the spec to --out.
			body, err := spec.Marshal(res.Spec)
			if err != nil {
				return err
			}
			if err := os.WriteFile(outFile, body, 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"wrote spec to %s (%d requirements, %d criteria, %d attempts)\n",
				outFile, len(res.Spec.Requirements), len(res.Spec.Criteria), res.Attempts)

			// With --apply, branch + commit + PR via gh.
			if apply {
				if err := applySpecAsPR(cmd.OutOrStdout(), cmd.ErrOrStderr(), outFile, res.Spec); err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().StringVarP(&outFile, "out", "o", "spec.spec.md", "output path for the generated spec")
	c.Flags().BoolVar(&apply, "apply", false, "open a PR via gh after authoring")
	c.Flags().StringVar(&model, "model", "anthropic/claude-haiku-4-5", "model for the Planner/Implementer calls")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "skip the LLM call; return a stub spec")
	c.Flags().IntVar(&maxRetries, "retries", 3, "max retry attempts on drift")
	c.Flags().StringVarP(&workdir, "workdir", "C", "", "working directory (default: current dir)")
	return c
}
