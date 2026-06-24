// SPDX-License-Identifier: MIT
// Purpose: `sin-code autodev` — Cobra constructor for the
// OpenSIN-Code/autodev-cli (Python, MIT, v0.4.0) bridge. Mirrors
// gh_cmd.go (M4 + v3.8.0+ Bridged-External pattern): own subcommand
// set, transparent stdout/stderr forwarding, no business logic,
// Python never vendored. Setup / doctor / version are pure shell-out
// to autodev-cli and rely on internal/autodev for binary discovery.
// Also contains: `sin-code autopr` subcommand — auto-fix trivial
// regressions (issue #158) (merged from autopr_cmd.go).
// Docs: autodev.doc.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autodev"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autopr"
)

// autodevHookVars holds injectable dependencies for the autodev subcommand.
// Coverage tests replace these fields to avoid requiring a real autodev-cli
// binary on PATH.
var autodevHookVars = struct {
	resolveAutodevBin func() error
	defaultBin        func() string
	runPassthrough    func(ctx context.Context, bin string, args ...string) error
	version           func() (string, error)
}{
	resolveAutodevBin: autodev.ResolveAutodevBin,
	defaultBin:        autodev.DefaultBin,
	runPassthrough:    runPassthrough,
	version:           autodev.Version,
}

// NewAutodevCmd builds the `autodev` cobra subcommand. Pattern matches
// NewGhCmd / NewVaneCmd: returns *cobra.Command with setup / doctor /
// version attached. All verbs are one-line shell-out via autodev-cli;
// no reserialization, no interception, no caching.
func NewAutodevCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autodev",
		Short: "Bridge to OpenSIN-Code/autodev-cli (Python autoresearch loop, never vendored)",
		Long: `sin-code autodev shells out to the user-installed autodev-cli
(https://github.com/OpenSIN-Code/autodev-cli, MIT, v0.4.0) without ever
vendoring its Python sources. This subcommand is the operator-facing
entry point: setup, doctor, version. Each verb forwards stdout and
stderr transparently — the calling agent loop sees exactly what
autodev-cli printed, with no reserialization or comment injection.

Binary resolution honors $AUTODEV_BIN (override) and falls back to
"autodev" on $PATH. Install autodev-cli separately (pipx / pip) —
this subcommand refuses to run if it cannot find the binary, with
the exact lookup error surfaced so the operator can fix PATH.`,
	}
	cmd.AddCommand(newAutodevSetupCmd())
	cmd.AddCommand(newAutodevDoctorCmd())
	cmd.AddCommand(newAutodevVersionCmd())
	return cmd
}

// ── setup ─────────────────────────────────────────────────────────────

// newAutodevSetupCmd runs `autodev init --json .` in the operator's
// working directory. Idempotent: autodev-cli's init tolerates a
// pre-initialized project. Exit code propagates through cobra.
func newAutodevSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Initialize a project for autodev-cli (runs `autodev init --json .`)",
		Long: `Idempotent. Forwards stdout/stderr from 'autodev init --json .'
verbatim. Non-zero exit propagates through cobra so CI / agent loops
can detect partial init. Set $AUTODEV_BIN to override the binary.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			return autodevHookVars.runPassthrough(ctx, autodevHookVars.defaultBin(), "init", "--json", ".")
		},
	}
}

// ── doctor ────────────────────────────────────────────────────────────

// newAutodevDoctorCmd runs `autodev status --json`. Output is the
// canonical JSON blob autodev-cli's autodev-mcp tool consumes; echoing
// it byte-for-byte lets downstream MCP tools round-trip.
func newAutodevDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Show current autodev project state (runs `autodev status --json`)",
		Long: `Runs 'autodev status --json' and forwards its JSON output
verbatim so MCP consumers receive the same document upstream produced.
Exit code propagates.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			return autodevHookVars.runPassthrough(ctx, autodevHookVars.defaultBin(), "status", "--json")
		},
	}
}

// ── version ───────────────────────────────────────────────────────────

// newAutodevVersionCmd shells out to `autodev --version` and prints
// the trimmed stdout on a single line. Errors surface the wrapped
// upstream message so the operator can tell whether the binary is
// absent (typical CI case) or upstream lacks the flag (transient).
func newAutodevVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Report upstream autodev-cli --version",
		Long:  `Shells out to 'autodev --version' (per upstream contract) and prints the trimmed stdout line. Non-zero exit / stderr / empty stdout surface as a cobra error.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			v, err := autodevHookVars.version()
			if err != nil {
				// Surface upstream's stderr verbatim so the operator
				// can see WHY `--version` failed (e.g. "No such
				// option --version" until upstream lands the flag).
				fmt.Fprintln(cmd.ErrOrStderr(), "✗ autodev version probe failed:", err)
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), v)
			return nil
		},
	}
}

// ── helpers ───────────────────────────────────────────────────────────

// runPassthrough executes bin <args...> with the given ctx and pipes
// stdout + stderr bit-for-bit to the parent process. Gates on
// autodev.ResolveAutodevBin() so the operator gets a clean install
// hint instead of a stack trace if autodev-cli is missing.
func runPassthrough(ctx context.Context, bin string, args ...string) error {
	if err := autodev.ResolveAutodevBin(); err != nil {
		return fmt.Errorf("autodev bridge: %w (set $AUTODEV_BIN or install %s)", err, bin)
	}
	c := exec.CommandContext(ctx, bin, args...)
	c.Stdout = os.Stdout // transparent: no reserialization
	c.Stderr = os.Stderr // transparent: no reserialization
	return c.Run()
}

// ── autopr (merged from autopr_cmd.go) ─────────────────────────────────
// `sin-code autopr` subcommand — auto-fix trivial regressions (issue #158).
// Three subcommands:
//   - run  : classify + render a PR plan (dry-run by default)
//   - show : print the most recent plan from a JSON file
//   - plan : render the plan only, do not emit commands
//
// The actual PR creation goes through the gh-bridge (M4 ask-classified).
// The verify gate (M3) must be green before any PR is opened.

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
