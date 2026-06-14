// SPDX-License-Identifier: MIT
// Purpose: `sin-code auto` — the single entrypoint for ultra-autonomous mode.
// Reads program.md, then runs OBSERVE->PROPOSE->ACT->VERIFY->MEASURE->KEEP/REVERT
// ->LEARN until the budget is spent. Self-registers via init() like eval/trace.
//
// NOTE: lives in package main (cmd/sin-code). Shown here for the issue; on
// integration it imports internal/autopilot, internal/loopbuilder, etc.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autopilot"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/spf13/cobra"
)

func init() { rootCmd.AddCommand(newAutoCmd()) }

func newAutoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auto",
		Short: "Ultra-autonomous mode: pursue a program.md objective on your behalf",
		Long: `sin-code auto reads program.md (objective + metric + budget) and
autonomously proposes, executes, verifies, measures, and keeps/reverts changes
until the budget is exhausted — no per-task prompting required.

Mandates: M3 (every kept change passes the verify gate) and M4 (hard budget) hold.`,
	}
	cmd.AddCommand(newAutoInitCmd(), newAutoRunCmd(), newAutoStatusCmd(), newAutoJournalCmd())
	return cmd
}

// ── auto init ───────────────────────────────────────────────────────────────

func newAutoInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write a program.md template into the current workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := os.Stat("program.md"); err == nil {
				return fmt.Errorf("program.md already exists")
			}
			if err := os.WriteFile("program.md", []byte(programTemplate), 0o644); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "wrote program.md — edit it, then run: sin-code auto run --verify-cmd \"...\"")
			return nil
		},
	}
}

// ── auto run ────────────────────────────────────────────────────────────────

func newAutoRunCmd() *cobra.Command {
	var verifyCmd string
	var budgetMinutes, maxExperiments, maxTurns int
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the autonomous loop until the budget is exhausted",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if verifyCmd == "" {
				return fmt.Errorf("auto run refuses to start without --verify-cmd (M3: autonomy requires a verify gate)")
			}
			workspace, err := os.Getwd()
			if err != nil {
				return err
			}
			prog, err := autopilot.LoadProgram(filepath.Join(workspace, "program.md"))
			if err != nil {
				return err
			}
			// CLI flags override program.md when set.
			if budgetMinutes > 0 {
				prog.BudgetMinutes = budgetMinutes
			}
			if maxExperiments > 0 {
				prog.MaxExperiments = maxExperiments
			}

			journal, err := autopilot.OpenJournal(autopilot.DefaultJournalPath(workspace))
			if err != nil {
				return err
			}
			defer journal.Close()

			lessonStore, _ := lessons.Open("")
			defer func() {
				if lessonStore != nil {
					lessonStore.Close()
				}
			}()

			sessStore, err := session.Open(session.DefaultPath())
			if err != nil {
				return err
			}
			defer sessStore.Close()

			runGoal := func(ctx context.Context, goal string) (autopilot.LoopResult, string, error) {
				sess, err := sessStore.StartOrResume("")
				if err != nil {
					return autopilot.LoopResult{}, "", err
				}
				loop, cleanup, err := loopbuilder.Build(ctx, loopbuilder.Config{
					Workspace:  workspace,
					SessionID:  sess.ID,
					MaxTurns:   maxTurns,
					VerifyMode: "poc",
					VerifyCmd:  verifyCmd,
					Headless:   true,
					ToolFactory: func(mgr *mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec) {
						return combinedTool(workspace, mgr), combinedSpecs(mgr)
					},
				}, lessonStore)
				if err != nil {
					return autopilot.LoopResult{}, "", err
				}
				defer cleanup()
				res, err := loop.Run(ctx, sess, goal)
				if err != nil {
					return autopilot.LoopResult{SessionID: sess.ID}, "", err
				}
				// verifyOut is captured by the gate; loopbuilder exposes the
				// last verify report on the result summary for metric parsing.
				return autopilot.LoopResult{SessionID: sess.ID, Verified: res.Verified, Turns: res.Turns}, res.Summary, nil
			}

			ap := autopilot.New(autopilot.Config{
				Workspace: workspace,
				Program:   prog,
				Proposer:  &autopilot.Proposer{Program: prog}, // deterministic fallback; wire LLM here later
				Journal:   journal,
				Budget:    autopilot.NewBudget(prog.BudgetMinutes, prog.MaxExperiments),
				Snap:      autopilot.NewSnapshotter(workspace),
				RunGoal:   runGoal,
				Lessons: func(ctx context.Context, ws string, n int) []string {
					if lessonStore == nil {
						return nil
					}
					entries, err := lessonStore.Query(ctx, ws, n)
					if err != nil {
						return nil
					}
					out := make([]string, 0, len(entries))
					for _, e := range entries {
						out = append(out, e.Lesson)
					}
					return out
				},
				Record: func(ctx context.Context, ws, lesson string) {
					if lessonStore != nil {
						_ = lessonStore.Record(ctx, lessons.Entry{Type: lessons.TypeFailedVerification, Workspace: ws, Lesson: lesson})
					}
				},
				Out: cmd.OutOrStdout(),
			})

			ctx, cancel := context.WithTimeout(cmd.Context(), time.Duration(prog.BudgetMinutes+5)*time.Minute)
			defer cancel()
			_, _, err = ap.Run(ctx)
			return err
		},
	}
	cmd.Flags().StringVar(&verifyCmd, "verify-cmd", os.Getenv("SIN_VERIFY_CMD"), "verification command (REQUIRED)")
	cmd.Flags().IntVar(&budgetMinutes, "budget-minutes", 0, "wall-clock budget (overrides program.md)")
	cmd.Flags().IntVar(&maxExperiments, "max-experiments", 0, "experiment cap (overrides program.md)")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 60, "max agent turns per experiment")
	return cmd
}

// ── auto status ───────────────────────────────────────────────────────────

func newAutoStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show budget, best metric, and recent experiment summary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			workspace, _ := os.Getwd()
			journal, err := autopilot.OpenJournal(autopilot.DefaultJournalPath(workspace))
			if err != nil {
				return err
			}
			defer journal.Close()
			prog, _ := autopilot.LoadProgram(filepath.Join(workspace, "program.md"))
			dir := autopilot.Minimize
			if prog != nil {
				dir = prog.Direction
			}
			kept, _ := journal.Count(cmd.Context(), autopilot.OutcomeKept)
			total, _ := journal.Count(cmd.Context(), "")
			best := journal.BestKept(cmd.Context(), dir)
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"experiments_total": total, "kept": kept, "best_metric": best,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "experiments: %d total, %d kept\nbest metric: %.4g\n", total, kept, best)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit JSON")
	return cmd
}

// ── auto journal ──────────────────────────────────────────────────────────

func newAutoJournalCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "journal",
		Short: "Print the experiment journal (newest first)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			workspace, _ := os.Getwd()
			journal, err := autopilot.OpenJournal(autopilot.DefaultJournalPath(workspace))
			if err != nil {
				return err
			}
			defer journal.Close()
			exps, err := journal.Recent(cmd.Context(), limit)
			if err != nil {
				return err
			}
			for _, e := range exps {
				fmt.Fprintf(cmd.OutOrStdout(), "#%d [%s] %s\n", e.ID, e.Outcome, e.Proposal)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "max entries")
	return cmd
}

const programTemplate = `# Objective
Describe the single high-level goal you want SIN-Code to pursue autonomously.

## Metric
name: my_metric
direction: minimize
extract: /my_metric=([0-9.]+)/

## Budget
minutes: 60
max_experiments: 12

## Invariants (DO NOT MODIFY)
- All existing tests must keep passing
- Public APIs stay source-compatible
`
