// SPDX-License-Identifier: MIT
// Purpose: shared loop factory — eliminates duplication of provider /
// permission / hooks / gate / mcp / memory setup across chat / swarm /
// serve (issue #64, DRY refactor).
package loopbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/eval"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/stopgate"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
)

type Config struct {
	Workspace   string
	SessionID   string
	AgentName   string
	Model       string
	BaseURL     string
	MaxTurns    int
	VerifyMode  string
	VerifyCmd   string
	Yolo        bool
	Headless    bool
	AskFunc     agentloop.AskFunc
	LocalTool   agentloop.LocalToolFunc
	LocalSpec   []agentloop.ToolSpec
	ToolFactory func(*mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec)
	SkipMCP     bool

	// Contract, when non-nil and non-empty, activates the stop-gate: the
	// worker's "done" is confirmed against this Definition-of-Done by an
	// independent hybrid evaluator before the loop returns DONE.
	Contract *goalcontract.GoalContract
	// AllowContinuation switches the maxTurns outcome from a hard error to a
	// resumable checkpoint (used by the daemon).
	AllowContinuation bool
}

// Build constructs a fully wired agentloop.Loop with all mandates applied
// (C1-C8, M1-M4). Returns the loop and a cleanup function (defer it).
func Build(ctx context.Context, cfg Config, memStore *lessons.Store) (*agentloop.Loop, func() error, error) {
	var agentCfg orchestrator.AgentConfig
	if cfg.AgentName != "" {
		loaded, _, err := internal.LoadEffectiveAgent(cfg.AgentName)
		if err != nil {
			return nil, nil, fmt.Errorf("load agent profile: %w", err)
		}
		agentCfg = loaded
	}

	baseURL := firstNonEmpty(cfg.BaseURL, agentCfg.BaseURL,
		os.Getenv("SIN_LLM_BASE_URL"), "https://integrate.api.nvidia.com/v1")
	apiKey := firstNonEmpty(os.Getenv("SIN_LLM_API_KEY"),
		os.Getenv("NVIDIA_API_KEY"), os.Getenv("OPENAI_API_KEY"))
	model := firstNonEmpty(cfg.Model, agentCfg.Model, os.Getenv("SIN_LLM_MODEL"))
	client := llm.NewClient(baseURL, apiKey)
	completion := agentloop.NewProviderCompletion(client, model, agentCfg.MaxTokens, agentCfg.Temperature)

	perm := permission.New(internal.RulesForAgent(agentCfg))
	perm.Yolo = cfg.Yolo
	perm.Headless = cfg.Headless

	hookEngine := hooks.New(loadHooks(cfg.Workspace))

	mode := cfg.VerifyMode
	if mode == "" {
		if cfg.VerifyCmd != "" {
			mode = "poc"
		} else {
			mode = "off"
		}
	}
	runner := commandRunner(cfg.VerifyCmd)
	gate := verify.NewGate(mode, runner, runner)

	mcpMgr := mcpclient.NewManager(mcpclient.LoadConfigs(cfg.Workspace))
	if !cfg.SkipMCP {
		if err := mcpMgr.ConnectAll(ctx); err != nil {
			return nil, nil, err
		}
	}

	// Tool wiring: explicit (LocalTool/LocalSpec) wins over factory.
	var localTool agentloop.LocalToolFunc = cfg.LocalTool
	var localSpec []agentloop.ToolSpec = cfg.LocalSpec
	if cfg.ToolFactory != nil && (localTool == nil || localSpec == nil) {
		localTool, localSpec = cfg.ToolFactory(mcpMgr)
	}

	ledgerStore, err := ledger.Open(ledger.DefaultPath())
	if err != nil {
		ledgerStore = nil // ledger is optional; do not fail the loop if it cannot open
	}

	loop := &agentloop.Loop{
		Gate:       gate,
		LocalTool:  localTool,
		LocalSpec:  localSpec,
		Workspace:  cfg.Workspace,
		MaxTurns:   cfg.MaxTurns,
		SessionID:  cfg.SessionID,
		Completion: completion,
		Hooks:      hookEngine,
		Perm:       perm,
		Ask:        cfg.AskFunc,
		Lessons:    memStore,
		Ledger:     ledgerStore,
	}

	// Stop-gate (anti-babysitting): when a Definition-of-Done contract is
	// supplied, completion authority is taken away from the worker. The
	// hybrid gate runs deterministic checks first, then a strong/equal LLM
	// judge (SIN_EVALUATOR_MODEL, falling back to the worker model) for the
	// non-mechanical criteria. Without a contract the loop is unchanged.
	loop.AllowContinuation = cfg.AllowContinuation
	if cfg.Contract != nil && !cfg.Contract.IsEmpty() {
		var gateOpts []stopgate.Option
		evalModel := firstNonEmpty(os.Getenv("SIN_EVALUATOR_MODEL"), model)
		if len(cfg.Contract.SemanticCriteria) > 0 && evalModel != "" {
			evalClient := client
			if base := os.Getenv("SIN_EVALUATOR_BASE_URL"); base != "" {
				evalClient = llm.NewClient(base, firstNonEmpty(os.Getenv("SIN_EVALUATOR_API_KEY"), apiKey))
			}
			if judge, jerr := eval.NewJudge(eval.JudgeConfig{Model: evalModel, Strict: true}, evalClient); jerr == nil {
				gateOpts = append(gateOpts, stopgate.WithJudge(judge))
			} else {
				fmt.Fprintf(os.Stderr, "warn: stop-gate semantic judge disabled: %v\n", jerr)
			}
		}
		gate := stopgate.New(cfg.Workspace, gateOpts...)
		loop.StopGate = gate.LoopGate(*cfg.Contract)

		// Tell the worker the rubric up front (SinCode Loop System): the same
		// semantic criteria the stop-gate will enforce are injected as a
		// Definition-of-Done preamble so tests/debug/docs/completeness are
		// handled on the first pass instead of after a rejection.
		loop.Preamble = goalcontract.Preamble(*cfg.Contract)
	}

	// Headroom context compression (issue #118): opt-in via HEADROOM_ENABLED.
	// When disabled or unavailable the hook is a no-op and is not wired.
	headroomHook := agentloop.NewHeadroomHook(headroom.LoadConfigFromEnv())
	if headroomHook.Enabled() {
		loop.CompressMessages = headroomHook.CompressMessages
	}

	cleanup := func() error {
		mcpMgr.Close()
		if ledgerStore != nil {
			_ = ledgerStore.Close()
		}
		_ = headroomHook.Close()
		return nil
	}
	return loop, cleanup, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func loadHooks(workspace string) []hooks.Hook {
	var all []hooks.Hook
	paths := []string{}
	if cfg, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfg, "sin-code", "hooks.json"))
	}
	paths = append(paths, filepath.Join(workspace, ".sin-code", "hooks.json"))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var hs []hooks.Hook
		if err := json.Unmarshal(data, &hs); err != nil {
			fmt.Fprintf(os.Stderr, "warn: skipping invalid hooks file %s: %v\n", p, err)
			continue
		}
		all = append(all, hs...)
	}
	return all
}

func commandRunner(command string) verify.Runner {
	if command == "" {
		return nil
	}
	return func(ctx context.Context, workspace string) (bool, string, error) {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(cctx, "sh", "-c", command)
		cmd.Dir = workspace
		out, err := cmd.CombinedOutput()
		report := strings.TrimSpace(string(out))
		if err != nil {
			return false, report, nil
		}
		return true, report, nil
	}
}
