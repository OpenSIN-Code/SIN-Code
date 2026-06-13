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
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
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

	cleanup := func() error {
		mcpMgr.Close()
		if ledgerStore != nil {
			_ = ledgerStore.Close()
		}
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
