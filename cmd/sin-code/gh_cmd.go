// SPDX-License-Identifier: MIT
// Purpose: `sin-code gh` — CLI binding for the GitHub CLI bridge (ghbridge).
// Wraps the `gh` binary behind a 3-tier policy (read-only / mutating /
// forbidden) and a stdio MCP server, per the Bridged-External-Contract
// (M4 + v3.8.0+ pattern shared with vane/superpowers/dox). Never vendored.
// Docs: gh.doc.md
package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
	"github.com/spf13/cobra"
)

// NewGhCmd builds the `gh` cobra subcommand. Pattern matches
// NewVaneCmd / NewSuperpowersCmd: returns *cobra.Command with the
// relevant subcommands attached.
func NewGhCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gh",
		Short: "Bridge to the GitHub CLI (gh) with a 3-tier verb-allowlist policy",
		Long: `sin-code gh bridges the official GitHub CLI (https://cli.github.com,
MIT, never vendored) behind a 3-tier policy and a stdio MCP server. The
Bridged-External-Contract (v3.8.0+, shared with vane / superpowers / dox)
guarantees that:

  1. We never vendor gh — we shell out to the user's installed binary.
  2. Every call is classified by ghbridge.Classify() into one of three
     tiers:
       TierReadOnly   — safe to call from autonomous loops (issue list,
                        pr view, repo view, run list, …)
       TierMutating   — issues writes, pr merge, repo edit, workflow
                        enable, release create, …
       TierForbidden  — hard-deny surface (e.g. destructive verbs the
                        bridge refuses to forward unconditionally)
  3. Mutating commands are reachable, but the CLI refuses to run them
     non-interactively: it points the user at the chat session, which
     has the permission engine + confirmation prompt.
  4. The stdio MCP server (gh serve) exposes only the read-only surface
     as MCP tools, so autonomous agents can never accidentally invoke a
     mutating verb via MCP.

This subcommand is the operator-facing entry point: setup / doctor /
run / surface / serve. The non-interactive run subcommand is the
workhorse for CI and shell pipelines.`,
	}

	cmd.AddCommand(newGhSetupCmd())
	cmd.AddCommand(newGhDoctorCmd())
	cmd.AddCommand(newGhRunCmd())
	cmd.AddCommand(newGhSurfaceCmd())
	cmd.AddCommand(newGhServeCmd())
	return cmd
}

// ── setup ─────────────────────────────────────────────────────────────

// newGhSetupCmd registers the gh stdio MCP bridge in mcp.json (at
// $SIN_CODE_HOME/mcp.json, mirrored on the vane/superpowers path).
// Idempotent: ghbridge.RegisterMCP performs the deep-merge.
func newGhSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Register the gh stdio MCP bridge in mcp.json (idempotent)",
		RunE: func(_ *cobra.Command, _ []string) error {
			writtenPath, err := ghbridge.RegisterMCP(ghbridge.MCPConfigPath())
			if err != nil {
				fmt.Println("✗ register gh MCP bridge:", err)
				return err
			}
			fmt.Println("✓ gh MCP bridge registered in:", writtenPath)
			fmt.Println("  server name:", ghbridge.ServerName)
			fmt.Println("  run `sin-code gh doctor` to verify gh + auth.")
			return nil
		},
	}
}

// ── doctor ────────────────────────────────────────────────────────────

// newGhDoctorCmd checks that the gh binary is on PATH and that the
// user is authenticated. Non-zero exit on any failure.
func newGhDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check gh install + auth status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ghPath, err := exec.LookPath("gh")
			if err != nil {
				fmt.Println("✗ gh binary not found in PATH")
				fmt.Println("  install: https://cli.github.com")
				return fmt.Errorf("gh not installed")
			}
			fmt.Println("✓ gh binary:", ghPath)

			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			if err := ghbridge.New().Health(ctx); err != nil {
				fmt.Println("✗ gh health:", err)
				fmt.Println("  run: gh auth login")
				return fmt.Errorf("gh unhealthy")
			}
			fmt.Println("✓ gh reachable + auth ok")
			return nil
		},
	}
}

// ── run ───────────────────────────────────────────────────────────────

// newGhRunCmd is the non-interactive workhorse: Classify args, run
// read-only commands directly, refuse mutating commands with a hint
// pointing the operator at sin-code chat (which has the permission
// engine + confirmation prompt). Forbidden commands are always denied.
func newGhRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run <args...>",
		Short: "Run a non-interactive gh command (read-only only)",
		Long: `Forwards argv to the local gh binary via the ghbridge. Behavior:

  TierReadOnly   → executes immediately, prints stdout
  TierMutating   → refused; use 'sin-code chat' for the interactive
                   confirmation prompt (permission engine + ask policy)
  TierForbidden  → refused unconditionally (destructive surface)

This is the workhorse for CI and shell pipelines — it never blocks on
stdin, never prompts, and returns the gh exit code via cobra's standard
error propagation.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tier, err := ghbridge.Classify(args)
			if err != nil {
				return fmt.Errorf("classify: %w", err)
			}
			switch tier {
			case ghbridge.TierForbidden:
				return fmt.Errorf("forbidden by ghbridge policy: %s", strings.Join(args, " "))
			case ghbridge.TierMutating:
				return fmt.Errorf(
					"gh %q is a mutating command (tier=mutating); "+
						"refused by non-interactive runner — use sin-code chat for interactive confirmation",
					strings.Join(args, " "),
				)
			case ghbridge.TierReadOnly:
				ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
				defer cancel()
				stdout, _, err := ghbridge.New().Execute(ctx, args)
				if err != nil {
					return err
				}
				fmt.Print(stdout)
				return nil
			default:
				return fmt.Errorf("unknown tier %d for args: %s", tier, strings.Join(args, " "))
			}
		},
	}
}

// ── surface ───────────────────────────────────────────────────────────

// newGhSurfaceCmd prints the allowlist groups and the read-only /
// mutating / forbidden verb lists. Intended for docs, audits, and
// ad-hoc operator inspection. Source of truth is ghbridge.AllowedSurface.
func newGhSurfaceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "surface",
		Short: "Print the gh 3-tier policy groups and verb allowlists",
		RunE: func(_ *cobra.Command, _ []string) error {
			fmt.Print(ghbridge.AllowedSurface())
			return nil
		},
	}
}

// ── serve ─────────────────────────────────────────────────────────────

// newGhServeCmd starts the gh stdio MCP server. Used by mcp.json (the
// entry registered by `gh setup`).
func newGhServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the gh stdio MCP bridge server (used by mcp.json)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return ghbridge.NewServer().Serve(cmd.Context())
		},
	}
}
