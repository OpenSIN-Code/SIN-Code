// SPDX-License-Identifier: MIT
// Purpose: evaluate Golden Datasets against the agent loop
// (agentloop.Loop) and capture per-case outcomes (issue #75, M2, C1).
//
// This package does NOT depend on a real LLM. The Runner takes a
// Loop pointer; that Loop either uses its real Completion func or
// (for tests + CI without API keys) the Loop.RunOverride hook that
// the agentloop package already exposes (loop.go:62–64). That
// keeps the dataset runner cheap to unit-test and safe to run in
// air-gapped CI.
//
// Docs: runner.doc.md
package dataset

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// RunResult is one row in the JSON output emitted by the CLI.
// All fields are plain JSON types so the n8n-CI layer can ingest
// without a custom deserializer.
type RunResult struct {
	TestCaseID    string        `json:"test_case_id"`
	Description   string        `json:"description,omitempty"`
	SessionID     string        `json:"session_id"`
	Success       bool          `json:"success"`
	Turns         int           `json:"turns"`
	Duration      time.Duration `json:"duration_ns"`
	ToolsUsed     []string      `json:"tools_used"`
	VerifyPassed  bool          `json:"verify_passed"`
	FinalOutput   string        `json:"final_output,omitempty"`
	Error         string        `json:"error,omitempty"`
	JudgeScore    float64       `json:"judge_score,omitempty"`
	JudgeFeedback string        `json:"judge_feedback,omitempty"`
	Tags          []string      `json:"tags,omitempty"`
	TimedOut      bool          `json:"timed_out"`
}

// RunnerConfig is the per-run configuration. Empty VerifyMode falls
// through to whatever the Loop already has wired.
type RunnerConfig struct {
	ProfileName    string
	HeadlessMode   bool
	VerifyMode     string // "poc" | "oracle" | "off"
	TimeoutPerCase time.Duration
	MaxConcurrency int // 0 -> 1 (serial, deterministic)
	// UseModel, when true, means the runner is plugged into a real chat
	// completion path (eval --use-model, issue #261). When false, the
	// loop runs against the offline stub and any ScorerConfig with
	// RequiresModel=true is skipped (issue #264).
	UseModel bool
}

// Runner executes one Dataset against one agentloop.Loop. Sessions
// are created in the supplied store; if store is nil the runner
// panics — that is a user bug, not a graceful degradation case.
type Runner struct {
	cfg    RunnerConfig
	loop   *agentloop.Loop
	store  *session.Store
	Scorer evalharness.Scorer // optional override scorer applied to every case
}

// NewRunner constructs a Runner. Zero-value fields get sensible
// defaults (1x concurrency, 5m timeout, headless=true).
func NewRunner(cfg RunnerConfig, loop *agentloop.Loop, store *session.Store) (*Runner, error) {
	if loop == nil {
		return nil, errors.New("dataset runner: loop is nil")
	}
	if store == nil {
		return nil, errors.New("dataset runner: session store is nil")
	}
	if cfg.MaxConcurrency == 0 {
		cfg.MaxConcurrency = 1
	}
	if cfg.TimeoutPerCase == 0 {
		cfg.TimeoutPerCase = 5 * time.Minute
	}
	cfg.HeadlessMode = true // datasets are always evaluated offline / headless
	if cfg.VerifyMode == "" {
		cfg.VerifyMode = string(verify.ModePoC)
	}
	return &Runner{cfg: cfg, loop: loop, store: store}, nil
}

// RunDataset evaluates every TestCase in ds serially. Concurrency >1
// is reserved for a future worker-pool implementation; the serial
// path is fully covered by tests today.
func (r *Runner) RunDataset(ctx context.Context, ds *Dataset) ([]RunResult, error) {
	if ds == nil {
		return nil, errors.New("dataset runner: nil dataset")
	}
	out := make([]RunResult, 0, len(ds.TestCases))
	for i := range ds.TestCases {
		fmt.Printf("[%d/%d] %s\n", i+1, len(ds.TestCases), ds.TestCases[i].ID)
		out = append(out, r.RunCase(ctx, &ds.TestCases[i]))
	}
	return out, nil
}

// RunCase evaluates a single TestCase. Exposed so the CLI `eval run`
// command can stream progress and re-run failing cases without
// re-walking the whole dataset.
func (r *Runner) RunCase(ctx context.Context, tc *TestCase) RunResult {
	res := RunResult{
		TestCaseID:  tc.ID,
		Description: tc.Description,
		Tags:        append([]string(nil), tc.Tags...),
	}
	timeout := r.cfg.TimeoutPerCase
	if tc.Constraints.Timeout != "" {
		if d, err := time.ParseDuration(tc.Constraints.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	if timeout <= 0 {
		timeout = 30 * time.Second // last-resort cap so a misconfigured dataset can't hang the CLI
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	sess, err := r.store.StartOrResume("")
	if err != nil {
		res.Error = fmt.Sprintf("session create: %v", err)
		return res
	}
	res.SessionID = sess.ID

	// Wire live tool-coverage enforcement from the dataset constraints so the
	// agent loop rejects premature completion inside the run, not just after it
	// (issue #248). The constraints are reset per case via the fresh enforcer
	// created in Loop.Run.
	r.loop.CoverageRequiredTools = tc.Constraints.MustUseTools
	r.loop.CoverageForbiddenTools = tc.Constraints.ForbiddenTools

	loopRes, runErr := r.loop.Run(cctx, sess, tc.Prompt)
	res.Duration = time.Since(start)
	if cctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
	}
	if runErr != nil {
		// agentloop.Loop.Run returns an error result on max-turns.
		// Surface it but still record what we did observe so the
		// caller can decide pass/fail (verified=false is the
		// default for an erroring run).
		res.Error = runErr.Error()
		if loopRes != nil {
			res.SessionID = loopRes.SessionID
			res.Turns = loopRes.Turns
			res.FinalOutput = loopRes.Summary
			res.VerifyPassed = loopRes.Verified
		}
		if r.loop.Coverage != nil {
			res.ToolsUsed = r.loop.Coverage.Used()
		}
		res.Success = false
		r.applyRules(tc, &res)
		return res
	}

	res.Turns = loopRes.Turns
	res.VerifyPassed = loopRes.Verified
	res.FinalOutput = loopRes.Summary
	res.Success = loopRes.Verified
	// Surface the tools the loop actually invoked for reporting and for the
	// post-hoc constraint checks (now redundant with live enforcement, but kept
	// for backward compatibility with test overrides that don't run the loop).
	if r.loop.Coverage != nil {
		res.ToolsUsed = r.loop.Coverage.Used()
	}
	r.applyRules(tc, &res)
	r.applyScorer(tc, &res)
	return res
}

// applyRules evaluates Constraints + Expected; if any rule fails the
// result's Success flips to false even when the loop verified.
// Mutates res in-place and returns it so callers can chain.
func (r *Runner) applyRules(tc *TestCase, res *RunResult) RunResult {
	violations := []string{}

	// MaxTurns: > 0 means strict upper bound.
	if tc.Constraints.MaxTurns > 0 && res.Turns > tc.Constraints.MaxTurns {
		violations = append(violations, fmt.Sprintf("turns=%d > max_turns=%d", res.Turns, tc.Constraints.MaxTurns))
	}

	// Required verify.
	if tc.Constraints.RequireVerify && !res.VerifyPassed {
		violations = append(violations, "verify not passed")
	}

	// MustUse / Forbidden / Forbidden — caller is expected to fill ToolsUsed.
	// If the loop does not populate it (e.g. stub override), we don't fail
	// the test just because the slice is empty — the runner doesn't know
	// what happened inside the LLM. Tests that care about Constraints.Must*
	// must supply a Loop that records tools_used.
	for _, need := range tc.Constraints.MustUseTools {
		if !slices.Contains(res.ToolsUsed, need) {
			violations = append(violations, "missing required tool: "+need)
		}
	}
	for _, ban := range tc.Constraints.ForbiddenTools {
		if slices.Contains(res.ToolsUsed, ban) {
			violations = append(violations, "used forbidden tool: "+ban)
		}
	}

	// Expected keyword checks against FinalOutput.
	output := res.FinalOutput
	for _, kw := range tc.Expected.OutputContains {
		if !strings.Contains(strings.ToLower(output), strings.ToLower(kw)) {
			violations = append(violations, "missing output keyword: "+kw)
		}
	}
	for _, kw := range tc.Expected.OutputAvoids {
		if strings.Contains(strings.ToLower(output), strings.ToLower(kw)) {
			violations = append(violations, "contains forbidden output keyword: "+kw)
		}
	}

	if len(violations) > 0 {
		res.Success = false
		// Surface violations in Error so the CI step can read them
		// without parsing nested rule docs.
		if res.Error == "" {
			res.Error = "constraint/expected violations: " + strings.Join(violations, "; ")
		}
	}
	return *res
}

// applyScorer evaluates a configured or override scorer against the
// model's final output. If the scorer fails, res.Success flips to false.
//
// A per-case ScorerConfig that sets RequiresModel=true is skipped
// whenever the runner is not in real-model mode (RunnerConfig.UseModel
// is false). This is the dual-mode contract: compile_and_run cases can
// live alongside stub-friendly output_contains cases in the same
// dataset (#264) without breaking byte-stable offline CI.
func (r *Runner) applyScorer(tc *TestCase, res *RunResult) {
	if tc.Scorer.RequiresModel && !r.cfg.UseModel && r.Scorer == nil {
		return
	}
	var scorer evalharness.Scorer
	var cfg map[string]any
	if r.Scorer != nil {
		scorer = r.Scorer
	} else if tc.Scorer.Type != "" {
		cfg = tc.Scorer.ToEvalharnessConfig()
		var err error
		scorer, err = evalharness.ScorerFromConfig(cfg)
		if err != nil {
			res.Success = false
			if res.Error == "" {
				res.Error = "scorer: " + err.Error()
			}
			return
		}
	}
	if scorer == nil {
		return
	}
	score, passed, detail := scorer.Score(evalharness.EvalCase{
		ID:     tc.ID,
		Prompt: tc.Prompt,
		Scorer: cfg,
	}, evalharness.Output{Text: res.FinalOutput, Success: res.VerifyPassed})
	res.Success = res.Success && passed
	if res.Error == "" && !passed {
		res.Error = fmt.Sprintf("scorer: score=%.2f detail=%s", score, detail)
	}
}
