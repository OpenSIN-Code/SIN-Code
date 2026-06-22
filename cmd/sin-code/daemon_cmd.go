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
	"io"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/resource"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// daemon hook variables — injected by coverage tests to avoid real disk or
// network calls. Production defaults point to the real implementations.
var (
	daemonResourceParseLimitsHook = resource.ParseLimits
	daemonOSGetwdHook             = os.Getwd
	daemonAutonomyOpenHook        = autonomy.Open
	memoryOpenHook                = memory.Open
	sessionOpenHook               = session.Open
	lessonsOpenHook               = lessons.Open
	autonomyLoadTriggersHook      = autonomy.LoadTriggers
	loopbuilderBuildHook          = loopbuilder.Build
	daemonDiskFreeHook            = resource.DiskFree
	daemonAutoDreamNewHook        = memory.NewAutoDream
	daemonLoadMergedConfigHook    = internal.LoadMergedConfig
)

// daemonOptions bundles the parsed CLI flags so the worker pool and
// trigger registration share one config value.
type daemonOptions struct {
	pollEvery           time.Duration
	leaseDur            time.Duration
	verifyCmd           string
	maxTurns            int
	concurrency         int
	repos               []string
	limits              resource.Limits
	maxContinuations    int
	maxDepth            int
	noContract          bool
	noBaseline          bool
	requireTools        string
	forbidTools         string
	fusionOnVerifyFail  bool
	autoPR              bool
	repetitionThreshold int
	containerEnabled    bool
	containerImage      string
	containerRunner     autonomy.ContainerRunner

	progress     string
	progressDest string
	progressFile string
}

func NewDaemonCmd() *cobra.Command {
	var pollEvery, leaseDur time.Duration
	var verifyCmd string
	var maxTurns, concurrency, maxProcs int
	var maxContinuations, maxDepth int
	var noContract, noBaseline, fusionOnVerifyFail, autoPR bool
	var repos []string
	var maxMemory, minDisk string
	var requireTools, forbidTools string
	var repetitionThreshold int
	var containerEnabled bool
	var containerImage string
	var progress, progressDest, progressFile string
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
			limits, err := daemonResourceParseLimitsHook(maxMemory, maxProcs, minDisk)
			if err != nil {
				return err
			}
			if concurrency < 1 {
				concurrency = 1
			}
			return runDaemon(cmd.Context(), daemonOptions{
				pollEvery:          pollEvery,
				leaseDur:           leaseDur,
				verifyCmd:          verifyCmd,
				maxTurns:           maxTurns,
				concurrency:        concurrency,
				repos:              repos,
				limits:             limits,
				maxContinuations:   maxContinuations,
				maxDepth:           maxDepth,
				noContract:         noContract,
				noBaseline:         noBaseline,
				requireTools:       requireTools,
				forbidTools:        forbidTools,
				fusionOnVerifyFail: fusionOnVerifyFail,
				containerEnabled:   containerEnabled,
				containerImage:     containerImage,
				progress:           progress,
				progressDest:       progressDest,
				progressFile:       progressFile,
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
	cmd.Flags().BoolVar(&noBaseline, "no-baseline", false, "disable the always-on SinCode loop baseline (tests/debug/docs/completeness DoD); also via SIN_BASELINE=off")
	cmd.Flags().StringVar(&requireTools, "require-tools", "", "comma-separated tool names the model must invoke before completion (issue #248)")
	cmd.Flags().StringVar(&forbidTools, "forbid-tools", "", "comma-separated tool names that block completion if invoked (issue #248)")
	cmd.Flags().BoolVar(&fusionOnVerifyFail, "fusion-on-verify-fail", false, "enable SIN Fusion verify-tournament on verify.fail (issue #290). Oracle mode is experimental; set fusion.oracle_mode=true via config.")
	cmd.Flags().BoolVar(&autoPR, "auto-pr", false, "auto-create PR after verification (issue #391)")
	cmd.Flags().IntVar(&repetitionThreshold, "repetition-threshold", 3, "observer-loop (issue #377)")
	cmd.Flags().BoolVar(&containerEnabled, "container", false, "run verification commands inside a container (issue #389)")
	cmd.Flags().StringVar(&containerImage, "container-image", os.Getenv("SIN_CONTAINER_IMAGE"), "container image for verification (defaults to config autonomy.container.image)")
	cmd.Flags().StringVar(&progress, "progress", "", "structured progress output: off|json (default from config, fallback off)")
	cmd.Flags().StringVar(&progressDest, "progress-dest", "stderr", "progress destination: stderr|stdout|file")
	cmd.Flags().StringVar(&progressFile, "progress-file", "", "progress file path when --progress-dest=file")
	return cmd
}

func runDaemon(ctx context.Context, opt daemonOptions) error {
	if opt.verifyCmd == "" {
		return fmt.Errorf("daemon refuses to start without --verify-cmd (autonomy requires a verification gate, mandate M3)")
	}
	cwd, err := daemonOSGetwdHook()
	if err != nil {
		return err
	}

	// M3 (verification gate) + M4 (headless: ask→deny) + issue #420:
	// the daemon is headless by mandate — there is no prompt to
	// clarify a destructive tool call and no human in front of the
	// terminal to spot a sandbox-escape. Force OS-level syscall
	// isolation for every `sin_bash` invocation across the worker
	// pool. There is no --no-sandbox opt-out for the autonomous
	// worker; the platform-native backend is selected automatically.
	setSandboxConfig("", cwd)
	fmt.Fprintf(os.Stderr,
		"daemon: sandbox enabled (workspace=%s, M3/M4 mandate, issue #420)\n", cwd)

	// Apply process-wide resource limits up front.
	opt.limits.Apply()

	queue, err := daemonAutonomyOpenHook(autonomy.DefaultPath())
	if err != nil {
		return err
	}
	defer queue.Close()

	memStore, err := memoryOpenHook("")
	if err != nil {
		return err
	}
	defer memStore.Close()

	store, err := sessionOpenHook(session.DefaultPath())
	if err != nil {
		return err
	}
	defer store.Close()

	hookEngine := hooks.New(nil) // no workspace hook loading for daemon
	memStoreLessons, _ := lessonsOpenHook("")
	defer func() {
		if memStoreLessons != nil {
			memStoreLessons.Close()
		}
	}()

	var dream *memory.AutoDream
	sinCfg, _ := daemonLoadMergedConfigHook()
	if sinCfg.MemoryAutoDream {
		interval := 5 * time.Minute
		if d, err := time.ParseDuration(sinCfg.MemoryAutoDreamInterval); err == nil && d > 0 {
			interval = d
		}
		dream = daemonAutoDreamNewHook(memStore, memory.WithInterval(interval))
		dream.Start(ctx)
		fmt.Printf("daemon: autoDream started (interval=%s)\n", interval)
		defer func() {
			dream.Stop()
			fmt.Println("daemon: autoDream stopped")
		}()
	}

	dreamFunc := func(ctx context.Context) error {
		if dream == nil {
			return nil
		}
		_, err := dream.RunOnce(ctx)
		return err
	}

	// Containerized verification (issue #389): when the CLI flag or the config
	// enables it, wire a Docker runner. The image must be explicit either from
	// --container-image or from autonomy.container.image.
	if opt.containerEnabled || sinCfg.AutonomyContainerEnabled {
		image := opt.containerImage
		if image == "" {
			image = sinCfg.AutonomyContainerImage
		}
		if image == "" {
			return fmt.Errorf("containerization requested but no container image provided; set --container-image or autonomy.container.image")
		}
		opt.containerImage = image
		opt.containerRunner = autonomy.NewDockerRunner()
		fmt.Printf("daemon: containerized verification enabled (image=%s)\n", image)
	}

	for _, repo := range dedupeRepos(cwd, opt.repos) {
		triggers := autonomyLoadTriggersHook(repo)
		if len(triggers) == 0 {
			continue
		}
		runner := &autonomy.Runner{Queue: queue, Workspace: repo, Triggers: triggers, DreamFunc: dreamFunc}
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
			executeGoal(ctx, queue, store, lessonsStore, memStore, hookEngine, goal, opt, worker)
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
	cwd, err := daemonOSGetwdHook()
	if err != nil {
		return true
	}
	free, ok := daemonDiskFreeHook(cwd)
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
	hookEngine *hooks.Engine, goal *autonomy.Goal, opt daemonOptions, worker int) {

	hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalStarted, Data: map[string]any{"goal_id": goal.ID, "attempt": goal.Attempts}})

	sinCfg, _ := internal.LoadMergedConfig()

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
			Workspace:       goal.Workspace,
			GoalID:          fmt.Sprintf("%d", goal.ID),
			Criteria:        persisted.SemanticCriteria,
			VerifyCmd:       opt.verifyCmd,
			AutoDetect:      true,
			IncludeBaseline: goalcontract.BaselineEnabled(opt.noBaseline),
		})
		if rerr != nil {
			fmt.Fprintf(os.Stderr, "daemon: goal %d contract resolve failed: %v\n", goal.ID, rerr)
		} else {
			// Merge persisted deterministic checks on top of resolved ones.
			resolved.DeterministicChecks = append(resolved.DeterministicChecks, persisted.DeterministicChecks...)
			contract = resolved
		}
	}

	loop, cleanup, err := loopbuilderBuildHook(ctx, loopbuilder.Config{
		Workspace:              goal.Workspace,
		SessionID:              sess.ID,
		GoalID:                 fmt.Sprintf("%d", goal.ID),
		MaxTurns:               opt.maxTurns,
		VerifyMode:             "poc",
		VerifyCmd:              opt.verifyCmd,
		Headless:               true,
		Contract:               contract,
		AllowContinuation:      opt.maxContinuations > 0,
		CoverageRequiredTools:  splitList(opt.requireTools),
		CoverageForbiddenTools: splitList(opt.forbidTools),
		FusionEnabled:          opt.fusionOnVerifyFail,
		RepetitionThreshold:    opt.repetitionThreshold,
		ContainerRunner:        opt.containerRunner,
		ContainerImage:         opt.containerImage,
		SessionStore:           store,
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

	if progress := firstNonEmpty(opt.progress, sinCfg.OutputProgress); progress != "off" && progress != "" {
		var w io.Writer = os.Stderr
		switch opt.progressDest {
		case "stdout":
			w = os.Stdout
		case "file":
			if opt.progressFile == "" {
				fmt.Fprintln(os.Stderr, "warn: --progress-dest=file requires --progress-file")
			} else {
				f, ferr := os.OpenFile(opt.progressFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filemode.Default())
				if ferr != nil {
					fmt.Fprintf(os.Stderr, "warn: cannot open progress file: %v\n", ferr)
				} else {
					w = f
					defer func() { _ = f.Close() }()
				}
			}
		}
		pw := agentloop.NewProgressWriter(w)
		pw.Decorate = func(ev agentloop.ProgressEvent) agentloop.ProgressEvent {
			ev.GoalID = goal.ID
			ev.WorkerID = worker
			return ev
		}
		loop.ProgressWriter = pw
		loop.SessionID = sess.ID
		defer pw.Close()
	}

	res, err := loop.Run(ctx, sess, goal.Prompt)
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

	_ = queue.Complete(ctx, goal.ID, sess.ID)
	hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalVerified, Data: map[string]any{
		"goal_id": goal.ID, "turns": res.Turns, "session_id": sess.ID}})
	fmt.Printf("daemon: goal %d VERIFIED in %d turns (session %s)\n", goal.ID, res.Turns, sess.ID)

	if opt.autoPR {
		if err := autoCreatePR(ctx, goal, res); err != nil {
			fmt.Fprintf(os.Stderr, "daemon: goal %d auto-pr failed: %v\n", goal.ID, err)
		}
	}
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
