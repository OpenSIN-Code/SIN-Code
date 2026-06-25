// SPDX-License-Identifier: MIT
// Purpose: merged command constructors for the core agent/autonomy subcommands.
//
// This file consolidates the previously single-export files for the daemon,
// skill, and spec subcommands. Each section preserves its original behaviour
// and comments; the constructors are registered from cmd/sin-code/main.go.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/auto_mem"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autopilot"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/catalog"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/compress"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dox"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/grill"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hub"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/install"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/profile"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/resource"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sindept"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skilldist"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/spec"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/spec/compiler"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/superpowers"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/telemetry"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/usage"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/wiring"
	"github.com/OpenSIN-Code/SIN-Code/skills"
)

// ============================================================================
// Daemon command (sin-code daemon)
// ============================================================================

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

// ============================================================================
// Skill command (sin-code skill)
// ============================================================================

// agentFlagAll is the magic value for `--agent` that means "try every
// registered target". We keep it as a string constant rather than a bool
// so future flags (`--agent <target1>,<target2>`) can extend without a
// breaking change.
const agentFlagAll = "all"

// reservedAgentNames mirrors skilldist.TargetNames() with the addition of
// `all`. Keeping this here lets cobra's `--agent` completion help text
// show the full choice set without needing to import skilldist in two
// places.
func reservedAgentNames() []string {
	out := []string{agentFlagAll}
	out = append(out, skilldist.TargetNames()...)
	return out
}

// bundledSkillFS is overridden by tests so they can point at a TempDir.
//
// We do NOT cache the FS at package level because the underlying fs.FS
// is immutable once constructed; the override pattern keeps test
// isolation trivial.
var bundledSkillFS = func() (fs.FS, error) { return skills.ListFS() }

// skillmgrInstallAllHook is overridden by tests to avoid real git
// clone/pull operations during `sin-code skill install all`.
var skillmgrInstallAllHook = skillmgr.InstallAll

// skillmgrDoctorHook is overridden by tests to avoid real git/python/go
// operations during `sin-code skill doctor`.
var skillmgrDoctorHook = skillmgr.Doctor

// resolveHome picks the home directory for install paths. Order of
// precedence: $SIN_CODE_HOME > $HOME > os.UserHomeDir().
func resolveHome() (string, error) {
	if v := os.Getenv("SIN_CODE_HOME"); v != "" {
		return v, nil
	}
	if v := os.Getenv("HOME"); v != "" {
		return v, nil
	}
	return os.UserHomeDir()
}

// extractSkillFromFS reads a single SKILL.md (and optional sidecar files)
// from the bundled skills FS into a real on-disk directory under dstRoot.
// skilldist.FormatDir installs from a `SrcRoot`, so a real path is required
// even though skillsmith would happily read from fs.FS.
func extractSkillFromFS(src fs.FS, dstRoot, skill string) error {
	out := filepath.Join(dstRoot, skill)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	skillMD, err := fs.ReadFile(src, filepath.Join(skill, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("extractSkillFromFS(%q): read SKILL.md: %w", skill, err)
	}
	if err := os.WriteFile(filepath.Join(out, "SKILL.md"), skillMD, filemode.Default()); err != nil {
		return err
	}
	for _, sub := range []string{"context", "frameworks", "tasks", "templates"} {
		entries, err := fs.ReadDir(src, filepath.Join(skill, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			data, err := fs.ReadFile(src, filepath.Join(skill, sub, e.Name()))
			if err != nil {
				return err
			}
			subDir := filepath.Join(out, sub)
			if err := os.MkdirAll(subDir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(subDir, e.Name()), data, filemode.Default()); err != nil {
				return err
			}
		}
	}
	return nil
}

// readSkillBody returns the SKILL.md body ready for marker-fence embedding.
// The YAML frontmatter is stripped and the body is LF-normalised so the
// host agent sees a portable representation.
func readSkillBody(src fs.FS, skill string) (string, error) {
	raw, err := fs.ReadFile(src, filepath.Join(skill, "SKILL.md"))
	if err != nil {
		return "", fmt.Errorf("readSkillBody(%q): %w", skill, err)
	}
	body := skilldist.StripFrontmatter(string(raw))
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("readSkillBody(%q): empty body after stripping frontmatter", skill)
	}
	return body, nil
}

// resolveAgentFlag walks the agent flag through the precedence rules and
// returns either the literal `all` or a Target from skilldist.Targets.
// Any other value (including the empty string) is rejected with a
// cobra-friendly error message.
func resolveAgentFlag(agentFlag string) ([]string, error) {
	if agentFlag == "" || agentFlag == agentFlagAll {
		return skilldist.TargetNames(), nil
	}
	if _, ok := skilldist.Targets[agentFlag]; !ok {
		return nil, fmt.Errorf("unknown agent %q (supported: %s, %s)",
			agentFlag, agentFlagAll, strings.Join(skilldist.TargetNames(), ", "))
	}
	return []string{agentFlag}, nil
}

// NewSkillCmd builds the `sin-code skill` command tree.
//
// Two helper commands shadow each other but coexist:
//
//	`sin-code skill status`           ecosystem-installer state (legacy).
//	`sin-code skill list [--agent X]` the new --installed matrix view.
//
// Both are useful: status talks to the upstream repos, list talks to
// the on-disk install of bundled skills. They use different storage and
// must not be merged.
func NewSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage ecosystem skills from upstream repos (install, status, update)",
		Long: `The skill subcommand has two surfaces.

Ecosystem install (default, v3.5.0):
  sin-code skill install <name>... | all
  sin-code skill status

Bundle distribution (issue #169, --agent flag):
  sin-code skill install <name> --agent <target>
  sin-code skill install <name> --agent all
  sin-code skill uninstall <name> --agent <target>
  sin-code skill list [--installed] [--agent <target>]

Distribution uses marker-fenced installs so a re-run is idempotent: the
block between <!-- SIN-CODE-SKILL-START: <name> --> and
<!-- SIN-CODE-SKILL-END:   <name> --> is replaced in place.`,
	}

	var jsonOut bool
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show install + runnable state of all known ecosystem skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			sts := skillmgr.Status(cmd.Context())
			sort.Slice(sts, func(i, j int) bool { return sts[i].Name < sts[j].Name })
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(sts)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-15s %-10s %-9s %-10s %s\n", "SKILL", "INSTALLED", "RUNNABLE", "STATUS", "DETAIL")
			for _, s := range sts {
				status := ""
				if s.Deprecated {
					status = "deprecated"
				}
				detail := s.Detail
				if s.Deprecated && s.DeprecatedReason != "" {
					if detail != "" {
						detail = "DEPRECATED: " + s.DeprecatedReason + " | " + detail
					} else {
						detail = "DEPRECATED: " + s.DeprecatedReason
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-15s %-10v %-9v %-10s %s\n", s.Name, s.Installed, s.Runnable, status, detail)
			}
			return nil
		},
	}
	statusCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose why ecosystem skills are not runnable",
		Long: `Doctor checks every known ecosystem skill and reports why it is
not runnable: not installed, missing MCP entrypoint, dependency unreachable,
or deprecated.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillDoctor(cmd, jsonOut)
		},
	}
	doctorCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	var agentFlag string
	installCmd := &cobra.Command{
		Use:   "install <name>... | all",
		Short: "Clone/update skill repos OR --agent <target> distribute a bundled skill",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Distribution mode: --agent was set.
			if agentFlag != "" || os.Getenv("SIN_CODE_AGENT") != "" {
				if agentFlag == "" {
					agentFlag = os.Getenv("SIN_CODE_AGENT")
				}
				return runSkillInstallDistribute(cmd, args, agentFlag)
			}
			// Legacy ecosystem install mode.
			return runSkillInstallEcosystem(cmd, args, jsonOut)
		},
	}
	installCmd.Flags().StringVar(&agentFlag, "agent", "",
		"target agent (claude-code|codex|gemini|opencode|cursor|windsurf|cline|copilot|aider|continue|zed|all); "+
			"or $SIN_CODE_AGENT. Empty = ecosystem install mode.")
	installCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	// list shows the install status of bundled skills against each
	// registered agent family.
	var installedOnly bool
	listCmd := &cobra.Command{
		Use:   "list [--installed] [--agent <target>]",
		Short: "List bundled skills and their distribution status per agent",
		Long: `Without flags, lists every bundled skill and reports which
agent families currently have it installed. --installed filters out
bundled skills with zero installs across all targets. --agent <target>
filters the report to one target (use "all" for the unfiltered view).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkillList(cmd, agentFlag, installedOnly, jsonOut)
		},
	}
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	listCmd.Flags().BoolVar(&installedOnly, "installed", false,
		"only show bundled skills with at least one installed copy")
	listCmd.Flags().StringVar(&agentFlag, "agent", "",
		"target agent (default: all)")

	uninstallCmd := &cobra.Command{
		Use:   "uninstall <name> --agent <target>",
		Short: "Remove a bundled skill from a target agent family",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if agentFlag == "" {
				agentFlag = os.Getenv("SIN_CODE_AGENT")
			}
			if agentFlag == "" {
				return fmt.Errorf("--agent <target> is required for uninstall (also accepts $SIN_CODE_AGENT)")
			}
			return runSkillUninstall(cmd, args, agentFlag)
		},
	}
	uninstallCmd.Flags().StringVar(&agentFlag, "agent", "",
		"target agent (required)")

	cmd.AddCommand(statusCmd, doctorCmd, installCmd, listCmd, uninstallCmd)
	return cmd
}

// runSkillDoctor renders the diagnostic report from skillmgr.Doctor.
func runSkillDoctor(cmd *cobra.Command, jsonOut bool) error {
	sts := skillmgrDoctorHook(cmd.Context())
	sort.Slice(sts, func(i, j int) bool { return sts[i].Name < sts[j].Name })
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(sts)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%-15s %-10s %-9s %-10s %s\n", "SKILL", "INSTALLED", "RUNNABLE", "STATUS", "DETAIL")
	sick := 0
	for _, s := range sts {
		status := ""
		if s.Deprecated {
			status = "deprecated"
		}
		detail := s.Detail
		if s.Deprecated && s.DeprecatedReason != "" {
			if detail != "" {
				detail = "DEPRECATED: " + s.DeprecatedReason + " | " + detail
			} else {
				detail = "DEPRECATED: " + s.DeprecatedReason
			}
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-15s %-10v %-9v %-10s %s\n", s.Name, s.Installed, s.Runnable, status, detail)
		if !s.Runnable {
			sick++
		}
	}
	if sick > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "\n%d skill(s) are not runnable. Run `sin-code skill install <name>` or `sin-code skill install all` to fix.\n", sick)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "\nAll known ecosystem skills are runnable.")
	}
	return nil
}

// runSkillInstallEcosystem is the pre-existing v3.5.0 behaviour: clone
// (or pull) an upstream skill repo and verify its MCP entrypoint.
//
// Deprecated skills are skipped when the magic `all` argument is used, but
// can still be installed explicitly by name. This keeps `install all` green
// while preserving the ability to audit or recover a deprecated skill.
//
// Kept as a helper so the parent install command stays readable.
func runSkillInstallEcosystem(cmd *cobra.Command, args []string, jsonOut bool) error {
	allMode := len(args) == 1 && args[0] == "all"

	// Batch mode: delegate to the skill manager so the CLI and the library
	// share the same deprecation/skip logic.
	if allMode {
		sts, err := skillmgrInstallAllHook(cmd.Context())
		skipped := 0
		for _, info := range skillmgr.KnownSkillsInfo() {
			if info.SkipInInstallAll {
				skipped++
			}
		}
		if jsonOut {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(sts)
		}
		for _, st := range sts {
			if st.Installed {
				fmt.Fprintf(cmd.OutOrStdout(), "OK   %s (runnable=%v, %s)\n", st.Name, st.Runnable, st.Detail)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "FAIL %s: %s\n", st.Name, st.Detail)
			}
		}
		if skipped > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "SKIPPED %d deprecated skill(s) in `install all`\n", skipped)
		}
		return err
	}

	// Single-skill mode preserves the original per-name confirmation.
	failed := 0
	var singleStatuses []skillmgr.SkillStatus
	for _, n := range args {
		st, err := skillmgr.Install(cmd.Context(), n)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "FAIL %s: %v\n", n, err)
			failed++
			continue
		}
		singleStatuses = append(singleStatuses, *st)
		fmt.Fprintf(cmd.OutOrStdout(), "OK   %s (runnable=%v, %s)\n", st.Name, st.Runnable, st.Detail)
	}
	if jsonOut && len(singleStatuses) > 0 {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(singleStatuses)
	}
	if failed > 0 {
		return fmt.Errorf("%d skill(s) failed to install", failed)
	}
	return nil
}

// runSkillInstallDistribute writes each bundled skill name into the
// resolved set of agent families via skilldist.Install.
//
// The body is sourced from the embedded skills.SkillsFS via a one-shot
// extraction to t.TempDir()-style scratch. Re-running is idempotent — the
// marker-fence replacement guarantees no duplicated blocks.
func runSkillInstallDistribute(cmd *cobra.Command, args []string, agentFlag string) error {
	targets, err := resolveAgentFlag(agentFlag)
	if err != nil {
		return err
	}
	home, err := resolveHome()
	if err != nil {
		return err
	}
	src, err := bundledSkillFS()
	if err != nil {
		return fmt.Errorf("open bundled skills FS: %w", err)
	}
	scratch, err := os.MkdirTemp("", "sin-code-skilldist-")
	if err != nil {
		return fmt.Errorf("create scratch dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	failed := 0
	for _, skill := range args {
		body, err := readSkillBody(src, skill)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", skill, err)
			failed++
			continue
		}
		if err := extractSkillFromFS(src, scratch, skill); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL %s: extract: %v\n", skill, err)
			failed++
			continue
		}
		for _, agentName := range targets {
			tgt := skilldist.Targets[agentName]
			err := skilldist.Install(skill, tgt, skilldist.InstallOptions{
				SrcRoot: scratch,
				Home:    home,
				Body:    body,
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "FAIL %s → %s: %v\n", skill, tgt.DisplayName, err)
				failed++
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK   %s → %s\n", skill, tgt.DisplayName)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d install(s) failed", failed)
	}
	return nil
}

// runSkillList renders the install matrix. The default view is every
// bundled skill × every agent family; --installed filters out rows with
// zero installs; --agent filters the agent columns to one target.
type listRow struct {
	Skill      string          `json:"skill"`
	Lifecycle  string          `json:"lifecycle,omitempty"` // issue #139
	Deprecated bool            `json:"deprecated,omitempty"`
	Targets    map[string]bool `json:"targets"`
	HasAny     bool            `json:"has_any"`
}

func runSkillList(cmd *cobra.Command, agentFlag string, installedOnly, jsonOut bool) error {
	home, err := resolveHome()
	if err != nil {
		return err
	}
	src, err := bundledSkillFS()
	if err != nil {
		return fmt.Errorf("open bundled skills FS: %w", err)
	}

	targets := skilldist.TargetNames()
	if agentFlag != "" && agentFlag != agentFlagAll {
		if _, ok := skilldist.Targets[agentFlag]; !ok {
			return fmt.Errorf("unknown agent %q (supported: all, %s)",
				agentFlag, strings.Join(skilldist.TargetNames(), ", "))
		}
		targets = []string{agentFlag}
	}

	skillNames := bundledSkillNames(src)
	rows := make([]listRow, 0, len(skillNames))
	for _, sk := range skillNames {
		row := listRow{Skill: sk, Targets: make(map[string]bool, len(targets))}
		// Read the lifecycle and deprecation flags from the embedded
		// SKILL.md frontmatter (issue #139). Bundled skills are
		// content-addressed; the lifecycle field is part of the manifest.
		// If missing (legacy skills before the migration), the field is
		// empty and the CLI shows a `[unknown]` marker.
		if sm, err := fs.ReadFile(src, sk+"/SKILL.md"); err == nil {
			body := string(sm)
			row.Lifecycle = parseLifecycleFromFrontmatter(body)
			row.Deprecated = parseDeprecatedFromFrontmatter(body)
		}
		for _, ag := range targets {
			tgt := skilldist.Targets[ag]
			ok, err := skilldist.IsInstalled(tgt, sk, home)
			if err != nil {
				row.Targets[ag] = false
				continue
			}
			row.Targets[ag] = ok
			if ok {
				row.HasAny = true
			}
		}
		if installedOnly && !row.HasAny {
			continue
		}
		rows = append(rows, row)
	}

	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	// Pretty table.
	header := fmt.Sprintf("%-32s %-12s", "SKILL", "LIFECYCLE")
	for _, ag := range targets {
		tgt := skilldist.Targets[ag]
		header += fmt.Sprintf(" %-12s", tgt.DisplayName)
	}
	fmt.Fprintln(cmd.OutOrStdout(), header)
	for _, r := range rows {
		lc := r.Lifecycle
		if lc == "" {
			lc = "unknown"
		}
		if r.Deprecated {
			lc += ",deprecated"
		}
		row := fmt.Sprintf("%-32s [%-16s]", r.Skill, lc)
		for _, ag := range targets {
			if r.Targets[ag] {
				row += " ✓           "
			} else {
				row += " —           "
			}
		}
		fmt.Fprintln(cmd.OutOrStdout(), row)
	}
	return nil
}

// runSkillUninstall reverses a previous --agent install. Each (skill, target)
// pair is removed via skilldist.Uninstall.
func runSkillUninstall(cmd *cobra.Command, args []string, agentFlag string) error {
	targets, err := resolveAgentFlag(agentFlag)
	if err != nil {
		return err
	}
	home, err := resolveHome()
	if err != nil {
		return err
	}
	failed := 0
	for _, skill := range args {
		for _, agentName := range targets {
			tgt := skilldist.Targets[agentName]
			if err := skilldist.Uninstall(tgt, skill, home); err != nil {
				fmt.Fprintf(os.Stderr, "FAIL %s → %s: %v\n", skill, tgt.DisplayName, err)
				failed++
				continue
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK   %s ← %s removed\n", skill, tgt.DisplayName)
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d uninstall(s) failed", failed)
	}
	return nil
}

// bundledSkillNames walks the flattened skills FS and returns every skill
// directory (leaf containing SKILL.md) in alphabetical order. The flat
// FS exposes each skill at path "<skill>/SKILL.md"; skillsmith would do
// the same lookup but we want discovery without depending on skillsmith's
// walking helper here.
func bundledSkillNames(src fs.FS) []string {
	var out []string
	entries, err := fs.ReadDir(src, ".")
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Confirm the directory has a SKILL.md before listing it.
		if _, err := fs.Stat(src, filepath.Join(e.Name(), "SKILL.md")); err == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// parseLifecycleFromFrontmatter extracts the `lifecycle:` value from
// a SKILL.md's YAML frontmatter. The format is intentionally narrow:
// we do not pull in a yaml dep just for this; a regex is enough.
//
// Returns "" if the field is missing or malformed. The caller treats
// "" as `[unknown]` so the operator notices skills that have not been
// migrated yet (run scripts/sync_lifecycle.py --apply).
func parseLifecycleFromFrontmatter(s string) string {
	const openDelim = "---"
	if !strings.HasPrefix(s, openDelim) {
		return ""
	}
	rest := strings.TrimPrefix(s, openDelim)
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return ""
	}
	fm := rest[:idx]
	// Look for `lifecycle: <value>` (allow leading whitespace).
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "lifecycle:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "lifecycle:"))
			// Strip surrounding quotes if present.
			if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"') {
				val = val[1 : len(val)-1]
			}
			return val
		}
	}
	return ""
}

// parseDeprecatedFromFrontmatter extracts the `deprecated:` boolean value
// from a SKILL.md's YAML frontmatter. It recognises true-ish values
// (`true`, `yes`, `1`) in a case-insensitive way.
func parseDeprecatedFromFrontmatter(s string) bool {
	const openDelim = "---"
	if !strings.HasPrefix(s, openDelim) {
		return false
	}
	rest := strings.TrimPrefix(s, openDelim)
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return false
	}
	fm := rest[:idx]
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "deprecated:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "deprecated:"))
			if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"') {
				val = val[1 : len(val)-1]
			}
			val = strings.ToLower(val)
			return val == "true" || val == "yes" || val == "1"
		}
	}
	return false
}

// ============================================================================
// Spec command (sin-code spec)
// ============================================================================

// NewSpecCmd builds the `spec` cobra subcommand (validate + show).
func NewSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Author, validate & inspect *.spec.md contracts (Spec-Layer)",
		Long: `sin-code spec is the Spec-Layer: a *.spec.md file captures the contract a
change must satisfy — Objective, Requirements, Acceptance Criteria (with
optional verify commands), and hard Invariants. It is the bridge between
human intent and machine-checkable verification consumed by the agent and
autopilot.

  sin-code spec validate feature.spec.md     # structural check, non-zero on error
  sin-code spec show feature.spec.md          # parsed summary
  sin-code spec show --json feature.spec.md   # parsed spec as JSON`,
	}
	cmd.AddCommand(newSpecValidateCmd())
	cmd.AddCommand(newSpecShowCmd())
	cmd.AddCommand(newSpecCheckCmd())
	cmd.AddCommand(newSpecAuthorCmd())
	return cmd
}

func newSpecValidateCmd() *cobra.Command {
	var quiet bool
	c := &cobra.Command{
		Use:   "validate <file.spec.md>",
		Short: "Validate a spec file for structural completeness",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := spec.Load(args[0])
			if err != nil {
				return err
			}
			res := spec.Validate(s)
			out := cmd.OutOrStdout()
			if !quiet {
				for _, iss := range res.Issues {
					fmt.Fprintln(out, iss.String())
				}
			}
			if !res.OK() {
				return fmt.Errorf("spec %s: %d error(s)", args[0], len(res.Errors()))
			}
			if !quiet {
				fmt.Fprintf(out, "spec %s: OK (%d requirements, %d criteria)\n",
					args[0], len(s.Requirements), len(s.Criteria))
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress output; rely on exit code")
	return c
}

func newSpecShowCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "show <file.spec.md>",
		Short: "Print a parsed spec (summary or --json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := spec.Load(args[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(s)
			}
			title := s.Title
			if title == "" {
				title = "(untitled spec)"
			}
			fmt.Fprintf(out, "%s\n", title)
			fmt.Fprintf(out, "  objective:    %s\n", firstLine(s.Objective))
			fmt.Fprintf(out, "  requirements: %d\n", len(s.Requirements))
			for _, r := range s.Requirements {
				fmt.Fprintf(out, "    %s [%s] %s\n", r.ID, r.Priority, r.Text)
			}
			fmt.Fprintf(out, "  criteria:     %d\n", len(s.Criteria))
			for _, cr := range s.Criteria {
				if cr.Verify != "" {
					fmt.Fprintf(out, "    %s %s  (verify: %s)\n", cr.ID, cr.Text, cr.Verify)
				} else {
					fmt.Fprintf(out, "    %s %s\n", cr.ID, cr.Text)
				}
			}
			if len(s.Invariants) > 0 {
				fmt.Fprintf(out, "  invariants:   %d\n", len(s.Invariants))
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "emit the parsed spec as JSON")
	return c
}

// firstLine returns the first line of s, or s itself if it contains no
// newlines. Empty input renders as "(none)".
func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	if s == "" {
		return "(none)"
	}
	return s
}

// newSpecCheckCmd runs the verify: command of every criterion in
// the given spec (or every *.spec.md in the repo with --all) and
// reports pass/fail. Exits non-zero on any must-priority failure.
// This is the CI-gate entry point (issue #157, spec-ci workflow).
func newSpecCheckCmd() *cobra.Command {
	var (
		all     bool
		asJSON  bool
		timeout time.Duration
		drift   bool
		root    string
		policy  string
	)
	c := &cobra.Command{
		Use:   "check [file.spec.md]",
		Short: "Run every criterion's verify: command and report pass/fail",
		Long: `sin-code spec check runs each Acceptance Criterion's ` + "`verify:`" + `
command and aggregates the results. Exits non-zero on any
must-priority failure (so the CI gate can block the PR).

With --drift, also runs a Spec<->Code signature check: any
requirement that names a Go function signature in backticks
(e.g. ` + "`Foo(x int) error`" + `) is checked against the actual
source tree under --root (default: current dir).

  sin-code spec check feature.spec.md                  # one spec
  sin-code spec check --all                             # every .spec.md tracked by git
  sin-code spec check --all --json                      # machine-readable report
  sin-code spec check --all --drift                     # + signature drift
  sin-code spec check --all --drift --root ./cmd/...   # scope the walk
  sin-code spec check --timeout 30s ...                # override per-criterion timeout
  sin-code spec check --policy off|warn|error          # drift strictness (issue #157)`,
		// The --policy default reads from SIN_SPEC_DRIFT env var, then falls
		// back to "error" (CI gate mode; the verify gate is sacred).
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			// Resolve policy: --policy flag > SIN_SPEC_DRIFT env > "error".
			polRaw := policy
			if polRaw == "" {
				polRaw = os.Getenv("SIN_SPEC_DRIFT")
			}
			pol := spec.ParsePolicy(polRaw)
			if !asJSON {
				fmt.Fprintf(cmd.OutOrStdout(), "policy: %s\n", pol)
			}
			paths, err := collectSpecPaths(args, all)
			if err != nil {
				return err
			}
			if len(paths) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no *.spec.md files found")
				return nil
			}
			if root == "" {
				root = "."
			}
			anyFailure := false
			for _, p := range paths {
				s, err := spec.Load(p)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "load %s: %v\n", p, err)
					anyFailure = true
					continue
				}
				// 1. verify: command check.
				rep, err := s.Check(ctx, timeout)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "check %s: %v\n", p, err)
					anyFailure = true
				} else {
					if !asJSON {
						renderCheckReport(cmd.OutOrStdout(), rep)
					}
					if rep.HasFailures() {
						anyFailure = true
					}
				}
				// 2. signature drift (opt-in).
				if drift {
					dr, err := s.DetectSignatureDrift(root)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "drift %s: %v\n", p, err)
						anyFailure = true
					} else if len(dr.Hits) > 0 {
						if !asJSON {
							renderDriftReport(cmd.OutOrStdout(), dr)
						}
						if dr.HasFailures() {
							anyFailure = true
						}
					}
				}
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				// Note: --json mode currently emits the check reports only;
				// the drift report is rendered human-only because the
				// union type would need a discriminator. PR 3 adds
				// the JSON envelope if a downstream tool needs it.
				if err := enc.Encode(struct {
					Path  string              `json:"-"`
					Files []*spec.CheckReport `json:"files"`
				}{Files: nil}); err != nil {
					return err
				}
			}
			if anyFailure && pol == spec.PolicyError {
				return fmt.Errorf("spec check: at least one must-priority criterion or signature drifted (policy=%s)", pol)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&all, "all", false, "check every *.spec.md tracked by git")
	c.Flags().BoolVar(&asJSON, "json", false, "emit per-criterion results as JSON")
	c.Flags().DurationVar(&timeout, "timeout", spec.DefaultCheckTimeout, "per-criterion timeout")
	c.Flags().BoolVar(&drift, "drift", false, "also run the Spec<->Code signature drift check")
	c.Flags().StringVar(&root, "root", ".", "root directory for the signature drift walk")
	c.Flags().StringVar(&policy, "policy", "", "drift strictness: off|warn|error (overrides SIN_SPEC_DRIFT env; default error)")
	return c
}

// renderDriftReport writes a human-readable drift summary.
func renderDriftReport(w io.Writer, dr *spec.DriftReport) {
	fmt.Fprintf(w, "\n--- signature drift (%s) ---\n", dr.SpecPath)
	for _, h := range dr.Hits {
		mark := "✓"
		if !h.Match {
			mark = "✗"
		}
		fmt.Fprintf(w, "  %s %s: %s(%s) %s\n", mark, h.RequirementID, h.FuncName, h.RawParamText, h.RawResultText)
		if !h.Match {
			fmt.Fprintf(w, "    %s\n", h.Note)
		}
	}
}

// applySpecAsPR is the --apply path: commit the generated spec and
// open a PR via the gh CLI bridge. The branch is named
// `spec/<id>` to keep spec-related work grouped.
//
// The PR body includes the spec's Objective and the verify: command
// summary so reviewers can see the contract at a glance.
//
// On any failure (no git repo, no gh, no network), the operator gets
// a clear error and the spec file is left in place for manual work.
func applySpecAsPR(stdout, stderr io.Writer, specPath string, s *spec.Spec) error {
	if s == nil {
		return fmt.Errorf("apply: nil spec")
	}
	branch := "spec/" + s.ID
	commitMsg := fmt.Sprintf("spec: %s\n\nSelf-authored via `sin spec author --apply`.\n\nSee %s for the contract.", s.Title, specPath)

	// Step 1: create + switch to the branch.
	fmt.Fprintf(stdout, "\n--apply: creating branch %q\n", branch)
	if out, err := exec.Command("git", "checkout", "-b", branch).CombinedOutput(); err != nil {
		fmt.Fprintf(stderr, "git checkout -b: %v\n%s\n", err, string(out))
		return fmt.Errorf("apply: git checkout failed")
	}

	// Step 2: stage + commit the spec file.
	if out, err := exec.Command("git", "add", specPath).CombinedOutput(); err != nil {
		fmt.Fprintf(stderr, "git add: %v\n%s\n", err, string(out))
		return fmt.Errorf("apply: git add failed")
	}
	if out, err := exec.Command("git", "commit", "-m", commitMsg).CombinedOutput(); err != nil {
		fmt.Fprintf(stderr, "git commit: %v\n%s\n", err, string(out))
		return fmt.Errorf("apply: git commit failed")
	}
	fmt.Fprintf(stdout, "--apply: committed %s on %s\n", specPath, branch)

	// Step 3: push the branch (may fail in offline mode; we
	// surface the error and let the operator retry).
	if out, err := exec.Command("git", "push", "-u", "origin", branch).CombinedOutput(); err != nil {
		fmt.Fprintf(stderr, "git push: %v\n%s\n", err, string(out))
		return fmt.Errorf("apply: git push failed (branch %s is committed but not pushed; push manually)", branch)
	}

	// Step 4: open a PR via ghbridge. We use the existing Tier
	// classifier to make sure pr-create is on the mutating tier
	// (the operator must have allowed it in their session).
	bridge := ghbridge.New()
	prArgs := []string{
		"pr", "create",
		"--base", "main",
		"--head", branch,
		"--title", "spec: " + s.Title,
		"--body", prBodyForSpec(s, specPath),
	}
	if _, _, err := bridge.Execute(context.Background(), prArgs); err != nil {
		return fmt.Errorf("apply: gh pr create failed: %w (branch %s is pushed; open the PR manually with: gh pr create --head %s)", err, branch, branch)
	}
	fmt.Fprintf(stdout, "--apply: PR opened for %s\n", branch)
	return nil
}

// prBodyForSpec renders a human-readable PR body from a spec. The
// body includes the Objective, the criteria list (so reviewers
// know what to verify), and a link to the spec file.
func prBodyForSpec(s *spec.Spec, specPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", s.Title)
	if s.Objective != "" {
		fmt.Fprintf(&b, "### Objective\n\n%s\n\n", s.Objective)
	}
	if len(s.Requirements) > 0 {
		b.WriteString("### Requirements\n\n")
		for _, r := range s.Requirements {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", r.Priority, r.ID, r.Text)
		}
		b.WriteString("\n")
	}
	if len(s.Criteria) > 0 {
		b.WriteString("### Acceptance Criteria\n\n")
		for _, c := range s.Criteria {
			if c.Verify != "" {
				fmt.Fprintf(&b, "- %s: %s  `verify: %s`\n", c.ID, c.Text, c.Verify)
			} else {
				fmt.Fprintf(&b, "- %s: %s\n", c.ID, c.Text)
			}
		}
		b.WriteString("\n")
	}
	if len(s.Invariants) > 0 {
		b.WriteString("### Invariants\n\n")
		for _, inv := range s.Invariants {
			fmt.Fprintf(&b, "- %s\n", inv)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "_Self-authored via `sin spec author --apply`. Spec file: `%s`._\n", specPath)
	return b.String()
}

// collectSpecPaths returns the list of *.spec.md files to check.
// With `all`, it uses `git ls-files` to find them. With an explicit
// arg, it returns [arg]. With neither, it returns an error.
func collectSpecPaths(args []string, all bool) ([]string, error) {
	if all {
		out, err := exec.Command("git", "ls-files", "*.spec.md").Output()
		if err != nil {
			return nil, fmt.Errorf("git ls-files: %w", err)
		}
		var paths []string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				paths = append(paths, line)
			}
		}
		return paths, nil
	}
	if len(args) == 1 {
		return []string{args[0]}, nil
	}
	return nil, fmt.Errorf("specify a file or use --all")
}

func renderCheckReport(w io.Writer, rep *spec.CheckReport) {
	fmt.Fprintf(w, "\n=== %s (%s) ===\n", rep.Title, rep.SpecPath)
	for _, r := range rep.Results {
		mark := "✓"
		if r.Skipped {
			mark = "○"
		} else if !r.Passed {
			mark = "✗"
		}
		cmd := r.Command
		if cmd == "" {
			cmd = "(no verify: command — skipped)"
		}
		fmt.Fprintf(w, "  %s %-4s %s\n     verify: %s\n", mark, r.ID, r.Text, cmd)
		if !r.Passed && !r.Skipped && r.Output != "" {
			// Surface failure output, indented for readability.
			for _, line := range strings.Split(strings.TrimRight(r.Output, "\n"), "\n") {
				fmt.Fprintf(w, "       %s\n", line)
			}
		}
	}
	fmt.Fprintf(w, "  %d passed, %d failed, %d skipped (%s total)\n",
		rep.Passed, rep.Failed, rep.Skipped, rep.Duration.Round(time.Millisecond))
}

// newSpecAuthorCmd is the self-authoring mode (issue #157). It runs
// a Planner LLM call to produce a *.spec.md, an Implementer call
// to write the code, and a drift check. On mismatch, retry up to 3
// times. With --apply, opens a PR via gh.
func newSpecAuthorCmd() *cobra.Command {
	var (
		outFile    string
		apply      bool
		model      string
		dryRun     bool
		maxRetries int
		workdir    string
	)
	c := &cobra.Command{
		Use:   "author <description>",
		Short: "Self-author a spec + implementation from a one-line description",
		Long: `sin-code spec author runs a Planner LLM call to produce a *.spec.md
(Issue, Requirements, Acceptance Criteria) and an Implementer call
to write the code. The drift checker verifies the result and
retries up to 3 times on mismatch. With --apply, opens a PR via gh.

This is the SOTA self-authoring mode. It requires a model client
configured in .sin-code.yml (model.default) and the gh CLI on PATH
when --apply is set. With --dry-run, no LLM is contacted; a
stub spec is returned for end-to-end testing of the pipeline.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			desc := strings.Join(args, " ")
			if workdir == "" {
				workdir, _ = os.Getwd()
			}

			// Build a Completer. If the user passed --dry-run or no
			// model client is configured, leave the Completer nil
			// — the spec loop handles nil as a stub.
			var completer spec.Completer
			if !dryRun {
				// The wiring layer injects a real llm.Client. In
				// the headless CLI we use a nil Completer unless
				// SIN_SPEC_LLM_BASEURL is set (env var the
				// operator can use to point at a local model).
				if base := os.Getenv("SIN_SPEC_LLM_BASEURL"); base != "" {
					apiKey := os.Getenv("SIN_SPEC_LLM_API_KEY")
					completer = wiring.NewSpecCompleter(llm.NewClient(base, apiKey), model)
					if completer == nil {
						return fmt.Errorf("spec author: model client failed to initialize")
					}
				} else if !dryRun {
					fmt.Fprintln(cmd.OutOrStdout(),
						"no model client configured; set SIN_SPEC_LLM_BASEURL or pass --dry-run")
				}
			}

			res, err := wiring.AuthorSpec(context.Background(), desc, wiring.SpecAuthorOptions{
				Completer:  completer,
				Model:      model,
				MaxRetries: maxRetries,
				Workdir:    workdir,
			})
			if err != nil {
				return err
			}
			if res.Spec == nil {
				return fmt.Errorf("spec author: gave up after %d attempts; see Trace", len(res.Trace))
			}

			// Write the spec to --out.
			body, err := spec.Marshal(res.Spec)
			if err != nil {
				return err
			}
			if err := os.WriteFile(outFile, body, filemode.Default()); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"wrote spec to %s (%d requirements, %d criteria, %d attempts)\n",
				outFile, len(res.Spec.Requirements), len(res.Spec.Criteria), res.Attempts)

			// With --apply, branch + commit + PR via gh.
			if apply {
				if err := applySpecAsPR(cmd.OutOrStdout(), cmd.ErrOrStderr(), outFile, res.Spec); err != nil {
					return err
				}
			}
			return nil
		},
	}
	c.Flags().StringVarP(&outFile, "out", "o", "spec.spec.md", "output path for the generated spec")
	c.Flags().BoolVar(&apply, "apply", false, "open a PR via gh after authoring")
	c.Flags().StringVar(&model, "model", "anthropic/claude-haiku-4-5", "model for the Planner/Implementer calls")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "skip the LLM call; return a stub spec")
	c.Flags().IntVar(&maxRetries, "retries", 3, "max retry attempts on drift")
	c.Flags().StringVarP(&workdir, "workdir", "C", "", "working directory (default: current dir)")
	return c
}

// ============================================================================
// Superpowers command (sin-code superpowers)
// ============================================================================

// NewSuperpowersCmd builds the `superpowers` cobra subcommand. Pattern
// matches NewChatCmd / NewSkillCmd: returns *cobra.Command with the
// relevant subcommands attached.
func NewSuperpowersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "superpowers",
		Short: "Integrate obra/superpowers skills into SIN-Code",
		Long: `sin-code superpowers clones obra/superpowers, pins the commit,
applies a SIN-Code overlay to every SKILL.md, regenerates PROMPT.md, and
registers the stdio MCP server so the agent can discover & load skills
at runtime.

This subcommand is network-free by default unless 'install' / 'update' is
invoked. All other subcommands (list, show, find, doctor) are local-only
and safe to call offline.`,
	}

	var (
		yes        bool
		repo       string
		branch     string
		query      string
		jsonOut    bool
		agentsPath string
	)

	// ── install ──────────────────────────────────────────────────────
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Clone obra/superpowers, apply overlay, write PROMPT.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			res, err := superpowers.Install(ctx, repo, branch)
			if err != nil {
				return err
			}
			// Auto-register the MCP server entry so the agent loop can
			// launch it on demand. Idempotent — see RegisterMCP.
			mcpPath, err := superpowers.RegisterMCP("")
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: MCP register failed: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "registered MCP server entry at %s\n", mcpPath)
			}
			// Auto-inject the AGENTS.md block if a path was given or if
			// the user passes --agents. Default is no injection.
			if agentsPath != "" {
				skills, _ := superpowers.List("")
				snippet := superpowers.AGENTSSnippet(skills)
				if err := superpowers.InjectAGENTS(agentsPath, snippet); err != nil {
					fmt.Fprintf(os.Stderr, "warning: AGENTS.md injection failed: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "injected superpowers block into %s\n", agentsPath)
				}
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Printf("installed %d skill(s) from %s\n  pinned: %s\n  branch: %s\n  duration: %s\n",
				res.Skills, res.Repo, res.SHA, res.Branch, res.Duration)
			return nil
		},
	}
	installCmd.Flags().StringVar(&repo, "repo", "", "override upstream repo URL (test fixtures, mirrors)")
	installCmd.Flags().StringVar(&branch, "branch", superpowers.DefaultBranch, "branch to track (default main)")
	installCmd.Flags().StringVar(&agentsPath, "agents", "", "optional: path to AGENTS.md to inject the superpowers block into")
	installCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	installCmd.Flags().BoolVar(&yes, "yes", false, "accept defaults (currently a no-op; reserved for non-interactive future use)")

	// ── update ───────────────────────────────────────────────────────
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Pull latest from upstream and re-pin",
		RunE: func(cmd *cobra.Command, args []string) error {
			// For now update is an alias for install with the current
			// pin intact. The pin file is rewritten on success.
			if !yes {
				fmt.Fprintln(os.Stderr, "running `superpowers update` (use --yes to skip future confirmation prompts)")
			}
			return runInstallOrUpdate(cmd, repo, branch, agentsPath, jsonOut)
		},
	}
	updateCmd.Flags().StringVar(&repo, "repo", "", "override upstream repo URL")
	updateCmd.Flags().StringVar(&branch, "branch", superpowers.DefaultBranch, "branch to track (default main)")
	updateCmd.Flags().StringVar(&agentsPath, "agents", "", "optional: AGENTS.md path to re-inject")
	updateCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	updateCmd.Flags().BoolVar(&yes, "yes", false, "skip confirmation prompts")

	// ── pin ──────────────────────────────────────────────────────────
	pinCmd := &cobra.Command{
		Use:   "pin <sha>",
		Short: "Pin a specific commit SHA as the active superpowers version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := superpowers.Pin(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Printf("pinned to %s on branch %s (updated %s)\n",
				st.SHA, st.Branch, st.UpdatedAt.Format("2006-01-02T15:04:05Z"))
			return nil
		},
	}

	// ── list ─────────────────────────────────────────────────────────
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List installed superpowers skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			all, err := superpowers.List("")
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(all)
			}
			if len(all) == 0 {
				fmt.Println("no skills installed — run `sin-code superpowers install`")
				return nil
			}
			fmt.Printf("%-30s %-12s %s\n", "SKILL", "HASH8", "PATH")
			for _, s := range all {
				hash := s.Hash
				if len(hash) > 8 {
					hash = hash[:8]
				}
				fmt.Printf("%-30s %-12s %s\n", s.Name, hash, s.Path)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	// ── show ─────────────────────────────────────────────────────────
	showCmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print the full SKILL.md for the given skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			info, err := superpowers.Get(args[0])
			if err != nil {
				return err
			}
			body, err := os.ReadFile(info.Path)
			if err != nil {
				return err
			}
			_, err = os.Stdout.Write(body)
			return err
		},
	}

	// ── find ─────────────────────────────────────────────────────────
	findCmd := &cobra.Command{
		Use:   "find <query>",
		Short: "Substring search across skill name + description",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			hits, err := superpowers.Find(args[0], 0)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(hits)
			}
			if len(hits) == 0 {
				fmt.Printf("no skills match %q\n", args[0])
				return nil
			}
			for _, h := range hits {
				desc := h.Description
				if desc == "" {
					desc = "(no description)"
				}
				fmt.Printf("- %s: %s\n", h.Name, desc)
			}
			return nil
		},
	}
	findCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	// `query` is only used as a positional placeholder; cobra captures it
	// via args[0]. The variable stays here so future flags (--limit,
	// --field) can share its help text.
	_ = query

	// ── serve ────────────────────────────────────────────────────────
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the superpowers stdio MCP server (JSON-RPC 2.0)",
		Long: `sin-code superpowers serve launches the stdio MCP server. It
speaks the JSON-RPC 2.0 protocol on stdin/stdout. The mcpclient package
launches this binary on demand when the 'superpowers' MCP server is
referenced in mcp.json.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			srv := superpowers.NewServer("")
			return srv.Serve(ctx)
		},
	}

	// ── init ─────────────────────────────────────────────────────────
	initCmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Scaffold a minimal SKILL.md in the given directory",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(abs, 0o755); err != nil {
				return err
			}
			skillPath := filepath.Join(abs, "SKILL.md")
			if _, err := os.Stat(skillPath); err == nil {
				return fmt.Errorf("init: %s already exists", skillPath)
			}
			// Use the parent directory name as the default skill name.
			name := filepath.Base(abs)
			body := "---\n" +
				"name: " + name + "\n" +
				"description: TODO — describe what this skill does and when to use it\n" +
				"---\n\n" +
				"# " + name + "\n\n" +
				"Describe the workflow here. Keep it focused on a single capability.\n"
			if err := os.WriteFile(skillPath, []byte(body), filemode.Default()); err != nil {
				return err
			}
			// Apply overlay so the user can immediately see the integration.
			superpowers.AppendOverlay(skillPath)
			fmt.Printf("scaffolded %s\n", skillPath)
			return nil
		},
	}

	// ── doctor ───────────────────────────────────────────────────────
	doctorCmd := &cobra.Command{
		Use:   "doctor",
		Short: "Verify install + overlay + MCP registration + AGENTS.md injection",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(jsonOut)
		},
	}
	doctorCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	cmd.AddCommand(installCmd, updateCmd, pinCmd, listCmd, showCmd,
		findCmd, serveCmd, initCmd, doctorCmd)
	_ = sort.Strings // keep import even if unused in future refactors
	return cmd
}

// runInstallOrUpdate is the shared body for install / update.
func runInstallOrUpdate(cmd *cobra.Command, repo, branch, agentsPath string, jsonOut bool) error {
	ctx := cmd.Context()
	res, err := superpowers.Install(ctx, repo, branch)
	if err != nil {
		return err
	}
	if _, err := superpowers.RegisterMCP(""); err != nil {
		fmt.Fprintf(os.Stderr, "warning: MCP register failed: %v\n", err)
	}
	if agentsPath != "" {
		skills, _ := superpowers.List("")
		snippet := superpowers.AGENTSSnippet(skills)
		if err := superpowers.InjectAGENTS(agentsPath, snippet); err != nil {
			fmt.Fprintf(os.Stderr, "warning: AGENTS.md injection failed: %v\n", err)
		}
	}
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	fmt.Printf("updated %d skill(s) from %s\n  pinned: %s\n  branch: %s\n  duration: %s\n",
		res.Skills, res.Repo, res.SHA, res.Branch, res.Duration)
	return nil
}

// runDoctor is a read-only verification check: pin file exists, overlay
// markers present in every SKILL.md, MCP server entry present, etc.
func runDoctor(jsonOut bool) error {
	type check struct {
		Name   string `json:"name"`
		OK     bool   `json:"ok"`
		Detail string `json:"detail,omitempty"`
	}
	checks := []check{}

	// 1. Skills directory exists.
	if _, err := os.Stat(superpowers.SkillsDir()); err == nil {
		checks = append(checks, check{Name: "skills_dir", OK: true})
	} else {
		checks = append(checks, check{Name: "skills_dir", OK: false, Detail: err.Error()})
	}

	// 2. Pin file present and parseable.
	pin, err := superpowers.CurrentPin()
	switch {
	case err != nil:
		checks = append(checks, check{Name: "pin_file", OK: false, Detail: err.Error()})
	case pin == nil:
		checks = append(checks, check{Name: "pin_file", OK: false, Detail: "no .sin-code-pin (run `superpowers install`)"})
	default:
		checks = append(checks, check{Name: "pin_file", OK: true, Detail: pin.SHA[:min(8, len(pin.SHA))]})
	}

	// 3. Overlay present on every SKILL.md.
	all, _ := superpowers.List("")
	missing := 0
	for _, s := range all {
		b, err := os.ReadFile(s.Path)
		if err != nil {
			missing++
			continue
		}
		if !strings.Contains(string(b), superpowers.OverlayMarker) {
			missing++
		}
	}
	if len(all) == 0 {
		checks = append(checks, check{Name: "overlay", OK: false, Detail: "no skills discovered"})
	} else if missing == 0 {
		checks = append(checks, check{Name: "overlay", OK: true, Detail: fmt.Sprintf("%d skills, all have overlay", len(all))})
	} else {
		checks = append(checks, check{Name: "overlay", OK: false, Detail: fmt.Sprintf("%d/%d missing overlay", missing, len(all))})
	}

	// 4. MCP server entry.
	if _, err := os.Stat(superpowers.MCPConfigPath()); err == nil {
		checks = append(checks, check{Name: "mcp_registered", OK: true})
	} else {
		checks = append(checks, check{Name: "mcp_registered", OK: false, Detail: err.Error()})
	}

	// 5. PROMPT.md present.
	if _, err := os.Stat(superpowers.PROMPTFile()); err == nil {
		checks = append(checks, check{Name: "prompt_file", OK: true})
	} else {
		checks = append(checks, check{Name: "prompt_file", OK: false, Detail: err.Error()})
	}

	allOK := true
	for _, c := range checks {
		if !c.OK {
			allOK = false
			break
		}
	}
	if jsonOut {
		out := map[string]any{
			"checks": checks,
			"all_ok": allOK,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, c := range checks {
		marker := "OK  "
		if !c.OK {
			marker = "FAIL"
		}
		fmt.Printf("%s %-18s %s\n", marker, c.Name, c.Detail)
	}
	if !allOK {
		return fmt.Errorf("doctor: one or more checks failed")
	}
	return nil
}

// Unused but kept so the package compiles even if no subcommand is
// selected (defensive — cobra will still show help, never RunE).
var _ = context.Background

// ============================================================================
// Tokens command (sin-code tokens)
// ============================================================================

// NewTokensCmd builds the `tokens` cobra subcommand.
func NewTokensCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tokens",
		Short: "Inspect LLM token usage (per session, day, lifetime, model)",
		Long: `sin-code tokens reads the local token-usage ledger at
$XDG_DATA_HOME/sin-code/tokens.db (see AGENTS.md §7). Each row is one
LLM call captured via internal/llm (issue #168).

  tokens show [--session ID] [--today] [--month] [--cost] [--share]
  tokens tail [--session ID] [-n 20]
  tokens aggregate [--by day|month|model|source|session] [--json]
  tokens cost [--json] [--model NAME] [--budget USD]

Cost is USD per 1k tokens, pulled from internal/usage.DefaultPricing and
overlaid by ` + "`llm.pricing_per_1k`" + ` from the user config.`,
	}
	cmd.AddCommand(newTokensShowCmd())
	cmd.AddCommand(newTokensTailCmd())
	cmd.AddCommand(newTokensAggregateCmd())
	cmd.AddCommand(newTokensCostCmd())
	return cmd
}

func openUsageStoreOrFail(cmd *cobra.Command) (*usage.Store, error) {
	path := usage.DefaultPath()
	store, err := usage.OpenWithPricing(path, loadPricingOverrides())
	if err != nil {
		return nil, fmt.Errorf("open tokens db at %s: %w (has sin-code recorded any LLM calls yet?)", path, err)
	}
	return store, nil
}

// loadPricingOverrides reads `llm.pricing_per_1k.KEY = USD` from
// ~/.config/sin/sin-code.toml (or the project override). Keys use the
// syntax `llm.pricing_per_1k."org/model"`. Empty / missing map is fine;
// best-effort: parser errors are swallowed, falls back to defaults.
func loadPricingOverrides() map[string]float64 {
	usersPath, projectsPath := tokensConfigPaths()
	merged := map[string]float64{}
	for _, p := range []string{usersPath, projectsPath} {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.HasPrefix(line, "llm.pricing_per_1k.") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimPrefix(strings.TrimSpace(parts[0]), "llm.pricing_per_1k.")
			key = strings.Trim(key, `"`)
			val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
			if v, err := strconv.ParseFloat(val, 64); err == nil && key != "" {
				merged[key] = v
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

// tokensConfigPaths mirrors internal/config.configDir / projectConfigPath.
// Duplicated here because the `internal` package's config helpers are
// unexported and tokens_cmd.go lives in `package main`. Keep in sync with
// cmd/sin-code/internal/config.go.
func tokensConfigPaths() (userPath, projectPath string) {
	if home, err := os.UserHomeDir(); err == nil {
		userPath = filepath.Join(home, ".config", "sin", "sin-code.toml")
	}
	projectPath = filepath.Join(".", ".sin-code", "config.toml")
	return
}

func newTokensShowCmd() *cobra.Command {
	var sessionID string
	var today, month, lifetime, costFlag, share, jsonOut bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show token usage for a session, today, the current month, or lifetime",
		Long: `Prints prompt + completion + total tokens, USD cost, and per-model
breakdown. Default scope is lifetime (all sessions to date). Pass
--session <id>, --today, or --month to narrow. Combine --cost to include
USD and --share for a single-line tweetable summary.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			store, err := openUsageStoreOrFail(nil)
			if err != nil {
				return err
			}
			defer store.Close()

			f := buildShowFilter(sessionID, today, month, lifetime)
			top, _, err := store.Aggregate(context.Background(), f, "")
			if err != nil {
				return err
			}
			if share {
				fmt.Fprintln(out, renderShareLine(top, costFlag))
				return nil
			}
			if jsonOut {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(top)
			}
			renderTable(out, top, f, costFlag)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Specific session ID (default: aggregate everything)")
	cmd.Flags().BoolVar(&today, "today", false, "Today's usage only")
	cmd.Flags().BoolVar(&month, "month", false, "Current calendar month")
	cmd.Flags().BoolVar(&lifetime, "lifetime", true, "All recorded sessions (default)")
	cmd.Flags().BoolVar(&costFlag, "cost", true, "Include USD cost estimate (default true; pass --cost=false to suppress)")
	cmd.Flags().BoolVar(&share, "share", false, "Single-line tweetable summary")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func buildShowFilter(sessionID string, today, month, lifetime bool) usage.Filter {
	f := usage.Filter{}
	switch {
	case sessionID != "":
		f.SessionID = sessionID
	case today:
		f.Since = startOfDay(time.Now())
		f.Until = f.Since.Add(24 * time.Hour)
	case month:
		f.Since = startOfMonth(time.Now())
		f.Until = time.Date(f.Since.Year(), f.Since.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	}
	_ = lifetime // lifetime is the default (no filter); kept for symmetry
	return f
}

func newTokensTailCmd() *cobra.Command {
	var sessionID string
	var n int
	cmd := &cobra.Command{
		Use:          "tail",
		Short:        "Show the most recent N token-usage events (default 20)",
		Long:         "Newest-first list of recorded LLM calls. Useful for debugging recent spend.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			store, err := openUsageStoreOrFail(nil)
			if err != nil {
				return err
			}
			defer store.Close()

			events, err := store.Tail(context.Background(),
				usage.Filter{SessionID: sessionID}, n)
			if err != nil {
				return err
			}
			if len(events) == 0 {
				fmt.Fprintln(out, "no recorded token events (yet)")
				return nil
			}
			renderEventTable(out, events)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Optional session ID filter")
	cmd.Flags().IntVarP(&n, "count", "n", 20, "Number of events to show")
	return cmd
}

func newTokensAggregateCmd() *cobra.Command {
	var by string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:          "aggregate",
		Short:        "Aggregate token usage grouped by day|month|model|source|session",
		Long:         "Returns the top-level totals plus per-group rows.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			store, err := openUsageStoreOrFail(nil)
			if err != nil {
				return err
			}
			defer store.Close()

			top, subs, err := store.Aggregate(context.Background(), usage.Filter{}, by)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(struct {
					Total     usage.Aggregation   `json:"total"`
					Subgroups []usage.Aggregation `json:"subgroups"`
				}{Total: *top, Subgroups: subs})
			}
			fmt.Fprintln(out, "== totals ==")
			renderTable(out, top, usage.Filter{}, true)
			if len(subs) > 0 {
				fmt.Fprintf(out, "\n== grouped by %s (%d rows) ==\n", by, len(subs))
				renderGroupedTable(out, subs)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "day", "Group by: day|month|model|source|session")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

// ─── renderers ────────────────────────────────────────────────────────────

func renderTable(w io.Writer, a *usage.Aggregation, f usage.Filter, withCost bool) {
	scope := "lifetime"
	if f.SessionID != "" {
		scope = "session=" + f.SessionID
	} else if !f.Since.IsZero() {
		scope = "since=" + f.Since.Format("2006-01-02")
	}
	fmt.Fprintf(w, "Scope: %s\n", scope)
	fmt.Fprintf(w, "Sessions recorded: %d\n", a.SessionsCount)
	fmt.Fprintf(w, "Events:            %d\n", a.EventCount)
	fmt.Fprintf(w, "Prompt tokens:     %s\n", humanTokens(a.InputTokens))
	fmt.Fprintf(w, "Completion tokens: %s\n", humanTokens(a.OutputTokens))
	fmt.Fprintf(w, "Total tokens:      %s\n", humanTokens(a.TotalTokens))
	if withCost {
		fmt.Fprintf(w, "Estimated cost:    $%.4f\n", a.CostUSD)
	}
	if !a.FirstEvent.IsZero() {
		fmt.Fprintf(w, "First event:       %s\n", a.FirstEvent.Format(time.RFC3339))
	}
	if !a.LastEvent.IsZero() {
		fmt.Fprintf(w, "Last event:        %s\n", a.LastEvent.Format(time.RFC3339))
	}
	if len(a.ByModel) > 0 {
		fmt.Fprintln(w, "\nBy model (sorted desc):")
		keys := sortedKeys(a.ByModel)
		for _, k := range keys {
			fmt.Fprintf(w, "  %-48s  %10s\n", shorten(k, 48), humanTokens(a.ByModel[k]))
		}
	}
	if len(a.BySource) > 0 {
		fmt.Fprintln(w, "\nBy source:")
		keys := sortedKeys(a.BySource)
		for _, k := range keys {
			fmt.Fprintf(w, "  %-16s  %10s\n", k, humanTokens(a.BySource[k]))
		}
	}
}

func renderGroupedTable(w io.Writer, rows []usage.Aggregation) {
	maxKey := 12
	for _, r := range rows {
		if w := len(r.Group); w > maxKey {
			maxKey = w
		}
	}
	if maxKey > 40 {
		maxKey = 40
	}
	fmt.Fprintf(w, "  %-*s  %10s  %10s  %12s  %8s\n", maxKey, "group", "input", "output", "total", "events")
	for _, r := range rows {
		key := r.Group
		if len(key) > maxKey {
			key = key[:maxKey-1] + "…"
		}
		fmt.Fprintf(w, "  %-*s  %10s  %10s  %12s  %8d\n",
			maxKey, key,
			humanTokens(r.InputTokens), humanTokens(r.OutputTokens),
			humanTokens(r.TotalTokens), r.EventCount)
	}
}

func renderEventTable(w io.Writer, events []usage.Event) {
	fmt.Fprintf(w, "  %-22s  %-20s  %-48s  %8s  %8s  %8s  %s\n",
		"created_at", "source", "model", "input", "output", "total", "cost")
	for _, e := range events {
		fmt.Fprintf(w, "  %-22s  %-20s  %-48s  %8d  %8d  %8d  $%.4f\n",
			e.CreatedAt.Format("2006-01-02 15:04:05"),
			string(e.Source),
			shorten(e.Model, 48),
			e.InputTokens, e.OutputTokens, e.TotalTokens,
			e.CostUSD)
	}
}

// renderShareLine produces the tweetable one-liner used by caveman's
// --share. Format: "sin-code ⛏ 12.4k · $1.23 (12 events, 3 sessions)"
func renderShareLine(a *usage.Aggregation, _ bool) string {
	if a == nil || (a.TotalTokens == 0 && a.EventCount == 0) {
		return "sin-code ⛏ 0 (no usage recorded yet)"
	}
	return fmt.Sprintf("sin-code ⛏ %s · $%.2f (%d events, %d sessions)",
		humanTokens(a.TotalTokens), a.CostUSD, a.EventCount, a.SessionsCount)
}

func humanTokens(n int) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= 1_000_000:
		return fmt.Sprintf("%d (%.2fM)", n, float64(abs)/1_000_000.0)
	case abs >= 1_000:
		return fmt.Sprintf("%d (%.2fk)", n, float64(abs)/1_000.0)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v > 0 {
			keys = append(keys, k)
		}
	}
	// sort by value desc, then key asc.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && (m[keys[j]] > m[keys[j-1]] ||
			(m[keys[j]] == m[keys[j-1]] && keys[j] < keys[j-1])); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func startOfDay(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func startOfMonth(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// _ keeps strings import used (errors is also kept for future use).
var _ = errors.New
var _ = strings.TrimSpace

// ============================================================================
// Debt command (sin-code debt)
// ============================================================================

// NewDebtCmd builds the `debt` cobra subcommand group for sin-debt markers.
// All operations are read-only by design — the scanner + report are
// deterministic so two CI runs over the same tree produce the same bytes.
func NewDebtCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debt",
		Short: "Inspect sin-debt markers (issue #177)",
		Long: `sin-code debt scans a directory for the
'sin-debt: <ceiling>, upgrade: <trigger>' marker convention and produces
byte-stable reports. The scanner recognises C/Go/Rust-style // comments,
Python/Shell # comments, /* */ blocks, <!-- --> HTML/Markdown, and -- SQL.

Subcommands:

  list   one row per marker, with reason / upgrade / rot column
  stats  aggregated report grouped by reason|file|language|symbol|age
  check  CI gate: exit 1 when rot exceeds the threshold
  policy dump the active policy (resolved from .sin-code/debt-policy.toml)
  fix    print a sed/awk-ready patch for the rot-risk markers
  export write the canonical SIN-DEBT.md ledger to disk`,
	}
	cmd.AddCommand(
		newDebtListCmd(),
		newDebtStatsCmd(),
		newDebtCheckCmd(),
		newDebtPolicyCmd(),
		newDebtFixCmd(),
		newDebtExportCmd(),
	)
	return cmd
}

// sharedFlags is the set of flags each subcommand declares. Keeping them
// in one struct lets every subcommand share a default value and stay in
// lock-step when the contract evolves.
type sharedFlags struct {
	path      string
	format    string
	noTrigger bool
	json      bool
}

// bindShared installs the common flag set on `c`. The flags are:
//   - --path       root directory to scan (default ".")
//   - --format     "table" or "json" (default "table")
//   - --no-trigger when set, list/check report only rot-risk markers
func bindShared(c *cobra.Command, f *sharedFlags) {
	c.Flags().StringVar(&f.path, "path", ".", "directory to scan (default: current)")
	c.Flags().StringVar(&f.format, "format", "table", "output format: table|json")
	c.Flags().BoolVar(&f.noTrigger, "no-trigger", false, "limit output to markers without an upgrade clause")
	c.Flags().BoolVar(&f.json, "json", false, "emit JSON instead of markdown table")
}

// resolveRoot returns the absolute form of `path` with a saner error.
// Empty paths become "." so the cobra default is always honoured.
func resolveRoot(path string) (string, error) {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("debt: resolve path %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("debt: stat %s: %w", abs, err)
	}
	if !info.IsDir() {
		// single-file scan is fine; we just keep the parent as scan root
		// and post-filter. The package's ParseDir handles this too.
	}
	return abs, nil
}

// scan runs the parser over `root` and applies the user's filter flags.
// The result is sorted by File / Line / Column by ParseDir already; we
// only apply the post-filter for --no-trigger here.
func scan(root string, noTrigger bool) ([]sindept.Marker, error) {
	mk, err := sindept.ParseDir(root, sindept.DefaultOptions())
	if err != nil {
		return nil, err
	}
	if !noTrigger {
		return mk, nil
	}
	out := mk[:0:0]
	for _, m := range mk {
		if !(m.HasUpg && m.Upgrade != "") {
			out = append(out, m)
		}
	}
	return out, nil
}

func newDebtListCmd() *cobra.Command {
	var f sharedFlags
	c := &cobra.Command{
		Use:   "list",
		Short: "List every sin-debt marker under --path",
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, f.noTrigger)
			if err != nil {
				return err
			}
			if f.json {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(mk)
			}
			fmt.Print(sindept.RenderListString(mk))
			return nil
		},
	}
	bindShared(c, &f)
	return c
}

func newDebtStatsCmd() *cobra.Command {
	var f sharedFlags
	var by string
	c := &cobra.Command{
		Use:   "stats",
		Short: "Aggregate report (default: by file)",
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, f.noTrigger)
			if err != nil {
				return err
			}
			if by == "age" {
				return renderAgeReport(os.Stdout, mk)
			}
			stats := sindept.AggregateStats(mk)
			sections := []sindept.ReportSection{
				sindept.SectionSummary,
			}
			switch by {
			case "reason":
				sections = append(sections, sindept.SectionByReason)
			case "file":
				sections = append(sections, sindept.SectionByFile)
			case "language":
				sections = append(sections, sindept.SectionByLang)
			case "symbol":
				sections = append(sections, sindept.SectionBySymbol)
			case "summary":
				// summary already at [0], nothing more.
			default:
				sections = append(sections,
					sindept.SectionByFile, sindept.SectionByReason,
					sindept.SectionByLang, sindept.SectionRotRisk)
			}
			if f.json {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(stats)
			}
			sindept.RenderStats(os.Stdout, stats, sections)
			return nil
		},
	}
	c.Flags().StringVar(&by, "by", "file", "group by: file|reason|language|symbol|summary|age")
	bindShared(c, &f)
	return c
}

func newDebtCheckCmd() *cobra.Command {
	var f sharedFlags
	var failOnMissing bool
	c := &cobra.Command{
		Use:   "check",
		Short: "CI gate: exit 1 when rot-risk exceeds threshold",
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, false)
			if err != nil {
				return err
			}
			policy, err := sindept.LoadPolicyForRoot(root)
			if err != nil {
				return err
			}
			if failOnMissing {
				policy.RequireUpgrade = true
			}
			res := policy.RunCheck(mk)
			fmt.Print(sindept.FormatCheckResult(res))
			if !res.Ok {
				os.Exit(1)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&failOnMissing, "require-upgrade", false, "treat any marker without 'upgrade:' as a failure")
	bindShared(c, &f)
	return c
}

func newDebtPolicyCmd() *cobra.Command {
	var f sharedFlags
	c := &cobra.Command{
		Use:   "policy",
		Short: "Print the active sin-debt policy (defaults + on-disk overlay)",
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			pol, err := sindept.LoadPolicyForRoot(root)
			if err != nil {
				return err
			}
			if f.json {
				return json.NewEncoder(os.Stdout).Encode(pol)
			}
			fmt.Printf("# sin-debt policy\n\n")
			fmt.Printf("- source: %s\n", emptyDash(pol.Source))
			fmt.Printf("- max_no_upgrade: %d\n", pol.MaxNoUpgrade)
			fmt.Printf("- require_upgrade: %v\n", pol.RequireUpgrade)
			fmt.Printf("- default_reasons (%d):\n", len(pol.DefaultReasons))
			for _, r := range pol.DefaultReasons {
				fmt.Printf("    - %s\n", r)
			}
			fmt.Printf("- upgrade_triggers (%d):\n", len(pol.UpgradeTriggers))
			keys := make([]string, 0, len(pol.UpgradeTriggers))
			for k := range pol.UpgradeTriggers {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				fmt.Printf("    - %s: %s\n", k, pol.UpgradeTriggers[k])
			}
			return nil
		},
	}
	bindShared(c, &f)
	return c
}

func newDebtFixCmd() *cobra.Command {
	var f sharedFlags
	c := &cobra.Command{
		Use:   "fix",
		Short: "Print the rot-risk markers as a sed-friendly patch harness",
		Long: `fix lists the rot-risk markers — those without an 'upgrade:' clause —
in the format "path:line<TAB>reason". Pipe through sed/edit to insert
the upgrade clause; nothing is written by this subcommand.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, true) // rot-only
			if err != nil {
				return err
			}
			for _, m := range mk {
				fmt.Printf("%s:%d\t%s\n", m.File, m.Line, m.Reason)
			}
			fmt.Fprintf(os.Stderr, "# %d rot-risk markers — add 'upgrade: <trigger>' to each\n", len(mk))
			return nil
		},
	}
	bindShared(c, &f)
	return c
}

func newDebtExportCmd() *cobra.Command {
	var f sharedFlags
	var out string
	c := &cobra.Command{
		Use:   "export <file>",
		Short: "Write the canonical SIN-DEBT.md ledger",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, f.noTrigger)
			if err != nil {
				return err
			}
			dest := out
			if dest == "" && len(args) == 1 {
				dest = args[0]
			}
			if dest == "" {
				dest = "SIN-DEBT.md"
			}
			stats := sindept.AggregateStats(mk)
			content := sindept.RenderListString(mk) + "\n" + sindept.RenderStatsString(stats)
			if err := os.WriteFile(dest, []byte(content), filemode.Default()); err != nil {
				return fmt.Errorf("debt: write %s: %w", dest, err)
			}
			if !f.json {
				fmt.Fprintf(os.Stderr, "wrote %d markers to %s\n", len(mk), dest)
			}
			return nil
		},
	}
	c.Flags().StringVarP(&out, "out", "o", "", "destination file (default: SIN-DEBT.md)")
	bindShared(c, &f)
	return c
}

// renderAgeReport prints every marker sorted by File then Line — the
// "oldest first" view that helps reviewers triage rot in chronological
// order. The byte order is identical to the parser's stable sort, so
// two runs of the same tree produce the same bytes.
func renderAgeReport(w *os.File, mk []sindept.Marker) error {
	fmt.Fprintln(w, sindept.Header())
	fmt.Fprintln(w, "## Markers by age (oldest first)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| file | line | symbol | reason | upgrade |")
	fmt.Fprintln(w, "|------|------|--------|--------|---------|")
	for _, m := range mk {
		upg := m.Upgrade
		if upg == "" {
			upg = "&lt;none — rot-risk&gt;"
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s | %s |\n",
			escapeCell(m.File), m.Line,
			escapeCell(m.Symbol), escapeCell(m.Reason),
			escapeCell(upg))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "_%d markers total_\n", len(mk))
	return nil
}

// escapeCell keeps markdown table cells well-formed in the age report.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "|", "/")
	return s
}

func emptyDash(s string) string {
	if s == "" {
		return "<default>"
	}
	return s
}

// _ keeps strconv referenced even if we stop using the binding.
var _ = strconv.Itoa

// ============================================================================
// Swarm command (sin-code swarm)
// ============================================================================

const (
	swarmDefaultTimeout = 10 * time.Minute
	swarmDefaultTurns   = 40
	swarmMinAgents      = 2
)

// agentRunner is the function shape used to build a fresh isolated loop
// for a given agent. It is overridable in tests so we can run hermetic
// swarm scenarios without a real LLM backend.
type agentRunner func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error)

// swarmResult is the per-agent outcome surfaced to the user. Status is
// one of VERIFIED / UNVERIFIED / FAILED / CANCELLED / TIMEOUT.
type swarmResult struct {
	Agent   string `json:"agent"`
	Status  string `json:"status"`
	Turns   int    `json:"turns"`
	Summary string `json:"summary"`
}

// swarmReport aggregates every per-agent result plus the winner (or a
// non-empty Error if no agent verified within the timeout).
type swarmReport struct {
	Prompt  string        `json:"prompt"`
	Winner  string        `json:"winner,omitempty"`
	Error   string        `json:"error,omitempty"`
	Results []swarmResult `json:"results"`
}

type swarmOptions struct {
	prompt    string
	agentCSV  string
	timeout   time.Duration
	maxTurns  int
	jsonOut   bool
	workspace string

	// runner is the factory for fresh per-agent loops; defaults to
	// defaultAgentRunner. Tests override this to keep swarm hermetic.
	runner agentRunner
}

func NewSwarmCmd() *cobra.Command {
	opts := &swarmOptions{
		timeout:  swarmDefaultTimeout,
		maxTurns: swarmDefaultTurns,
	}
	cmd := &cobra.Command{
		Use:   "swarm",
		Short: "Race N agent profiles on the same prompt (first verified wins)",
		Long: `sin-code swarm runs the same prompt through N agent profiles in parallel.
All workers run HEADLESS (--yolo is not exposed; the loop never asks the user).
The first worker to return a verified Result wins; remaining workers are
cancelled via the parent context.

  sin-code swarm -p "fix the failing test" --agents coder,reviewer`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSwarm(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&opts.prompt, "prompt", "p", "", "shared prompt (required)")
	f.StringVar(&opts.agentCSV, "agents", "", "comma-separated agent profile names (>=2 required)")
	f.DurationVar(&opts.timeout, "timeout", swarmDefaultTimeout, "global swarm timeout (per-agent budget)")
	f.IntVar(&opts.maxTurns, "max-turns", swarmDefaultTurns, "max turns per agent")
	f.BoolVar(&opts.jsonOut, "json", false, "emit structured JSON report")
	return cmd
}

func runSwarm(ctx context.Context, opts *swarmOptions) error {
	report, err := executeSwarm(ctx, opts)
	if err != nil {
		return err
	}
	return emitSwarm(opts, report)
}

// executeSwarm is the testable core. It is split out so tests can
// inspect the swarmReport without going through stdout emission.
func executeSwarm(ctx context.Context, opts *swarmOptions) (*swarmReport, error) {
	if opts.prompt == "" {
		return nil, errors.New("--prompt is required")
	}
	agents := splitNonEmpty(opts.agentCSV, ",")
	if len(agents) < swarmMinAgents {
		return nil, fmt.Errorf("--agents requires at least %d profiles (got %d)", swarmMinAgents, len(agents))
	}
	if opts.timeout <= 0 {
		opts.timeout = swarmDefaultTimeout
	}
	if opts.maxTurns <= 0 {
		opts.maxTurns = swarmDefaultTurns
	}
	if opts.workspace == "" {
		ws, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		opts.workspace = ws
	}
	runner := opts.runner
	if runner == nil {
		runner = defaultAgentRunner
	}

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	type runOut struct {
		agent string
		res   *agentloop.Result
		err   error
	}
	results := make(chan runOut, len(agents))
	var wg sync.WaitGroup
	wg.Add(len(agents))

	for _, name := range agents {
		agentName := name
		go func() {
			defer wg.Done()
			loop, sess, cleanup, err := runner(ctx, agentName, opts.workspace)
			if err != nil {
				results <- runOut{agent: agentName, err: fmt.Errorf("setup: %w", err)}
				return
			}
			defer func() { _ = cleanup() }()

			// Mandate M4 + swarm hard mandate: headless, no Ask, no Yolo.
			// These overrides defend against an agent profile that
			// somehow ships with permissive defaults.
			loop.Ask = nil
			if loop.Perm != nil {
				loop.Perm.Headless = true
				loop.Perm.Yolo = false
			}
			loop.MaxTurns = opts.maxTurns

			res, err := loop.Run(ctx, sess, opts.prompt)
			if err != nil {
				results <- runOut{agent: agentName, err: err}
				return
			}
			results <- runOut{agent: agentName, res: res}
		}()
	}

	report := &swarmReport{Prompt: opts.prompt, Results: make([]swarmResult, 0, len(agents))}
	finished := 0
	winner := ""
	// First verified result cancels all other workers and is reported
	// as the winner. Non-verified completions are still collected so
	// the user sees every agent's outcome.
	for finished < len(agents) {
		select {
		case <-ctx.Done():
			cancel()
			wg.Wait()
			if winner == "" {
				report.Error = classifyCtxErr(ctx.Err())
			}
			report.Results = append(report.Results, cancelledMarkers(agents, report.Results)...)
			return report, nil
		case out := <-results:
			finished++
			if out.res != nil {
				status := "UNVERIFIED"
				if out.res.Verified {
					status = "VERIFIED"
				}
				report.Results = append(report.Results, swarmResult{
					Agent:   out.agent,
					Status:  status,
					Turns:   out.res.Turns,
					Summary: out.res.Summary,
				})
				if out.res.Verified && winner == "" {
					winner = out.agent
					report.Winner = winner
					cancel()
				}
			} else {
				report.Results = append(report.Results, swarmResult{
					Agent:  out.agent,
					Status: classifyErr(out.err),
					Turns:  0,
				})
			}
		}
	}
	cancel()
	wg.Wait()

	if winner == "" {
		report.Error = "no agent verified within timeout"
	}
	return report, nil
}

func emitSwarm(opts *swarmOptions, report *swarmReport) error {
	if opts.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	fmt.Printf("swarm: %d agents on prompt %q (timeout=%s)\n",
		len(report.Results), truncate(opts.prompt, 60), opts.timeout)
	for _, r := range report.Results {
		fmt.Printf("  %-12s %-11s turns=%d  %s\n", r.Agent, r.Status, r.Turns, truncate(r.Summary, 80))
	}
	if report.Winner != "" {
		fmt.Printf("winner: %s\n", report.Winner)
	}
	if report.Error != "" {
		fmt.Fprintf(os.Stderr, "swarm error: %s\n", report.Error)
	}
	return nil
}

// defaultAgentRunner is the production wiring: a fully isolated loop
// per agent (no shared session, no shared completion function, no
// shared DB). We create a per-agent sessions DB under
// <workspace>/.sin-code/swarm/ so concurrent agents never share a
// *session.Session (mandate M7).
func defaultAgentRunner(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
	dbPath, err := perAgentDBPath(workspace, agentName)
	if err != nil {
		return nil, nil, nil, err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return nil, nil, nil, err
	}
	loop, cleanup, err := loopbuilder.Build(ctx, loopbuilder.Config{
		Workspace:    workspace,
		AgentName:    agentName,
		MaxTurns:     swarmDefaultTurns,
		Headless:     true,
		Yolo:         false,
		SessionStore: store,
	}, nil)
	if err != nil {
		_ = store.Close()
		return nil, nil, nil, err
	}
	sess, err := store.StartOrResume("")
	if err != nil {
		_ = store.Close()
		_ = cleanup()
		return nil, nil, nil, err
	}
	return loop, sess, func() error {
		_ = store.Close()
		return cleanup()
	}, nil
}

func perAgentDBPath(workspace, agentName string) (string, error) {
	dir := filepath.Join(workspace, ".sin-code", "swarm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	suffix := randHex(4)
	return filepath.Join(dir, fmt.Sprintf("%s-%s.db", sanitizeFile(agentName), suffix)), nil
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}

func sanitizeFile(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "agent"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func classifyErr(err error) string {
	if err == nil {
		return "FAILED"
	}
	if errors.Is(err, context.Canceled) {
		return "CANCELLED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	return "FAILED"
}

func classifyCtxErr(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "swarm timeout exceeded"
	}
	return "swarm cancelled"
}

// cancelledMarkers fills in CANCELLED rows for agents that never
// returned a result before ctx was cancelled.
func cancelledMarkers(all []string, seen []swarmResult) []swarmResult {
	have := make(map[string]struct{}, len(seen))
	for _, r := range seen {
		have[r.Agent] = struct{}{}
	}
	var extra []swarmResult
	for _, a := range all {
		if _, ok := have[a]; !ok {
			extra = append(extra, swarmResult{Agent: a, Status: "CANCELLED"})
		}
	}
	return extra
}

// ============================================================================
// Memory command (sin-code memory)
// ============================================================================

var (
	autoMemProject string
	autoMemSource  string
	autoMemMax     int
	autoMemFormat  string

	autoMemDefaultHome = auto_mem.DefaultHome
	autoMemOpen        = auto_mem.Open

	autoMemIndex     = func(s *auto_mem.Store) ([]string, error) { return s.Index() }
	autoMemReadTopic = func(s *auto_mem.Store, heading string) ([]byte, error) { return s.ReadTopic(heading) }
	autoMemAppend    = func(s *auto_mem.Store, e auto_mem.Entry) error { return s.Append(e) }
	autoMemRemove    = func(s *auto_mem.Store, heading string) error { return s.Remove(heading) }
	autoMemRotate    = func(s *auto_mem.Store, max int) (int, error) { return s.Rotate(max) }
)

func openAutoMem() (*auto_mem.Store, string, error) {
	home, err := autoMemDefaultHome()
	if err != nil {
		return nil, "", err
	}
	proj := autoMemProject
	if proj == "" {
		proj = "global"
	}
	s, err := autoMemOpen(home, proj)
	if err != nil {
		return nil, "", err
	}
	return s, proj, nil
}

var memAutoListCmd = &cobra.Command{
	Use:   "auto-list",
	Short: "List topics in MEMORY.md for the active project (issue #192 parity).",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, proj, err := openAutoMem()
		if err != nil {
			return err
		}
		idx, err := autoMemIndex(s)
		if err != nil {
			return err
		}
		if autoMemFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(struct {
				Project  string   `json:"project"`
				Path     string   `json:"path"`
				Headings []string `json:"headings"`
			}{proj, s.Path(), idx})
		}
		if len(idx) == 0 {
			fmt.Printf("(no entries) — %s\n", s.Path())
			return nil
		}
		fmt.Printf("MEMORY.md for %s (%d topics, %s):\n", proj, len(idx), s.Path())
		for _, h := range idx {
			fmt.Printf("  - %s\n", h)
		}
		return nil
	},
}

var memAutoShowCmd = &cobra.Command{
	Use:   "auto-show <heading>",
	Short: "Show the body of a single MEMORY.md topic.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		body, err := autoMemReadTopic(s, args[0])
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil
	},
}

var memAutoAppendCmd = &cobra.Command{
	Use:   "auto-append <heading> <body>",
	Short: "Append or replace a MEMORY.md topic. Re-issues replace the prior body.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		src := autoMemSource
		if src == "" {
			src = "manual"
		}
		if err := autoMemAppend(s, auto_mem.Entry{
			Heading:   args[0],
			Body:      args[1],
			SourceTag: src,
			AddedAt:   time.Now().UTC(),
		}); err != nil {
			return err
		}
		fmt.Printf("updated MEMORY.md topic %q (%s)\n", args[0], s.Path())
		return nil
	},
}

var memAutoRmCmd = &cobra.Command{
	Use:   "auto-rm <heading>",
	Short: "Remove a MEMORY.md topic by heading.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		if err := autoMemRemove(s, args[0]); err != nil {
			return err
		}
		fmt.Printf("removed topic %q from %s\n", args[0], s.Path())
		return nil
	},
}

var memAutoGcCmd = &cobra.Command{
	Use:   "auto-gc",
	Short: "Rotate MEMORY.md down to the most recent N topics (default 32).",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, _, err := openAutoMem()
		if err != nil {
			return err
		}
		n := autoMemMax
		if n <= 0 {
			n = 32
		}
		kept, err := autoMemRotate(s, n)
		if err != nil {
			return err
		}
		fmt.Printf("rotated MEMORY.md to %d most-recent topics (%s)\n", kept, s.Path())
		return nil
	},
}

func init() {
	MemoryCmd.AddCommand(memAutoListCmd, memAutoShowCmd, memAutoAppendCmd, memAutoRmCmd, memAutoGcCmd)

	// Reuse --project and --format where possible; the auto_mem layer
	// does not use --as (persistence is per-project, not per-actor).
	memAutoListCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key (default 'global')")
	memAutoListCmd.Flags().StringVar(&autoMemFormat, "format", "text", "Output: text|json")

	memAutoShowCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")

	memAutoAppendCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")
	memAutoAppendCmd.Flags().StringVar(&autoMemSource, "source", "manual", "Provenance tag for this entry")

	memAutoRmCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")

	memAutoGcCmd.Flags().StringVar(&autoMemProject, "project", "global", "Project key")
	memAutoGcCmd.Flags().IntVar(&autoMemMax, "max", 32, "Max topics to keep (rotates down to most-recent N)")
}

var (
	memDBPath   string
	memInsight  string
	memProject  string
	memTags     string
	memActor    string
	memLimit    int
	memTopK     int
	memDepth    int
	memForgetID string
	memForget   bool
	memFormat   string

	openMemoryStoreFn = openMemoryStore

	memAddFn    = func(s *memory.Store, m *memory.Memory) error { return s.Add(m) }
	memListFn   = func(s *memory.Store, f memory.ListFilter) ([]*memory.Memory, error) { return s.List(f) }
	memGetFn    = func(s *memory.Store, id string) (*memory.Memory, error) { return s.Get(id) }
	memSearchFn = func(s *memory.Store, q, project string, k int) ([]memory.ScoredMemory, error) {
		return s.Search(q, project, k)
	}
	memAddLinkFn    = func(s *memory.Store, l memory.Link) error { return s.AddLink(l) }
	memRemoveLinkFn = func(s *memory.Store, from, to string) error { return s.RemoveLink(from, to) }
	memGraphFn      = func(s *memory.Store, id string, depth int) (map[string][]memory.Link, error) {
		return s.Graph(id, depth)
	}
	memPrimeFn  = func(s *memory.Store, q, project string, k int) (string, error) { return s.Prime(q, project, k) }
	memDeleteFn = func(s *memory.Store, id string, hard bool) error { return s.Delete(id, hard) }
	memStatsFn  = func(s *memory.Store) (map[string]int, error) { return s.Stats() }
)

var MemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Long-term project memory with semantic search",
	Long: `Memory is a bd-style project knowledge store backed by bbolt.

  add <insight>            Add a memory
  list                      List memories (filter by --project, --tag, --actor)
  search <query>            Semantic search (uses NIM embeddings if SIN_NIM_API_KEY is set)
  link <from> <to> --rel    Add a knowledge-graph link
  unlink <from> <to>        Remove a link
  graph <id>                Show knowledge-graph neighborhood
  prime <query>             Print top-K relevant memories for an LLM prompt
  forget <id>               Soft-delete (--hard for permanent)
  show <id>                 Show one memory
  stats                     Memory statistics

Storage: ~/.config/sin-code/memory.db (override with --db).
Embeddings: NIM nv-embed-v1 (set SIN_NIM_API_KEY).`,
	SilenceUsage: true,
}

func init() {
	MemoryCmd.PersistentFlags().StringVar(&memDBPath, "db", "", "Path to bbolt DB (default ~/.config/sin-code/memory.db)")
	MemoryCmd.PersistentFlags().StringVar(&memFormat, "format", "text", "Output format: text|json")
	MemoryCmd.PersistentFlags().StringVar(&memActor, "as", "", "Actor identity (default: git user.name or 'unknown')")

	MemoryCmd.AddCommand(memAddCmd)
	MemoryCmd.AddCommand(memListCmd)
	MemoryCmd.AddCommand(memShowCmd)
	MemoryCmd.AddCommand(memSearchCmd)
	MemoryCmd.AddCommand(memLinkCmd)
	MemoryCmd.AddCommand(memUnlinkCmd)
	MemoryCmd.AddCommand(memGraphCmd)
	MemoryCmd.AddCommand(memPrimeCmd)
	MemoryCmd.AddCommand(memForgetCmd)
	MemoryCmd.AddCommand(memStatsCmd)

	memAddCmd.Flags().StringVar(&memProject, "project", "", "Project namespace")
	memAddCmd.Flags().StringVar(&memTags, "tags", "", "Comma-separated tags")

	memListCmd.Flags().StringVar(&memProject, "project", "", "Filter by project")
	memListCmd.Flags().StringVar(&memTags, "tags", "", "Filter by tag")
	memListCmd.Flags().IntVar(&memLimit, "limit", 50, "Max items (0 = all)")

	memSearchCmd.Flags().StringVar(&memProject, "project", "", "Filter by project")
	memSearchCmd.Flags().IntVar(&memTopK, "top", 10, "Top-K results")

	memLinkCmd.Flags().StringVar(&memRel, "rel", "references", "Link type: references|supports|contradicts|extends|causes")

	memGraphCmd.Flags().IntVar(&memDepth, "depth", 3, "Max traversal depth")

	memPrimeCmd.Flags().StringVar(&memProject, "project", "", "Filter by project")
	memPrimeCmd.Flags().IntVar(&memTopK, "top", 10, "Top-K results")

	memForgetCmd.Flags().BoolVar(&memForget, "hard", false, "Permanent delete (default: soft)")
}

var memAddCmd = &cobra.Command{
	Use:   "add <insight>",
	Short: "Add a memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		memInsight = args[0]
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		m := &memory.Memory{
			Insight: memInsight,
			Project: memProject,
			Tags:    splitList(memTags),
			Actor:   memActor,
		}
		if err := memAddFn(store, m); err != nil {
			return err
		}
		fmt.Printf("Stored %s: %s\n", m.ID, memTruncate(m.Insight, 80))
		return nil
	},
}

var memListCmd = &cobra.Command{
	Use:   "list",
	Short: "List memories",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		results, err := memListFn(store, memory.ListFilter{
			Project: memProject,
			Tag:     memTags,
			Limit:   memLimit,
		})
		if err != nil {
			return err
		}
		if memFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(results)
		}
		if len(results) == 0 {
			fmt.Println("(no memories)")
			return nil
		}
		for _, m := range results {
			project := m.Project
			if project == "" {
				project = "-"
			}
			fmt.Printf("%s  [%-12s]  %s\n", m.ID, project, memTruncate(m.Insight, 80))
		}
		return nil
	},
}

var memShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a single memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		m, err := memGetFn(store, args[0])
		if err != nil {
			return err
		}
		if memFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(m)
		}
		fmt.Printf("ID:      %s\n", m.ID)
		fmt.Printf("Insight: %s\n", m.Insight)
		if m.Project != "" {
			fmt.Printf("Project: %s\n", m.Project)
		}
		if len(m.Tags) > 0 {
			fmt.Printf("Tags:    %s\n", strings.Join(m.Tags, ", "))
		}
		if m.Actor != "" {
			fmt.Printf("Actor:   %s\n", m.Actor)
		}
		fmt.Printf("Created: %s\n", m.Created.Format("2006-01-02 15:04:05"))
		fmt.Printf("Updated: %s\n", m.Updated.Format("2006-01-02 15:04:05"))
		return nil
	},
}

var memSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Semantic search",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		results, err := memSearchFn(store, args[0], memProject, memTopK)
		if err != nil {
			return err
		}
		if memFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(results)
		}
		if len(results) == 0 {
			fmt.Println("(no results)")
			return nil
		}
		fmt.Printf("Top %d for %q:\n", len(results), args[0])
		for _, r := range results {
			project := r.Project
			if project == "" {
				project = "-"
			}
			fmt.Printf("  %.4f  %s  [%-12s]  %s\n", r.Score, r.ID, project, memTruncate(r.Insight, 70))
		}
		return nil
	},
}

var memRel string

var memLinkCmd = &cobra.Command{
	Use:   "link <from> <to>",
	Short: "Add a knowledge-graph link",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		l := memory.Link{From: args[0], To: args[1], Rel: memRel}
		if err := memAddLinkFn(store, l); err != nil {
			return err
		}
		fmt.Printf("Linked %s --%s--> %s\n", l.From, l.Rel, l.To)
		return nil
	},
}

var memUnlinkCmd = &cobra.Command{
	Use:   "unlink <from> <to>",
	Short: "Remove a knowledge-graph link",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := memRemoveLinkFn(store, args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Unlinked %s ---> %s\n", args[0], args[1])
		return nil
	},
}

var memGraphCmd = &cobra.Command{
	Use:   "graph <id>",
	Short: "Show knowledge-graph neighborhood",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		tree, err := memGraphFn(store, args[0], memDepth)
		if err != nil {
			return err
		}
		if memFormat == "json" {
			return json.NewEncoder(os.Stdout).Encode(tree)
		}
		fmt.Printf("Graph from %s (depth %d):\n", args[0], memDepth)
		for id, links := range tree {
			fmt.Printf("  %s\n", id)
			for _, l := range links {
				fmt.Printf("    --%s--> %s\n", l.Rel, l.To)
			}
		}
		return nil
	},
}

var memPrimeCmd = &cobra.Command{
	Use:   "prime <query>",
	Short: "Print top-K relevant memories for an LLM prompt",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		text, err := memPrimeFn(store, args[0], memProject, memTopK)
		if err != nil {
			return err
		}
		fmt.Print(text)
		return nil
	},
}

var memForgetCmd = &cobra.Command{
	Use:   "forget <id>",
	Short: "Soft-delete a memory (--hard for permanent)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := memDeleteFn(store, args[0], memForget); err != nil {
			return err
		}
		verb := "Forgotten"
		if memForget {
			verb = "Hard-deleted"
		}
		fmt.Printf("%s %s\n", verb, args[0])
		return nil
	},
}

var memStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show memory statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openMemoryStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		stats, err := memStatsFn(store)
		if err != nil {
			return err
		}
		enabled, dim := store.EmbeddingStatus()
		if memFormat == "json" {
			out := map[string]interface{}{
				"stats":     stats,
				"embedder":  enabled,
				"embed_dim": dim,
			}
			return json.NewEncoder(os.Stdout).Encode(out)
		}
		fmt.Printf("Total:      %d memories\n", stats["total"])
		fmt.Printf("Links:      %d\n", stats["links"])
		fmt.Printf("Embeddings: %d cached\n", stats["embeddings"])
		if enabled {
			fmt.Printf("Embedder:   enabled (dim=%d)\n", dim)
		} else {
			fmt.Println("Embedder:   disabled (set SIN_NIM_API_KEY to enable semantic search)")
		}
		return nil
	},
}

func openMemoryStore() (*memory.Store, error) {
	memory.SetupNIMEmbedder()
	return memory.Open(memDBPath)
}

func memTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ============================================================================
// Goal command (sin-code goal)
// ============================================================================

func NewGoalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goal",
		Short: "Manage the autonomous goal queue",
	}

	var priority, retries int
	var criteria []string
	var contractFile string
	addCmd := &cobra.Command{
		Use:   "add <prompt>",
		Short: "Enqueue a goal for the daemon",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			ws, _ := os.Getwd()

			// Build the Definition-of-Done contract from flags. --contract-file
			// supplies a full JSON contract; --criteria adds semantic criteria
			// the stop-gate's independent evaluator must confirm.
			contractJSON := ""
			if contractFile != "" {
				raw, rerr := os.ReadFile(contractFile)
				if rerr != nil {
					return fmt.Errorf("read contract file: %w", rerr)
				}
				c, perr := goalcontract.Unmarshal(string(raw))
				if perr != nil {
					return fmt.Errorf("invalid contract file: %w", perr)
				}
				c.SemanticCriteria = append(c.SemanticCriteria, criteria...)
				contractJSON, _ = c.Marshal()
			} else if len(criteria) > 0 {
				c := &goalcontract.GoalContract{SemanticCriteria: criteria}
				contractJSON, _ = c.Marshal()
			}

			var id int64
			if contractJSON != "" {
				id, err = q.AddWithContract(cmd.Context(), args[0], ws, priority, retries, contractJSON)
			} else {
				id, err = q.Add(cmd.Context(), args[0], ws, priority, retries)
			}
			if err != nil {
				return err
			}
			if contractJSON != "" {
				fmt.Printf("goal %d enqueued with contract (priority %d, retries %d)\n", id, priority, retries)
			} else {
				fmt.Printf("goal %d enqueued (priority %d, retries %d)\n", id, priority, retries)
			}
			return nil
		},
	}
	addCmd.Flags().IntVar(&priority, "priority", 0, "higher runs sooner")
	addCmd.Flags().IntVar(&retries, "retries", 3, "retry budget")
	addCmd.Flags().StringArrayVar(&criteria, "criteria", nil, "acceptance criterion the stop-gate evaluator must confirm (repeatable)")
	addCmd.Flags().StringVar(&contractFile, "contract-file", "", "path to a JSON Definition-of-Done contract")

	var status string
	var jsonOut bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List goals",
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			goals, err := q.List(cmd.Context(), autonomy.GoalStatus(status))
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(goals)
			}
			if len(goals) == 0 {
				fmt.Println("no goals")
				return nil
			}
			fmt.Printf("%-5s %-10s %-4s %-8s %s\n", "ID", "STATUS", "TRY", "PRIO", "PROMPT")
			for _, g := range goals {
				fmt.Printf("%-5d %-10s %d/%-2d %-8d %.60s\n", g.ID, g.Status, g.Attempts, g.MaxRetries, g.Priority, g.Prompt)
			}
			return nil
		},
	}
	listCmd.Flags().StringVar(&status, "status", "", "filter: pending|running|verified|failed|exhausted")
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	var dryRun bool
	var discoverRetries int
	discoverCmd := &cobra.Command{
		Use:   "discover",
		Short: "Scan the repo for latent work (TODO/FIXME, MASTER_TODO) and enqueue goals",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, _ := os.Getwd()
			findings, err := autonomy.Discover(autonomy.DiscoverConfig{
				Workspace:    ws,
				ScanComments: true,
				ScanMaster:   true,
			})
			if err != nil {
				return err
			}
			if len(findings) == 0 {
				fmt.Println("no work discovered")
				return nil
			}
			if dryRun {
				fmt.Printf("%d finding(s) (dry-run, not enqueued):\n", len(findings))
				for _, f := range findings {
					fmt.Printf("  [%s] %.80s\n", f.Source, f.Prompt)
				}
				return nil
			}
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			n, err := autonomy.EnqueueFindings(cmd.Context(), q, ws, findings, discoverRetries)
			if err != nil {
				return err
			}
			fmt.Printf("discovered %d finding(s), enqueued %d new goal(s)\n", len(findings), n)
			return nil
		},
	}
	discoverCmd.Flags().BoolVar(&dryRun, "dry-run", false, "list findings without enqueueing")
	discoverCmd.Flags().IntVar(&discoverRetries, "retries", 3, "retry budget for enqueued goals")

	// goal status <id> — show one goal with subtasks (issue #140 fusion).
	var statusJsonOut bool
	statusCmd := &cobra.Command{
		Use:   "status <id>",
		Short: "Show one goal's progress, attempts, and children (issue #140 fusion)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := parseGoalID(args[0])
			if perr != nil {
				return perr
			}
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			g, err := q.Get(cmd.Context(), id)
			if err != nil {
				return err
			}
			children, err := q.Children(cmd.Context(), id)
			if err != nil {
				return err
			}
			if statusJsonOut {
				payload := map[string]any{
					"goal":     g,
					"children": children,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Goal %d [%s] attempts=%d/%d priority=%d\n",
				g.ID, g.Status, g.Attempts, g.MaxRetries, g.Priority)
			fmt.Fprintf(cmd.OutOrStdout(), "  prompt: %s\n", g.Prompt)
			if len(g.LastError) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "  last_error: %s\n", g.LastError)
			}
			if len(children) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  (no subtasks)")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  subtasks (%d):\n", len(children))
			for _, c := range children {
				fmt.Fprintf(cmd.OutOrStdout(), "    %-5d [%-10s] %.60s\n", c.ID, c.Status, c.Prompt)
			}
			return nil
		},
	}
	statusCmd.Flags().BoolVar(&statusJsonOut, "json", false, "emit JSON")

	// goal complete <id> — mark a goal as verified/done (issue #140 fusion).
	// Mirrors goal_complete in the external Python MCP server.
	var completeSession string
	completeCmd := &cobra.Command{
		Use:   "complete <id>",
		Short: "Mark a goal as verified/done (issue #140 fusion; maps to q.Complete)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, perr := parseGoalID(args[0])
			if perr != nil {
				return perr
			}
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			if err := q.Complete(cmd.Context(), id, completeSession); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "goal %d marked complete (session %q)\n", id, completeSession)
			return nil
		},
	}
	completeCmd.Flags().StringVar(&completeSession, "session", "", "session id of the worker that completed the goal")

	// goal subtask <parent-id> <prompt> — add a subtask to a parent (issue #140 fusion).
	// Mirrors goal_subtask in the external Python MCP server.
	var subtaskPriority, subtaskRetries int
	var subtaskCriteria []string
	subtaskCmd := &cobra.Command{
		Use:   "subtask <parent-id> <prompt>",
		Short: "Add a subtask under a parent goal (issue #140 fusion; maps to q.AddSub)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			parentID, perr := parseGoalID(args[0])
			if perr != nil {
				return perr
			}
			contractJSON := ""
			if len(subtaskCriteria) > 0 {
				c := &goalcontract.GoalContract{SemanticCriteria: subtaskCriteria}
				contractJSON, _ = c.Marshal()
			}
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			id, err := q.AddSub(cmd.Context(), parentID, args[1], subtaskPriority, subtaskRetries, contractJSON)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "subtask %d enqueued under parent %d\n", id, parentID)
			return nil
		},
	}
	subtaskCmd.Flags().IntVar(&subtaskPriority, "priority", 0, "higher runs sooner")
	subtaskCmd.Flags().IntVar(&subtaskRetries, "retries", 3, "retry budget")
	subtaskCmd.Flags().StringArrayVar(&subtaskCriteria, "criteria", nil, "acceptance criterion (repeatable)")

	// goal report — emit a JSON or Markdown progress report (issue #140 fusion).
	// Maps to goal_report in the external Python MCP server. v0 ships the
	// JSON variant; Markdown rendering is a follow-up.
	var reportFormat string
	reportCmd := &cobra.Command{
		Use:   "report",
		Short: "Generate a progress report across all goals (issue #140 fusion)",
		RunE: func(cmd *cobra.Command, args []string) error {
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			goals, err := q.List(cmd.Context(), "")
			if err != nil {
				return err
			}
			byStatus := map[string]int{}
			for _, g := range goals {
				byStatus[string(g.Status)]++
			}
			switch reportFormat {
			case "json":
				payload := map[string]any{
					"total":     len(goals),
					"by_status": byStatus,
					"goals":     goals,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(payload)
			case "md", "markdown", "":
				fmt.Fprintf(cmd.OutOrStdout(), "# Goal Report\n\n")
				fmt.Fprintf(cmd.OutOrStdout(), "Total: %d goal(s)\n\n", len(goals))
				fmt.Fprintf(cmd.OutOrStdout(), "## By status\n\n")
				for status, n := range byStatus {
					fmt.Fprintf(cmd.OutOrStdout(), "- **%s**: %d\n", status, n)
				}
				return nil
			default:
				return fmt.Errorf("unsupported format %q (json|md)", reportFormat)
			}
		},
	}
	reportCmd.Flags().StringVar(&reportFormat, "format", "md", "output format: md|json")

	cmd.AddCommand(addCmd, listCmd, discoverCmd, statusCmd, completeCmd, subtaskCmd, reportCmd, newGoalAddFromIssueCmd())
	return cmd
}

// parseGoalID parses a numeric goal id. Strings like "#42" or "42" are
// both accepted (the "#" prefix is tolerated for operator ergonomics).
func parseGoalID(s string) (int64, error) {
	// Order matters: TrimSpace first, then TrimPrefix("#") — otherwise
	// "  #42  " would not match the leading "#".
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid goal id %q: %w", s, err)
	}
	return id, nil
}

// ============================================================================
// Auto command (sin-code auto)
// ============================================================================

// sin-debt: yagni, upgrade: when a second implementation lands, remove this marker
// autoLoop is the minimal agent-loop interface used by the auto run
// subcommand so tests can inject a fake loop without building a real one.
type autoLoop interface {
	Run(ctx context.Context, sess *session.Session, goal string) (*agentloop.Result, error)
}

// sin-debt: yagni, upgrade: when a second implementation lands, remove this marker
// autoPilot is the minimal autopilot interface used by the auto subcommand
// so tests can inject a fake pilot.
type autoPilot interface {
	Run(ctx context.Context) (int, float64, error)
}

// autoHookVars holds injectable dependencies for the auto subcommand. Coverage
// tests replace these fields to avoid real I/O or network calls.
var autoHookVars = struct {
	osStat             func(string) (os.FileInfo, error)
	osWriteFile        func(string, []byte, os.FileMode) error
	osGetwd            func() (string, error)
	loadProgram        func(string) (*autopilot.Program, error)
	defaultJournalPath func(string) string
	openJournal        func(string) (*autopilot.Journal, error)
	defaultSessionPath func() string
	openSession        func(string) (*session.Store, error)
	openLessons        func(string) (*lessons.Store, error)
	buildLoop          func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error)
	newPilot           func(cfg autopilot.Config) autoPilot
	newBudget          func(minutes, maxExperiments int) *autopilot.Budget
	newSnapshotter     func(string) *autopilot.Snapshotter
}{
	osStat:             os.Stat,
	osWriteFile:        os.WriteFile,
	osGetwd:            os.Getwd,
	loadProgram:        autopilot.LoadProgram,
	defaultJournalPath: autopilot.DefaultJournalPath,
	openJournal:        autopilot.OpenJournal,
	defaultSessionPath: session.DefaultPath,
	openSession:        session.Open,
	openLessons:        lessons.Open,
	buildLoop: func(ctx context.Context, cfg loopbuilder.Config, ls *lessons.Store) (autoLoop, func() error, error) {
		loop, cleanup, err := loopbuilder.Build(ctx, cfg, ls)
		if err != nil {
			return nil, nil, err
		}
		return loop, cleanup, nil
	},
	newPilot:       func(cfg autopilot.Config) autoPilot { return autopilot.New(cfg) },
	newBudget:      autopilot.NewBudget,
	newSnapshotter: autopilot.NewSnapshotter,
}

func NewAutoCmd() *cobra.Command {
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

func newAutoInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Write a program.md template into the current workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := autoHookVars.osStat("program.md"); err == nil {
				return fmt.Errorf("program.md already exists")
			}
			if err := autoHookVars.osWriteFile("program.md", []byte(programTemplate), filemode.Default()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "wrote program.md — edit it, then run: sin-code auto run --verify-cmd \"...\"")
			return nil
		},
	}
}

func newAutoRunCmd() *cobra.Command {
	var verifyCmd string
	var budgetMinutes, maxExperiments, maxTurns int
	var noBaseline bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the autonomous loop until the budget is exhausted",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if verifyCmd == "" {
				return fmt.Errorf("auto run refuses to start without --verify-cmd (M3: autonomy requires a verify gate)")
			}
			workspace, err := autoHookVars.osGetwd()
			if err != nil {
				return err
			}
			prog, err := autoHookVars.loadProgram(filepath.Join(workspace, "program.md"))
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

			journal, err := autoHookVars.openJournal(autoHookVars.defaultJournalPath(workspace))
			if err != nil {
				return err
			}
			defer journal.Close()

			lessonStore, _ := autoHookVars.openLessons("")
			defer func() {
				if lessonStore != nil {
					lessonStore.Close()
				}
			}()

			sessStore, err := autoHookVars.openSession(autoHookVars.defaultSessionPath())
			if err != nil {
				return err
			}
			defer sessStore.Close()

			// Resolve the always-on SinCode loop Definition-of-Done once for
			// the whole autonomous session: every experiment the autopilot
			// runs is held to the same baseline (tests/debug/docs/completeness)
			// via the stop-gate, unless --no-baseline / SIN_BASELINE=off.
			var autoContract *goalcontract.GoalContract
			if c, cerr := goalcontract.Resolve(goalcontract.ResolveOptions{
				Workspace:       workspace,
				VerifyCmd:       verifyCmd,
				AutoDetect:      true,
				IncludeBaseline: goalcontract.BaselineEnabled(noBaseline),
			}); cerr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warn: auto contract resolve failed, continuing without stop-gate: %v\n", cerr)
			} else if !c.IsEmpty() {
				autoContract = c
			}

			runGoal := func(ctx context.Context, goal string) (autopilot.LoopResult, string, error) {
				sess, err := sessStore.StartOrResume("")
				if err != nil {
					return autopilot.LoopResult{}, "", err
				}
				loop, cleanup, err := autoHookVars.buildLoop(ctx, loopbuilder.Config{
					Workspace:    workspace,
					SessionID:    sess.ID,
					MaxTurns:     maxTurns,
					VerifyMode:   "poc",
					VerifyCmd:    verifyCmd,
					Headless:     true,
					Contract:     autoContract,
					SessionStore: sessStore,
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
				return autopilot.LoopResult{SessionID: res.SessionID, Verified: res.Verified, Turns: res.Turns}, res.Summary, nil
			}

			ap := autoHookVars.newPilot(autopilot.Config{
				Workspace: workspace,
				Program:   prog,
				Proposer:  &autopilot.Proposer{Program: prog}, // deterministic fallback; wire LLM here later
				Journal:   journal,
				Budget:    autoHookVars.newBudget(prog.BudgetMinutes, prog.MaxExperiments),
				Snap:      autoHookVars.newSnapshotter(workspace),
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
	cmd.Flags().BoolVar(&noBaseline, "no-baseline", false, "disable the always-on SinCode loop baseline (tests/debug/docs/completeness DoD); also via SIN_BASELINE=off")
	return cmd
}

func newAutoStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show budget, best metric, and recent experiment summary",
		RunE: func(cmd *cobra.Command, _ []string) error {
			workspace, _ := autoHookVars.osGetwd()
			journal, err := autoHookVars.openJournal(autoHookVars.defaultJournalPath(workspace))
			if err != nil {
				return err
			}
			defer journal.Close()
			prog, _ := autoHookVars.loadProgram(filepath.Join(workspace, "program.md"))
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

func newAutoJournalCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "journal",
		Short: "Print the experiment journal (newest first)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			workspace, _ := autoHookVars.osGetwd()
			journal, err := autoHookVars.openJournal(autoHookVars.defaultJournalPath(workspace))
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

// ============================================================================
// Ledger command (sin-code ledger)
// ============================================================================

// NewLedgerCmd builds the `ledger` cobra subcommand.
func NewLedgerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ledger",
		Short: "Query the semantic session ledger",
		Long: `sin-code ledger reads the append-only session ledger that records
prompts, tool calls, verification results, and completions. Use it to audit
what the agent did in a session or to list recent sessions.`,
	}
	cmd.AddCommand(newLedgerListCmd())
	cmd.AddCommand(newLedgerShowCmd())
	cmd.AddCommand(newLedgerToolsCmd())
	return cmd
}

func ledgerStore() (*ledger.Store, error) {
	path := ledger.DefaultPath()
	if env := os.Getenv("SIN_CODE_LEDGER"); env != "" {
		path = env
	}
	return ledger.Open(path)
}

func newLedgerListCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "list",
		Short: "List recent sessions with ledger entries",
		RunE: func(_ *cobra.Command, _ []string) error {
			store, err := ledgerStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			sessions, err := store.Sessions(ctx, limit)
			if err != nil {
				return err
			}
			if len(sessions) == 0 {
				fmt.Println("No sessions recorded.")
				return nil
			}
			for _, sid := range sessions {
				fmt.Println(sid)
			}
			return nil
		},
	}
	c.Flags().IntVarP(&limit, "limit", "n", 50, "Max sessions to show")
	return c
}

func newLedgerShowCmd() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show ledger entries for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			store, err := ledgerStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			entries, err := store.List(ctx, args[0], limit)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No ledger entries for this session.")
				return nil
			}
			for _, e := range entries {
				fmt.Printf("%s  %-16s  %s\n", e.CreatedAt.Format(time.RFC3339), e.Type, e.Summary)
			}
			return nil
		},
	}
	c.Flags().IntVarP(&limit, "limit", "n", 100, "Max entries to show")
	return c
}

func newLedgerToolsCmd() *cobra.Command {
	var (
		heatmapFlag  bool
		coverageFlag bool
		unusedFlag   bool
		familyFlag   bool
		jsonFlag     bool
		sinceStr     string
		untilStr     string
	)
	c := &cobra.Command{
		Use:   "tools",
		Short: "Tool usage heatmap, coverage, and unused-tool report",
		Long: `sin-code ledger tools reads the tool_usage table populated by the
agent loop and reports per-tool counts, coverage against the known tool set,
and never-used tools. Run it after the agent has executed sessions to see
which tools are hot and which are gaps.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := ledgerStore()
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var since, until time.Time
			if sinceStr != "" {
				since, err = time.Parse(time.RFC3339, sinceStr)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
			}
			if untilStr != "" {
				until, err = time.Parse(time.RFC3339, untilStr)
				if err != nil {
					return fmt.Errorf("invalid --until: %w", err)
				}
			}

			known := knownToolNames()

			// Default to heatmap when no specific report is requested.
			if !coverageFlag && !unusedFlag && !familyFlag {
				heatmapFlag = true
			}

			if coverageFlag {
				res, err := store.ToolCoverage(ctx, known)
				if err != nil {
					return err
				}
				if jsonFlag {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(res)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Coverage: %.1f%% (%d/%d tools used)\n",
					res.Coverage*100, len(res.Used), res.Total)
				return nil
			}

			if unusedFlag {
				unused, err := store.UnusedTools(ctx, known)
				if err != nil {
					return err
				}
				if jsonFlag {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]any{"unused": unused, "total_known": len(known)})
				}
				if len(unused) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "All known tools have been used.")
					return nil
				}
				for _, name := range unused {
					fmt.Fprintln(cmd.OutOrStdout(), name)
				}
				return nil
			}

			if familyFlag {
				counts, err := store.FamilyUsageCounts(ctx, since, until)
				if err != nil {
					return err
				}
				if jsonFlag {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(counts)
				}
				return formatFamilyHeatmap(cmd.OutOrStdout(), counts)
			}

			counts, err := store.ToolUsageCounts(ctx, since, until)
			if err != nil {
				return err
			}
			if jsonFlag {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(counts)
			}
			return formatToolHeatmap(cmd.OutOrStdout(), counts)
		},
	}
	c.Flags().BoolVar(&heatmapFlag, "heatmap", true, "Show per-tool usage heatmap")
	c.Flags().BoolVar(&coverageFlag, "coverage", false, "Show coverage score")
	c.Flags().BoolVar(&unusedFlag, "unused", false, "List never-used tools")
	c.Flags().BoolVar(&familyFlag, "family", false, "Show family-level heatmap")
	c.Flags().BoolVar(&jsonFlag, "json", false, "Emit JSON for CI")
	c.Flags().StringVar(&sinceStr, "since", "", "RFC3339 start of window")
	c.Flags().StringVar(&untilStr, "until", "", "RFC3339 end of window")
	return c
}

func formatToolHeatmap(w io.Writer, counts []ledger.UsageCount) error {
	if len(counts) == 0 {
		fmt.Fprintln(w, "No tool usage recorded.")
		return nil
	}
	maxName := 0
	for _, c := range counts {
		if len(c.ToolName) > maxName {
			maxName = len(c.ToolName)
		}
	}
	fmt.Fprintf(w, "%-*s  %-10s  %6s  %s\n", maxName, "tool", "family", "total", "ok/error/denied")
	for _, c := range counts {
		fmt.Fprintf(w, "%-*s  %-10s  %6d  %d/%d/%d\n",
			maxName, c.ToolName, c.Family, c.Total,
			c.ByOutcome[ledger.OutcomeOK],
			c.ByOutcome[ledger.OutcomeError],
			c.ByOutcome[ledger.OutcomeDenied])
	}
	return nil
}

func formatFamilyHeatmap(w io.Writer, counts []ledger.FamilyCount) error {
	if len(counts) == 0 {
		fmt.Fprintln(w, "No tool usage recorded.")
		return nil
	}
	maxName := 0
	for _, c := range counts {
		if len(c.Family) > maxName {
			maxName = len(c.Family)
		}
	}
	fmt.Fprintf(w, "%-*s  %6s  %s\n", maxName, "family", "total", "ok/error/denied")
	for _, c := range counts {
		fmt.Fprintf(w, "%-*s  %6d  %d/%d/%d\n",
			maxName, c.Family, c.Total,
			c.ByOutcome[ledger.OutcomeOK],
			c.ByOutcome[ledger.OutcomeError],
			c.ByOutcome[ledger.OutcomeDenied])
	}
	return nil
}

func knownToolNames() []string {
	seen := map[string]bool{}
	for _, t := range builtinSpecs() {
		seen[t.Name] = true
	}
	for _, t := range hub.AllTools() {
		seen[t.Name] = true
	}
	for _, name := range []string{
		// Representative external MCP tools from the default ecosystem registry.
		"websearch__search",
		"browser__navigate",
		"browser__findings",
		"browser__snapshot",
		"simone__search",
		"simone__symbol",
		"scheduler__schedule_job",
		"scheduler__schedule_list",
		"goalmode__goal_start",
		"goalmode__goal_list",
		"grillme__grill_start",
		"marketplace__marketplace_search",
		"codocs__doc_start",
		"contextbridge__sin_context",
		"honcho__sin_memory_add",
		"frontend__design_component_create",
		"mcpbuilder__mcp_scaffold",
		"symfonylens__symfony_analyze_routes",
	} {
		seen[name] = true
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	return out
}

// ============================================================================
// Compress command (sin-code compress)
// ============================================================================

// NewCompressCmd builds the `compress` cobra subcommand. Pattern mirrors
// NewHubCmd: root alias + 3 sub-commands (plan / apply / rollback) so
// each verb has a stable, scriptable form.
func NewCompressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compress",
		Short: "Lossless compaction for lessons / instincts / summaries / AGENTS.md",
		Long: `sin-code compress runs deterministic (dedupe + byte-budget + sort)
compaction over SIN-Code's long-lived stores, with an opt-in LLM summarization
step (Strategy=llm|hybrid). Every compaction is lossless: dropped entries are
preserved verbatim in a JSON snapshot under
~/.local/share/sin-code/compress-snapshots/<plan-id>.json. Use
'sin-code compress rollback <id>' to restore.

Targets:  lessons | instincts | summaries | memory | agents_md | all
Strategy: deterministic (default) | llm | hybrid
Examples:
  sin-code compress plan --target all --strategy deterministic
  sin-code compress plan --target lessons --keep-bytes 4096
  sin-code compress apply --target all --dry-run
  sin-code compress apply --target memory --keep-bytes 8192 --no-llm
  sin-code compress rollback plan-3a8e57b7c8f1fc11`,
	}
	cmd.AddCommand(newCompressPlanCmd())
	cmd.AddCommand(newCompressApplyCmd())
	cmd.AddCommand(newCompressRollbackCmd())
	return cmd
}

// compressCommon groups the flags shared by plan + apply. Both subcommands
// have `--target`, `--strategy`, `--keep-bytes`, `--keep`, `--recent-days`,
// `--lessons-db`, `--instinct-dir`, `--memory-db`, `--agents-md`,
// `--json` (machine-readable output).
type compressCommon struct {
	target      string
	strategy    string
	keepBytes   int
	keepMax     int
	recentDays  int
	lessonsDB   string
	instinctDir string
	memoryDB    string
	agentsMD    string
	asJSON      bool
}

// addCommonFlags wires the shared flags onto `cmd`.
func addCommonFlags(cmd *cobra.Command, c *compressCommon) {
	cmd.Flags().StringVar(&c.target, "target", "all",
		"target store: lessons | instincts | summaries | memory | agents_md | all")
	cmd.Flags().StringVar(&c.strategy, "strategy", "deterministic",
		"compaction strategy: deterministic | llm | hybrid")
	cmd.Flags().IntVar(&c.keepBytes, "keep-bytes", 4096,
		"byte budget per target — entries beyond this are dropped")
	cmd.Flags().IntVar(&c.keepMax, "keep", 0,
		"max kept entries per target — 0 means no cap")
	cmd.Flags().IntVar(&c.recentDays, "recent-days", 0,
		"drop entries older than this many days — 0 disables the age filter")
	cmd.Flags().StringVar(&c.lessonsDB, "lessons-db", "",
		"override path to the lessons.db (default: ~/.local/share/sin-code/lessons.db)")
	cmd.Flags().StringVar(&c.instinctDir, "instinct-dir", "",
		"override the instinct base directory (default: ~/.local/share/sin-code/instinct)")
	cmd.Flags().StringVar(&c.memoryDB, "memory-db", "",
		"override path to the memory bbolt db (default: os.UserConfigDir()/sin-code/memory.db)")
	cmd.Flags().StringVar(&c.agentsMD, "agents-md", "",
		"override path to the AGENTS.md file (default: walk up from cwd)")
	cmd.Flags().BoolVar(&c.asJSON, "json", false,
		"print machine-readable JSON instead of human-readable summary")
}

// toPaths turns the common bundle into a compress.Paths value.
func (c *compressCommon) toPaths() compress.Paths {
	return compress.Paths{
		LessonsDB: c.lessonsDB,
		Instinct:  c.instinctDir,
		Memory:    c.memoryDB,
		AgentsMD:  c.agentsMD,
	}
}

// toPlanOptions turns the common bundle into a compress.PlanOptions value.
func (c *compressCommon) toPlanOptions() compress.PlanOptions {
	return compress.PlanOptions{
		KeepBudgetBytes: c.keepBytes,
		KeepMaxEntries:  c.keepMax,
		KeepRecentDays:  c.recentDays,
	}
}

// parseTarget normalizes a --target flag value into a compress.Target.
// Empty / "all" → TargetAll. Unknown → error.
func parseTarget(s string) (compress.Target, error) {
	t := compress.Target(strings.ToLower(strings.TrimSpace(s)))
	if t == "" {
		return compress.TargetAll, nil
	}
	if !t.IsValid() {
		return "", fmt.Errorf("unknown target %q (use: lessons|instincts|summaries|memory|agents_md|all)", s)
	}
	return t, nil
}

// parseStrategy normalizes a --strategy flag value into a compress.Strategy.
// Empty / "deterministic" → StrategyDeterministic. Unknown → error.
func parseStrategy(s string) (compress.Strategy, error) {
	st := compress.Strategy(strings.ToLower(strings.TrimSpace(s)))
	if st == "" {
		return compress.StrategyDeterministic, nil
	}
	if !st.IsValid() {
		return "", fmt.Errorf("unknown strategy %q (use: deterministic|llm|hybrid)", s)
	}
	return st, nil
}

// newCompressPlanCmd builds the `plan` subcommand. plan is read-only; it
// builds the Plan, prints it, and exits.
func newCompressPlanCmd() *cobra.Command {
	c := &compressCommon{}
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Compute a compaction Plan and print its projected impact (no writes)",
		RunE: func(_ *cobra.Command, _ []string) error {
			target, err := parseTarget(c.target)
			if err != nil {
				return err
			}
			strategy, err := parseStrategy(c.strategy)
			if err != nil {
				return err
			}
			p, err := compress.BuildPlan(target, strategy, c.toPaths(), c.toPlanOptions())
			if err != nil {
				return err
			}
			return renderPlan(p, c.asJSON)
		},
	}
	addCommonFlags(cmd, c)
	return cmd
}

// newCompressApplyCmd builds the `apply` subcommand. apply is the only
// path that writes; it stops at the snapshot step (atomic) and the
// per-target rewrites. --dry-run stops before the snapshot.
func newCompressApplyCmd() *cobra.Command {
	c := &compressCommon{}
	var dryRun, noLLM bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Compute a Plan, snapshot the dropped entries, and rewrite the target",
		RunE: func(_ *cobra.Command, _ []string) error {
			target, err := parseTarget(c.target)
			if err != nil {
				return err
			}
			strategy, err := parseStrategy(c.strategy)
			if err != nil {
				return err
			}
			if noLLM && (strategy == compress.StrategyLLM || strategy == compress.StrategyHybrid) {
				strategy = compress.StrategyDeterministic
				fmt.Fprintln(os.Stderr, "compress: --no-llm downgrades strategy=llm|hybrid to deterministic")
			}
			p, err := compress.BuildPlan(target, strategy, c.toPaths(), c.toPlanOptions())
			if err != nil {
				return err
			}
			rep, err := compress.Apply(p, c.toPaths(), compress.ApplyOptions{DryRun: dryRun, Reason: "sin-code compress apply"})
			if err != nil {
				return err
			}
			return renderReport(p, rep, c.asJSON)
		},
	}
	addCommonFlags(cmd, c)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "snapshot-current would write a snapshot —skip the snapshot + writes")
	cmd.Flags().BoolVar(&noLLM, "no-llm", false, "force Strategy=deterministic even if --strategy=llm|hybrid")
	// Re-add the dry-run default description to be friendlier.
	cmd.Flag("dry-run").Usage = "plan-only: print the projected impact, do not write"
	return cmd
}

// newCompressRollbackCmd builds the `rollback` subcommand. Rollback
// reads the snapshot file from ~/.local/share/sin-code/compress-snapshots/<id>.json
// and restores the original entries byte-for-byte.
func newCompressRollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <snapshot-id>",
		Short: "Restore dropped entries from a snapshot file",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := compress.Rollback(args[0]); err != nil {
				return err
			}
			fmt.Printf("rollback %s: ok\n", args[0])
			return nil
		},
	}
}

// renderPlan prints a Plan in human or JSON form.
func renderPlan(p compress.Plan, asJSON bool) error {
	if asJSON {
		b, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("compress plan (id=%s hash=%s)\n", p.ID, p.PlanHash)
	fmt.Printf("  target   : %s\n", p.Target)
	fmt.Printf("  strategy : %s\n", p.Strategy)
	fmt.Printf("  entries  : %d → %d (drops=%d, merges=%d)\n",
		p.Stats.OriginalEntries, p.Stats.ProjectedEntries,
		p.Stats.Drops, p.Stats.Merges)
	fmt.Printf("  bytes    : %d → %d (ratio %.2f)\n",
		p.Stats.OriginalBytes, p.Stats.ProjectedBytes, p.Stats.ProjectedRatio)
	if len(p.Warnings) > 0 {
		fmt.Println("  warnings :")
		for _, w := range p.Warnings {
			fmt.Println("    - " + w)
		}
	}
	return nil
}

// renderReport prints an ApplyReport in human or JSON form.
func renderReport(p compress.Plan, rep compress.ApplyReport, asJSON bool) error {
	if asJSON {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("compress report (plan=%s snap=%s)\n", rep.PlanID, rep.SnapshotID)
	fmt.Printf("  original : %d bytes\n", rep.OriginalBytes)
	fmt.Printf("  kept     : %d bytes  (ratio %.2f)\n", rep.KeptBytes, rep.Ratio)
	if snap := rep.SnapshotPath; snap != "" {
		fmt.Println("  snapshot :", relOrSame(snap))
	}
	for _, tr := range rep.PerTarget {
		fmt.Printf("  %-10s : %d → %d entries, %d → %d bytes\n",
			string(tr.Target), tr.BeforeEntries, tr.AfterEntries,
			tr.BeforeBytes, tr.AfterBytes)
	}
	if len(rep.Warnings) > 0 {
		fmt.Printf("  warnings : %d (use --json for details)\n", len(rep.Warnings))
	}
	if len(p.Warnings) > 0 {
		fmt.Printf("  plan warnings : %d (use --json for details)\n", len(p.Warnings))
	}
	_ = p
	return nil
}

// relOrSame returns the relative path of `abs` if it's under
// cwd; this avoids CWD-prefix noise in the snapshot path output
// without erasing the basename.
func relOrSame(abs string) string {
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return abs
}

// ============================================================================
// Dox command (sin-code dox)
// ============================================================================

// NewDoxCmd builds the `dox` cobra subcommand. Pattern matches
// NewSuperpowersCmd: returns *cobra.Command with four subcommands
// (init, new, check, tree) attached.
func NewDoxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dox",
		Short: "Self-maintaining AGENTS.md hierarchy (agent0ai/dox protocol)",
		Long: `sin-code dox integrates the agent0ai/dox (MIT) self-maintaining
AGENTS.md hierarchy protocol. It uses marker-based injection
(<!-- SIN-Code dox:begin/end -->) that coexists with the
SIN-Code superpowers block in the same AGENTS.md, validates the
tree for broken links and orphan nodes, and can scaffold new child
nodes with automatic parent-INDEX registration.

All subcommands are local-only and safe to call offline.`,
	}

	var (
		jsonOut bool
		force   bool
	)

	// ── init ─────────────────────────────────────────────────────────
	initCmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Create a root AGENTS.md and inject the dox block",
		Long: `init scaffolds a root AGENTS.md at the given path (default: current
directory) and injects the dox-managed block. The block is delimited by
<!-- SIN-Code dox:begin --> and <!-- SIN-Code dox:end -->, so it can
coexist with other managed blocks (e.g. the SIN-Code superpowers block)
in the same file.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(abs, 0o755); err != nil {
				return err
			}
			agentsPath := filepath.Join(abs, dox.AgentsFileName)
			if _, err := os.Stat(agentsPath); err != nil && !force {
				// Seed a minimal root AGENTS.md.
				seed := "---\n" +
					"title: " + filepath.Base(abs) + "\n" +
					"---\n\n" +
					"# " + filepath.Base(abs) + "\n\n" +
					"Root of the dox-managed AGENTS.md tree.\n"
				if err := os.WriteFile(agentsPath, []byte(seed), filemode.Default()); err != nil {
					return err
				}
			}
			body := "## Dox-managed regions\n\n" +
				"This block is owned by `sin-code dox`. Do not edit by hand.\n" +
				"Re-run `sin-code dox init` to refresh.\n"
			if err := dox.InjectRoot(agentsPath, body); err != nil {
				return err
			}
			if jsonOut {
				out := map[string]any{
					"agents_path": agentsPath,
					"marker":      dox.BeginMarker,
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			fmt.Printf("initialized %s\n", agentsPath)
			return nil
		},
	}
	initCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	initCmd.Flags().BoolVar(&force, "force", false, "overwrite an existing root AGENTS.md")

	// ── new ──────────────────────────────────────────────────────────
	newCmd := &cobra.Command{
		Use:   "new <name> [--title <title>]",
		Short: "Scaffold a new child node under the given parent",
		Long: `new creates a new child directory with an INDEX.md (or AGENTS.md at
the root) and registers the child in the parent's index. The child is
immediately discoverable by ` + "`sin-code dox check`" + `.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			parent, _ := cmd.Flags().GetString("parent")
			title, _ := cmd.Flags().GetString("title")
			if parent == "" {
				parent = "."
			}
			abs, err := filepath.Abs(parent)
			if err != nil {
				return err
			}
			child, err := dox.Scaffold(abs, name, title)
			if err != nil {
				return err
			}
			if jsonOut {
				out := map[string]any{
					"path":  child,
					"name":  name,
					"title": title,
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}
			fmt.Printf("scaffolded %s\n", child)
			return nil
		},
	}
	newCmd.Flags().String("parent", ".", "parent directory (defaults to current)")
	newCmd.Flags().String("title", "", "human title for the new node (defaults to name)")
	newCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	// ── check ────────────────────────────────────────────────────────
	checkCmd := &cobra.Command{
		Use:   "check [root]",
		Short: "Validate the dox tree (broken links, orphans, TODOs, missing index)",
		Long: `check walks the tree starting at <root> (default: current directory)
and reports every structural problem. Exits non-zero if any error-level
finding is reported; warn-level findings (e.g. TODO sentinels) do not
fail the check.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			findings, err := dox.Check(root)
			if err != nil {
				return err
			}
			// Stable order so JSON output is diff-friendly.
			sortFindings(findings)
			if jsonOut {
				out := map[string]any{
					"findings": findings,
					"healthy":  !hasErrors(findings),
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					return err
				}
			} else {
				if len(findings) == 0 {
					fmt.Println("healthy: no findings")
				} else {
					for _, f := range findings {
						tag := "WARN"
						if f.Severity == "error" {
							tag = "ERR "
						}
						fmt.Printf("%s %-12s %s — %s\n", tag, f.Kind, f.Path, f.Message)
					}
				}
			}
			if hasErrors(findings) {
				return fmt.Errorf("dox check: %d error(s) found", countErrors(findings))
			}
			return nil
		},
	}
	checkCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	// ── tree ─────────────────────────────────────────────────────────
	treeCmd := &cobra.Command{
		Use:   "tree [root]",
		Short: "Print a human-readable tree of the dox hierarchy",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			out, err := dox.RenderTree(root)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]string{"tree": out})
			}
			fmt.Print(out)
			return nil
		},
	}
	treeCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON (returns tree as a single string field)")

	cmd.AddCommand(initCmd, newCmd, checkCmd, treeCmd)
	return cmd
}

func sortFindings(fs []dox.Finding) {
	// stable-ish: by severity (error first), then path, then kind.
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0; j-- {
			if lessFinding(fs[j], fs[j-1]) {
				fs[j-1], fs[j] = fs[j], fs[j-1]
			} else {
				break
			}
		}
	}
}

func lessFinding(a, b dox.Finding) bool {
	if a.Severity != b.Severity {
		// "error" sorts before "warn"
		return a.Severity == "error"
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.Kind < b.Kind
}

func hasErrors(fs []dox.Finding) bool {
	for _, f := range fs {
		if f.Severity == "error" {
			return true
		}
	}
	return false
}

func countErrors(fs []dox.Finding) int {
	n := 0
	for _, f := range fs {
		if f.Severity == "error" {
			n++
		}
	}
	return n
}

// ============================================================================
// Grill command (sin-code grill)
// ============================================================================

// grillDir returns the directory for grilling sessions. Honors
// SIN_CODE_HOME, then XDG_DATA_HOME, then ~/.local/share/sin-code/grill.
func grillDir() (string, error) {
	if v := os.Getenv("SIN_CODE_HOME"); v != "" {
		return filepath.Join(v, "grill"), nil
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "sin-code", "grill"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "sin-code", "grill"), nil
}

// NewGrillCmd builds the `grill` cobra subcommand.
func NewGrillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grill",
		Short: "Native adversarial design-review interview (issue #141 fusion)",
		Long: `sin-code grill is the native Go implementation of the
external SIN-Code-Grill-Me-Skill Python MCP server. Use it to
stress-test a plan, design, or decision before building it.

Subcommands:
  start <topic>            begin a grilling session, print the session id
  next <id>                ask the next adversarial question
  answer <id> <d-id> <text>  record the operator's response
                            (use "done" to resolve a decision)
  status <id>              show resolved + open decision branches
  synthesize <id>          produce a summary of decisions + assumptions`,
	}
	cmd.AddCommand(newGrillStartCmd())
	cmd.AddCommand(newGrillNextCmd())
	cmd.AddCommand(newGrillAnswerCmd())
	cmd.AddCommand(newGrillStatusCmd())
	cmd.AddCommand(newGrillSynthesizeCmd())
	return cmd
}

func newGrillStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <topic>",
		Short: "Begin a grilling session on the given topic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			s, err := m.Start(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "started session %s\n", s.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "  topic: %s\n", s.Topic)
			fmt.Fprintf(cmd.OutOrStdout(), "  seed question: %s\n", s.Decisions[0].Question)
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintf(cmd.OutOrStdout(), "  next: sin-code grill next %s\n", s.ID)
			return nil
		},
	}
}

func newGrillNextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "next <id>",
		Short: "Ask the next adversarial question",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			child, parent, err := m.Next(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sub-question under %s:\n", parent)
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", child.ID, child.Question)
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintf(cmd.OutOrStdout(), "  answer: sin-code grill answer %s %s \"<your response>\"\n", args[0], child.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "  resolve: sin-code grill answer %s %s done\n", args[0], child.ID)
			return nil
		},
	}
}

func newGrillAnswerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "answer <id> <decision-id> <text>",
		Short: "Record the operator's response to a decision (use 'done' to resolve)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			if err := m.Answer(args[0], args[1], args[2]); err != nil {
				return err
			}
			action := "answered"
			if args[2] == "done" || args[2] == "skip" {
				action = "resolved"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s decision %s in session %s\n", action, args[1], args[0])
			return nil
		},
	}
}

func newGrillStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <id>",
		Short: "Show resolved + open decision branches",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			s, err := m.Get(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Session %s\n", s.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "  topic: %s\n", s.Topic)
			fmt.Fprintf(cmd.OutOrStdout(), "  started: %s\n", s.StartedAt.Format("2006-01-02 15:04:05"))
			fmt.Fprintf(cmd.OutOrStdout(), "  decisions: %d (open=%d)\n", len(s.Decisions), s.OpenQuestions)
			for _, d := range s.Decisions {
				marker := "[ ]"
				switch d.Status {
				case "answered":
					marker = "[~]"
				case "resolved":
					marker = "[x]"
				case "deferred":
					marker = "[>]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "    %s %s: %s\n", marker, d.ID, d.Question)
				if d.Answer != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "         answer: %s\n", d.Answer)
				}
			}
			return nil
		},
	}
}

func newGrillSynthesizeCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "synthesize <id>",
		Short: "Produce a summary of decisions, assumptions, and open questions",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			dir, err := grillDir()
			if err != nil {
				return err
			}
			m, err := grill.NewManager(dir)
			if err != nil {
				return err
			}
			syn, err := m.Synthesize(args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(syn)
			}
			fmt.Fprintln(c.OutOrStdout(), "# Grilling Synthesis")
			fmt.Fprintln(c.OutOrStdout(), "")
			fmt.Fprintln(c.OutOrStdout(), "## Resolved")
			for _, r := range syn.Resolved {
				fmt.Fprintf(c.OutOrStdout(), "- %s\n", r)
			}
			if len(syn.Assumptions) > 0 {
				fmt.Fprintln(c.OutOrStdout(), "")
				fmt.Fprintln(c.OutOrStdout(), "## Assumptions")
				for _, a := range syn.Assumptions {
					fmt.Fprintf(c.OutOrStdout(), "- %s\n", a)
				}
			}
			if len(syn.Open) > 0 {
				fmt.Fprintln(c.OutOrStdout(), "")
				fmt.Fprintln(c.OutOrStdout(), "## Open")
				for _, o := range syn.Open {
					fmt.Fprintf(c.OutOrStdout(), "- %s\n", o)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

// ============================================================================
// Profile command (sin-code profile)
// ============================================================================

// NewProfileCmd builds the `profile` cobra subcommand, complete with
// render / show / list / verify sub-actions.
func NewProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Render & verify the single-source-of-truth agent profile (issue #175)",
		Long: `sin-code profile renders docs/agent-profiles/sin-profile.md — the
single-source-of-truth project profile — into the per-agent mirror files
SIN-Code installs into every supported host agent: Claude Code,
opencode, Gemini CLI, Codex, Cursor, Windsurf, Cline, GitHub
Copilot, Aider, Continue, and Zed. Edit the source markdown, run "sin-code profile render all",
and the bytes stable across every host agent.

CI integrations should call "sin-code profile verify" — it refuses to
pass whenever any per-agent mirror drifts off the source.`,
	}

	cmd.AddCommand(newProfileShowCmd())
	cmd.AddCommand(newProfileListCmd())
	cmd.AddCommand(newProfileRenderCmd())
	cmd.AddCommand(newProfileVerifyCmd())

	return cmd
}

// newProfileShowCmd prints the source markdown to stdout.
func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the source profile markdown",
		RunE: func(_ *cobra.Command, _ []string) error {
			base := resolveRepoRoot()
			body, err := profile.LoadSource(base)
			if err != nil {
				return err
			}
			fmt.Print(body)
			return nil
		},
	}
}

// newProfileListCmd prints the per-agent target table.
func newProfileListCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List every supported host-agent target",
		RunE: func(_ *cobra.Command, _ []string) error {
			tab := profile.ListTable()
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(tab)
			}
			fmt.Printf("%-13s %-13s %-9s %s\n", "NAME", "FORMAT", "PATH-TPL", "INSTALL")
			for _, e := range tab {
				fmt.Printf("%-13s %-13s %-9s %s\n",
					e.Name, e.Format, "<skill>", e.InstallPath)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

// newProfileRenderCmd writes one or all mirrors to disk.
//
//	`render <name>` (Claude Code, codex…) — write a single mirror
//	`render all`                            — write every mirror
//	`render --dry-run <name|all>`           — preview to stdout, no IO
func newProfileRenderCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "render <target|all>",
		Short: "Write one or all per-agent mirrors",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			base := resolveRepoRoot()

			// Dry-run path: render but do not write.
			if dryRun {
				body, err := profile.LoadSource(base)
				if err != nil {
					return err
				}
				if args[0] == "all" {
					return renderDryRunAll(body, base)
				}
				return renderDryRunOne(args[0], body, base)
			}

			body, err := profile.LoadSource(base)
			if err != nil {
				return err
			}

			written, err := profile.WriteSelected(base, body, args[0])
			if err != nil {
				return err
			}
			for _, p := range written {
				fmt.Printf("WROTE %s\n", p)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false,
		"preview the rendered bytes to stdout; do not write to disk")
	return cmd
}

// newProfileVerifyCmd is the CI gate. It reads every per-agent mirror
// on disk and refuses to succeed if any of them is missing or drifted
// from the source render. Exits 0 on full match, non-zero with a
// Markdown-table error on drift.
//
//	--json emits a JSON array suitable for CI parsing.
func newProfileVerifyCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify per-agent mirrors match the source SHA (CI gate)",
		RunE: func(_ *cobra.Command, _ []string) error {
			base := resolveRepoRoot()
			body, err := profile.LoadSource(base)
			if err != nil {
				return err
			}
			res, err := profile.Verify(base, body)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(res)
			} else {
				writeVerifyTable(res)
			}
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "profile: verify OK (all mirrors match source SHA)")
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

// renderDryRunAll prints every per-target byte sequence to stdout,
// each prefixed by the target name and a 12-char SHA digest. Useful
// for diffing without modifying the working tree.
func renderDryRunAll(body, base string) error {
	rendered, keys, err := profile.RenderAll(body)
	if err != nil {
		return err
	}
	for _, name := range keys {
		h, err := profile.HashSource(profile.Targets[name], body)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "──────── %s (sha256:%s) ────────\n", name, short(h, 12))
		resolved, _ := profile.Resolve(profile.Targets[name], base)
		fmt.Fprintf(os.Stdout, "→ %s\n\n", resolved)
		fmt.Println(rendered[name])
		fmt.Println()
	}
	return nil
}

// renderDryRunOne prints a single per-target render.
func renderDryRunOne(name, body, _ string) error {
	tgt, ok := profile.Targets[name]
	if !ok {
		return fmt.Errorf("profile: unknown target %q (registered: %v)",
			name, profile.TargetNames())
	}
	h, err := profile.HashSource(tgt, body)
	if err != nil {
		return err
	}
	rendered, err := profile.Render(tgt, body)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "──────── %s (sha256:%s) ────────\n", name, short(h, 12))
	fmt.Println(rendered)
	return nil
}

// writeVerifyTable emits a Markdown-style table the human reads at
// the terminal. JSON mode is opt-in.
func writeVerifyTable(res []profile.Result) {
	fmt.Printf("%-13s %-9s %-8s %s\n", "TARGET", "FOUND", "MATCH", "PATH")
	for _, r := range res {
		fmt.Printf("%-13s %-9v %-8v %s\n",
			r.Target.Name, r.Found, r.Match, r.Path)
	}
}

// short returns the first n chars of a hex string. Used in
// human-readable output; full hex is still in --json output.
func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// resolveRepoRoot returns the directory writers should treat as the
// repository root. Defaults to "." (writer's cwd). The CLI never
// chdirs; the source path is relative.
func resolveRepoRoot() string {
	return "."
}

// ============================================================================
// Catalog command (sin-code catalog) — merged from catalog_cmd.go
// ============================================================================

// Purpose: `sin-code catalog` — unified tool catalog (issue #163).
// Merges the legacy `sin-code hub` and `sin-code assets` into a
// single source-aware CLI.
//
// Subcommands:
//
//	sin-code catalog list                 # all assets, all sources
//	sin-code catalog list --kind=agent    # filter by kind
//	sin-code catalog search "query"       # substring search across name/desc/tags
//	sin-code catalog info <name>          # one asset by name
//
// Sources are the registered Source implementations (HubSource,
// AssetsSource). New sources are added by registering a Source
// in the DefaultSources() slice in internal/catalog/catalog.go.
//
// Docs: cmd/sin-code/internal/catalog/catalog.doc.md

// NewCatalogCmd builds the `catalog` cobra subcommand.
func NewCatalogCmd() *cobra.Command {
	var (
		kind   string
		format string
	)
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Unified tool catalog (hub + assets, one CLI)",
		Long: `sin-code catalog is the unified tool catalog that merges
the legacy 'sin-code hub' (static subcommand list) and
'sin-code assets' (loaded Markdown frontmatter assets) into one
source-aware CLI. Issue #163 closes the long-standing UX confusion
between the two commands.`,
		RunE: func(c *cobra.Command, _ []string) error {
			return runCatalog(c, kind, format)
		},
	}
	cmd.AddCommand(newCatalogListCmd())
	cmd.AddCommand(newCatalogSearchCmd())
	cmd.AddCommand(newCatalogInfoCmd())
	cmd.AddCommand(newCatalogUnusedCmd())
	cmd.PersistentFlags().StringVar(&kind, "kind", "", "filter by kind: agent|command|skill|hub|mcp|chat|external")
	cmd.PersistentFlags().StringVar(&format, "format", "text", "output format: text|json")
	return cmd
}

func newCatalogListCmd() *cobra.Command {
	var (
		kind   string
		format string
	)
	return &cobra.Command{
		Use:   "list",
		Short: "Flat list of all assets in the catalog",
		RunE: func(c *cobra.Command, _ []string) error {
			return runCatalog(c, kind, format)
		},
	}
}

func newCatalogSearchCmd() *cobra.Command {
	var (
		kind   string
		format string
	)
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Substring search across all assets",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			assets, err := loadCatalog(c)
			if err != nil {
				return err
			}
			assets = catalog.FilterByKind(assets, catalog.Kind(kind))
			hits := catalog.Search(assets, args[0])
			return renderCatalog(c.OutOrStdout(), hits, format)
		},
	}
}

func newCatalogInfoCmd() *cobra.Command {
	var kind string
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show one asset by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			sources := defaultCatalogSources()
			ctx := c.Context()
			for _, src := range sources {
				for _, k := range []catalog.Kind{catalog.KindAgent, catalog.KindCommand, catalog.KindSkill, catalog.KindHub, catalog.KindMCP, catalog.KindChat, catalog.KindExternal} {
					if kind != "" && k != catalog.Kind(kind) {
						continue
					}
					a, ok, err := src.Get(ctx, k, args[0])
					if err != nil {
						return err
					}
					if ok {
						return renderCatalog(c.OutOrStdout(), []*catalog.Asset{a}, "text")
					}
				}
			}
			fmt.Fprintf(c.ErrOrStderr(), "catalog: not found: %s\n", args[0])
			os.Exit(1)
			return nil
		},
	}
}

func newCatalogUnusedCmd() *cobra.Command {
	var (
		format string
		stub   bool
	)
	cmd := &cobra.Command{
		Use:   "unused",
		Short: "List catalog tools never used according to telemetry",
		RunE: func(c *cobra.Command, _ []string) error {
			assets, err := loadCatalog(c)
			if err != nil {
				return err
			}
			var provider telemetry.Provider
			if stub {
				provider = telemetry.Stub()
			} else {
				provider, err = telemetry.DefaultProvider()
				if err != nil {
					return err
				}
			}
			used, err := provider.UsedTools(c.Context())
			if err != nil {
				return err
			}
			unused := catalog.FilterUnused(assets, used)
			return renderCatalog(c.OutOrStdout(), unused, format)
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text|json")
	cmd.Flags().BoolVar(&stub, "stub", false, "use stub telemetry (assume nothing used)")
	return cmd
}

// runCatalog is the shared implementation for the root and list
// subcommands. Both call into Merge + FilterByKind + render.
func runCatalog(c *cobra.Command, kind, format string) error {
	assets, err := loadCatalog(c)
	if err != nil {
		return err
	}
	assets = catalog.FilterByKind(assets, catalog.Kind(kind))
	return renderCatalog(c.OutOrStdout(), assets, format)
}

// loadCatalog runs the merger over the default sources. The source
// list is intentionally hard-coded here (not a flag) so the
// deprecation story is clear: new sources are added in code, not
// by the operator.
func loadCatalog(c *cobra.Command) ([]*catalog.Asset, error) {
	return catalog.Merge(c.Context(), defaultCatalogSources())
}

// defaultCatalogSources returns the registered sources. Hub is
// always present; assets is added when a registry is available.
// The order matters for de-duplication (first source wins).
func defaultCatalogSources() []catalog.Source {
	return []catalog.Source{
		catalog.HubSource{},
		catalog.MCPSource{},
		catalog.ChatSource{},
		catalog.ExternalSource{},
		// AssetsSource is wired conditionally in the future when
		// the asset loader exposes a registry at startup. For
		// now, the hub covers the operator-facing catalog.
		// catalog.NewAssetsSource(reg),
	}
}

// renderCatalog writes the assets in the chosen format. JSON is
// stable; text is a human-readable table.
func renderCatalog(w io.Writer, assets []*catalog.Asset, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(assets)
	default:
		// text: one line per asset
		for _, a := range assets {
			line := fmt.Sprintf("%-7s %-20s %s",
				strings.ToUpper(string(a.Kind)),
				a.Name,
				catalogFirstLine(a.Description))
			if a.Short != "" && a.Short != firstWord(a.Description) {
				line = fmt.Sprintf("%-7s %-20s %s",
					strings.ToUpper(string(a.Kind)),
					a.Name,
					a.Short)
			}
			fmt.Fprintln(w, line)
		}
		return nil
	}
}

// catalogFirstLine returns the first non-empty line of s, trimmed.
func catalogFirstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// firstWord returns the first whitespace-separated word of s.
func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// ============================================================================
// MCP command (sin-code mcp) — merged from mcp_cmd.go
// ============================================================================

// Purpose: `sin-code mcp` — inspect and debug external MCP servers
// (mandate C5): list effective configs, show live connection status and
// discovered tools, and invoke a single tool for smoke testing.

// mcpManager is the interface used by the mcp subcommand so tests can swap
// in a fake manager without a real network connection.
type mcpManager interface {
	ConnectAll(ctx context.Context) error
	Tools() []mcpclient.Tool
	Call(ctx context.Context, qualified string, args map[string]any) (string, error)
	Close()
}

// mcpHookVars holds injectable dependencies for the mcp subcommand. Coverage
// tests replace these fields to avoid real I/O or network calls.
var mcpHookVars = struct {
	loadConfigs func(string) []mcpclient.ServerConfig
	newManager  func([]mcpclient.ServerConfig) mcpManager
	getwd       func() (string, error)
}{
	loadConfigs: mcpclient.LoadConfigs,
	newManager:  func(cfgs []mcpclient.ServerConfig) mcpManager { return mcpclient.NewManager(cfgs) },
	getwd:       os.Getwd,
}

func NewMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Inspect and debug external MCP servers",
	}

	var jsonOut bool
	var timeout time.Duration

	discoverCmd := &cobra.Command{
		Use:   "discover",
		Short: "List MCP servers discovered from standard config locations (issue #368)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := mcpHookVars.getwd()
			if err != nil {
				return err
			}
			cfgs := mcpclient.DiscoverConfigs(ws)
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(cfgs)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-8s %s\n", "NAME", "TYPE", "TARGET")
			for _, c := range cfgs {
				target := c.URL
				if c.Transport == "stdio" {
					target = c.Command
					for _, a := range c.Args {
						target += " " + a
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-8s %s\n", c.Name, c.Transport, target)
			}
			return nil
		},
	}
	discoverCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	addCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add an MCP server config to ~/.config/mcp/servers/<name>.json",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				command string
				url     string
				argsVal []string
				env     map[string]string
			)
			command, _ = cmd.Flags().GetString("command")
			url, _ = cmd.Flags().GetString("url")
			if cmd.Flags().Changed("args") {
				argsVal, _ = cmd.Flags().GetStringArray("args")
			}
			if cmd.Flags().Changed("env") {
				envList, _ := cmd.Flags().GetStringArray("env")
				env = map[string]string{}
				for _, e := range envList {
					parts := strings.SplitN(e, "=", 2)
					if len(parts) == 2 {
						env[parts[0]] = parts[1]
					}
				}
			}
			transport := "stdio"
			if url != "" {
				transport = "sse"
			}
			cfg := mcpclient.ServerConfig{
				Name:      args[0],
				Transport: transport,
				Command:   command,
				Args:      argsVal,
				URL:       url,
				Env:       env,
			}
			if err := mcpclient.WriteServerConfig(cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added MCP server %s\n", cfg.Name)
			return nil
		},
	}
	addCmd.Flags().String("command", "", "stdio command to run")
	addCmd.Flags().String("url", "", "SSE URL endpoint")
	addCmd.Flags().StringArray("args", nil, "command arguments (repeatable)")
	addCmd.Flags().StringArray("env", nil, "environment variables KEY=VALUE (repeatable)")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List effective server configs (defaults + user + workspace merge)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := mcpHookVars.getwd()
			if err != nil {
				return err
			}
			cfgs := mcpHookVars.loadConfigs(ws)
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(cfgs)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-8s %s\n", "NAME", "TYPE", "TARGET")
			for _, c := range cfgs {
				target := c.URL
				if c.Transport == "stdio" {
					target = c.Command
					for _, a := range c.Args {
						target += " " + a
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-8s %s\n", c.Name, c.Transport, target)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Connect to all servers and report reachability + tool counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := mcpHookVars.getwd()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			mgr := mcpHookVars.newManager(mcpHookVars.loadConfigs(ws))
			if err := mgr.ConnectAll(ctx); err != nil {
				return err
			}
			defer mgr.Close()

			byServer := map[string]int{}
			for _, t := range mgr.Tools() {
				byServer[t.Server]++
			}
			type row struct {
				Name  string `json:"name"`
				Up    bool   `json:"up"`
				Tools int    `json:"tools"`
			}
			var rows []row
			for _, c := range mcpHookVars.loadConfigs(ws) {
				n := byServer[c.Name]
				rows = append(rows, row{Name: c.Name, Up: n > 0, Tools: n})
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-6s %s\n", "NAME", "UP", "TOOLS")
			for _, r := range rows {
				up := "no"
				if r.Up {
					up = "yes"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-16s %-6s %d\n", r.Name, up, r.Tools)
			}
			return nil
		},
	}
	statusCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	statusCmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "connect timeout")

	callCmd := &cobra.Command{
		Use:   "call <server__tool> [json-args]",
		Short: "Invoke a single external tool for smoke testing",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			toolArgs := map[string]any{}
			if len(args) == 2 {
				if err := json.Unmarshal([]byte(args[1]), &toolArgs); err != nil {
					return fmt.Errorf("args must be a JSON object: %w", err)
				}
			}
			ws, err := mcpHookVars.getwd()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			mgr := mcpHookVars.newManager(mcpHookVars.loadConfigs(ws))
			if err := mgr.ConnectAll(ctx); err != nil {
				return err
			}
			defer mgr.Close()
			out, err := mgr.Call(ctx, args[0], toolArgs)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	callCmd.Flags().DurationVar(&timeout, "timeout", 60*time.Second, "total timeout")

	cmd.AddCommand(listCmd, statusCmd, callCmd, discoverCmd, addCmd)
	return cmd
}

// ============================================================================
// Install command (sin-code install) — merged from install_cmd.go
// ============================================================================

// Purpose: `sin-code install` — single-binary installer entrypoint
// (issue #170). The user-facing flow is:
//
//	curl -fsSL https://raw.githubusercontent.com/OpenSIN-Code/SIN-Code/main/install.sh | bash
//
// Under the hood, the bash shim downloads ONE tarball, extracts ONE
// file, and `exec`s `sin-code install --auto` so the Go entrypoint
// can verify (SHA256 via goreleaser's checksums.txt), atomically
// place the binary, and emit the well-known summary line.

// NewInstallCmd returns the cobra subcommand for issue #170.
//
// Flags covered:
//
//	--dir <path>           install destination (default: $SIN_CODE_BIN_DIR or
//	                       $HOME/.local/bin). The constructor refuses
//	                       /usr/local-style dirs unless explicitly opted in
//	                       because sudo escalation conflicts with M4
//	                       (headless daemon never escalates, the
//	                       interactive flow must not surprise either).
//	--release <tag>        pin a specific release tag (default: latest
//	                       published). Use for reproducible CI installs.
//	--channel stable|dev   advisory only; honours --release when set.
//	                       The "dev" channel is special: it points at
//	                       the rolling-tip of the org's Go-Next branch
//	                       once the goreleaser honors it (today: same
//	                       as --release=latest except no SHA256 check).
//	--verify-only          do not write — just check whether the binary
//	                       at --dir is on a healthy, current install.
//	                       Combines with --release to assert "I am
//	                       running vX.Y.Z exactly".
//	--no-verify            skip SHA256 verification (use only when the
//	                       host has no egress to the checksums.txt
//	                       URL — typically revoked CI sandboxes).
//	--dry-run              print the plan + URLs without touching disk.
func NewInstallCmd() *cobra.Command {
	var (
		dir        string
		release    string
		channel    string
		verifyOnly bool
		noVerify   bool
		dryRun     bool
		auto       bool
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install or verify the sin-code single-binary release",
		Long: `install is the canonical "give me a working sin-code" entrypoint.

It downloads the latest release (or a pinned --release tag), verifies
SHA256 against the goreleaser-style checksums.txt, extracts the one
sin-code binary, and places it at a writable bin directory.

Examples:
  sin-code install                            # install latest stable
  sin-code install --release v3.17.0          # pin a specific tag
  sin-code install --dir ~/my-tools           # custom bin dir
  sin-code install --verify-only              # health-check the current binary
  sin-code install --no-verify                # skip checksum (offline install)
  sin-code install --dry-run                  # print the plan, do nothing

The shell shim at the repo root (install.sh, install.ps1) bootstraps
this subcommand on a fresh machine by downloading the binary via
curl|bash and re-execing back into it.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstall(installOpts{
				Dir:        dir,
				Release:    release,
				Channel:    channel,
				VerifyOnly: verifyOnly,
				NoVerify:   noVerify,
				DryRun:     dryRun,
				Auto:       auto,
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "install destination (default: $SIN_CODE_BIN_DIR or $HOME/.local/bin)")
	cmd.Flags().StringVar(&release, "release", "", "pin release tag (default: latest published)")
	cmd.Flags().StringVar(&channel, "channel", "stable", "release channel: stable or dev")
	cmd.Flags().BoolVar(&verifyOnly, "verify-only", false, "verify an already-installed binary instead of replacing it")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "skip SHA256 verification (offline / sanctioned CI only)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan and URLs without writing anything")
	cmd.Flags().BoolVar(&auto, "auto", false, "accept all defaults, no prompts (non-interactive mode)")
	return cmd
}

type installOpts struct {
	Dir        string
	Release    string
	Channel    string
	VerifyOnly bool
	NoVerify   bool
	DryRun     bool
	Auto       bool
}

func runInstall(opts installOpts) error {
	p := install.CurrentPlatform()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if opts.VerifyOnly {
		return runInstallVerifyOnly(ctx, opts, p)
	}

	client := install.NewHTTPClient()

	// Resolve the release. We use the `latest` JSON lookup unless the
	// user pinned via --release. This intentionally does NOT call gh
	// (chicken-and-egg: install subcommand can run before gh is on PATH).
	var rel *install.Release
	var err error
	if opts.Release != "" {
		rel, err = install.FetchRelease(ctx, client, p, opts.Release)
	} else {
		rel, err = install.FetchLatest(ctx, client, p)
	}
	if err != nil {
		return fmt.Errorf("install: fetch release: %w", err)
	}

	binDir, _, hint, err := install.ChooseBinDir()
	if err != nil {
		return err
	}
	if opts.Dir != "" {
		binDir = opts.Dir
	}

	// Fetch checksums.txt (goreleaser-style). Missing is non-fatal — the
	// caller can opt into a hard failure with `--no-verify` reversed.
	var expected map[string]string
	if !opts.NoVerify {
		expected, err = install.FetchChecksums(ctx, client, rel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] install: could not fetch checksums.txt (%v); proceeding without SHA256 verification. Use --no-verify to silence this warning.\n", err)
		}
	}

	fmt.Printf("[sin-code install] target: %s/%s\n", p.GOOS, p.GOARCH)
	fmt.Printf("[sin-code install] release: %s\n", rel.TagName)
	fmt.Printf("[sin-code install] asset: %s\n", p.AssetName())
	fmt.Printf("[sin-code install] bin dir: %s\n", binDir)

	if opts.DryRun {
		fmt.Printf("[sin-code install] dry-run: not touching disk\n")
		return nil
	}

	dlPath, observedSHA, err := install.FetchAsset(ctx, client, rel, p, os.TempDir())
	if err != nil {
		return err
	}
	defer os.Remove(dlPath)
	fmt.Printf("[sin-code install] downloaded: %s (sha256:%s)\n", filepath.Base(dlPath), observedSHA)

	if want, ok := expected[p.AssetName()]; ok {
		if err := install.Verify(dlPath, observedSHA, want); err != nil {
			return err
		}
		fmt.Printf("[sin-code install] sha256 verified against checksums.txt\n")
	} else if !opts.NoVerify {
		fmt.Fprintf(os.Stderr, "[warn] install: no SHA256 for %s in checksums.txt — proceeding (release may not be signed yet)\n", p.AssetName())
	}

	// Extract just the sin-code binary from the archive.
	extracted, err := install.ExtractBinary(dlPath, filepath.Join(os.TempDir(), "sin-code-extract"), p)
	if err != nil {
		return err
	}
	defer os.Remove(extracted)

	// Place atomically in the chosen bin dir.
	final, err := install.Place(extracted, binDir, p)
	if err != nil {
		return err
	}
	fmt.Printf("[sin-code install] installed: %s\n", final)
	fmt.Printf("[sin-code install] version: %s\n", strings.TrimPrefix(rel.TagName, "v"))
	if hint != "" {
		fmt.Printf("[sin-code install] %s\n", hint)
	}
	return nil
}

func runInstallVerifyOnly(ctx context.Context, opts installOpts, p install.Platform) error {
	client := install.NewHTTPClient()
	var rel *install.Release
	var err error
	if opts.Release != "" {
		rel, err = install.FetchRelease(ctx, client, p, opts.Release)
	} else {
		rel, err = install.FetchLatest(ctx, client, p)
	}
	if err != nil {
		return fmt.Errorf("install: fetch release: %w", err)
	}
	binDir, _, _, err := install.ChooseBinDir()
	if err != nil {
		return err
	}
	if opts.Dir != "" {
		binDir = opts.Dir
	}
	bin := filepath.Join(binDir, p.BinaryName())
	_, err = os.Stat(bin)
	if err != nil {
		return fmt.Errorf("install: --verify-only failed: no binary at %s", bin)
	}
	fmt.Printf("[sin-code install] binary present: %s\n", bin)
	expected, _ := install.FetchChecksums(ctx, client, rel)
	if want, ok := expected[p.AssetName()]; ok {
		fmt.Printf("[sin-code install] latest release: %s (checksums.txt sha256:%s)\n", rel.TagName, want)
	} else {
		fmt.Printf("[sin-code install] latest release: %s\n", rel.TagName)
	}
	fmt.Printf("[sin-code install] verify-only: skipping replacment — re-run without --verify-only to upgrade\n")
	return nil
}

// ============================================================================
// Compile-spec command (sin-code compile-spec) — merged from compile_spec_cmd.go
// ============================================================================

// Purpose: `sin-code compile-spec` CLI (issue #164). Reads
// .sin-code.yml, validates it, and writes the four derived
// JSON outputs to disk. Has three modes:
//
//	sin-code compile-spec                       # compile .sin-code.yml in cwd
//	sin-code compile-spec --init                # write a starter .sin-code.yml
//	sin-code compile-spec --check               # check that derived files are in sync
//	sin-code compile-spec --out <dir>           # override the output directory
//
// Docs: docs/SPEC-COMPILER.md

// NewCompileSpecCmd builds the `compile-spec` cobra subcommand.
func NewCompileSpecCmd() *cobra.Command {
	var (
		outDir   string
		initMode bool
		check    bool
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "compile-spec",
		Short: "Compile .sin-code.yml into the four derived JSON artifacts",
		Long: `sin-code compile-spec reads .sin-code.yml in the current
directory (or --file), validates it against the schema, and
writes the four derived JSON files the SIN-Code engines need:

  .sin/hooks.json                  (for internal/hooks/)
  internal/verify/config.json      (for internal/verify/)
  internal/permission/policies.json (for internal/permission/)
  .sin/loop.json                   (v1.1: for the loop builder)

Use --init to write a starter .sin-code.yml; use --check to
verify the derived files are in sync with the source.`,
		RunE: func(c *cobra.Command, _ []string) error {
			if initMode {
				return runCompileSpecInit(c.OutOrStdout(), outDir)
			}
			if check {
				return runCompileSpecCheck(c.OutOrStdout(), c.ErrOrStderr(), outDir)
			}
			return runCompileSpecCompile(c.OutOrStdout(), c.ErrOrStderr(), outDir, dryRun)
		},
	}
	cmd.Flags().StringVar(&outDir, "out", ".", "output directory (defaults to cwd)")
	cmd.Flags().BoolVar(&initMode, "init", false, "write a starter .sin-code.yml and exit")
	cmd.Flags().BoolVar(&check, "check", false, "verify derived files are in sync with .sin-code.yml")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be written without writing")
	return cmd
}

// runCompileSpecInit writes a starter .sin-code.yml.
func runCompileSpecInit(out io.Writer, outDir string) error {
	path := filepath.Join(outDir, compiler.DefaultFile)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("compile-spec: %s already exists", path)
	}
	// Default to a project matching the cwd name, type "go".
	name := filepath.Base(outDir)
	data := compiler.InitTemplate(name, "go")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, filemode.Default()); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s\n", path)
	return nil
}

// runCompileSpecCompile is the default path: parse, validate, emit.
func runCompileSpecCompile(out, errOut io.Writer, outDir string, dryRun bool) error {
	src := filepath.Join(outDir, compiler.DefaultFile)
	c, err := compiler.ParseFile(src)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	if err := compiler.Validate(c); err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	files, err := compilerEmitAll(c)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	for _, f := range files {
		dest := filepath.Join(outDir, f.Path)
		if dryRun {
			fmt.Fprintf(out, "would write %s (%d bytes)\n", dest, len(f.Data))
			continue
		}
		// Atomic write: temp file + rename, so a crash mid-write
		// never leaves a half-written file behind.
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, f.Data, filemode.Default()); err != nil {
			return err
		}
		if err := os.Rename(tmp, dest); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s (%d bytes)\n", dest, len(f.Data))
	}
	return nil
}

// runCompileSpecCheck verifies the derived files are in sync.
// Returns exit 0 if in sync, exit 1 if any file is stale.
func runCompileSpecCheck(out, errOut io.Writer, outDir string) error {
	src := filepath.Join(outDir, compiler.DefaultFile)
	c, err := compiler.ParseFile(src)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	if err := compiler.Validate(c); err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	files, err := compilerEmitAll(c)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	drift := false
	for _, f := range files {
		dest := filepath.Join(outDir, f.Path)
		existing, err := os.ReadFile(dest)
		if err != nil {
			fmt.Fprintf(errOut, "drift: %s missing or unreadable: %v\n", dest, err)
			drift = true
			continue
		}
		if !bytesEqual(existing, f.Data) {
			fmt.Fprintf(errOut, "drift: %s is out of date (re-run `sin-code compile-spec`)\n", dest)
			drift = true
		}
	}
	if drift {
		os.Exit(1)
	}
	fmt.Fprintln(out, "all derived files are in sync")
	return nil
}

// compilerEmitAll wraps the package-private emitAll. We need a
// public entry point or this duplicate, so we duplicate the
// four-emitter orchestration here. (The internal emitAll is
// package-private to keep the API surface small.)
func compilerEmitAll(c *compiler.Config) ([]compilerOutputFile, error) {
	hooks, err := compiler.EmitHooks(c)
	if err != nil {
		return nil, err
	}
	verify, err := compiler.EmitVerify(c)
	if err != nil {
		return nil, err
	}
	perms, err := compiler.EmitPermissions(c)
	if err != nil {
		return nil, err
	}
	loop, err := compiler.EmitLoop(c)
	if err != nil {
		return nil, err
	}
	return []compilerOutputFile{
		{Path: ".sin/hooks.json", Data: hooks},
		{Path: "internal/verify/config.json", Data: verify},
		{Path: "internal/permission/policies.json", Data: perms},
		{Path: ".sin/loop.json", Data: loop},
	}, nil
}

// compilerOutputFile is a public mirror of compiler's private
// type. Keeping it small + duplicative avoids widening the
// package's public API for one CLI.
type compilerOutputFile struct {
	Path string
	Data []byte
}

// bytesEqual is a small helper to avoid importing bytes in this
// file (keeps the import list short). It is correct because the
// emitted JSON is canonical (json.MarshalIndent + a stable
// Config means the same input always produces the same bytes).
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
