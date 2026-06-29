// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when loopbuilder is refactored
// Purpose: SIN Fusion v1 verify-tournament wiring (issue #290) extracted
// from builder.go to keep each file ≤500 lines.
package loopbuilder

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/fusion"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// WireFusion wires a SIN Fusion v1 verify-tournament (issue #290) into
// an existing agentloop.Loop. It is extracted from Build so callers that
// construct their loop manually (e.g. chat_cmd.go) can opt into fusion
// without duplicating the wiring logic.
//
// Only active when cfg.FusionEnabled is true and the gate is in PoC or Oracle
// mode (issue #344). Oracle mode requires explicit FusionOracleMode=true and
// wires a judge that evaluates all candidates together, not first-pass-wins.
// Requires >=2 providers from the Fireworks pool; otherwise the call is a
// no-op and the loop keeps legacy behavior.
func WireFusion(loop *agentloop.Loop, cfg Config, gate *verify.Gate, client *llm.Client,
	memStore *lessons.Store, ledgerStore *ledger.Store, hookEngine *hooks.Engine) {
	if !cfg.FusionEnabled || (gate.Mode() != verify.ModePoC && gate.Mode() != verify.ModeOracle) {
		return
	}
	providers := fusion.LoadFireworksPool(nil, cfg.FusionProviders)
	if len(providers) < 2 {
		return
	}
	forkFunc := fusion.ForkFunc(nil)
	runFunc := fusion.RunFunc(nil)
	if cfg.SessionStore != nil {
		forkFunc = func(srcSessionID string, turn int) (*session.Session, error) {
			return cfg.SessionStore.Fork(srcSessionID, turn)
		}
		runFunc = func(ctx context.Context, prov fusion.ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
			provClient := llm.NewClient(prov.BaseURL, prov.APIKey)
			provCompletion := agentloop.NewProviderCompletion(provClient, prov.Model, prov.MaxTokens, 0)
			provLoop := &agentloop.Loop{
				Gate:         gate,
				LocalTool:    loop.LocalTool,
				LocalSpec:    loop.LocalSpec,
				Workspace:    loop.Workspace,
				MaxTurns:     loop.MaxTurns,
				SessionID:    sess.ID,
				SystemPrompt: loop.SystemPrompt,
				Completion:   provCompletion,
				Hooks:        hookEngine,
				Perm:         loop.Perm,
				Lessons:      memStore,
				Ledger:       ledgerStore,
				ResultPolicy: loop.ResultPolicy,
			}
			return provLoop.Run(ctx, sess, prompt)
		}
	}
	mode := fusion.ModeOracle // issue #394: Oracle is default
	if cfg.FusionMode != "" {
		mode = cfg.FusionMode
	}
	maxCost := cfg.FusionMaxCostUSD
	if cfg.FusionOracleMode && maxCost > 2.0 {
		// Oracle mode defaults to a tighter cap unless explicitly higher.
		maxCost = 2.0
	}
	tournament := &fusion.Tournament{
		Providers:          providers,
		MaxCostUSD:         maxCost,
		MinQuorum:          cfg.FusionMinQuorum,
		PerProviderTimeout: time.Duration(cfg.FusionPerProviderTimeoutS) * time.Second,
		Workspace:          cfg.Workspace,
		SourceSessionID:    cfg.SessionID,
		Lessons:            memStore,
		Ledger:             ledgerStore,
		Hooks:              hookEngine,
		HookSessionID:      cfg.SessionID,
		VerifyFn:           func(ctx context.Context, ws string) verify.Result { return gate.Run(ctx, ws) },
		ForkFunc:           forkFunc,
		RunFunc:            runFunc,
		Mode:               mode,
	}
	if mode == fusion.ModeOracle {
		judgeModel := firstNonEmpty(os.Getenv("SIN_EVALUATOR_MODEL"), cfg.Model)
		judgeClient := client
		if evalBase := os.Getenv("SIN_EVALUATOR_BASE_URL"); evalBase != "" {
			judgeClient = llm.NewClient(evalBase, os.Getenv("SIN_EVALUATOR_API_KEY"))
		}
		judge := fusion.NewLLMOracleJudge(judgeClient, judgeModel)
		tournament.OracleJudge = judge.Judge
	}
	if mode == fusion.ModePlanMerge {
		judgeModel := firstNonEmpty(os.Getenv("SIN_EVALUATOR_MODEL"), cfg.Model)
		judgeClient := client
		if evalBase := os.Getenv("SIN_EVALUATOR_BASE_URL"); evalBase != "" {
			judgeClient = llm.NewClient(evalBase, os.Getenv("SIN_EVALUATOR_API_KEY"))
		}
		mergeJudge := fusion.NewLLMPlanMergeJudge(judgeClient, judgeModel)
		tournament.PlanMergeJudge = mergeJudge.Merge
	}
	loop.TournamentRunner = &fusionAdapter{t: tournament, gate: gate, cfg: cfg, client: client, memStore: memStore}
}

// fusionAdapter wraps a fusion.Tournament to satisfy the
// agentloop.TournamentRunner interface. It bridges the loop's
// ShouldRun/Run calls to the tournament's internal logic, injecting
// the fork function and run function that require loopbuilder-scoped
// dependencies (session store, llm client, etc.) (issue #290).
type fusionAdapter struct {
	t        *fusion.Tournament
	gate     *verify.Gate
	cfg      Config
	client   *llm.Client
	memStore *lessons.Store
}

func (a *fusionAdapter) ShouldRun(vr verify.Result) bool {
	if !a.cfg.FusionDifficultyGate {
		return !vr.Passed
	}
	return fusion.ShouldTournament(vr)
}

func (a *fusionAdapter) Run(ctx context.Context, prompt string) (string, int, error) {
	if a.t.ForkFunc == nil || a.t.RunFunc == nil {
		return "", 0, fmt.Errorf("fusion: tournament not fully wired (phase 1 — fork/run funcs nil)")
	}
	a.t.Prompt = prompt
	if a.t.SourceSessionID == "" {
		a.t.SourceSessionID = a.cfg.SessionID
	}
	result, err := a.t.Run(ctx)
	if err != nil {
		return "", 0, err
	}
	if result.Winner == nil {
		return "", 0, fmt.Errorf("fusion: no winner")
	}
	return result.Winner.Output, result.Winner.TokensUsed, nil
}
