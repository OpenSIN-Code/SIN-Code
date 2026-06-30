// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` flag/setup helpers — permission mode, worktree,
// checkpoint, rewind, hooks/MCP wiring, and loop configuration.
// sin-debt: shrink, upgrade: when a second chat-run-related function is needed, merge
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentmode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autolevel"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/checkpoint"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife/autoactivate"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/isolation"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
	"github.com/OpenSIN-Code/SIN-Code/skills"
)

// setupPermissionMode configures the permission engine's mode from
// CLI flags. --mode takes precedence over --autolevel (explicit >
// inferred). The autolevel classifier is deterministic (regex, no
// LLM) so M3's no-silent-mode-shift invariant holds.
func setupPermissionMode(opts *chatOptions, perm *permission.Engine) error {
	if opts.mode != "" {
		if err := perm.SetMode(permission.Mode(opts.mode)); err != nil {
			return fmt.Errorf("chat: --mode: %w", err)
		}
	} else if opts.autolevel && opts.prompt != "" {
		rec := autolevel.Classify(opts.prompt)
		if err := perm.SetMode(rec.Mode); err != nil {
			return fmt.Errorf("chat: --autolevel: %w", err)
		}
		fmt.Fprintf(chatStderr, "sin-code chat: --autolevel picked mode=%q (%s)\n",
			rec.Mode, rec.Reason)
	}
	return nil
}

// setupWorktree provisions a fresh git worktree from HEAD and returns
// the worktree path. M3 mandate: the worktree path is printed to
// stderr so the user can verify what is running.
func setupWorktree(opts *chatOptions, sinCfg internal.SinCodeConfig, workspace string) (string, error) {
	// Conflict prediction (issue #319): use the target branch from the
	// CLI flag or project/user config, and the configured action mode.
	checkMode := opts.conflictCheck
	if checkMode == "" {
		checkMode = sinCfg.WorktreeConflictCheck
	}
	targetBranch := opts.targetBranch
	if targetBranch == "" {
		targetBranch = sinCfg.WorktreeTargetBranch
	}
	if checkMode != "off" && targetBranch != "" {
		report, perr := isolation.PredictWorktreeConflicts(workspace, opts.worktree, targetBranch)
		if perr != nil {
			return "", fmt.Errorf("chat: --worktree=%s conflict check: %w", opts.worktree, perr)
		}
		if !report.Clean {
			switch checkMode {
			case "abort":
				return "", fmt.Errorf("chat: --worktree=%s: predicted conflicts with %s: %s; aborting",
					opts.worktree, targetBranch, strings.Join(report.ConflictPaths, ", "))
			case "warn":
				fmt.Fprintf(chatStderr, "sin-code chat: predicted conflicts with %s: %s\n",
					targetBranch, strings.Join(report.ConflictPaths, ", "))
			}
		}
	}

	wt, werr := isolation.Create(workspace, opts.worktree)
	if werr != nil {
		return "", fmt.Errorf("chat: --worktree=%s: %w", opts.worktree, werr)
	}
	fmt.Fprintf(chatStderr, "sin-code chat: worktree provisioned at %s\n", wt)
	if werr := os.Chdir(wt); werr != nil {
		return "", fmt.Errorf("chat: chdir into worktree: %w", werr)
	}
	return wt, nil
}

// setupAutoCheckpoint creates a git-based checkpoint (git tag + SQLite
// metadata) before the agent loop starts. M3: the checkpoint is a
// safety net, not a verification signal. M4: create is non-destructive
// (git tag only) so no --yolo needed. If the workspace is not a git
// repo, this is a no-op (graceful).
func setupAutoCheckpoint(opts *chatOptions, workspace string) {
	gstore, gerr := checkpoint.OpenGit(workspace)
	if gerr != nil {
		fmt.Fprintf(chatStderr, "sin-code chat: --checkpoint: open git store: %v (skipping)\n", gerr)
		return
	}
	cp, cerr := gstore.Create(context.Background(), "pre-chat auto-checkpoint")
	gstore.Close()
	if cerr != nil {
		fmt.Fprintf(chatStderr, "sin-code chat: --checkpoint: create: %v (skipping)\n", cerr)
		return
	}
	fmt.Fprintf(chatStderr, "sin-code chat: checkpoint %s created (rollback: sin-code checkpoint rollback %s --force)\n", cp.ID, cp.ID)
}

// setupRewind restores the workspace to a previously captured
// checkpoint BEFORE the agent loop starts. Combines with --worktree:
// the restore happens on the worktree path so each parallel-checkout
// rewinds independently. (M3: restore is silent to the conversation;
// we emit a marker to stderr instead.)
func setupRewind(opts *chatOptions, workspace string) error {
	cstore, cwerr := checkpoint.Open(workspace)
	if cwerr != nil {
		return fmt.Errorf("chat: --rewind=%s: open checkpoint store: %w",
			opts.rewind, cwerr)
	}
	if rwerr := cstore.Restore(context.Background(), workspace, opts.rewind); rwerr != nil {
		cstore.Close()
		return fmt.Errorf("chat: --rewind=%s: restore: %w", opts.rewind, rwerr)
	}
	cstore.Close()
	fmt.Fprintf(chatStderr, "sin-code chat: workspace restored to checkpoint %s\n", opts.rewind)
	return nil
}

// setupHooksAndMCP wires the hook engine, auto-activation hooks, and
// external MCP servers. Returns the hook engine, activator, hooklife
// runner, and MCP manager. The caller is responsible for deferring
// mcpMgr.Close().
func setupHooksAndMCP(ctx context.Context, opts *chatOptions, sinCfg internal.SinCodeConfig,
	workspace string, headless bool) (
	hookEngine *hooks.Engine,
	act *chatActivator,
	hooklifeRunner *hooklife.Runner,
	mcpMgr *mcpclient.Manager,
	err error,
) {
	hookEngine = chatNewHooksFn(loadHooks(workspace))
	// --- post-edit auto listeners (issue #376) ------------------
	// Register the lint + test listeners ONLY when the operator has opted
	// in via config. Default behaviour (no listener registered) preserves
	// the legacy single-shot semantics and stays off in headless / CI runs.
	if sinCfg.AutoLintEnabled {
		hookEngine.RegisterPostListener(hooks.AutoLintListener(hooks.AutoHookConfig{
			Timeout: time.Duration(sinCfg.AgentLoopAutoLintTimeout) * time.Second,
		}))
	}
	if sinCfg.AutoTestEnabled {
		hookEngine.RegisterPostListener(hooks.AutoTestListener(hooks.AutoHookConfig{
			Timeout: time.Duration(sinCfg.AgentLoopAutoTestTimeout) * time.Second,
		}))
	}

	// --- auto-activation hook (issue #176) ------------------------------
	// Off by default. Privacy-first: only opens when the operator sets
	// `--activate` or ships `.sin-code/autoactivate.toml`. The activator
	// keeps a per-session state and emits the rule body via hooklife
	// Decision.Message — informative stderr output today; LLM system-
	// prompt injection is tracked separately.
	act = newChatActivator(workspace, opts)
	hooklifeReg := hooklife.NewRegistry()
	autoOn := act.Def.AutoOn || len(act.Rules) > 0 || len(act.Defaults) > 0
	hooklifeReg.Register(autoactivate.SessionStartHook{
		Act:      act.Act,
		Defaults: act.Defaults,
		AutoOn:   autoOn,
	})
	hooklifeReg.Register(autoactivate.UserPromptHook{Act: act.Act})
	hooklifeRunner = hooklife.NewRunner(hooklifeReg).WithTimeout(2 * time.Second)

	// --- External MCP servers (mandate C5, ecosystem skills) -------------
	mcpMgr = chatNewMCPManagerFn(chatLoadMCPConfigsFn(workspace))
	mcpMgr.Quiet = headless && !opts.verbose
	if sinCfg.MCPConnectTimeoutS > 0 {
		mcpMgr.SetConnectTimeout(time.Duration(sinCfg.MCPConnectTimeoutS) * time.Second)
	}
	if err = chatMCPConnectAllFn(mcpMgr, ctx); err != nil {
		return nil, nil, nil, nil, err
	}
	return hookEngine, act, hooklifeRunner, mcpMgr, nil
}

// configureLoop applies agent mode, loop detector, compaction, tool
// coverage, lazy tools, fusion, and risk classifier configuration to
// the loop. The sinCfg pointer allows CLI overrides to persist back
// to the caller. Returns an error if agent mode is invalid.
func configureLoop(loop *agentloop.Loop, opts *chatOptions, sinCfg *internal.SinCodeConfig,
	act *chatActivator, mcpMgr *mcpclient.Manager, workspace string, sessID string,
	store *session.Store, gate *verify.Gate, client *llm.Client, hookEngine *hooks.Engine,
	perm *permission.Engine, headless bool) error {

	// Agent mode (issue #485): filter tools and prepend mode system prompt.
	// CLI --agent-mode overrides config agentloop.mode. Empty = default.
	agentModeStr := opts.agentMode
	if agentModeStr == "" {
		agentModeStr = sinCfg.AgentLoopMode
	}
	agentMode, amErr := agentmode.GetMode(agentModeStr)
	if amErr != nil {
		return fmt.Errorf("chat: --agent-mode: %w", amErr)
	}
	if agentMode.IsRestricted() {
		loop.LocalSpec = agentMode.FilterTools(loop.LocalSpec)
	}
	if modePrompt := agentMode.SystemPrompt(); modePrompt != "" {
		if loop.SystemPrompt != "" {
			loop.SystemPrompt = modePrompt + "\n\n" + loop.SystemPrompt
		} else {
			loop.SystemPrompt = modePrompt
		}
	}
	loop.AgentMode = string(agentMode)
	if agentMode.IsRestricted() && headless && !opts.jsonOut {
		fmt.Fprintf(chatStderr, "sin-code chat: agent mode=%s (tools restricted)\n", agentMode)
	}

	if opts.repetitionThreshold > 0 {
		loop.LoopDetector = agentloop.NewSimpleLoopDetector(opts.repetitionThreshold, opts.repetitionWindow)
	}

	// CLI flags override config-file defaults for context compaction mode.
	if opts.contextCompaction != "" {
		sinCfg.AgentLoopContextCompaction = opts.contextCompaction
	}
	if opts.compactionTrigger != "" {
		sinCfg.AgentLoopCompactionTrigger = opts.compactionTrigger
	}
	if opts.compactionMaxTokens > 0 {
		sinCfg.AgentLoopCompactionMaxTokens = opts.compactionMaxTokens
	}
	if opts.contextWindow > 0 {
		sinCfg.AgentLoopContextWindow = opts.contextWindow
	}
	if opts.preserveEvidence {
		sinCfg.AgentLoopCompactionPreserveEvidence = true
	}
	if opts.compactionRecentTurns > 0 {
		sinCfg.AgentLoopCompactionRecentTurns = opts.compactionRecentTurns
	}

	// Apply config-file defaults for tool coverage (issue #248) and merge
	// required_tools from activated skills' SKILL.md frontmatter. The
	// --activate list may contain skill names (e.g. "skill-code-build")
	// whose required_tools are additive to any config-level constraints.
	// Non-skill rule names are silently skipped by MergeRequiredTools.
	{
		coverageReq := loop.CoverageRequiredTools
		if len(coverageReq) == 0 {
			coverageReq = sinCfg.AgentLoopRequiredTools
		}
		if len(act.Rules) > 0 {
			if skillFS, err := skills.ListFS(); err == nil {
				coverageReq = skillmgr.MergeRequiredTools(coverageReq, act.Rules, skillFS)
			}
		}
		loop.CoverageRequiredTools = coverageReq
		if len(loop.CoverageForbiddenTools) == 0 {
			loop.CoverageForbiddenTools = sinCfg.AgentLoopForbiddenTools
		}
	}

	lazyTools := opts.lazyTools || sinCfg.ChatLazyTools || os.Getenv("SIN_LAZY_TOOLS") == "1"
	if lazyTools {
		loader := mcpclient.NewLazyToolLoader(allSpecsAsMCPClient(mcpMgr))
		semanticTools := opts.semanticTools || sinCfg.ChatSemanticTools || os.Getenv("SIN_SEMANTIC_TOOLS") == "1"
		if semanticTools {
			loader.UseSemantic(true, mcpclient.DefaultSemanticIndexCache())
		}
		loop.LocalSpec = lazyCombinedSpecs()
		loop.LocalTool = lazyCombinedTool(workspace, mcpMgr, loader, loop)
		// Re-apply agent mode filtering after lazy tools override (issue #485).
		if agentMode.IsRestricted() {
			loop.LocalSpec = agentMode.FilterTools(loop.LocalSpec)
		}
	}

	if sinCfg.AgentLoopCompactionStrategy != "off" && sinCfg.AgentLoopCompactionStrategy != "" {
		strategy, err := agentloop.ParseCompactionStrategy(sinCfg.AgentLoopCompactionStrategy)
		if err != nil {
			fmt.Fprintf(chatStderr, "warn: invalid compaction strategy %q: %v\n", sinCfg.AgentLoopCompactionStrategy, err)
		} else {
			compactor := agentloop.NewCompactor(nil)
			compactor.Threshold = sinCfg.AgentLoopCompactionThreshold
			if compactor.Threshold <= 0 {
				compactor.Threshold = agentloop.DefaultCompactionThreshold
			}
			loop.Compactor = compactor
			loop.CompactionStrategy = strategy
		}
	}

	if sinCfg.AgentLoopFrustrationDetection {
		loop.Frustration = agentloop.NewFrustrationDetector()
	}

	// Context compaction mode (issue: compaction-modes): when non-off,
	// mode-based compaction replaces strategy-based. CLI flags take
	// precedence over config-file defaults (already merged into sinCfg).
	if sinCfg.AgentLoopContextCompaction != "" && sinCfg.AgentLoopContextCompaction != "off" {
		mode, _ := agentloop.ParseContextCompactionMode(sinCfg.AgentLoopContextCompaction)
		trigger, _ := agentloop.ParseCompactionTrigger(sinCfg.AgentLoopCompactionTrigger)
		if loop.Compactor == nil {
			loop.Compactor = agentloop.NewCompactor(nil)
		}
		loop.Compactor.Configure(agentloop.CompactorConfig{
			Mode:             mode,
			Trigger:          trigger,
			PreserveEvidence: sinCfg.AgentLoopCompactionPreserveEvidence,
			RecentTurns:      sinCfg.AgentLoopCompactionRecentTurns,
			MaxTokens:        sinCfg.AgentLoopCompactionMaxTokens,
		})
		if sinCfg.AgentLoopContextWindow > 0 {
			loop.ContextWindow = sinCfg.AgentLoopContextWindow
		}
	}

	if opts.fusionOnVerifyFail {
		fusionCfg := loopbuilder.Config{
			FusionEnabled:    true,
			FusionProviders:  splitList(opts.fusionProviders),
			FusionMaxCostUSD: opts.fusionMaxCost,
			Workspace:        workspace,
			SessionID:        sessID,
			SessionStore:     store,
		}
		if sinCfg, err := internal.LoadMergedConfig(); err == nil {
			if fusionCfg.FusionMinQuorum == 0 {
				fusionCfg.FusionMinQuorum = sinCfg.FusionMinQuorum
			}
			if fusionCfg.FusionPerProviderTimeoutS == 0 {
				fusionCfg.FusionPerProviderTimeoutS = sinCfg.FusionPerProviderTimeoutS
			}
			fusionCfg.FusionDifficultyGate = sinCfg.FusionDifficultyGate
		}
		if fusionCfg.FusionMinQuorum == 0 {
			fusionCfg.FusionMinQuorum = 2
		}
		if fusionCfg.FusionPerProviderTimeoutS == 0 {
			fusionCfg.FusionPerProviderTimeoutS = 120
		}
		loopbuilder.WireFusion(loop, fusionCfg, gate, client, nil, nil, hookEngine)
	}

	if sinCfg, err := internal.LoadMergedConfig(); err == nil {
		if sinCfg.AgentLoopCompactionStrategy != "" {
			strategy, serr := agentloop.ParseCompactionStrategy(sinCfg.AgentLoopCompactionStrategy)
			if serr != nil {
				strategy = agentloop.DefaultCompactionStrategy()
			}
			loop.Compactor = agentloop.NewCompactor(nil)
			loop.CompactionStrategy = strategy
		}
		if sinCfg.AgentLoopFrustrationDetection {
			loop.Frustration = agentloop.NewFrustrationDetector()
		}
		if opts.yolo && sinCfg.PermissionYoloRiskThreshold != "" {
			classifier := permission.NewRiskClassifier()
			if level, perr := permission.ParseRiskLevel(sinCfg.PermissionYoloRiskThreshold); perr == nil {
				classifier.SetThreshold(level)
			}
			perm.Risk = classifier
		}
	}

	return nil
}
