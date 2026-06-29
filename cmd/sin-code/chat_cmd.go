// SPDX-License-Identifier: MIT
// Purpose: `sin-code chat` — CLI binding for the C1-C5 packages
// (agentloop, session, verify, permission, mcpclient). Issue #44.
// REPL mode by default; headless one-shot via -p/--prompt with a stable
// JSON contract: {session_id, summary, verified, turns}.
package main

// sin-debt: shrink, upgrade: when a second chat-related command is added, merge into a shared file

import (
	"context"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// chat hook variables — injected by coverage tests to avoid real I/O, network
// or LLM calls. Production defaults point to the real implementations.
var (
	chatLoadAgentFn                 = internal.LoadEffectiveAgent
	chatNewLLMClientFn              = llm.NewClient
	chatNewProviderCompletionFn     = agentloop.NewProviderCompletion
	chatNewProviderCompletionFullFn = agentloop.NewProviderCompletionFull
	chatNewProviderCompletionStreamFn = agentloop.NewProviderCompletionStream
	chatRulesForAgentFn             = internal.RulesForAgent
	chatGetwdFn                     = os.Getwd
	chatNewHooksFn                  = hooks.New
	chatLoadMCPConfigsFn            = mcpclient.LoadConfigs
	chatNewMCPManagerFn             = mcpclient.NewManager
	chatMCPConnectAllFn             = func(mgr *mcpclient.Manager, ctx context.Context) error { return mgr.ConnectAll(ctx) }
	chatOpenSessionFn               = session.Open
	chatNewGateFn                   = verify.NewGate
	chatAskFn                       agentloop.AskFunc
	chatPrintResultFn               = printResult
	chatRunOverrideFn               func(context.Context, *session.Session, string) (*agentloop.Result, error)
	chatStdout                      io.Writer = os.Stdout
	chatStderr                      io.Writer = os.Stderr
	chatStdin                       io.Reader = os.Stdin
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
	// noSandbox is an explicit escape hatch that disables OS-level
	// isolation for `sin_bash` regardless of --sandbox or the headless
	// default. Headless mode (M3/M4, issue #420) defaults to sandbox=ON;
	// --no-sandbox prints a WARN to stderr and is intended for
	// debugging only. Issue #420.
	noSandbox bool
	// autolevel flips the chat loop into auto-classification mode:
	// if --mode is empty AND --autolevel is set, the chat reads
	// `opts.prompt` through `internal/autolevel.Classify` to pick
	// `plan | acceptEdits | bypass | default` automatically. The
	// classifier is deterministic (regex + substring, no LLM) so
	// M3 is honoured: every mode pick is operator-visible.
	autolevel          bool
	lazyTools          bool
	semanticTools      bool
	fusionOnVerifyFail bool
	fusionProviders    string
	fusionMaxCost      float64
	thinkingEnabled    bool
	thinkingBudget     int
	noTUI              bool
	watch              string

	contextCompaction     string
	compactionTrigger     string
	compactionMaxTokens   int
	contextWindow         int
	preserveEvidence      bool
	compactionRecentTurns int
	repetitionThreshold   int
	repetitionWindow      int

	// progress output controls (headless mode only). Default off.
	progress     string
	progressDest string
	progressFile string
	setup        bool
	verbose      bool

	// autoCommit creates a git commit after the verification gate
	// passes (issue #487). M3: only after verification, never before.
	autoCommit   bool
	commitPrefix string
	// agentMode selects a specialized sub-agent mode (issue #485):
	// default | architect | debug | code | review. Empty = default
	// (non-breaking). Restricted modes filter the tool set and prepend
	// a mode-specific system prompt. M4: mode filtering is additive to
	// the permission engine.
	agentMode string
	// checkpoint auto-creates a git-based workspace checkpoint before
	// the agent loop starts (issue #483). M3: the checkpoint is a
	// safety net, not a verification signal. M4: create is non-
	// destructive (git tag only) so no permission gate is needed.
	checkpoint bool
	// voice enables voice-to-code input in the REPL (issue #481).
	// When set, the user can press Ctrl+M to record and transcribe
	// voice input. Requires sox/ffmpeg + whisper on PATH.
	voice bool
}

func NewChatCmd() *cobra.Command {
	opts := &chatOptions{}
	cmd := &cobra.Command{
		Use:           "chat",
		Short:         "Run the SIN-Code agent loop (interactive REPL or headless one-shot)",
		SilenceUsage:  true,
		SilenceErrors: true,
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
  sin-code chat --auto-commit              auto-commit after the verification gate passes (issue #487, M3)
  sin-code chat --checkpoint               auto-create a git checkpoint before the agent loop starts (issue #483)
  sin-code chat --agent-mode <mode>        specialized sub-agent mode: architect|debug|code|review (issue #485)
  sin-code chat --sandbox <backend>      landlock|seatbelt|bubblewrap|none (issue #199)
  sin-code chat --autolevel              prompt-intent based permission auto-classifier (issue #198)
  sin-code chat --lazy-tools             lazy tool loading via tool_search (issue #270)
  sin-code chat --semantic-tools        use offline semantic retrieval for tool_search (issue #364)
  sin-code chat --fusion-on-verify-fail  enable SIN Fusion verify-tournament on verify.fail (issue #290)
  sin-code chat --fusion-providers <list> override Fireworks models for the tournament (comma-separated)
  sin-code chat --fusion-max-cost <usd>   USD kill-switch per tournament invocation (default 5.0)
  sin-code chat --thinking-enabled       send thinking{type:"enabled"} on each request (per-provider reasoning budget)
  sin-code chat --thinking-budget <n>    per-request thinking.budget_tokens cap (0 = unbounded / provider default)
  Oracle-mode fusion is experimental; set fusion.oracle_mode=true via config. Prefer PoC mode for verifiable tasks.

Post-edit automation (issue #376, opt-in via ~/.config/sin/sin-code.toml):
  agentloop.auto_lint=true   after every sin_write/sin_edit to a .go file: run gofmt -l + go vet (read-only — advisory)
  agentloop.auto_test=true   after every sin_write/sin_edit to a *_test.go file: run go test -race -count=1 on the file's package (may mutate state — advisory)
  Both default off. Set agentloop.auto_lint=true to auto-lint after edits. Both keys are advisory: warnings only, never block.

First-run setup:
  sin-code chat --setup               interactive onboarding wizard (provider, API key, model)
  sin-code config init --setup         same wizard via the config subcommand`,
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
	f.StringVar(&opts.sandbox, "sandbox", "", "sandbox backend: landlock|seatbelt|bubblewrap|none (default: platform-native, on by default in headless mode per M3/M4)")
	f.BoolVar(&opts.noSandbox, "no-sandbox", false, "disable OS-level sandbox for sin_bash (escape hatch; headless mode defaults to ON, issue #420, debugging only)")
	f.BoolVar(&opts.autolevel, "autolevel", false, "auto-classify permission mode from prompt intent (issue #198)")
	f.BoolVar(&opts.lazyTools, "lazy-tools", false, "enable lazy tool loading: send only tool_search meta-tool instead of all tools (issue #270)")
	f.BoolVar(&opts.semanticTools, "semantic-tools", false, "use offline semantic retrieval for tool_search instead of keyword matching (issue #364)")
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
	f.IntVar(&opts.repetitionThreshold, "repetition-threshold", 0, "observer-loop detection: number of repetitions before aborting (0 = disabled, issue #377)")
	f.IntVar(&opts.repetitionWindow, "repetition-window", 0, "observer-loop detection: window size for sequence detection (0 = default 1)")
	f.StringVar(&opts.progress, "progress", "", "structured progress output: off|json (default from config, fallback off)")
	f.StringVar(&opts.progressDest, "progress-dest", "stderr", "progress destination: stderr|stdout|file")
	f.StringVar(&opts.progressFile, "progress-file", "", "progress file path when --progress-dest=file")
	f.BoolVar(&opts.setup, "setup", false, "run interactive onboarding wizard to configure LLM backend")
	f.BoolVarP(&opts.verbose, "verbose", "v", false, "show warnings (MCP, sandbox) and diagnostic info in headless mode")
	f.BoolVar(&opts.autoCommit, "auto-commit", false, "auto-commit after the verification gate passes (issue #487, M3)")
	f.StringVar(&opts.commitPrefix, "commit-prefix", "", "conventional commit prefix for --auto-commit (e.g. feat, fix, docs; default: auto-detect)")
	f.BoolVar(&opts.checkpoint, "checkpoint", false, "auto-create a git checkpoint before the agent loop starts (issue #483)")
	f.StringVar(&opts.agentMode, "agent-mode", "", "specialized sub-agent mode: default|architect|debug|code|review (issue #485)")
	f.BoolVar(&opts.voice, "voice", false, "enable voice-to-code input in the REPL (Ctrl+M to record, issue #481)")
	return cmd
}
