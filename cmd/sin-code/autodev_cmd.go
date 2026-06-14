// SPDX-License-Identifier: MIT
// Purpose: `sin-code autodev` — Cobra constructor for the
// OpenSIN-Code/autodev-cli (Python, MIT, v0.4.0) bridge. Mirrors
// gh_cmd.go (M4 + v3.8.0+ Bridged-External pattern): own subcommand
// set, transparent stdout/stderr forwarding, no business logic,
// Python never vendored. Setup / doctor / version are pure shell-out
// to autodev-cli and rely on internal/autodev for binary discovery.
// Docs: autodev.doc.md
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autodev"
	"github.com/spf13/cobra"
)

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
			return runPassthrough(ctx, autodev.DefaultBin(), "init", "--json", ".")
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
			return runPassthrough(ctx, autodev.DefaultBin(), "status", "--json")
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
			v, err := autodev.Version()
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
