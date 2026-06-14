// SPDX-License-Identifier: MIT
// Purpose: `sin-code daemon` — autonomous worker: leases goals, runs the
// verified loop, learns from outcomes. M3+M4 hold (gate required,
// headless means ask->deny).
//
// Multi-repo (issue #71): goals carry their own Workspace, so a single
// daemon already executes goals across many repos. This command adds
// (a) trigger registration for every configured repo, (b) a worker
// pool for configurable parallel execution, and (c) best-effort
// resource limits (memory/CPU caps + a free-disk floor that
// back-pressures leasing).
package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/resource"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// daemonOptions bundles the parsed CLI flags so the worker pool and
// trigger registration share one config value.
type daemonOptions struct {
	pollEvery   time.Duration
	leaseDur    time.Duration
	verifyCmd   string
	maxTurns    int
	concurrency int
	repos       []string
	limits      resource.Limits
}

func NewDaemonCmd() *cobra.Command {
	var pollEvery, leaseDur time.Duration
	var verifyCmd string
	var maxTurns, concurrency, maxProcs int
	var repos []string
	var maxMemory, minDisk string
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the autonomous worker: lease goals, execute, verify, learn",
		Long: `sin-code daemon turns SIN-Code from reactive to autonomous:
- leases goals from the queue (sin-code goal add) across one OR MANY repos
- fires triggers from .sin-code/triggers.json in every configured repo
- runs each goal through the FULL verified agent loop
- executes goals in parallel with a configurable worker pool
- enforces best-effort resource limits (memory/CPU/disk)
- records outcomes in the knowledge base (learning loop)
- M4 holds: headless means ask -> deny; the daemon cannot self-escalate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			limits, err := resource.ParseLimits(maxMemory, maxProcs, minDisk)
			if err != nil {
				return err
			}
			if concurrency < 1 {
				concurrency = 1
			}
			return runDaemon(cmd.Context(), daemonOptions{
				pollEvery:   pollEvery,
				leaseDur:    leaseDur,
				verifyCmd:   verifyCmd,
				maxTurns:    maxTurns,
				concurrency: concurrency,
				repos:       repos,
				limits:      limits,
			})
		},
	}
	cmd.Flags().DurationVar(&pollEvery, "poll", 15*time.Second, "queue poll interval")
	cmd.Flags().DurationVar(&leaseDur, "lease", 30*time.Minute, "goal lease duration")
	cmd.Flags().StringVar(&verifyCmd, "verify-cmd", os.Getenv("SIN_VERIFY_CMD"), "verification command (REQUIRED for autonomy)")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 60, "max turns per goal")
	cmd.Flags().IntVar(&concurrency, "concurrency", 1, "number of goals to execute in parallel")
	cmd.Flags().StringSliceVar(&repos, "repos", nil, "additional repo paths to watch for triggers (multi-repo); cwd is always included")
	cmd.Flags().StringVar(&maxMemory, "max-memory", "", "soft heap memory limit, e.g. 2GiB (0/empty = unlimited)")
	cmd.Flags().IntVar(&maxProcs, "max-procs", 0, "cap GOMAXPROCS (0 = leave default)")
	cmd.Flags().StringVar(&minDisk, "min-disk", "", "minimum free disk before leasing new goals, e.g. 1GiB (empty = off)")
	return cmd
}

func runDaemon(ctx context.Context, opt daemonOptions) error {
	if opt.verifyCmd == "" {
		return fmt.Errorf("daemon refuses to start without --verify-cmd (autonomy requires a verification gate, mandate M3)")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Apply process-wide resource limits up front.
	opt.limits.Apply()

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
	defer func() {
		if memStoreLessons != nil {
			memStoreLessons.Close()
		}
	}()

	// Register triggers for every configured repo (cwd + --repos),
	// de-duplicated so passing cwd explicitly is harmless.
	for _, repo := range dedupeRepos(cwd, opt.repos) {
		triggers := autonomy.LoadTriggers(repo)
		if len(triggers) == 0 {
			continue
		}
		runner := &autonomy.Runner{Queue: queue, Workspace: repo, Triggers: triggers}
		go func() { _ = runner.Run(ctx) }()
		fmt.Printf("daemon: %d triggers active for %s\n", len(triggers), repo)
	}

	fmt.Printf("daemon: polling every %s, concurrency=%d, verify=%q, %s\n",
		opt.pollEvery, opt.concurrency, opt.verifyCmd, opt.limits.Describe())

	// Worker pool: N goroutines each lease + execute independently.
	// Queue.Lease is an atomic SQL transaction, so concurrent leasing
	// never hands the same goal to two workers.
	var wg sync.WaitGroup
	for i := 0; i < opt.concurrency; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			runWorker(ctx, worker, queue, store, memStoreLessons, memStore, hookEngine, opt)
		}(i + 1)
	}
	wg.Wait()
	return ctx.Err()
}

// runWorker is one worker-pool goroutine: it polls the shared queue,
// honours the disk floor, and runs each leased goal to completion.
func runWorker(ctx context.Context, worker int, queue *autonomy.Queue, store *session.Store,
	lessonsStore *lessons.Store, memStore *memory.Store, hookEngine *hooks.Engine, opt daemonOptions) {

	ticker := time.NewTicker(opt.pollEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Disk back-pressure: refuse to lease when the working
			// filesystem is below the configured floor.
			if !diskOK(opt.limits) {
				fmt.Fprintf(os.Stderr, "daemon[w%d]: free disk below floor %s; skipping lease\n",
					worker, resource.HumanBytes(opt.limits.MinDiskBytes))
				continue
			}
			goal, err := queue.Lease(ctx, opt.leaseDur)
			if err != nil {
				fmt.Fprintf(os.Stderr, "daemon[w%d]: lease error: %v\n", worker, err)
				continue
			}
			if goal == nil {
				continue
			}
			fmt.Printf("daemon[w%d]: executing goal %d (attempt %d/%d) repo=%s: %.60s\n",
				worker, goal.ID, goal.Attempts, goal.MaxRetries, goal.Workspace, goal.Prompt)
			executeGoal(ctx, queue, store, lessonsStore, memStore, hookEngine, goal, opt.verifyCmd, opt.maxTurns)
		}
	}
}

// diskOK reports whether leasing is allowed under the disk floor.
// When no floor is set, or the probe is unavailable on this platform,
// it returns true (fail-open, never block the daemon by guessing).
func diskOK(l resource.Limits) bool {
	if l.MinDiskBytes <= 0 {
		return true
	}
	cwd, err := os.Getwd()
	if err != nil {
		return true
	}
	free, ok := resource.DiskFree(cwd)
	if !ok {
		return true
	}
	return free >= l.MinDiskBytes
}

// dedupeRepos returns cwd plus the extra repos, removing duplicates
// while preserving order (cwd first).
func dedupeRepos(cwd string, extra []string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(cwd)
	for _, r := range extra {
		add(r)
	}
	return out
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
