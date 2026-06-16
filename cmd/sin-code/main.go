// SPDX-License-Identifier: MIT
// Purpose: sin-code — unified Go binary for all SIN-Code analysis/manipulation tools.
// Replaces 13 separate binaries (discover, execute, map, grasp, scout, harvest,
// orchestrate, ibd, poc, sckg, adw, oracle, efm) with a single cobra-based CLI.
// Docs: cmd/sin-code/main.go.doc.md
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sandbox"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

var Version = internal.Version // Re-export from internal/version.go; set at build time via -ldflags

var rootCmd = &cobra.Command{
	Use:   "sin-code",
	Short: "SIN-Code unified analysis & manipulation toolchain",
	Long: `sin-code is the unified Go binary for the SIN-Code tool suite.
It consolidates 44+ subcommands into a single cobra-based CLI:

  Core analysis:    discover, execute, map, grasp, scout, harvest, orchestrate
  Advanced tools:   ibd, poc, sckg, adw, oracle, efm
  Utility commands: security, sbom, config, self-update, tui, serve, update
  Agent ecosystem:  chat, sessions, mcp, goal, daemon, skill, superpowers,
                    vane, stack, gh, hub, ledger, summary, install, compress
  Other:            completion, read, write, edit, lsp, plugin, index,
                    orchestrator-run, orchestrator-agents, orchestrator-plan,
                    todo, notifications, memory, assets, evalset, hooks,
                    instinct, prp, skills, catalog, compile-spec, triage

Each subcommand is also a thin pass-through to the standalone tool repos
for backwards compatibility — the standalone binaries are still maintained
but "sin-code" is now the primary distribution channel.`,
	Version: Version,
}

func init() {
	rootCmd.AddCommand(internal.DiscoverCmd)
	rootCmd.AddCommand(internal.ExecuteCmd)
	rootCmd.AddCommand(internal.MapCmd)
	rootCmd.AddCommand(internal.GraspCmd)
	rootCmd.AddCommand(internal.ScoutCmd)
	rootCmd.AddCommand(internal.HarvestCmd)
	rootCmd.AddCommand(internal.OrchestrateCmd)
	rootCmd.AddCommand(internal.IbdCmd)
	rootCmd.AddCommand(internal.PocCmd)
	rootCmd.AddCommand(internal.SckgCmd)
	rootCmd.AddCommand(internal.AdwCmd)
	rootCmd.AddCommand(internal.OracleCmd)
	rootCmd.AddCommand(internal.EfmCmd)
	rootCmd.AddCommand(internal.ServeCmd)
	rootCmd.AddCommand(internal.SecurityCmd)
	rootCmd.AddCommand(internal.SbomCmd)
	rootCmd.AddCommand(internal.ConfigCmd)
	rootCmd.AddCommand(internal.SelfUpdateCmd)
	rootCmd.AddCommand(internal.UpdateCmd)
	rootCmd.AddCommand(todo.TodoCmd)
	rootCmd.AddCommand(notifications.NotificationsCmd)
	rootCmd.AddCommand(internal.MemoryCmd)
	rootCmd.AddCommand(internal.ReadCmd)
	rootCmd.AddCommand(internal.WriteCmd)
	rootCmd.AddCommand(internal.EditCmd)
	rootCmd.AddCommand(internal.LSPCmd)
	rootCmd.AddCommand(internal.PluginCmd)
	rootCmd.AddCommand(internal.IndexCmd)
	rootCmd.AddCommand(internal.OrchestratorRunCmd)
	rootCmd.AddCommand(internal.OrchestratorAgentsCmd)
	rootCmd.AddCommand(internal.OrchestratorPlanCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(webuiCmd)
	rootCmd.AddCommand(NewChatCmd(), NewSessionsCmd(), NewMCPCmd(),
		NewGoalCmd(), NewDaemonCmd(), NewSkillCmd(), NewSwarmCmd(), NewSuperpowersCmd(), NewDoxCmd(),
		NewVaneCmd(), NewStackCmd(), NewGhCmd(), NewHubCmd(),
		NewLedgerCmd(), NewSummaryCmd(), NewAutodevCmd(), // v3.4.0 + v3.5.0 + v3.6.0 + v3.7.0 + v3.8.0 + v3.9.0 + v3.12.0 + v3.13.0 + autodev-bridge
		NewCompressCmd(),            // v3.18.0 — deterministic + LLM compaction (issue #172)
		NewReviewCmd(),              // v3.19.0 — review --complexity (issue #179)
		NewSkillsCmd(),              // bundled project-local agent skills
		NewEvalCmd(), NewTraceCmd(), // v3.18.0: Eval & Observability System (issue #75)
		NewProfileCmd(),                 // v3.18.0: single-source-of-truth per-agent profile renderer (issue #175)
		NewRtkCmd(),                     // rtk (Rust Token Killer) bridge (issue #123)
		NewCodeGraphCmd(),               // CodeGraph multi-language analysis bridge (issue #126)
		NewSpecCmd(),                    // Spec-Layer: *.spec.md contracts (issue #122)
		NewInstallCmd(),                 // v3.18.0 — single-binary installer entrypoint (issue #170)
		NewTriageCmd(),                  // v3.18.0 — backlog auto-prioritizer via gh (issue #162)
		NewCatalogCmd(),                 // v3.18.0 — unified tool catalog (issue #163, supersedes hub + assets)
		NewCompileSpecCmd(),             // v3.21.0 — declarative .sin-code.yml compiler (issue #164)
		NewGrillCmd(),                   // v3.18.0 — native adversarial design-review (issue #141 fusion)
		NewSubagentCmd(),                 // v3.18.0 — isolated-context sub-agent (issue #192, wraps #153)
		NewCheckpointCmd(), NewRewindCmd(), // v3.20.0 — workspace checkpointing + rewind (issue #194)
		NewDebtCmd(),                    // v3.18.0 — sin-debt marker manager (issue #177)
		NewAuditCmd(), NewCEOAUDITCmd(), // v3.18.0 — complexity audit (issue #180) + 48-gate CEO audit
		internal.InstinctCmd, internal.HooksCmd, internal.AssetsCmd, internal.EvalCmd, internal.PRPCmd, // continuous learning + lifecycle hooks + asset harvest + evalset + prp workflow
	)

	// Pass build-time version to self-update module.
	internal.SetCurrentVersion(internal.Version)

	// Root --version uses the same template as per-subcommand --version.
	internal.RegisterVersionCmd(rootCmd)
}

// checkUpdateFn is the network probe used by checkUpdate. It is a package
// variable so tests can stub it out and stay fully hermetic (no GitHub calls).
var checkUpdateFn = internal.CheckUpdateAvailable

// updateCheckDisabled reports whether the background update check is
// disabled via environment:
//   - SIN_CODE_NO_UPDATE_CHECK / NO_UPDATE_CHECK: explicit user opt-out
//   - SIN_CODE_OFFLINE: generic offline switch
func updateCheckDisabled() bool {
	for _, key := range []string{"SIN_CODE_NO_UPDATE_CHECK", "NO_UPDATE_CHECK", "SIN_CODE_OFFLINE"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

func checkUpdate() {
	// Only run when invoked with no args or --version/-v.
	if len(os.Args) > 1 {
		first := os.Args[1]
		if first != "--version" && first != "-v" {
			return
		}
	}

	if updateCheckDisabled() {
		return
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	stampDir := filepath.Join(configDir, "sin")
	stampPath := filepath.Join(stampDir, ".last-update-check")

	if info, err := os.Stat(stampPath); err == nil {
		if time.Since(info.ModTime()) < 24*time.Hour {
			return
		}
	}

	// Touch the stamp file immediately so repeated invocations don't hammer GitHub.
	os.MkdirAll(stampDir, 0755)
	os.WriteFile(stampPath, []byte(time.Now().Format(time.RFC3339)), 0644)

	// Query GitHub with a short timeout so the CLI stays responsive.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type result struct {
		version string
		has     bool
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		v, h, e := checkUpdateFn()
		ch <- result{v, h, e}
	}()

	select {
	case <-ctx.Done():
		return
	case res := <-ch:
		if res.err != nil || !res.has {
			return
		}
		fmt.Printf("\n🔄 A new version of sin-code is available: %s → %s\n", Version, res.version)
		fmt.Println("   Run 'sin-code self-update' to install.")
	}
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
