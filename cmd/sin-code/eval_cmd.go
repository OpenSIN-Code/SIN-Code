// SPDX-License-Identifier: MIT
// Purpose: `sin-code eval` — run Golden Datasets, emit a JSON
// report, and gate the CI job on `--min-pass-rate` (issue #75).
//
// Subcommands:
//
//	eval run --dataset <path> [--min-pass-rate N] [--json] [--trace]
//	eval run --dataset <path> --arm a,b,c   (issue #171 four-arm comparator)
//	eval compare --dataset <path>           (run all four arms and print matrix)
//	eval snapshot --dataset <path> --out snap.json  (write a snapshot row)
//	eval diff --snapshot a.json --snapshot b.json    (row-level delta)
//	eval list [--dir path]
//
// Driver logic lives here (eval/trace ARE first-party CLI, not
// Bridged-External — see cmd/sin-code/autodev_cmd.go for the
// opposite pattern). Without --judge-model the runner uses a
// deterministic stub Loop.Completion so CI runs succeed offline.
//
// Docs: eval_cmd.doc.md
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
	sinctrace "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/trace"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func NewEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run Golden Dataset evaluation suites",
		Long: `sin-code eval runs a Golden Dataset (JSON) against the agent loop
and reports the pass rate. Common CI pattern:

    sin-code eval run \
        --dataset evals/critical.json \
        --min-pass-rate 0.95 \
        --json

The four-arm comparator (issue #171) is opt-in via --arm:

    sin-code eval run --dataset evals/three-arm-example.json \
        --arm baseline,terse,lazy_skill,skill-code-create

Shortcuts:

    sin-code eval compare --dataset evals/three-arm-example.json
    sin-code eval snapshot --dataset evals/three-arm-example.json --out snap.json
    sin-code eval diff --snapshot snap-a.json --snapshot snap-b.json

Tracing is opt-in via --trace and ships to the chosen exporter.`,
	}
	cmd.AddCommand(
		newEvalRunCmd(),
		newEvalListCmd(),
		newEvalCompareCmd(),
		newEvalSnapshotCmd(),
		newEvalDiffCmd(),
	)
	return cmd
}

// ── eval run ────────────────────────────────────────────────────────

func newEvalRunCmd() *cobra.Command {
	var (
		datasetPath   string
		profile       string
		minPassRate   float64
		jsonOutput    bool
		timeout       time.Duration
		enableTracing bool
		traceExporter string
		traceEndpoint string
		traceInsecure bool
		judgeModel    string
		judgeEndpoint string
		judgeKeyEnv   string
		armsFlag      string
		userSkill     string
		modelPricing  string
		scorerType    string
		scorerLang    string
		scorerSelfChk string
		scorerSkip    bool
		scorerBinary  string
		// useModel switches the offline stub to a real OpenAI-compatible
		// chat completion. Wired through internal/llm.Client + agentloop's
		// provider adapter. Off by default — preserves the byte-stable
		// CI behaviour callers have relied on since #75.
		useModel bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a Golden Dataset and emit the pass-rate report",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if datasetPath == "" {
				return errors.New("eval run: --dataset is required")
			}
			// Issue #171 four-arm comparator: route to the comparator
			// path when --arm is set. Without --arm we keep the
			// legacy Golden-Dataset behaviour (single arm, full
			// agent loop) so we don't regress existing dashboards.
			if armsFlag != "" {
				return runArmComparator(ctx, datasetPath, armsFlag, userSkill, modelPricing, timeout, jsonOutput)
			}
			startedAt := time.Now().UTC()

			if enableTracing {
				tp, err := sinctrace.InitProvider(ctx, &sinctrace.ProviderConfig{
					ServiceName:    "sin-code-eval",
					ServiceVersion: "v3.18.0",
					Environment:    os.Getenv("SIN_ENV"),
					Exporter:       mustParseExporter(traceExporter),
					OTLPEndpoint:   traceEndpoint,
					OTLPInsecure:   traceInsecure,
				})
				if err != nil {
					return fmt.Errorf("eval run: init trace: %w", err)
				}
				defer func() {
					shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					_ = sinctrace.Shutdown(shCtx, tp)
				}()
			}

			ds, err := dataset.LoadDataset(datasetPath)
			if err != nil {
				return fmt.Errorf("eval run: load dataset: %w", err)
			}

			store, err := session.Open(filepath.Join(os.TempDir(), "sin-code-eval-sessions.db"))
			if err != nil {
				return fmt.Errorf("eval run: open session store: %w", err)
			}
			defer store.Close()

			// Verify gate is "off" — datasets own their own verify_cmd
			// expectation; we don't run a real go-test inside an eval
			// (the LLMs model would just hallucinate the verify-mode field).
			gate := verify.NewGate("off", nil, nil)

			// --use-model opt-in switch (issue #261): route to a real
			// chat completion instead of the offline stub. The
			// default is the stub so existing CI runs stay byte-stable
			// unless they actively opt in.
			useModel = useModel || strings.TrimSpace(os.Getenv("SIN_EVAL_USE_MODEL")) == "1"

			loop := &agentloop.Loop{
				Gate:         gate,
				Workspace:    workspaceRoot(datasetPath),
				MaxTurns:     80,
				SystemPrompt: style.RenderSystemPrompt("default"),
				Hooks:        &hooks.Engine{}, // empty: no user hooks during eval
			}
			if useModel {
				completion, merr := buildEvalCompletion()
				if merr != nil {
					return fmt.Errorf("eval run: --use-model: %w", merr)
				}
				loop.Completion = completion
			} else {
				// Default legacy path: offline stub. The runner does not
				// need a real agent loop to evaluate structural rules; it
				// is byte-stable across releases (#75).
				loop.RunOverride = stubRunOverride
			}

			var overrideScorer evalharness.Scorer
			if scorerType != "" {
				scorer, err := buildScorer(scorerType, scorerLang, scorerSelfChk, scorerSkip, scorerBinary)
				if err != nil {
					return fmt.Errorf("eval run: scorer: %w", err)
				}
				overrideScorer = scorer
			}

			runner, err := dataset.NewRunner(dataset.RunnerConfig{
				ProfileName:    profile,
				HeadlessMode:   true,
				VerifyMode:     "off",
				TimeoutPerCase: timeout,
				MaxConcurrency: 1,
			}, loop, store)
			if err != nil {
				return fmt.Errorf("eval run: new runner: %w", err)
			}
			runner.Scorer = overrideScorer

			results, err := runner.RunDataset(ctx, ds)
			if err != nil {
				return fmt.Errorf("eval run: run dataset: %w", err)
			}

			// Optional LLM-as-a-Judge pass. Wires defer-and-warn so a
			// missing API key degrades gracefully in CI; the JSON
			// report still gets emitted with judge fields zeroed.
			if judgeModel != "" {
				if err := applyJudge(ctx, results, judgeModel, judgeEndpoint, judgeKeyEnv); err != nil {
					fmt.Fprintf(os.Stderr, "warn: judge pass skipped: %v\n", err)
				}
			}

			report := eval.NewReport(ds, profile, minPassRate, results, startedAt, time.Now().UTC())

			if jsonOutput {
				if err := eval.WriteJSON(os.Stdout, report); err != nil {
					return fmt.Errorf("eval run: write json: %w", err)
				}
			} else {
				fmt.Printf("Dataset: %s (v%s) — Profile: %s\n", ds.Name, ds.Version, profile)
				fmt.Print(eval.FormatHuman(report.Summary))
			}

			return eval.PassRateFloor(report.Summary)
		},
	}

	cmd.Flags().StringVarP(&datasetPath, "dataset", "d", "", "Path to Golden Dataset JSON file (required)")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "Agent profile name")
	cmd.Flags().Float64Var(&minPassRate, "min-pass-rate", 0.9, "Minimum pass rate (0.0–1.0)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format (for CI)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Timeout per test case")
	cmd.Flags().BoolVar(&enableTracing, "trace", false, "Enable OpenTelemetry tracing")
	cmd.Flags().StringVar(&traceExporter, "trace-exporter", "stdout", "Trace exporter (stdout|otlp|noop)")
	cmd.Flags().StringVar(&traceEndpoint, "trace-endpoint", "localhost:4318", "OTLP HTTP endpoint (host:port)")
	cmd.Flags().BoolVar(&traceInsecure, "trace-insecure", true, "OTLP HTTP plaintext (no TLS)")
	cmd.Flags().StringVar(&judgeModel, "judge-model", "", "If set, run an LLM-as-a-Judge pass over successes (model id)")
	cmd.Flags().StringVar(&judgeEndpoint, "judge-endpoint", "https://api.openai.com/v1", "OpenAI-compatible base URL for the judge")
	cmd.Flags().StringVar(&judgeKeyEnv, "judge-key-env", "OPENAI_API_KEY", "Env var that holds the judge API key")
	cmd.Flags().StringVar(&armsFlag, "arm", "", "Comma-separated arm list (issue #171 comparator). Reserved tokens: baseline, terse, lazy_skill. Anything else is treated as a user-skill name. Empty = single-arm (legacy behavior).")
	cmd.Flags().StringVar(&userSkill, "skill", "skill-code-create", "User-skill arm name used when --arm contains a non-reserved token (issue #171).")
	cmd.Flags().StringVar(&modelPricing, "model-pricing", "stub", "Model-price entry from the comparator price book (issue #171). Default 'stub' = USD 0 in CI.")
	cmd.Flags().StringVar(&scorerType, "scorer", "", "Override scorer type (compile_and_run|exact|contains)")
	cmd.Flags().StringVar(&scorerLang, "language", "", "Language for --scorer compile-and-run (go|python|javascript|bash)")
	cmd.Flags().StringVar(&scorerSelfChk, "self-check", "", "Self-check code for --scorer compile-and-run")
	cmd.Flags().BoolVar(&scorerSkip, "skip-test", false, "YAGNI mode: accept compile-only for trivial one-liners")
	cmd.Flags().StringVar(&scorerBinary, "scorer-binary", "", "Explicit compiler/interpreter for --scorer compile-and-run")
	cmd.Flags().BoolVar(&useModel, "use-model", false, "Run each dataset case through the configured LLM instead of the offline stub (issue #261). Requires llm.api_key in config or LLM_API_KEY env.")

	cmd.MarkFlagRequired("dataset")
	return cmd
}

// ── eval list ───────────────────────────────────────────────────────

func newEvalListCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available datasets in a directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := dataset.ListDatasets(dir)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				fmt.Printf("no datasets found under %s\n", dir)
				return nil
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{"dir": dir, "datasets": files})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "evals", "Directory to scan for *.json datasets")
	return cmd
}

// ── helpers ─────────────────────────────────────────────────────────

// stubRunOverride is the offline / CI Loop.RunOverride. It echoes a
// canonical "stub" marker as the agent's final output so structural
// rules in the dataset (contains / avoids / max_turns) have something
// to check without a real LLM. The contents are predictable so unit
// tests can assert exact bytes. Verified=true because the stub is
// treated as a successful loop by the runner; dataset-level rules
// (keywords, max_turns) still cause pass/fail flips.
func stubRunOverride(_ context.Context, sess *session.Session, prompt string) (*agentloop.Result, error) {
	out := "stub echo: " + prompt
	return &agentloop.Result{
		SessionID: sess.ID,
		Summary:   out,
		Verified:  true,
		Turns:     1,
	}, nil
}

// newLLMClientFor returns a *llm.Client. Centralised so the cmd
// package keeps a single import path for the LLM bridge.
func newLLMClientFor(endpoint, apiKey string) *llm.Client { //nolint:unused // reserved for follow-up
	return llm.NewClient(endpoint, apiKey)
}

// mustParseExporter returns ExporterKind, falling back to noop for
// unknown values with a stderr warning. We don't return the error to
// keep the RunE flow linear; the unknown kind is not fatal.
func mustParseExporter(s string) sinctrace.ExporterKind {
	kind, err := sinctrace.ParseExporter(s)
	if err != nil {
		var ue *sinctrace.UnknownExporterError
		if errors.As(err, &ue) {
			fmt.Fprintf(os.Stderr, "warn: %s — using noop\n", ue.Error())
		}
		return sinctrace.ExporterNoop
	}
	return kind
}

// buildEvalCompletion constructs a chat-completion func for the agent
// loop when --use-model is in effect (issue #261). The client honours
// llm.base_url / llm.api_key / llm.model from the merged config, with
// env vars LLM_API_KEY / LLM_MODEL overriding transparently.
func buildEvalCompletion() (func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error), error) {
	cfg, err := internal.LoadMergedConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.LLMAPIKey)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("LLM_API_KEY env or llm.api_key config required")
	}
	baseURL := strings.TrimSpace(cfg.LLMBaseURL)
	if baseURL == "" {
		baseURL = "https://integrate.api.nvidia.com/v1"
	}
	model := strings.TrimSpace(os.Getenv("LLM_MODEL"))
	if model == "" {
		model = strings.TrimSpace(cfg.LLMModel)
	}
	if model == "" {
		return nil, fmt.Errorf("LLM_MODEL env or llm.model config required")
	}
	client := llm.NewClient(baseURL, apiKey)
	maxTokens := cfg.LLMMaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	temperature := cfg.LLMTemperature
	if temperature == 0 {
		temperature = 0.2
	}
	return agentloop.NewProviderCompletion(client, model, maxTokens, temperature), nil
}

// applyJudge runs the LLM-as-a-Judge over results and writes the
// score/feedback back into each RunResult. Returns an error when the
// judge can't even be constructed (no API key, no model). The CLI
// caller turns that into a stderr warning but keeps the JSON report.
func applyJudge(ctx context.Context, results []dataset.RunResult, model, endpoint, keyEnv string) error {
	apiKey := os.Getenv(keyEnv)
	if apiKey == "" {
		return fmt.Errorf("judge: env %s is empty", keyEnv)
	}
	client := newLLMClientFor(endpoint, apiKey)
	judge, err := eval.NewJudge(eval.JudgeConfig{Model: model}, client)
	if err != nil {
		return fmt.Errorf("judge: build: %w", err)
	}
	for i := range results {
		if !results[i].Success {
			continue
		}
		traj := eval.Trajectory{
			Prompt:       results[i].TestCaseID,
			Turns:        results[i].Turns,
			ToolsUsed:    results[i].ToolsUsed,
			VerifyPassed: results[i].VerifyPassed,
			FinalOutput:  results[i].FinalOutput,
			SessionID:    results[i].SessionID,
			Duration:     results[i].Duration.String(),
		}
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		r, err := judge.Evaluate(cctx, traj)
		cancel()
		if err != nil {
			// Per-result errors don't kill the batch.
			results[i].JudgeFeedback = "judge error: " + err.Error()
			continue
		}
		results[i].JudgeScore = r.Score
		results[i].JudgeFeedback = r.Feedback
		if !r.Pass {
			results[i].Success = false
			if results[i].Error == "" {
				results[i].Error = "judge failed"
			}
		}
	}
	return nil
}

// workspaceRoot derives a reasonable workspace for the agent loop
// when a dataset lives under evals/<file>. Picks the parent-of-parent
// so the agent can read evals/ without chasing its own tails.
func workspaceRoot(datasetPath string) string {
	abs, err := filepath.Abs(datasetPath)
	if err != nil {
		return "."
	}
	dir := filepath.Dir(abs)
	if filepath.Base(dir) == "evals" {
		return filepath.Dir(dir)
	}
	return dir
}

// ── eval comparator (issue #171) ────────────────────────────────────
//
// The comparator bypasses the agent loop entirely: each arm is a
// system-prompt rendering wired straight into a stub Subject, so
// the run stays offline and deterministic. This matches the
// caveman evals/README.md guarantee that "reading the snapshot
// requires no LLM, no API key, runs in CI".

// parseArms turns the --arm flag value into a []evalharness.Arm.
// Reserved tokens ("baseline", "terse", "lazy_skill") map to
// pinned arms; everything else is treated as a bundled skill name.
// userSkill is the stand-in for the "__user_skill__" arm reserved
// token.
func parseArms(value, userSkill string) ([]evalharness.Arm, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("arms flag is empty")
	}
	tokens := strings.Split(value, ",")
	out := make([]evalharness.Arm, 0, len(tokens))
	seen := map[string]bool{}
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if seen[tok] {
			continue
		}
		seen[tok] = true
		switch tok {
		case "baseline", "__baseline__":
			out = append(out, evalharness.NoSystemPromptArm())
		case "terse", "__terse__":
			out = append(out, evalharness.StandardTerseArm())
		case "lazy_skill", "__lazy_skill__":
			out = append(out, evalharness.LazySkillArm(func() (string, error) { return evalharness.ReadBundledSkillBody(evalharness.LazySkillName) }))
		case "user_skill", "__user_skill__":
			out = append(out, evalharness.SkillArm(userSkill, func() (string, error) {
				if userSkill == "" {
					return "", errors.New("__user_skill__ arm: --skill is empty")
				}
				return evalharness.ReadBundledSkillBody(userSkill)
			}))
		default:
			out = append(out, evalharness.SkillArm(tok, func() (string, error) {
				return evalharness.ReadBundledSkillBody(tok)
			}))
		}
	}
	if len(out) == 0 {
		return nil, errors.New("arms flag produced no arms")
	}
	return out, nil
}

// runArmComparator executes the four-arm comparator path. It does
// not produce the Golden-Dataset JSON envelope; instead it emits
// the matrix-shaped table whose columns mirror ponytail's
// benchmarks/README.md:34-58.
func runArmComparator(ctx context.Context, datasetPath, armsFlag, userSkill, modelPricing string, timeout time.Duration, jsonOutput bool) error {
	arms, err := parseArms(armsFlag, userSkill)
	if err != nil {
		return fmt.Errorf("eval run: --arm: %w", err)
	}
	for i := range arms {
		if arms[i].PricingName == "" {
			arms[i].PricingName = modelPricing
		}
	}
	evalSet, err := loadEvalSetFromGoldenDataset(datasetPath)
	if err != nil {
		return fmt.Errorf("eval run: load evalset: %w", err)
	}
	opts := evalharness.CompareOptions{}
	if timeout > 0 {
		opts.PerCaseTimeout = timeout
	}
	report, err := evalharness.Compare(ctx, evalSet, arms, opts)
	if err != nil {
		return fmt.Errorf("eval run: compare: %w", err)
	}
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	return printCompareMatrix(os.Stdout, report)
}

// loadEvalSetFromGoldenDataset reuses the dataset JSON parser so
// the comparator harness accepts the SAME files as `eval run`.
// We translate TestCase to EvalCase 1:1.
func loadEvalSetFromGoldenDataset(path string) (evalharness.EvalSet, error) {
	ds, err := dataset.LoadDataset(path)
	if err != nil {
		return evalharness.EvalSet{}, err
	}
	if ds == nil {
		return evalharness.EvalSet{}, errors.New("nil dataset")
	}
	out := evalharness.EvalSet{Name: ds.Name + " (via dataset)", Description: ds.Description}
	out.Cases = make([]evalharness.EvalCase, 0, len(ds.TestCases))
	for _, tc := range ds.TestCases {
		meta := map[string]string{}
		for k, v := range tc.Metadata {
			meta[k] = v
		}
		// Surface the canonical "expected.keywords" as the EvalCase
		// Expected string so ContainsAll scorer works transparently.
		expected := strings.Join(tc.Expected.OutputContains, "\n")
		if expected == "" {
			expected = strings.Join(tc.Expected.ContainsKeywords, "\n")
		}
		ec := evalharness.EvalCase{
			ID:       tc.ID,
			Prompt:   tc.Prompt,
			Expected: expected,
			Tags:     tc.Tags,
			Meta:     meta,
		}
		out.Cases = append(out.Cases, ec)
	}
	return out, nil
}

// printCompareMatrix renders the report as a ponytail-shaped table:
//
//	| arm        | pass_rate | med_LOC | med_MS | med_USD | med_tokens | med_score |
//	|------------|-----------|---------|--------|---------|------------|-----------|
//	| __baseline__|  1.00     |    0    |    0   | 0.000.. |        288 |     1.00  |
//
// Output goes to w (typically os.Stdout).
func printCompareMatrix(w io.Writer, rep evalharness.CompareReport) error {
	if w == nil {
		return errors.New("matrix writer is nil")
	}
	fmt.Fprintln(w, "| arm            | pass_rate | med_LOC | med_latency_ms | med_usd     | med_tokens | med_score |")
	fmt.Fprintln(w, "|----------------|-----------|---------|----------------|-------------|------------|-----------|")
	// Output rows in declaration order of arms (stable, byte-stable).
	for _, arm := range rep.Arms {
		tot, ok := rep.TotalsByArm[arm.ID]
		if !ok {
			continue
		}
		fmt.Fprintf(w, "| %-14s | %9.2f | %7d | %14d | %11.6f | %10d | %9.2f |\n",
			arm.ID,
			tot.PassRate(),
			medianIntLocal(tot.LOC),
			medianIntLocal(tot.LatencyMS),
			medianFloatLocal(tot.USD),
			medianIntLocal(tot.Tokens),
			medianFloatLocal(tot.Scores),
		)
	}
	fmt.Fprintf(w, "\n(honest delta = user-skill row - terse row)\n")
	if len(rep.Warnings) > 0 {
		for _, msg := range rep.Warnings {
			fmt.Fprintf(w, "warn: %s\n", msg)
		}
	}
	return nil
}

// medianIntLocal / medianFloatLocal are tiny local copies of
// medianInt/medianFloat — they live in snapshot.go but are
// unexported; we replicate them here so the comparator CLI
// doesn't need to widen the package API.
func medianIntLocal(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	c := append([]int(nil), xs...)
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j-1] > c[j]; j-- {
			c[j-1], c[j] = c[j], c[j-1]
		}
	}
	return c[len(c)/2]
}

func medianFloatLocal(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j-1] > c[j]; j-- {
			c[j-1], c[j] = c[j], c[j-1]
		}
	}
	return c[len(c)/2]
}

// ── eval compare / snapshot / diff ──────────────────────────────────

// newEvalCompareCmd is the shortcut for "run all four arms and
// print the matrix". Same wiring as --arm baseline,terse,lazy_skill,<user>.
func newEvalCompareCmd() *cobra.Command {
	var (
		userSkill    string
		modelPricing string
		timeout      time.Duration
	)
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Run all four arms (baseline/terse/lazy_skill/<user>) on a dataset",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			dsPath, _ := cmd.Flags().GetString("dataset")
			if dsPath == "" {
				return errors.New("eval compare: --dataset is required")
			}
			armsFlag := "baseline,terse,lazy_skill," + userSkill
			return runArmComparator(ctx, dsPath, armsFlag, userSkill, modelPricing, timeout, false)
		},
	}
	cmd.Flags().String("dataset", "", "Path to Golden Dataset JSON file (required)")
	cmd.Flags().StringVar(&userSkill, "skill", "skill-code-create", "User-skill arm name")
	cmd.Flags().StringVar(&modelPricing, "model-pricing", "stub", "Price-book entry (issue #171)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Per-case timeout")
	_ = cmd.MarkFlagRequired("dataset")
	return cmd
}

// newEvalSnapshotCmd writes a snapshot (one row per arm) to disk
// so CI can diff the resulting JSON against the committed baseline.
func newEvalSnapshotCmd() *cobra.Command {
	var (
		userSkill    string
		modelPricing string
		outPath      string
		timeout      time.Duration
	)
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Write a one-row-per-arm snapshot file (issue #171)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			dsPath, _ := cmd.Flags().GetString("dataset")
			if dsPath == "" {
				return errors.New("eval snapshot: --dataset is required")
			}
			if outPath == "" {
				return errors.New("eval snapshot: --out is required")
			}
			arms, err := parseArms("baseline,terse,lazy_skill,"+userSkill, userSkill)
			if err != nil {
				return fmt.Errorf("eval snapshot: %w", err)
			}
			for i := range arms {
				if arms[i].PricingName == "" {
					arms[i].PricingName = modelPricing
				}
			}
			es, err := loadEvalSetFromGoldenDataset(dsPath)
			if err != nil {
				return fmt.Errorf("eval snapshot: %w", err)
			}
			opts := evalharness.CompareOptions{PerCaseTimeout: timeout}
			rep, err := evalharness.Compare(ctx, es, arms, opts)
			if err != nil {
				return fmt.Errorf("eval snapshot: compare: %w", err)
			}
			hdr := evalharness.SnapshotHeader{
				SetName:       filepath.Base(dsPath),
				SinCodeVer:    "v3.18.0",
				SchemaVersion: evalharness.SnapshotSchemaVersion,
			}
			if err := evalharness.WriteSnapshotFile(outPath, rep, hdr); err != nil {
				return fmt.Errorf("eval snapshot: write: %w", err)
			}
			fmt.Fprintf(os.Stderr, "snapshot written: %s (arms=%d cases=%d)\n", outPath, len(rep.Arms), len(rep.PerCase))
			return nil
		},
	}
	cmd.Flags().String("dataset", "", "Path to Golden Dataset JSON file (required)")
	cmd.Flags().StringVar(&outPath, "out", "", "Output snapshot file path (required)")
	cmd.Flags().StringVar(&userSkill, "skill", "skill-code-create", "User-skill arm name")
	cmd.Flags().StringVar(&modelPricing, "model-pricing", "stub", "Price-book entry (issue #171)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Per-case timeout")
	_ = cmd.MarkFlagRequired("dataset")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

// newEvalDiffCmd produces a row-by-row delta between two snapshot
// files. Used in CI to deep-diff PRs against the committed
// baseline snapshot (caveman evals/README.md §3).
func newEvalDiffCmd() *cobra.Command {
	var snapA, snapB string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff two snapshot files (issue #171)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if snapA == "" || snapB == "" {
				return errors.New("eval diff: --snapshot and --snapshot-b are both required")
			}
			A, err := evalharness.LoadSnapshotFile(snapA)
			if err != nil {
				return fmt.Errorf("eval diff: load %s: %w", snapA, err)
			}
			B, err := evalharness.LoadSnapshotFile(snapB)
			if err != nil {
				return fmt.Errorf("eval diff: load %s: %w", snapB, err)
			}
			deltas, err := evalharness.DiffSnapshots(A, B)
			if err != nil {
				return fmt.Errorf("eval diff: %w", err)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{
				"snapshot_a": A.Header.SetName,
				"snapshot_b": B.Header.SetName,
				"deltas":     deltas,
			})
		},
	}
	cmd.Flags().StringVar(&snapA, "snapshot", "", "Path to snapshot A (must compare against)")
	cmd.Flags().StringVar(&snapB, "snapshot-b", "", "Path to snapshot B (the candidate)")
	_ = cmd.MarkFlagRequired("snapshot")
	_ = cmd.MarkFlagRequired("snapshot-b")
	return cmd
}

// buildScorer constructs an evalharness.Scorer from the CLI flags.
// Supported types: compile_and_run, exact, contains.
func buildScorer(typ, lang, selfCheck string, skipTest bool, binary string) (evalharness.Scorer, error) {
	switch typ {
	case "compile_and_run":
		if lang == "" {
			return nil, errors.New("--language is required for compile_and_run scorer")
		}
		if !evalharness.IsCompileAndRunLanguage(lang) {
			return nil, fmt.Errorf("unsupported language %q", lang)
		}
		return evalharness.CompileAndRun{
			Language:  lang,
			SelfCheck: selfCheck,
			SkipTest:  skipTest,
			Binary:    binary,
		}, nil
	case "exact":
		return evalharness.ExactMatch{}, nil
	case "contains":
		return evalharness.ContainsAll{}, nil
	default:
		return nil, fmt.Errorf("unsupported scorer %q", typ)
	}
}
