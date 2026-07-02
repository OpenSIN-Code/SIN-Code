// SPDX-License-Identifier: MIT
// Purpose: cobra command registration — wires every subcommand to rootCmd.
// Split from main.go for single-responsibility file layout.
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package main

import (
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
)

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
	rootCmd.AddCommand(MemoryCmd)
	rootCmd.AddCommand(internal.RulesCmd)
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
	rootCmd.AddCommand(NewChatCmd(), NewSessionsCmd(), NewMCPCmd(), NewToolSearchCmd(),
		NewGoalCmd(), NewDaemonCmd(), NewSkillCmd(), NewSwarmCmd(), NewSuperpowersCmd(), NewDoxCmd(),
		NewVaneCmd(), NewStackCmd(), NewGhCmd(), NewHubCmd(),
		NewLedgerCmd(), NewSummaryCmd(), NewAutodevCmd(), // v3.4.0 + v3.5.0 + v3.6.0 + v3.7.0 + v3.8.0 + v3.9.0 + v3.12.0 + v3.13.0 + autodev-bridge
		NewCompressCmd(),            // v3.18.0 — deterministic + LLM compaction (issue #172)
		NewReviewCmd(),              // v3.19.0 — review --complexity (issue #179)
		NewDiffCmd(),                // git diff with complexity + sin-debt overlay
		NewSkillsCmd(),              // bundled project-local agent skills
		NewEvalCmd(), NewTraceCmd(), // v3.18.0: Eval & Observability System (issue #75)
		NewProfileCmd(),                    // v3.18.0 — single-source-of-truth per-agent profile renderer (issue #175)
		NewRtkCmd(),                        // rtk (Rust Token Killer) bridge (issue #123)
		NewCodeGraphCmd(),                  // CodeGraph multi-language analysis bridge (issue #126)
		NewSpecCmd(),                       // Spec-Layer: *.spec.md contracts (issue #122)
		NewInstallCmd(),                    // v3.18.0 — single-binary installer entrypoint (issue #170)
		NewTriageCmd(),                     // v3.18.0 — backlog auto-prioritizer via gh (issue #162)
		NewCatalogCmd(),                    // v3.18.0 — unified tool catalog (issue #163, supersedes hub + assets)
		NewCompileSpecCmd(),                // v3.21.0 — declarative .sin-code.yml compiler (issue #164)
		NewGrillCmd(),                      // v3.18.0 — native adversarial design-review (issue #141 fusion)
		NewSubagentCmd(),                   // v3.18.0 — isolated-context sub-agent (issue #192, wraps #153)
		NewAutoPRCmd(),                     // v3.18.0 — self-healing pipeline (issue #158)
		NewCheckpointCmd(), NewRewindCmd(), // v3.20.0 — workspace checkpointing + rewind (issue #194)
		NewDebtCmd(),                    // v3.18.0 — sin-debt marker manager (issue #177)
		NewAuditCmd(), NewCEOAUDITCmd(), // v3.18.0 — complexity audit (issue #180) + 48-gate CEO audit
		NewCoverCmd(),                                                                                     // Coverage-Drohne: scan, check, gaps, generate, hook
		internal.InstinctCmd, internal.HooksCmd, internal.AssetsCmd, internal.EvalSetCmd, internal.PRPCmd, // continuous learning + lifecycle hooks + asset harvest + evalset + prp workflow
		NewImageGraphCmd(),   // image-graph: deterministic chart generation (bar/line/pie/area)
		NewStatusCmd(),       // v3.22.0
		NewFusionCmd(),       // v3.22.0 — fusion benchmark/rank/recommend (issue #395) — readiness/status snapshot (issue #326)
		NewResearchCmd(),     // v3.23.0 — autonomous research-report generation (issue #384)
		NewPermissionCmd(),   // v3.23.0 — reactive permission engine inspection (issue #374)
		NewTokensCmd(),       // v3.23.0 — token usage inspection
		NewAnalyseCmd(),      // v3.23.0 — static analysis runner
		NewAnalyseImageCmd(), // v3.24.0 — vision-based image analysis (issue #423)
		NewAutoCmd(),         // v3.23.0 — ultra-autonomous mode
		NewDoctorCmd(),       // unified health check
		NewBenchmarkCmd(),    // benchmark — run eval golden datasets with scoring report
		NewWatchCmd(),        // watch — workspace file watcher, run commands on save (issue #486)
		newContextCmd(),      // context — context window usage meter (issue #484)
		newDecisionCmd(),     // decision — architectural decision memory (issue #488)
		newBackgroundCmd(),   // background — fire-and-forget async agent jobs (issue #479)
		newSpecDrivenCmd(),   // spec-driven — EARS→arch→code pipeline (issue #480)
		newShareCmd(),        // share — session export/import (issue #482)
		newMCPInstallCmd(),   // mcp-install — discover/install MCP servers (issue #490)
		newLSPConfigCmd(),    // lsp-config — auto-detect+configure LSP servers (issue #492)
		NewGSDCmd(),          // gsd — Get Shit Done project lifecycle management
	)

	// Pass build-time version to self-update module.
	internal.SetCurrentVersion(internal.Version)

	// Root --version uses the same template as per-subcommand --version.
	internal.RegisterVersionCmd(rootCmd)
}
