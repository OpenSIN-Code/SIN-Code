// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` run logic — the main agent-loop dispatch.
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autolevel"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/checkpoint"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife/autoactivate"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/isolation"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/OpenSIN-Code/SIN-Code/skills"
)

func runChat(ctx context.Context, opts *chatOptions) error {
	headless := opts.prompt != ""

	// Feature 3: clean --json mode. When --json is set in headless mode,
	// suppress ALL ad-hoc stderr output (MCP warnings, sandbox
	// announcements, autoactivate messages, etc.) so stdout contains
	// ONLY the JSON result. The progress writer is explicitly opt-in
	// via --progress, so it uses the original stderr — not the
	// suppressed one. Errors are still surfaced via cobra's handler.
	origStderr := chatStderr
	if headless && opts.jsonOut {
		chatStderr = io.Discard
	}

	if opts.setup {
		return runSetupWizard()
	}

	sinCfg, _ := internal.LoadMergedConfig()

	apiKey := firstNonEmpty(os.Getenv("SIN_LLM_API_KEY"),
		os.Getenv("NVIDIA_API_KEY"), os.Getenv("OPENAI_API_KEY"),
		sinCfg.LLMAPIKey)
	if strings.TrimSpace(apiKey) == "" {
		if !headless && isTerminal(os.Stdin) {
			fmt.Fprintln(chatStderr, "No LLM API key configured. Starting setup wizard...")
			fmt.Fprintln(chatStderr)
			return runSetupWizard()
		}
		fmt.Fprintln(chatStderr, "Error: No LLM API key configured.")
		fmt.Fprintln(chatStderr, "")
		fmt.Fprintln(chatStderr, "Run 'sin-code config init' to set up your configuration, or manually set")
		fmt.Fprintln(chatStderr, "llm.api_key in your config file. Run 'sin-code config path' to find the file.")
		fmt.Fprintln(chatStderr, "")
		fmt.Fprintln(chatStderr, "Or set one of these environment variables:")
		fmt.Fprintln(chatStderr, "  export SIN_LLM_API_KEY=...")
		fmt.Fprintln(chatStderr, "  export NVIDIA_API_KEY=...")
		fmt.Fprintln(chatStderr, "  export OPENAI_API_KEY=...")
		return fmt.Errorf("no LLM API key configured")
	}

	// Feature 2: `sin-code chat` with no args launches the TUI when
	// both stdin and stdout are terminals. The stdin check prevents
	// launching the full-screen TUI when input is piped (e.g.,
	// `echo "foo" | sin-code chat`), which would hang or crash.
	if !opts.noTUI && !headless && !opts.jsonOut && isTerminal(os.Stdout) && isTerminal(os.Stdin) {
		return runChatTUI(ctx, opts)
	}

	var agentCfg orchestrator.AgentConfig
	if opts.agent != "" {
		cfg, _, err := chatLoadAgentFn(opts.agent)
		if err != nil {
			return err
		}
		agentCfg = cfg
	}

	baseURL := firstNonEmpty(opts.baseURL, agentCfg.BaseURL,
		sinCfg.LLMBaseURL, os.Getenv("SIN_LLM_BASE_URL"), "https://integrate.api.nvidia.com/v1")
	model := firstNonEmpty(opts.model, agentCfg.Model,
		sinCfg.LLMModel, os.Getenv("SIN_LLM_MODEL"))
	if model == "" {
		model = "nvidia/nemotron-3-nano-30b-a3b"
		fmt.Fprintln(chatStderr, "WARN: no model configured, using default: "+model)
		fmt.Fprintln(chatStderr, "Set it with: sin-code config set llm.model <model>")
	}
	client := chatNewLLMClientFn(baseURL, apiKey)

	enableCache := sinCfg.LLMPromptCache
	thinkingEnabled := opts.thinkingEnabled || sinCfg.LLMThinkingEnabled
	thinkingBudget := opts.thinkingBudget
	if thinkingBudget == 0 {
		thinkingBudget = sinCfg.LLMThinkingBudget
	}
	thinkingCfg := &agentloop.ThinkingConfig{
		Enabled: thinkingEnabled,
		Budget:  thinkingBudget,
	}
	completion := chatNewProviderCompletionFn(client, model, agentCfg.MaxTokens, agentCfg.Temperature)
	if enableCache {
		cache := llm.NewPromptCache(llm.DefaultCacheTTL)
		completion = chatNewProviderCompletionFullFn(client, model, agentCfg.MaxTokens, agentCfg.Temperature, cache, thinkingCfg)
	} else if thinkingCfg.Enabled {
		// Thinking-budget requires the *Full constructor so the
		// thinking{type:"enabled"} block ends up on the wire. With
		// the legacy factories the request body would not carry it.
		completion = chatNewProviderCompletionFullFn(client, model, agentCfg.MaxTokens, agentCfg.Temperature, nil, thinkingCfg)
	}

	// Feature 1: streaming output for headless -p mode. When the user
	// runs `sin-code chat -p "..."` without --json, tokens are printed
	// to stdout as they arrive. The streaming factory uses SSE
	// (stream=true) and forwards each content delta to the callback.
	// Tool-call deltas are accumulated so the agent loop's PLAN→ACT
	// cycle works identically to the non-streaming path.
	if headless && !opts.jsonOut {
		spinnerTTY := isTerminal(os.Stderr)
		var spinnerActive atomic.Bool
		spinnerActive.Store(spinnerTTY) // only start if stderr is a TTY
		spinnerDone := make(chan struct{})
		if spinnerTTY {
			go func() {
				defer close(spinnerDone)
				chars := `|/-\`
				i := 0
				for spinnerActive.Load() {
					fmt.Fprintf(chatStderr, "\r\033[KThinking... %c", chars[i%len(chars)])
					i++
					time.Sleep(100 * time.Millisecond)
				}
				fmt.Fprint(chatStderr, "\r\033[K")
			}()
		} else {
			close(spinnerDone)
		}

		streamCB := func(text string) {
			if spinnerActive.CompareAndSwap(true, false) {
				<-spinnerDone
			}
			fmt.Fprint(chatStdout, text)
		}

		// Ensure the spinner is stopped if loop.Run returns without
		// any streaming callback firing (e.g. immediate error).
		defer func() {
			if spinnerActive.CompareAndSwap(true, false) {
				<-spinnerDone
			}
		}()

		var streamCache *llm.PromptCache
		if enableCache {
			streamCache = llm.NewPromptCache(llm.DefaultCacheTTL)
		}
		completion = chatNewProviderCompletionStreamFn(client, model, agentCfg.MaxTokens, agentCfg.Temperature, streamCache, thinkingCfg, streamCB)
	}

	perm := permission.New(chatRulesForAgentFn(agentCfg))
	perm.Yolo = opts.yolo
	perm.Headless = headless
	if opts.mode != "" {
		if err := perm.SetMode(permission.Mode(opts.mode)); err != nil {
			return fmt.Errorf("chat: --mode: %w", err)
		}
	} else if opts.autolevel && opts.prompt != "" {
		// Auto-classify the prompt intent → permission mode.
		// Deterministic regex classifier (no LLM); result is
		// announced on stderr so M3's no-silent-mode-shift
		// invariant holds. --mode overrides --autolevel when
		// both are set (explicit > inferred).
		rec := autolevel.Classify(opts.prompt)
		if err := perm.SetMode(rec.Mode); err != nil {
			return fmt.Errorf("chat: --autolevel: %w", err)
		}
		fmt.Fprintf(chatStderr, "sin-code chat: --autolevel picked mode=%q (%s)\n",
			rec.Mode, rec.Reason)
	}

	workspace, err := chatGetwdFn()
	if err != nil {
		return err
	}

	applyChatSandboxPolicy(opts, headless, workspace)
	// --- worktree isolation (issue #194 part 2) --------------------------
	// If --worktree=<name> is set, provision a fresh git worktree from
	// HEAD and run the entire session from inside it. The worktree is
	// auto-locked while the chat runs so any cleanup timer refuses it.
	// M3 mandate: we never change CWD silently — the agent always sees
	// the worktree path printed to stderr so the user can verify what is
	// running, and the JSON contract's `summary` includes the worktree
	// path so headless consumers can pipe it for bookkeeping.
	if opts.worktree != "" {
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
				return fmt.Errorf("chat: --worktree=%s conflict check: %w", opts.worktree, perr)
			}
			if !report.Clean {
				switch checkMode {
				case "abort":
					return fmt.Errorf("chat: --worktree=%s: predicted conflicts with %s: %s; aborting",
						opts.worktree, targetBranch, strings.Join(report.ConflictPaths, ", "))
				case "warn":
					fmt.Fprintf(chatStderr, "sin-code chat: predicted conflicts with %s: %s\n",
						targetBranch, strings.Join(report.ConflictPaths, ", "))
				}
			}
		}

		wt, werr := isolation.Create(workspace, opts.worktree)
		if werr != nil {
			return fmt.Errorf("chat: --worktree=%s: %w", opts.worktree, werr)
		}
		fmt.Fprintf(chatStderr, "sin-code chat: worktree provisioned at %s\n", wt)
		if werr := os.Chdir(wt); werr != nil {
			return fmt.Errorf("chat: chdir into worktree: %w", werr)
		}
		workspace = wt
	}

	// --- rewind to checkpoint (issue #194 part 3) --------------------
	// Restores the workspace to a previously captured checkpoint
	// BEFORE the agent loop starts. Combines with --worktree: the
	// restore happens on the worktree path so each parallel-checkout
	// rewinds independently. Files don't exist after restore? We
	// still continue — only the bytes that were captured are
	// restored; the rest stays as-is. (M3: restore is silent to
	// the conversation; we emit a marker to stderr instead.)
	if opts.rewind != "" {
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
	}

	hookEngine := chatNewHooksFn(loadHooks(workspace))
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
	act := newChatActivator(workspace, opts)
	hooklifeReg := hooklife.NewRegistry()
	// Wire the activator's two hooks. Defaults + AutoOn are baked into
	// the SessionStartHook at registration time so the hook handles
	// per-session OnSessionStart internally when Dispatch fires.
	autoOn := act.Def.AutoOn || len(act.Rules) > 0 || len(act.Defaults) > 0
	hooklifeReg.Register(autoactivate.SessionStartHook{
		Act:      act.Act,
		Defaults: act.Defaults,
		AutoOn:   autoOn,
	})
	hooklifeReg.Register(autoactivate.UserPromptHook{Act: act.Act})
	hooklifeRunner := hooklife.NewRunner(hooklifeReg).WithTimeout(2 * time.Second)

	// --- External MCP servers (mandate C5, ecosystem skills) -------------
	mcpMgr := chatNewMCPManagerFn(chatLoadMCPConfigsFn(workspace))
	mcpMgr.Quiet = headless && !opts.verbose
	if sinCfg.MCPConnectTimeoutS > 0 {
		mcpMgr.SetConnectTimeout(time.Duration(sinCfg.MCPConnectTimeoutS) * time.Second)
	}
	if err := chatMCPConnectAllFn(mcpMgr, ctx); err != nil {
		return err
	}
	defer mcpMgr.Close()

	mode := opts.verifyMode
	if mode == "" {
		if opts.verifyCmd != "" {
			mode = "poc"
		} else {
			mode = "off"
		}
	}
	runner := commandRunner(opts.verifyCmd)
	gate := chatNewGateFn(mode, runner, runner)

	dbPath := opts.dbPath
	if dbPath == "" {
		dbPath = session.DefaultPath()
	}
	store, err := chatOpenSessionFn(dbPath)
	if err != nil {
		return fmt.Errorf("open sessions db: %w", err)
	}
	defer store.Close()
	sess, err := store.StartOrResume(opts.resume)
	if err != nil {
		return err
	}

	// Dispatch SessionStart once the session id is known. The hook
	// itself initialises the activator's per-session state, including
	// any CLI `--activate <rule>` names added after the fact.
	d := hooklifeRunner.Dispatch(ctx, hooklife.Event{
		Phase:   hooklife.SessionStart,
		Workdir: workspace,
		Meta: map[string]string{
			"session_id": sess.ID,
			"no_trigger": boolStr(opts.noTrigger),
		},
	})
	if d.Message != "" {
		fmt.Fprintln(chatStderr, "[autoactivate] session start rules:\n"+d.Message)
	}
	// Apply the CLI `--activate` list now that the hook has wired the
	// state for this session id.
	for _, name := range act.Rules {
		act.Act.Activate(sess.ID, autoactivate.Rule{Name: name})
	}

	var ask agentloop.AskFunc
	if !headless {
		ask = chatAskFn
		if ask == nil {
			ask = terminalAsk
		}
	}

	loop := &agentloop.Loop{
		Gate:                     gate,
		LocalTool:                combinedTool(workspace, mcpMgr),
		LocalSpec:                combinedSpecs(mcpMgr),
		Workspace:                workspace,
		MaxTurns:                 opts.maxTurns,
		SessionID:                sess.ID,
		Completion:               completion,
		Hooks:                    hookEngine,
		Perm:                     perm,
		Ask:                      ask,
		ThinkingEnabled:          thinkingCfg.Enabled,
		ThinkingBudgetPerRequest: thinkingCfg.Budget,
		ResultPolicy:             permission.NewResultPolicy(),
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
			SessionID:        sess.ID,
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

	dispatchUserPrompt := func(prompt string) {
		pd := hooklifeRunner.Dispatch(ctx, hooklife.Event{
			Phase:   hooklife.UserPrompt,
			Tool:    "ChatPrompt",
			Workdir: workspace,
			Meta: map[string]string{
				"session_id": sess.ID,
				"prompt":     prompt,
			},
		})
		if pd.Message != "" {
			fmt.Fprintln(chatStderr, "[autoactivate] per-turn rules:\n"+pd.Message)
		}
	}

	if headless {
		dispatchUserPrompt(opts.prompt)
		progress := opts.progress
		if progress == "" {
			progress = sinCfg.OutputProgress
		}
		var progressFile *os.File
		if progress != "off" && progress != "" {
			var w io.Writer = origStderr
			switch opts.progressDest {
			case "stdout":
				w = chatStdout
			case "file":
				if opts.progressFile == "" {
					fmt.Fprintln(chatStderr, "warn: --progress-dest=file requires --progress-file")
				} else {
					f, ferr := os.OpenFile(opts.progressFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, filemode.Default())
					if ferr != nil {
						fmt.Fprintf(chatStderr, "warn: cannot open progress file: %v\n", ferr)
					} else {
						w = f
						progressFile = f
					}
				}
			}
			pw := agentloop.NewProgressWriter(w)
			loop.ProgressWriter = pw
			loop.SessionID = sess.ID
			defer func() {
				pw.Close()
				if progressFile != nil {
					_ = progressFile.Close()
				}
			}()
		}
		var res *agentloop.Result
		var err error
		if chatRunOverrideFn != nil {
			res, err = chatRunOverrideFn(ctx, sess, opts.prompt)
		} else {
			res, err = loop.Run(ctx, sess, opts.prompt)
		}
		if err != nil {
			act.Act.EndSession(sess.ID)
			return friendlyError(err)
		}
		act.Act.EndSession(sess.ID)
		// Feature 1: when streaming was active (headless && !jsonOut),
		// the model's text was already printed token-by-token to stdout
		// during loop.Run(). Skip the duplicate summary and only emit
		// the trailing newline + session metadata line.
		if headless && !opts.jsonOut {
			fmt.Fprintln(chatStdout)
			fmt.Fprintf(chatStdout, "[session=%s verified=%v turns=%d]\n", res.SessionID, res.Verified, res.Turns)
			return nil
		}
		return chatPrintResultFn(res, opts.jsonOut)
	}

	fmt.Fprintf(chatStdout, "sin-code chat — session %s (verify=%s).", sess.ID, gate.Mode())
	if st, ok := act.Act.Snapshot(sess.ID); ok && len(st.ActiveRules.Names()) > 0 {
		fmt.Fprintf(chatStdout, " Active rules: %s", strings.Join(st.ActiveRules.Names(), ", "))
	}
	fmt.Fprintln(chatStdout, " Type 'exit' to quit.")
	scanner := bufio.NewScanner(chatStdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Fprint(chatStdout, "> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			break
		}
		dispatchUserPrompt(line)
		res, err := loop.Run(ctx, sess, line)
		if err != nil {
			fmt.Fprintf(chatStderr, "error: %v\n", friendlyError(err))
			continue
		}
		_ = chatPrintResultFn(res, opts.jsonOut)
	}
	act.Act.EndSession(sess.ID)
	return scanner.Err()
}
