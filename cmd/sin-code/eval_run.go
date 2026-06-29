// SPDX-License-Identifier: MIT
// Purpose: `sin-code eval run` command — Golden Dataset runner with
// tracing, LLM-as-a-Judge, scorers, and four-arm comparator routing.
// Extracted from eval_cmd.go for single-responsibility file layout.
// sin-debt: shrink, upgrade: consolidate when eval is refactored
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
	sinctrace "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/trace"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

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
				UseModel:       useModel,
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
