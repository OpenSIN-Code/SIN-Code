// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` — CLI binding for the C1-C5 packages
// (agentloop, session, verify, permission, mcpclient). Issue #44.
// REPL mode by default; headless one-shot via -p/--prompt with a stable
// JSON contract: {session_id, summary, verified, turns}.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autolevel"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/checkpoint"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/commands"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife/autoactivate"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/isolation"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/logger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/skillmgr"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/tui"
	"github.com/OpenSIN-Code/SIN-Code/skills"
)

// chat hook variables — injected by coverage tests to avoid real I/O, network
// or LLM calls. Production defaults point to the real implementations.
var (
	chatLoadAgentFn             = internal.LoadEffectiveAgent
	chatNewLLMClientFn          = llm.NewClient
	chatNewProviderCompletionFn = agentloop.NewProviderCompletion
	chatRulesForAgentFn         = internal.RulesForAgent
	chatGetwdFn                 = os.Getwd
	chatNewHooksFn              = hooks.New
	chatLoadMCPConfigsFn        = mcpclient.LoadConfigs
	chatNewMCPManagerFn         = mcpclient.NewManager
	chatMCPConnectAllFn         = func(mgr *mcpclient.Manager, ctx context.Context) error { return mgr.ConnectAll(ctx) }
	chatOpenSessionFn           = session.Open
	chatNewGateFn               = verify.NewGate
	chatAskFn                   agentloop.AskFunc
	chatPrintResultFn           = printResult
	chatRunOverrideFn           func(context.Context, *session.Session, string) (*agentloop.Result, error)
	chatStdout                  io.Writer = os.Stdout
	chatStderr                  io.Writer = os.Stderr
	chatStdin                   io.Reader = os.Stdin
)

type chatOptions struct {
	prompt     string
	jsonOut    bool
	resume     string
	agent      string
	yolo       bool
	model      string
	baseURL    string
	verifyMode string
	verifyCmd  string
	maxTurns   int
	dbPath     string
	// activate is a comma-separated list of rule names to auto-activate
	// for this session (issue #176). Empty = no CLI-level activation;
	// a project-local .sin-code/autoactivate.toml may still apply.
	activate  string
	noTrigger bool
	mode      string
	// worktree provisions a git-worktree at .sin-code/worktrees/<name>
	// for the chat session and runs the entire agent loop from inside
	// it. The worktree is auto-locked while the chat runs and is left
	// intact on exit (use `sin-code sessions rm` or `git worktree
	// remove --force` to clean up). Issue #194 part 2.
	worktree string
	// rewind restores the workspace to the named checkpoint before
	// the chat starts. Empty means start from current state.
	// Combined with --worktree: the restore happens on the
	// worktree path, so per-checkout branches can be rewound
	// independently. Mirrors Claude Code's headless --from-pr
	// rerun-on-checkpoint pattern.
	rewind string
	// targetBranch names the integration branch used for conflict
	// prediction before creating a worktree (issue #319).
	targetBranch string
	// conflictCheck controls whether we predict merge conflicts with the
	// target branch before creating a worktree: off, warn, abort.
	conflictCheck string
	// sandbox selects the syscall-filter backend used to wrap every
	// `sin_bash` invocation in this chat session.
	//   landlock    — Linux-only, kernel-level (default when on Linux)
	//   seatbelt    — macOS sandbox-exec SBPL profile (default on Darwin)
	//   bubblewrap  — Linux bwrap(1) wrapper
	//   none        — disable syscall filtering entirely (debugging only)
	// Empty value picks the platform default at startup.
	sandbox string
	// autolevel flips the chat loop into auto-classification mode:
	// if --mode is empty AND --autolevel is set, the chat reads
	// `opts.prompt` through `internal/autolevel.Classify` to pick
	// `plan | acceptEdits | bypass | default` automatically. The
	// classifier is deterministic (regex + substring, no LLM) so
	// M3 is honoured: every mode pick is operator-visible.
	autolevel          bool
	lazyTools          bool
	fusionOnVerifyFail bool
	fusionProviders    string
	fusionMaxCost      float64
	thinkingEnabled    bool
	thinkingBudget     int
	noTUI              bool
	watch              string

	contextCompaction      string
	compactionTrigger      string
	compactionMaxTokens    int
	contextWindow          int
	preserveEvidence       bool
	compactionRecentTurns  int
}

func NewChatCmd() *cobra.Command {
	opts := &chatOptions{}
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Run the SIN-Code agent loop (interactive REPL or headless one-shot)",
		Long: `sin-code chat starts the PLAN -> ACT -> VERIFY -> DONE agent loop.

  sin-code chat                          interactive REPL
  sin-code chat -p "..." --json          headless one-shot (stable JSON contract)
  sin-code chat --resume <session-id>    continue an existing session
  sin-code chat --agent <name>           use a specific agent profile
  sin-code chat --yolo                   bypass 'ask' permissions (M4)
  sin-code chat --activate terse,skill-x auto-activate the named rules (issue #176)
  sin-code chat --no-trigger             disable prompt-phrase activation
  sin-code chat --mode plan|acceptEdits|bypass  session-wide permission mode (issue #193)
  sin-code chat --worktree <name>        run inside a git worktree (issue #194 part 2)
  sin-code chat --target-branch <name>   integration branch for worktree conflict prediction (issue #319)
  sin-code chat --conflict-check <mode>  off|warn|abort — action on predicted worktree conflicts (issue #319)
  sin-code chat --rewind <checkpoint>    restore workspace to a checkpoint before running
  sin-code chat --sandbox <backend>      landlock|seatbelt|bubblewrap|none (issue #199)
  sin-code chat --autolevel              prompt-intent based permission auto-classifier (issue #198)
  sin-code chat --lazy-tools             lazy tool loading via tool_search (issue #270)
  sin-code chat --fusion-on-verify-fail  enable SIN Fusion verify-tournament on verify.fail (issue #290)
  sin-code chat --fusion-providers <list> override Fireworks models for the tournament (comma-separated)
  sin-code chat --fusion-max-cost <usd>   USD kill-switch per tournament invocation (default 5.0)
  sin-code chat --thinking-enabled       send thinking{type:"enabled"} on each request (per-provider reasoning budget)
  sin-code chat --thinking-budget <n>    per-request thinking.budget_tokens cap (0 = unbounded / provider default)
  Oracle-mode fusion is experimental; set fusion.oracle_mode=true via config. Prefer PoC mode for verifiable tasks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChat(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&opts.prompt, "prompt", "p", "", "headless one-shot prompt")
	f.BoolVar(&opts.jsonOut, "json", false, "emit the stable JSON contract {session_id, summary, verified, turns}")
	f.StringVar(&opts.resume, "resume", "", "resume an existing session by id")
	f.StringVar(&opts.agent, "agent", "", "agent profile name (see `sin-code agents`)")
	f.BoolVar(&opts.yolo, "yolo", false, "bypass 'ask' permissions (deny is NEVER bypassed)")
	f.StringVar(&opts.model, "model", "", "override LLM model (default: agent profile / SIN_LLM_MODEL)")
	f.StringVar(&opts.baseURL, "base-url", "", "override LLM base URL (default: agent profile / SIN_LLM_BASE_URL)")
	f.StringVar(&opts.verifyMode, "verify-mode", "", "verification gate mode: poc|oracle|off (default: poc if --verify-cmd set, else off)")
	f.StringVar(&opts.verifyCmd, "verify-cmd", os.Getenv("SIN_VERIFY_CMD"), "shell command used as verification runner (exit 0 = pass)")
	f.IntVar(&opts.maxTurns, "max-turns", 0, "max agent turns (default 80)")
	f.StringVar(&opts.dbPath, "db", "", "sessions db path (default ~/.local/share/sin-code/sessions.db)")
	f.StringVar(&opts.activate, "activate", "", "comma-separated rule names to auto-activate for this session (issue #176)")
	f.BoolVar(&opts.noTrigger, "no-trigger", false, "disable prompt-phrase activation (issue #176)")
	f.StringVar(&opts.mode, "mode", "", "permission mode: default|plan|acceptEdits|bypass (issue #193)")
	f.StringVar(&opts.worktree, "worktree", "", "run inside a fresh git worktree at .sin-code/worktrees/<name> (issue #194)")
	f.StringVar(&opts.targetBranch, "target-branch", "", "integration branch for worktree conflict prediction (issue #319)")
	f.StringVar(&opts.conflictCheck, "conflict-check", "", "conflict prediction before worktree: off|warn|abort (default from config)")
	f.StringVar(&opts.rewind, "rewind", "", "restore workspace to the named checkpoint before the chat starts")
	f.StringVar(&opts.sandbox, "sandbox", "", "sandbox backend: landlock|seatbelt|bubblewrap|none (default: platform-native)")
	f.BoolVar(&opts.autolevel, "autolevel", false, "auto-classify permission mode from prompt intent (issue #198)")
	f.BoolVar(&opts.lazyTools, "lazy-tools", false, "enable lazy tool loading: send only tool_search meta-tool instead of all tools (issue #270)")
	f.BoolVar(&opts.fusionOnVerifyFail, "fusion-on-verify-fail", false, "enable SIN Fusion verify-tournament on verify.fail (issue #290)")
	f.StringVar(&opts.fusionProviders, "fusion-providers", "", "comma-separated Fireworks model names for the tournament (e.g. minimax-m3,kimi-k2p7-code,glm-5p2)")
	f.Float64Var(&opts.fusionMaxCost, "fusion-max-cost", 5.0, "USD kill-switch per tournament invocation (issue #290)")
	f.BoolVar(&opts.thinkingEnabled, "thinking-enabled", false, "send thinking{type:\"enabled\"} on each LLM request (issue: thinking-budget-enforcement)")
	f.IntVar(&opts.thinkingBudget, "thinking-budget", 0, "per-request thinking.budget_tokens cap (0 = unbounded; requires --thinking-enabled)")
	f.BoolVar(&opts.noTUI, "no-tui", false, "skip TUI and use plain CLI loop")
	f.StringVar(&opts.watch, "watch", "", "watch file patterns (comma-separated, e.g. *.go,*.py) and re-run the last prompt on change")
	f.StringVar(&opts.contextCompaction, "context-compaction", "", "context compaction mode: off|deterministic|llm|hybrid (issue: compaction-modes)")
	f.StringVar(&opts.compactionTrigger, "compaction-trigger", "", "compaction trigger: turns|tokens|both (default from config)")
	f.IntVar(&opts.compactionMaxTokens, "compaction-max-tokens", 0, "compaction token budget (default 8000)")
	f.IntVar(&opts.contextWindow, "context-window", 0, "effective token cap for compaction (0 = auto)")
	f.BoolVar(&opts.preserveEvidence, "compaction-preserve-evidence", false, "enable evidence preservation during compaction (M3, default true)")
	f.IntVar(&opts.compactionRecentTurns, "compaction-recent-turns", 0, "number of recent human turns to retain (default 4)")
	return cmd
}

func runChat(ctx context.Context, opts *chatOptions) error {
	headless := opts.prompt != ""

	if !opts.noTUI && !headless && !opts.jsonOut && isTerminal(os.Stdout) {
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
		os.Getenv("SIN_LLM_BASE_URL"), "https://integrate.api.nvidia.com/v1")
	apiKey := firstNonEmpty(os.Getenv("SIN_LLM_API_KEY"),
		os.Getenv("NVIDIA_API_KEY"), os.Getenv("OPENAI_API_KEY"))
	model := firstNonEmpty(opts.model, agentCfg.Model, os.Getenv("SIN_LLM_MODEL"))
	client := chatNewLLMClientFn(baseURL, apiKey)

	sinCfg, _ := internal.LoadMergedConfig()

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
		completion = agentloop.NewProviderCompletionFull(client, model, agentCfg.MaxTokens, agentCfg.Temperature, cache, thinkingCfg)
	} else if thinkingCfg.Enabled {
		// Thinking-budget requires the *Full constructor so the
		// thinking{type:"enabled"} block ends up on the wire. With
		// the legacy factories the request body would not carry it.
		completion = agentloop.NewProviderCompletionFull(client, model, agentCfg.MaxTokens, agentCfg.Temperature, nil, thinkingCfg)
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
		fmt.Fprintf(os.Stderr, "sin-code chat: worktree provisioned at %s\n", wt)
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
		fmt.Fprintf(os.Stderr, "sin-code chat: workspace restored to checkpoint %s\n", opts.rewind)
	}

	hookEngine := chatNewHooksFn(loadHooks(workspace))

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
		Gate:                    gate,
		LocalTool:               combinedTool(workspace, mcpMgr),
		LocalSpec:               combinedSpecs(mcpMgr),
		Workspace:               workspace,
		MaxTurns:                opts.maxTurns,
		SessionID:               sess.ID,
		Completion:              completion,
		Hooks:                   hookEngine,
		Perm:                    perm,
		Ask:                     ask,
		ThinkingEnabled:         thinkingCfg.Enabled,
		ThinkingBudgetPerRequest: thinkingCfg.Budget,
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
		var res *agentloop.Result
		var err error
		if chatRunOverrideFn != nil {
			res, err = chatRunOverrideFn(ctx, sess, opts.prompt)
		} else {
			res, err = loop.Run(ctx, sess, opts.prompt)
		}
		if err != nil {
			act.Act.EndSession(sess.ID)
			return err
		}
		act.Act.EndSession(sess.ID)
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
			fmt.Fprintf(chatStderr, "error: %v\n", err)
			continue
		}
		_ = chatPrintResultFn(res, opts.jsonOut)
	}
	act.Act.EndSession(sess.ID)
	return scanner.Err()
}

func printResult(res *agentloop.Result, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(chatStdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	fmt.Fprintln(chatStdout, res.Summary)
	fmt.Fprintf(chatStdout, "[session=%s verified=%v turns=%d]\n", res.SessionID, res.Verified, res.Turns)
	return nil
}

func terminalAsk(tc agentloop.ToolCall) bool {
	fmt.Fprintf(chatStdout, "Permission required: tool %q with args %v — allow? [y/N] ", tc.Name, tc.Args)
	reader := bufio.NewReader(chatStdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
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

func loadHooks(workspace string) []hooks.Hook {
	var all []hooks.Hook
	paths := []string{}
	if cfg, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfg, "sin-code", "hooks.json"))
		paths = append(paths, filepath.Join(cfg, "sin-code", "hooks.yaml"))
		paths = append(paths, filepath.Join(cfg, "sin-code", "hooks.yml"))
	}
	paths = append(paths, filepath.Join(workspace, ".sin-code", "hooks.json"))
	paths = append(paths, filepath.Join(workspace, ".sin-code", "hooks.yaml"))
	paths = append(paths, filepath.Join(workspace, ".sin-code", "hooks.yml"))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var hs []hooks.Hook
		if strings.HasSuffix(p, ".json") {
			err = json.Unmarshal(data, &hs)
		} else {
			// YAML may be a top-level list or wrapped under `hooks:`.
			var list []hooks.Hook
			if yerr := yaml.Unmarshal(data, &list); yerr == nil {
				hs = list
			} else {
				var wrapped struct {
					Hooks []hooks.Hook `yaml:"hooks"`
				}
				err = yaml.Unmarshal(data, &wrapped)
				hs = wrapped.Hooks
			}
		}
		if err != nil {
			fmt.Fprintf(chatStderr, "warn: skipping invalid hooks file %s: %v\n", p, err)
			continue
		}
		all = append(all, hs...)
	}
	return all
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// chatActivator bundles the autoactivate.Activator with the CLI flags
// that should be applied when a real session id is known. A single
// instance is created per chat invocation; it is GC'd on exit.
type chatActivator struct {
	Act      *autoactivate.Activator
	Defaults autoactivate.RuleSet
	Def      autoactivate.Default
	Rules    []string // CLI --activate list (names only — bodies come from TOML)
}

// newChatActivator constructs a chatActivator from workspace +
// the optional `--activate <list>` and `--no-trigger` flags. Reads
// `.sin-code/autoactivate.toml` silently when present (privacy-first).
func newChatActivator(workspace string, opts *chatOptions) *chatActivator {
	defaults, def, _ := autoactivate.LoadFile(filepath.Join(workspace, ".sin-code", "autoactivate.toml"))
	return &chatActivator{
		Act:      autoactivate.NewActivator(defaults),
		Defaults: defaults,
		Def:      def,
		Rules:    parseActivateFlag(opts.activate),
	}
}

// parseActivateFlag splits a comma-separated rule list into trimmed
// non-empty names. Empty input returns nil.
func parseActivateFlag(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, raw := range strings.Split(s, ",") {
		n := strings.TrimSpace(raw)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// splitList splits a comma-separated list into trimmed, non-empty tokens.
// Empty input returns nil.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, raw := range strings.Split(s, ",") {
		n := strings.TrimSpace(raw)
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// chatSideLLM adapts *llm.Client to the commands.SideLLM interface so
// built-in slash commands (issue #276 /btw) can fire one-shot completions
// without depending on the llm package directly.
type chatSideLLM struct {
	c     *llm.Client
	model string
}

func (a chatSideLLM) Complete(ctx context.Context, system, user string) (string, error) {
	resp, err := a.c.Chat(ctx, llm.ChatRequest{
		Model: a.model,
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ExtractText(), nil
}

// chatUndercover is the session-wide undercover mode shared between the
// /undercover slash command and the commit path. Construction is cheap;
// the chat loop reads .Enabled() before committing (issue #274).
var chatUndercover = commands.NewUndercoverMode()

// newBuiltinCommandRegistry builds the registry of Go-implemented slash
// commands for a chat session. The LLM adapter is wired from the live
// client/model so /btw can answer side questions (issue #276). The
// /undercover command is always available and reuses the package-level
// chatUndercover mode so toggles persist across turns (issue #274).
// This is registration only — the chat loop itself is unchanged.
func newBuiltinCommandRegistry(client *llm.Client, model string) *commands.Registry {
	r := commands.NewRegistry()
	r.Register(commands.NewBTWCommand(chatSideLLM{c: client, model: model}, ""))
	r.Register(commands.NewUndercoverCommand(chatUndercover))
	r.Register(commands.NewInitCommand())
	return r
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func runChatTUI(ctx context.Context, opts *chatOptions) error {
	logger.SetLevel(logger.LevelError)

	pm := tui.NewModel()
	pm.SwitchView(tui.ViewChat)
	ws, _ := chatGetwdFn()
	pm.Workspace = ws
	pm.SetContextFn(func() context.Context { return ctx })

	maxTurns := opts.maxTurns
	if maxTurns == 0 {
		maxTurns = 80
	}
	pm.AgentConfig = tui.AgentRunnerConfig{
		Yolo:       opts.yolo,
		MaxTurns:   maxTurns,
		Model:      opts.model,
		VerifyMode: opts.verifyMode,
		VerifyCmd:  opts.verifyCmd,
	}

	pm.OnRun = func(name string, args []string) error {
		c := getSubcommand(name)
		if c == nil {
			return fmt.Errorf("unknown subcommand: %s", name)
		}
		c.SetArgs(args)
		c.SetOut(os.Stdout)
		c.SetErr(os.Stderr)
		return c.Execute()
	}

	if ov := loadTUIKeyOverrides(); ov != nil {
		km := tui.DefaultKeymap()
		km.ApplyOverrides(*ov)
		tui.SetKeymap(km)
	}

	guard := tui.SetupPlatformGuard()
	defer guard.Cleanup()

	return tui.RunProgram(pm, tui.ProgramOptions{
		Sigusr2Reload: true,
	})
}
