// SPDX-License-Identifier: MIT
// Purpose: sin-code — unified Go binary for all SIN-Code analysis/manipulation tools.
// Replaces 13 separate binaries (discover, execute, map, grasp, scout, harvest,
// orchestrate, ibd, poc, sckg, adw, oracle, efm) with a single cobra-based CLI.
// Docs: cmd/sin-code/main.go.doc.md
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sandbox"
)

var rootCmd = &cobra.Command{
	Use:   "sin-code",
	Short: "SIN-Code unified analysis & manipulation toolchain",
	Long: `sin-code is the unified Go binary for the SIN-Code tool suite.
It consolidates 90+ subcommands into a single cobra-based CLI:

  Core analysis:    discover, execute, map, grasp, scout, harvest, orchestrate
  Advanced tools:   ibd, poc, sckg, adw, oracle, efm
  Utility commands: security, sbom, config, self-update, tui, serve, update,
                      tool-search, doctor, diff, benchmark, tokens
  Agent ecosystem:  chat, sessions, mcp, goal, daemon, skill, skills, swarm,
                      superpowers, vane, stack, gh, hub, ledger, summary, install,
                      compress, review, audit, ceo-audit, fusion, research,
                      image-graph, analyse, analyse-image, auto
  Lifecycle:        orchestrator-run, orchestrator-agents, orchestrator-plan,
                      todo, notifications, memory, assets, evalset, hooks,
                      instinct, prp, catalog, compile-spec, triage, grill,
                      subagent, auto-pr, checkpoint, rewind, debt, cover,
                      profile, rtk, codegraph, spec, permission, status
  Frontend:         serve, tui, webui
  Other:            completion, read, write, edit, lsp, plugin, index, rules

Each subcommand is also a thin pass-through to the standalone tool repos
for backwards compatibility — the standalone binaries are still maintained
but "sin-code" is now the primary distribution channel.`,
	Version: Version,
}

func main() {
	// Sandbox shim: if invoked as the re-exec target (second arg =
	// "__sandbox_exec"), apply Landlock and exec the real command. The
	// parent process stays unconfined; only the child runs sandboxed.
	if len(os.Args) > 2 && os.Args[1] == "__sandbox_exec" {
		if err := sandbox.ApplyAndExec(); err != nil {
			fmt.Fprintf(os.Stderr, "sin-code sandbox: %v\n", err)
			os.Exit(126)
		}
		return // unreachable after successful exec
	}

	// If invoked via a symlink named after a subcommand (e.g. `discover` ->
	// `sin-code discover`), automatically route to that subcommand.
	if len(os.Args) > 0 {
		name := filepath.Base(os.Args[0])
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == name {
				args := append([]string{name}, os.Args[1:]...)
				rootCmd.SetArgs(args)
				break
			}
		}
	}

	checkUpdate()

	if err := rootCmd.Execute(); err != nil {
		internal.PrintError(err)
	}
}
