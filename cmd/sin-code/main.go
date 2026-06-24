// SPDX-License-Identifier: MIT
// Purpose: sin-code — unified Go binary for all SIN-Code analysis/manipulation tools.
// Replaces 13 separate binaries (discover, execute, map, grasp, scout, harvest,
// orchestrate, ibd, poc, sckg, adw, oracle, efm) with a single cobra-based CLI.
// Docs: cmd/sin-code/main.go.doc.md
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
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sandbox"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
	swebench "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/swebench"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/todo"
	sinctrace "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/trace"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

var Version = internal.Version // Re-export from internal/version.go; set at build time via -ldflags

var rootCmd = &cobra.Command{
	Use:   "sin-code",
	Short: "SIN-Code unified analysis & manipulation toolchain",
	Long: `sin-code is the unified Go binary for the SIN-Code tool suite.
It consolidates 44+ subcommands into a single cobra-based CLI:

  Core analysis:    discover, execute, map, grasp, scout, harvest, orchestrate
  Advanced tools:   ibd, poc, sckg, adw, oracle, efm
  Utility commands: security, sbom, config, self-update, tui, serve, update,
                      tool-search
  Agent ecosystem:  chat, sessions, mcp, goal, daemon, skill, superpowers,
                    vane, stack, gh, hub, ledger, summary, install, compress,
                    cover
  Other:            completion, read, write, edit, lsp, plugin, index,
                    orchestrator-run, orchestrator-agents, orchestrator-plan,
                    todo, notifications, memory, assets, evalset, hooks,
                    instinct, prp, skills, catalog, compile-spec, triage

Each subcommand is also a thin pass-through to the standalone tool repos
for backwards compatibility — the standalone binaries are still maintained
but "sin-code" is now the primary distribution channel.`,
	Version: Version,
}

func init() {
	rootCmd.AddCommand(internal.DiscoverCmd)
	rootCmd.AddCommand(internal.ExecuteCmd)
	rootCmd.AddCommand(internal.MapCmd)
	rootCmd.AddCommand(internal.GraspCmd)
	rootCmd.AddCommand(internal.ScoutCmd)
	rootCmd.AddCommand(internal.HarvestCmd)
	rootCmd.AddCommand(internal.OrchestrateCmd)
	rootCmd.AddCommand(internal.IbdCmd)
	rootCmd.AddCommand(internal.PocCmd)
	rootCmd.AddCommand(internal.SckgCmd)
	rootCmd.AddCommand(internal.AdwCmd)
	rootCmd.AddCommand(internal.OracleCmd)
	rootCmd.AddCommand(internal.EfmCmd)
	rootCmd.AddCommand(internal.ServeCmd)
	rootCmd.AddCommand(internal.SecurityCmd)
	rootCmd.AddCommand(internal.SbomCmd)
	rootCmd.AddCommand(internal.ConfigCmd)
	rootCmd.AddCommand(internal.SelfUpdateCmd)
	rootCmd.AddCommand(internal.UpdateCmd)
	rootCmd.AddCommand(todo.TodoCmd)
	rootCmd.AddCommand(notifications.NotificationsCmd)
	rootCmd.AddCommand(MemoryCmd)
	rootCmd.AddCommand(internal.RulesCmd)
	rootCmd.AddCommand(internal.ReadCmd)
	rootCmd.AddCommand(internal.WriteCmd)
	rootCmd.AddCommand(internal.EditCmd)
	rootCmd.AddCommand(internal.LSPCmd)
	rootCmd.AddCommand(internal.PluginCmd)
	rootCmd.AddCommand(internal.IndexCmd)
	rootCmd.AddCommand(internal.OrchestratorRunCmd)
	rootCmd.AddCommand(internal.OrchestratorAgentsCmd)
	rootCmd.AddCommand(internal.OrchestratorPlanCmd)
	rootCmd.AddCommand(tuiCmd)
	rootCmd.AddCommand(webuiCmd)
	rootCmd.AddCommand(NewChatCmd(), NewSessionsCmd(), NewMCPCmd(), NewToolSearchCmd(),
		NewGoalCmd(), NewDaemonCmd(), NewSkillCmd(), NewSwarmCmd(), NewSuperpowersCmd(), NewDoxCmd(),
		NewVaneCmd(), NewStackCmd(), NewGhCmd(), NewHubCmd(),
		NewLedgerCmd(), NewSummaryCmd(), NewAutodevCmd(), // v3.4.0 + v3.5.0 + v3.6.0 + v3.7.0 + v3.8.0 + v3.9.0 + v3.12.0 + v3.13.0 + autodev-bridge
		NewCompressCmd(),            // v3.18.0 — deterministic + LLM compaction (issue #172)
		NewReviewCmd(),              // v3.19.0 — review --complexity (issue #179)
		NewSkillsCmd(),              // bundled project-local agent skills
		NewEvalCmd(), NewTraceCmd(), // v3.18.0: Eval & Observability System (issue #75)
		NewProfileCmd(),                    // v3.18.0 — single-source-of-truth per-agent profile renderer (issue #175)
		NewRtkCmd(),                        // rtk (Rust Token Killer) bridge (issue #123)
		NewCodeGraphCmd(),                  // CodeGraph multi-language analysis bridge (issue #126)
		NewSpecCmd(),                       // Spec-Layer: *.spec.md contracts (issue #122)
		NewInstallCmd(),                    // v3.18.0 — single-binary installer entrypoint (issue #170)
		NewTriageCmd(),                     // v3.18.0 — backlog auto-prioritizer via gh (issue #162)
		NewCatalogCmd(),                    // v3.18.0 — unified tool catalog (issue #163, supersedes hub + assets)
		NewCompileSpecCmd(),                // v3.21.0 — declarative .sin-code.yml compiler (issue #164)
		NewGrillCmd(),                      // v3.18.0 — native adversarial design-review (issue #141 fusion)
		NewSubagentCmd(),                   // v3.18.0 — isolated-context sub-agent (issue #192, wraps #153)
		NewAutoPRCmd(),                     // v3.18.0 — self-healing pipeline (issue #158)
		NewCheckpointCmd(), NewRewindCmd(), // v3.20.0 — workspace checkpointing + rewind (issue #194)
		NewDebtCmd(),                    // v3.18.0 — sin-debt marker manager (issue #177)
		NewAuditCmd(), NewCEOAUDITCmd(), // v3.18.0 — complexity audit (issue #180) + 48-gate CEO audit
		NewCoverCmd(),                                                                                     // Coverage-Drohne: scan, check, gaps, generate, hook
		internal.InstinctCmd, internal.HooksCmd, internal.AssetsCmd, internal.EvalSetCmd, internal.PRPCmd, // continuous learning + lifecycle hooks + asset harvest + evalset + prp workflow
		NewImageGraphCmd(),   // image-graph: deterministic chart generation (bar/line/pie/area)
		NewStatusCmd(),       // v3.22.0
		NewFusionCmd(),       // v3.22.0 — fusion benchmark/rank/recommend (issue #395) — readiness/status snapshot (issue #326)
		NewResearchCmd(),     // v3.23.0 — autonomous research-report generation (issue #384)
		NewPermissionCmd(),   // v3.23.0 — reactive permission engine inspection (issue #374)
		NewTokensCmd(),       // v3.23.0 — token usage inspection
		NewAnalyseCmd(),      // v3.23.0 — static analysis runner
		NewAnalyseImageCmd(), // v3.24.0 — vision-based image analysis (issue #423)
		NewAutoCmd(),         // v3.23.0 — ultra-autonomous mode
	)

	// Pass build-time version to self-update module.
	internal.SetCurrentVersion(internal.Version)

	// Root --version uses the same template as per-subcommand --version.
	internal.RegisterVersionCmd(rootCmd)
}

// checkUpdateFn is the network probe used by checkUpdate. It is a package
// variable so tests can stub it out and stay fully hermetic (no GitHub calls).
var checkUpdateFn = internal.CheckUpdateAvailable

// updateCheckDisabled reports whether the background update check is
// disabled via environment:
//   - SIN_CODE_NO_UPDATE_CHECK / NO_UPDATE_CHECK: explicit user opt-out
//   - SIN_CODE_OFFLINE: generic offline switch
func updateCheckDisabled() bool {
	for _, key := range []string{"SIN_CODE_NO_UPDATE_CHECK", "NO_UPDATE_CHECK", "SIN_CODE_OFFLINE"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

func checkUpdate() {
	// Only run when invoked with no args or --version/-v.
	if len(os.Args) > 1 {
		first := os.Args[1]
		if first != "--version" && first != "-v" {
			return
		}
	}

	if updateCheckDisabled() {
		return
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	stampDir := filepath.Join(configDir, "sin")
	stampPath := filepath.Join(stampDir, ".last-update-check")

	if info, err := os.Stat(stampPath); err == nil {
		if time.Since(info.ModTime()) < 24*time.Hour {
			return
		}
	}

	// Touch the stamp file immediately so repeated invocations don't hammer GitHub.
	os.MkdirAll(stampDir, 0755)
	os.WriteFile(stampPath, []byte(time.Now().Format(time.RFC3339)), 0644)

	// Query GitHub with a short timeout so the CLI stays responsive.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	type result struct {
		version string
		has     bool
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		v, h, e := checkUpdateFn()
		ch <- result{v, h, e}
	}()

	select {
	case <-ctx.Done():
		return
	case res := <-ch:
		if res.err != nil || !res.has {
			return
		}
		fmt.Printf("\n🔄 A new version of sin-code is available: %s → %s\n", Version, res.version)
		fmt.Println("   Run 'sin-code self-update' to install.")
	}
}

func main() {
	// Sandbox shim: if invoked as the re-exec target (second arg =
	// "__sandbox_exec"), apply Landlock and exec the real command. The
	// parent process stays unconfined; only the child runs sandboxed.
	if len(os.Args) > 2 && os.Args[1] == "__sandbox_exec" {
		if err := sandbox.ApplyAndExec(); err != nil {
			fmt.Fprintf(os.Stderr, "sin-code sandbox: %v\n", err)
			os.Exit(126)
		}
		return // unreachable after successful exec
	}

	// If invoked via a symlink named after a subcommand (e.g. `discover` ->
	// `sin-code discover`), automatically route to that subcommand.
	if len(os.Args) > 0 {
		name := filepath.Base(os.Args[0])
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == name {
				args := append([]string{name}, os.Args[1:]...)
				rootCmd.SetArgs(args)
				break
			}
		}
	}

	checkUpdate()

	if err := rootCmd.Execute(); err != nil {
		internal.PrintError(err)
	}
}

// ── eval command tree (merged from eval_cmd.go) ─────────────────────

// NewEvalCmd returns the `sin-code eval` cobra command tree.
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
		newEvalSWEBenchCmd(),
		newEvalSwebenchCmd(),
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

// ── eval swe-bench (issue #363) ──────────────────────────────────────

func newEvalSWEBenchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "swe-bench",
		Short: "SWE-bench dataset conversion and scoring (issue #363)",
		Long: `sin-code eval swe-bench converts SWE-bench instances to the
Golden Dataset format and scores verification output against expected
test results.

    sin-code eval swe-bench convert --input swe-bench.json [--output eval.json] [--limit N]
    sin-code eval swe-bench score   --input swe-bench.json --verify-output out.txt [--json]`,
	}
	cmd.AddCommand(newEvalSWEBenchConvertCmd(), newEvalSWEBenchScoreCmd())
	return cmd
}

func newEvalSWEBenchConvertCmd() *cobra.Command {
	var (
		inputPath  string
		outputPath string
		limit      int
	)
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert a SWE-bench JSON dataset to Golden Dataset format",
		RunE: func(cmd *cobra.Command, args []string) error {
			ds, err := swebench.LoadDataset(inputPath)
			if err != nil {
				return fmt.Errorf("swe-bench convert: load: %w", err)
			}
			if limit > 0 && limit < len(ds.Instances) {
				ds.Instances = ds.Instances[:limit]
			}
			cases := swebench.ConvertDataset(ds)
			if outputPath != "" {
				if err := swebench.WriteEvalDataset(cases, outputPath); err != nil {
					return fmt.Errorf("swe-bench convert: write: %w", err)
				}
				fmt.Fprintf(os.Stderr, "converted %d instances → %s\n", len(cases), outputPath)
				return nil
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{"name": "swe-bench", "version": "1.0", "test_cases": cases})
		},
	}
	cmd.Flags().StringVar(&inputPath, "input", "", "Path to SWE-bench JSON file (required)")
	cmd.Flags().StringVar(&outputPath, "output", "", "Output Golden Dataset JSON path (default: stdout)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit to first N instances (0 = all)")
	_ = cmd.MarkFlagRequired("input")
	return cmd
}

func newEvalSWEBenchScoreCmd() *cobra.Command {
	var (
		inputPath    string
		verifyOutput string
		jsonOut      bool
	)
	cmd := &cobra.Command{
		Use:   "score",
		Short: "Score SWE-bench verification output against expected tests",
		RunE: func(cmd *cobra.Command, args []string) error {
			ds, err := swebench.LoadDataset(inputPath)
			if err != nil {
				return fmt.Errorf("swe-bench score: load: %w", err)
			}
			verifyData, err := os.ReadFile(verifyOutput)
			if err != nil {
				return fmt.Errorf("swe-bench score: read verify output: %w", err)
			}
			verifyStr := string(verifyData)
			results := make([]swebench.ScorerResult, 0, len(ds.Instances))
			for i := range ds.Instances {
				results = append(results, swebench.ScoreInstance(&ds.Instances[i], verifyStr))
			}
			summary := swebench.SummarizeResults(results)
			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(summary)
			}
			fmt.Printf("SWE-bench Score: %d/%d resolved (%.1f%%), mean score %.3f\n",
				summary.Resolved, summary.Total, summary.ResolveRate*100, summary.MeanScore)
			return nil
		},
	}
	cmd.Flags().StringVar(&inputPath, "input", "", "Path to SWE-bench JSON file (required)")
	cmd.Flags().StringVar(&verifyOutput, "verify-output", "", "Path to verification output file (required)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output summary as JSON")
	_ = cmd.MarkFlagRequired("input")
	_ = cmd.MarkFlagRequired("verify-output")
	return cmd
}

// ── eval swebench (issue #363) ──────────────────────────────────────

func newEvalSwebenchCmd() *cobra.Command {
	var (
		datasetPath string
		outputPath  string
		workspace   string
		maxTurns    int
		timeout     time.Duration
		sinCodeBin  string
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "swebench",
		Short: "Run SWE-bench evaluation harness (issue #363)",
		Long: `Run sin-code against a SWE-bench dataset and evaluate the results.

SWE-bench measures an agent's ability to fix real GitHub issues. This harness:
  - Loads SWE-bench JSON instances
  - Runs sin-code against each issue
  - Applies the predicted patch
  - Evaluates with test_patch
  - Records pass/fail

Examples:

  sin-code eval swebench --dataset swebench.json --output results.json
  sin-code eval swebench --dataset swebench.json --dry-run
  sin-code eval swebench --dataset swebench.json --workspace /tmp/swe --max-turns 200`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := eval.SweConfig{
				DatasetPath: datasetPath,
				OutputPath:  outputPath,
				Workspace:   workspace,
				MaxTurns:    maxTurns,
				Timeout:     timeout,
				SinCodeBin:  sinCodeBin,
				DryRun:      dryRun,
			}
			report, err := eval.RunSweBench(cmd.Context(), cfg)
			if err != nil {
				return fmt.Errorf("swebench: %w", err)
			}
			eval.SwePrintSummary(os.Stdout, report)
			return nil
		},
	}

	cmd.Flags().StringVarP(&datasetPath, "dataset", "d", "", "Path to SWE-bench JSON dataset (required)")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "swebench-results.json", "Output results JSON path")
	cmd.Flags().StringVarP(&workspace, "workspace", "w", "", "Workspace directory for repo clones")
	cmd.Flags().IntVar(&maxTurns, "max-turns", 100, "Max agent turns per instance")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Timeout per instance")
	cmd.Flags().StringVar(&sinCodeBin, "sin-code-bin", "", "Path to sin-code binary (default: auto-detect)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate dataset without running agents")

	_ = cmd.MarkFlagRequired("dataset")
	return cmd
}
