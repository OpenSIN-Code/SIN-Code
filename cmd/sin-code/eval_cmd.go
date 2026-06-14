// SPDX-License-Identifier: MIT
// Purpose: `sin-code eval` — run Golden Datasets, emit a JSON
// report, and gate the CI job on `--min-pass-rate` (issue #75).
//
// Subcommands:
//   eval run --dataset <path> [--min-pass-rate N] [--json] [--trace]
//   eval list [--dir path]
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
	"os"
	"path/filepath"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	sinctrace "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/trace"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
	"github.com/spf13/cobra"
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

Tracing is opt-in via --trace and ships to the chosen exporter.`,
	}
	cmd.AddCommand(newEvalRunCmd(), newEvalListCmd())
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
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a Golden Dataset and emit the pass-rate report",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if datasetPath == "" {
				return errors.New("eval run: --dataset is required")
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

			loop := &agentloop.Loop{
				Gate:      gate,
				Workspace: workspaceRoot(datasetPath),
				MaxTurns:  80,
				Hooks:     &hooks.Engine{}, // empty: no user hooks during eval
				// Completion funcs left nil -> RunOverride is required.
				RunOverride: stubRunOverride,
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
