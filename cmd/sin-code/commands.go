// SPDX-License-Identifier: MIT
// Purpose: merged command constructors for the core agent/autonomy subcommands.
//
// This file consolidates the previously single-export files for the daemon,
// skill, and spec subcommands. Each section preserves its original behaviour
// and comments; the constructors are registered from cmd/sin-code/main.go.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/resource"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skilldist"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/spec"
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
