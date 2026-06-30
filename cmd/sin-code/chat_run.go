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
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife/autoactivate"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/voice"
)

func runChat(ctx context.Context, opts *chatOptions) error {
	headless := opts.prompt != ""

	if opts.voice && !headless {
		if voice.IsAvailable() {
			fmt.Fprintln(chatStderr, "Voice input enabled (Ctrl+M to record, issue #481)")
		} else {
			fmt.Fprintln(chatStderr, "WARN: --voice set but no voice backend found (install sox/ffmpeg + whisper)")
		}
	}

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

	var streamCache *llm.PromptCache
	var streamCB func(string)

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

		streamCB = func(text string) {
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

		if enableCache {
			streamCache = llm.NewPromptCache(llm.DefaultCacheTTL)
		}
		completion = chatNewProviderCompletionStreamFn(client, model, agentCfg.MaxTokens, agentCfg.Temperature, streamCache, thinkingCfg, streamCB)
	}

	var completionBuilder agentloop.CompletionBuilder
	{
		switch {
		case headless && !opts.jsonOut:
			completionBuilder = func(m string) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
				return chatNewProviderCompletionStreamFn(client, m, agentCfg.MaxTokens, agentCfg.Temperature, streamCache, thinkingCfg, streamCB)
			}
		case enableCache:
			completionBuilder = func(m string) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
				cache := llm.NewPromptCache(llm.DefaultCacheTTL)
				return chatNewProviderCompletionFullFn(client, m, agentCfg.MaxTokens, agentCfg.Temperature, cache, thinkingCfg)
			}
		case thinkingCfg.Enabled:
			completionBuilder = func(m string) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
				return chatNewProviderCompletionFullFn(client, m, agentCfg.MaxTokens, agentCfg.Temperature, nil, thinkingCfg)
			}
		default:
			completionBuilder = func(m string) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
				return chatNewProviderCompletionFn(client, m, agentCfg.MaxTokens, agentCfg.Temperature)
			}
		}
	}

	perm := permission.New(chatRulesForAgentFn(agentCfg))
	perm.Yolo = opts.yolo
	perm.Headless = headless
	if err := setupPermissionMode(opts, perm); err != nil {
		return err
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
		ws, werr := setupWorktree(opts, sinCfg, workspace)
		if werr != nil {
			return werr
		}
		workspace = ws
	}

	// --- auto-checkpoint before agent loop (issue #483) ----------------
	// Creates a git-based checkpoint (git tag + SQLite metadata) before
	// the agent loop starts, giving the operator a rollback target.
	// M3: the checkpoint is a safety net, not a verification signal.
	// M4: create is non-destructive (git tag only) so no --yolo needed.
	// If the workspace is not a git repo, this is a no-op (graceful).
	if opts.checkpoint {
		setupAutoCheckpoint(opts, workspace)
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
		if err := setupRewind(opts, workspace); err != nil {
			return err
		}
	}

	hookEngine, act, hooklifeRunner, mcpMgr, err := setupHooksAndMCP(ctx, opts, sinCfg, workspace, headless)
	if err != nil {
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
		Model:                    model,
		CompletionBuilder:        completionBuilder,
		Hooks:                    hookEngine,
		Perm:                     perm,
		Ask:                      ask,
		ThinkingEnabled:          thinkingCfg.Enabled,
		ThinkingBudgetPerRequest: thinkingCfg.Budget,
		ResultPolicy:             permission.NewResultPolicy(),
		AutoCommit:               opts.autoCommit || sinCfg.AgentLoopAutoCommit,
		CommitPrefix:             firstNonEmpty(opts.commitPrefix, sinCfg.AgentLoopCommitPrefix),
	}

	if err := configureLoop(loop, opts, &sinCfg, act, mcpMgr, workspace, sess.ID, store, gate, client, hookEngine, perm, headless); err != nil {
		return err
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
		return runHeadlessMode(ctx, opts, loop, sess, act, dispatchUserPrompt, sinCfg, origStderr)
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
		if handleSlashModel(line, loop, sinCfg, agentCfg) {
			continue
		}
		if handleSlashMode(line, loop, mcpMgr, sinCfg) {
			continue
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
