// SPDX-License-Identifier: MIT
// Purpose: `sin-code daemon` — autonomous worker: leases goals, runs the
// verified loop, learns from outcomes. M3+M4 hold (gate required,
// headless means ask->deny).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/spf13/cobra"
)

func NewDaemonCmd() *cobra.Command {
	var pollEvery, leaseDur time.Duration
	var verifyCmd string
	var maxTurns int
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the autonomous worker: lease goals, execute, verify, learn",
		Long: `sin-code daemon turns SIN-Code from reactive to autonomous:
- leases goals from the queue (sin-code goal add)
- fires triggers from .sin-code/triggers.json (cron + file watch)
- runs each goal through the FULL verified agent loop
- records outcomes in the knowledge base (learning loop)
- M4 holds: headless means ask -> deny; the daemon cannot self-escalate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(cmd.Context(), pollEvery, leaseDur, verifyCmd, maxTurns)
		},
	}
	cmd.Flags().DurationVar(&pollEvery, "poll", 15*time.Second, "queue poll interval")
	cmd.Flags().DurationVar(&leaseDur, "lease", 30*time.Minute, "goal lease duration")
	cmd.Flags().StringVar(&verifyCmd, "verify-cmd", os.Getenv("SIN_VERIFY_CMD"), "verification command (REQUIRED for autonomy)")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 60, "max turns per goal")
	return cmd
}

func runDaemon(ctx context.Context, pollEvery, leaseDur time.Duration, verifyCmd string, maxTurns int) error {
	if verifyCmd == "" {
		return fmt.Errorf("daemon refuses to start without --verify-cmd (autonomy requires a verification gate, mandate M3)")
	}
	workspace, err := os.Getwd()
	if err != nil {
		return err
	}

	queue, err := autonomy.Open(autonomy.DefaultPath())
	if err != nil {
		return err
	}
	defer queue.Close()

	memStore, err := memory.Open("")
	if err != nil {
		return err
	}
	defer memStore.Close()

	store, err := session.Open(session.DefaultPath())
	if err != nil {
		return err
	}
	defer store.Close()

	hookEngine := hooks.New(nil) // no workspace hook loading for daemon
	memStoreLessons, _ := lessons.Open("")
	defer func() { if memStoreLessons != nil { memStoreLessons.Close() } }()

	if triggers := autonomy.LoadTriggers(workspace); len(triggers) > 0 {
		runner := &autonomy.Runner{Queue: queue, Workspace: workspace, Triggers: triggers}
		go func() { _ = runner.Run(ctx) }()
		fmt.Printf("daemon: %d triggers active\n", len(triggers))
	}

	fmt.Printf("daemon: polling every %s, verify=%q\n", pollEvery, verifyCmd)
	ticker := time.NewTicker(pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			goal, err := queue.Lease(ctx, leaseDur)
			if err != nil {
				fmt.Fprintf(os.Stderr, "daemon: lease error: %v\n", err)
				continue
			}
			if goal == nil {
				continue
			}
			fmt.Printf("daemon: executing goal %d (attempt %d/%d): %.60s\n",
				goal.ID, goal.Attempts, goal.MaxRetries, goal.Prompt)
			executeGoal(ctx, queue, store, memStoreLessons, memStore, hookEngine, goal, verifyCmd, maxTurns)
		}
	}
}

func executeGoal(ctx context.Context, queue *autonomy.Queue, store *session.Store,
	lessonsStore *lessons.Store, memStore *memory.Store,
	hookEngine *hooks.Engine, goal *autonomy.Goal, verifyCmd string, maxTurns int) {

	hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalStarted, Data: map[string]any{"goal_id": goal.ID, "attempt": goal.Attempts}})

	sess, err := store.StartOrResume(goal.SessionID)
	if err != nil {
		_ = queue.Fail(ctx, goal.ID, "", "open session: "+err.Error())
		return
	}
	loop, cleanup, err := loopbuilder.Build(ctx, loopbuilder.Config{
		Workspace:  goal.Workspace,
		SessionID:  sess.ID,
		MaxTurns:   maxTurns,
		VerifyMode: "poc",
		VerifyCmd:  verifyCmd,
		Headless:   true,
		ToolFactory: func(mgr *mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec) {
			return combinedTool(goal.Workspace, mgr), combinedSpecs(mgr)
		},
	}, lessonsStore)
	if err != nil {
		_ = queue.Fail(ctx, goal.ID, sess.ID, "build loop: "+err.Error())
		return
	}
	defer cleanup()

	res, err := loop.Run(ctx, sess, goal.Prompt)
	if err != nil {
		_ = queue.Fail(ctx, goal.ID, sess.ID, err.Error())
		hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalExhausted, Data: map[string]any{"goal_id": goal.ID, "error": err.Error()}})
		fmt.Printf("daemon: goal %d failed: %v\n", goal.ID, err)
		return
	}
	_ = queue.Complete(ctx, goal.ID, sess.ID)
	hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalVerified, Data: map[string]any{
		"goal_id": goal.ID, "turns": res.Turns, "session_id": sess.ID}})
	fmt.Printf("daemon: goal %d VERIFIED in %d turns (session %s)\n", goal.ID, res.Turns, sess.ID)
}

func loadTriggers(workspace string) []autonomy.Trigger {
	data, err := os.ReadFile(filepath.Join(workspace, ".sin-code", "triggers.json"))
	if err != nil {
		return nil
	}
	var ts []autonomy.Trigger
	if err := json.Unmarshal(data, &ts); err != nil {
		fmt.Fprintf(os.Stderr, "warn: invalid triggers.json: %v\n", err)
		return nil
	}
	return ts
}
