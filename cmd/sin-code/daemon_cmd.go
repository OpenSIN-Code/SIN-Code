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
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/gitops"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
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
	pollEvery        time.Duration
	leaseDur         time.Duration
	verifyCmd        string
	maxTurns         int
	concurrency      int
	repos            []string
	limits           resource.Limits
	maxContinuations int
	maxDepth         int
	noContract       bool
	noPostGoals      bool
	autoCommit       bool
	pushRemote       string
	openPR           bool
}

func NewDaemonCmd() *cobra.Command {
	var pollEvery, leaseDur time.Duration
	var verifyCmd string
	var maxTurns, concurrency, maxProcs int
	var maxContinuations, maxDepth int
	var noContract, noPostGoals, autoCommit, openPR bool
	var pushRemote string
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
				pollEvery:        pollEvery,
				leaseDur:         leaseDur,
				verifyCmd:        verifyCmd,
				maxTurns:         maxTurns,
				concurrency:      concurrency,
				repos:            repos,
				limits:           limits,
				maxContinuations: maxContinuations,
				maxDepth:         maxDepth,
				noContract:       noContract,
				noPostGoals:      noPostGoals,
				autoCommit:       autoCommit || os.Getenv("SIN_AUTO_COMMIT") == "1",
				pushRemote:       pushRemote,
				openPR:           openPR,
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
	cmd.Flags().IntVar(&maxContinuations, "max-continuations", 5, "max times a goal may checkpoint+resume past max-turns before failing (0 = disabled)")
	cmd.Flags().IntVar(&maxDepth, "max-depth", 3, "max sub-goal nesting depth an agent may spawn via spawn_subgoal")
	cmd.Flags().BoolVar(&noContract, "no-contract", false, "disable Definition-of-Done contracts (revert to single verify-gate)")
	cmd.Flags().BoolVar(&noPostGoals, "no-post-goals", false, "disable auto-spawned post-completion doc/changelog goals (loop-001)")
	cmd.Flags().BoolVar(&autoCommit, "auto-commit", false, "automatically git commit verified work (env: SIN_AUTO_COMMIT=1) (loop-007)")
	cmd.Flags().StringVar(&pushRemote, "push-remote", "", "git remote to push to after commit (e.g. origin); empty = no push")
	cmd.Flags().BoolVar(&openPR, "open-pr", false, "open a GitHub PR after push (requires GH_TOKEN, implies --push-remote=origin)")
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
			executeGoal(ctx, queue, store, lessonsStore, memStore, hookEngine, goal, opt)
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
	hookEngine *hooks.Engine, goal *autonomy.Goal, opt daemonOptions) {

	hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalStarted, Data: map[string]any{"goal_id": goal.ID, "attempt": goal.Attempts}})

	sess, err := store.StartOrResume(goal.SessionID)
	if err != nil {
		_ = queue.Fail(ctx, goal.ID, "", "open session: "+err.Error())
		return
	}

	// Resolve the Definition-of-Done contract: persisted contract (from
	// `goal add --contract/--criteria`) merged with auto-detected repo checks
	// and the verify-cmd fallback. The stop-gate enforces it so completion is
	// confirmed independently of the worker.
	var contract *goalcontract.GoalContract
	if !opt.noContract {
		persisted, perr := goalcontract.Unmarshal(goal.Contract)
		if perr != nil {
			fmt.Fprintf(os.Stderr, "daemon: goal %d has invalid contract, ignoring: %v\n", goal.ID, perr)
			persisted = &goalcontract.GoalContract{}
		}
		resolved, rerr := goalcontract.Resolve(goalcontract.ResolveOptions{
			Workspace:   goal.Workspace,
			GoalID:      fmt.Sprintf("%d", goal.ID),
			Prompt:      goal.Prompt,
			Criteria:    persisted.SemanticCriteria,
			VerifyCmd:   opt.verifyCmd,
			AutoDetect:  true,
			NoPostGoals: opt.noPostGoals || persisted.DisablePostGoals,
		})
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "daemon: goal %d contract resolve failed: %v\n", goal.ID, rerr)
		} else {
			// Merge persisted deterministic checks on top of resolved ones.
			resolved.DeterministicChecks = append(resolved.DeterministicChecks, persisted.DeterministicChecks...)
			contract = resolved
		}
	}

	loop, cleanup, err := loopbuilder.Build(ctx, loopbuilder.Config{
		Workspace:         goal.Workspace,
		SessionID:         sess.ID,
		MaxTurns:          opt.maxTurns,
		VerifyMode:        "poc",
		VerifyCmd:         opt.verifyCmd,
		Headless:          true,
		Contract:          contract,
		AllowContinuation: opt.maxContinuations > 0,
		ToolFactory: func(mgr *mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec) {
			baseTool := combinedTool(goal.Workspace, mgr)
			baseSpecs := combinedSpecs(mgr)
			return wrapWithSpawn(queue, goal, opt.maxDepth, baseTool, baseSpecs)
		},
	}, lessonsStore)
	if err != nil {
		_ = queue.Fail(ctx, goal.ID, sess.ID, "build loop: "+err.Error())
		return
	}
	defer cleanup()

	// Pre-flight decomposition directive (loop-005): for a fresh, top-level
	// goal, prepend the autonomous-execution protocol so the agent decomposes
	// large scope up front and treats tests/docs/build as non-negotiable —
	// instead of discovering spawn_subgoal mid-run after hitting max-turns.
	effectivePrompt := goal.Prompt
	if goal.Depth == 0 && goal.Attempts <= 1 && len(sess.History()) == 0 {
		effectivePrompt = buildDecompositionDirective(opt.maxDepth) +
			"\n\n---\nORIGINAL GOAL:\n" + goal.Prompt
	}

	res, err := loop.Run(ctx, sess, effectivePrompt)
	if err != nil {
		_ = queue.Fail(ctx, goal.ID, sess.ID, err.Error())
		hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalExhausted, Data: map[string]any{"goal_id": goal.ID, "error": err.Error()}})
		fmt.Printf("daemon: goal %d failed: %v\n", goal.ID, err)
		return
	}

	// Continuation: the run checkpointed at max-turns without completing.
	// Re-enqueue for resumption without burning the retry budget, bounded by
	// --max-continuations so "work forever" still has a ceiling.
	if res.Continuation {
		count, cerr := queue.Continue(ctx, goal.ID, sess.ID, "checkpoint: "+res.Summary)
		if cerr != nil {
			_ = queue.Fail(ctx, goal.ID, sess.ID, "continue: "+cerr.Error())
			return
		}
		if count >= opt.maxContinuations {
			_ = queue.Fail(ctx, goal.ID, sess.ID, fmt.Sprintf("exceeded max continuations (%d)", opt.maxContinuations))
			hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalExhausted, Data: map[string]any{"goal_id": goal.ID, "reason": "max continuations"}})
			fmt.Printf("daemon: goal %d exhausted continuations (%d)\n", goal.ID, count)
			return
		}
		fmt.Printf("daemon: goal %d CHECKPOINTED (continuation %d/%d), will resume\n", goal.ID, count, opt.maxContinuations)
		return
	}

	// Auto-commit the verified work first (loop-007) so the post-completion
	// doc/changelog goals can diff against a clean baseline (HEAD~1..HEAD) and
	// the human never has to run git by hand. Non-fatal on failure.
	if opt.autoCommit {
		commitMsg := fmt.Sprintf(
			"feat(agent): complete goal #%d in %d turns\n\n%s\n\n[sin-code goal-id: %d]",
			goal.ID, res.Turns, res.Summary, goal.ID)
		remote := opt.pushRemote
		if opt.openPR && remote == "" {
			remote = "origin"
		}
		if cerr := gitops.AutoCommit(ctx, gitops.CommitOptions{
			Workspace:  goal.Workspace,
			Message:    commitMsg,
			PushRemote: remote,
			CreatePR:   opt.openPR,
			PRTitle:    fmt.Sprintf("Agent: goal #%d — %s", goal.ID, truncatePrompt(goal.Prompt, 60)),
			PRBody:     "Autonomously completed by SIN-Code daemon.\n\n" + res.Summary,
		}); cerr != nil {
			fmt.Fprintf(os.Stderr, "warn: auto-commit failed: %v\n", cerr)
		}
	}

	// Spawn post-completion doc/changelog/MASTER_TODO goals (loop-001). These
	// run as tree children, so queue.Complete marks the parent blocked until
	// every doc goal is verified — the loop can never leave docs stale.
	if !opt.noContract && contract != nil && len(contract.PostCompletionGoals) > 0 {
		spawnPostGoals(ctx, queue, goal, res, contract.PostCompletionGoals)
	}

	_ = queue.Complete(ctx, goal.ID, sess.ID)
	hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalVerified, Data: map[string]any{
		"goal_id": goal.ID, "turns": res.Turns, "session_id": sess.ID}})
	fmt.Printf("daemon: goal %d VERIFIED in %d turns (session %s)\n", goal.ID, res.Turns, sess.ID)
}

// spawnPostGoals enqueues each post-completion follow-up as a child of the
// parent goal, rendering its prompt template with the parent Result and
// honouring OnlyIfChanged globs against the last commit's diff (loop-001).
func spawnPostGoals(ctx context.Context, q *autonomy.Queue,
	parent *autonomy.Goal, res *agentloop.Result, posts []goalcontract.PostGoal) {

	data := map[string]any{
		"Summary":   res.Summary,
		"SessionID": res.SessionID,
		"Turns":     res.Turns,
	}
	for _, pg := range posts {
		if pg.OnlyIfChanged != "" && !changedFilesMatch(parent.Workspace, pg.OnlyIfChanged) {
			continue
		}
		prompt, err := renderTemplate(pg.PromptTemplate, data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: post-goal template error: %v\n", err)
			continue
		}
		var contractJSON string
		if len(pg.Criteria) > 0 {
			c := &goalcontract.GoalContract{SemanticCriteria: pg.Criteria}
			contractJSON, _ = c.Marshal()
		}
		id, err := q.AddSub(ctx, parent.ID, prompt, parent.Priority, parent.MaxRetries, contractJSON)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: could not spawn post-goal: %v\n", err)
			continue
		}
		fmt.Printf("daemon: spawned post-completion goal %d (docs/changelog) under goal %d\n", id, parent.ID)
	}
}

// renderTemplate renders a Go text/template with data.
func renderTemplate(tmpl string, data map[string]any) (string, error) {
	t, err := template.New("postgoal").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// changedFilesMatch reports whether any file changed in the last commit
// matches the glob pattern. Fail-open: when the diff can't be read (no prior
// commit, not a git repo), it returns true so the post-goal still runs.
func changedFilesMatch(workspace, pattern string) bool {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD~1", "HEAD")
	cmd.Dir = workspace
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		if ok, _ := filepath.Match(pattern, filepath.Base(line)); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, line); ok {
			return true
		}
	}
	return false
}

// buildDecompositionDirective returns the autonomous-execution protocol that is
// prepended to fresh top-level goals (loop-005).
func buildDecompositionDirective(maxDepth int) string {
	return fmt.Sprintf(`AUTONOMOUS EXECUTION PROTOCOL — read before starting:

You are an autonomous coding agent. Your job is to FULLY complete the goal
below without any human intervention. Before writing a single line of code:

1. ASSESS SCOPE: Estimate how many distinct units of work this goal requires.
   - If it requires changes in 3+ independent packages or concerns: USE spawn_subgoal
     to decompose it into child goals FIRST, then work on each child.
   - If it is a single self-contained change: proceed directly.

2. FOR EVERY CODE CHANGE YOU MAKE, the following are NON-NEGOTIABLE:
   a. Write or update _test.go files for every changed package.
   b. Ensure go build ./... passes.
   c. Ensure go test -race ./... passes.
   d. Ensure go vet ./... is clean.
   e. Remove any TODO/FIXME you introduced.
   f. Update README, CHANGELOG, AGENTS.md, and affected doc.md files.

3. DON'T STOP EARLY. The stop-gate is independent of you. Even if you think
   you're done, continue working until it confirms completion.

4. spawn_subgoal is available (max depth %d). Use it freely for independent
   units of work — child goals run in parallel and are verified independently.

5. When ALL work is done: summarize exactly what changed and why.`, maxDepth)
}

// truncatePrompt shortens s to n runes with an ellipsis for PR titles.
func truncatePrompt(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

// wrapWithSpawn augments the daemon toolset with `spawn_subgoal`, letting the
// agent decompose a large goal into child goals that the daemon drains
// depth-first. Depth is bounded by maxDepth to prevent runaway recursion.
func wrapWithSpawn(queue *autonomy.Queue, goal *autonomy.Goal, maxDepth int,
	base agentloop.LocalToolFunc, specs []agentloop.ToolSpec) (agentloop.LocalToolFunc, []agentloop.ToolSpec) {

	spec := agentloop.ToolSpec{
		Name: "spawn_subgoal",
		Description: "Decompose the current goal into a child sub-goal that an autonomous worker will " +
			"complete and verify before this goal can finalize. Use for independent units of work.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt":   map[string]any{"type": "string", "description": "The sub-goal instruction."},
				"criteria": map[string]any{"type": "string", "description": "Optional acceptance criteria for the sub-goal."},
			},
			"required": []any{"prompt"},
		},
	}
	specs = append(specs, spec)

	tool := func(ctx context.Context, name string, args map[string]any) (string, error) {
		if name != "spawn_subgoal" {
			return base(ctx, name, args)
		}
		if goal.Depth >= maxDepth {
			return fmt.Sprintf("REFUSED: max sub-goal depth (%d) reached; do this work inline instead of spawning.", maxDepth), nil
		}
		prompt, _ := args["prompt"].(string)
		if prompt == "" {
			return "ERROR: spawn_subgoal requires a non-empty 'prompt'", nil
		}
		var contractJSON string
		if crit, _ := args["criteria"].(string); crit != "" {
			c := &goalcontract.GoalContract{SemanticCriteria: []string{crit}}
			contractJSON, _ = c.Marshal()
		}
		// Children get higher priority so the tree drains before the parent.
		id, err := queue.AddSub(ctx, goal.ID, prompt, goal.Priority+1, goal.MaxRetries, contractJSON)
		if err != nil {
			return "ERROR: could not enqueue sub-goal: " + err.Error(), nil
		}
		return fmt.Sprintf("Sub-goal %d enqueued under goal %d. It will be completed and verified by a worker before this goal finalizes.", id, goal.ID), nil
	}
	return tool, specs
}
