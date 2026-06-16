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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife/autoactivate"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/isolation"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
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
  sin-code chat --worktree <name>        run inside a git worktree (issue #194 part 2)`,
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
	return cmd
}

func runChat(ctx context.Context, opts *chatOptions) error {
	headless := opts.prompt != ""

	var agentCfg orchestrator.AgentConfig
	if opts.agent != "" {
		cfg, _, err := internal.LoadEffectiveAgent(opts.agent)
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
	client := llm.NewClient(baseURL, apiKey)
	completion := agentloop.NewProviderCompletion(client, model, agentCfg.MaxTokens, agentCfg.Temperature)

	perm := permission.New(internal.RulesForAgent(agentCfg))
	perm.Yolo = opts.yolo
	perm.Headless = headless
	if opts.mode != "" {
		if err := perm.SetMode(permission.Mode(opts.mode)); err != nil {
			return fmt.Errorf("chat: --mode: %w", err)
		}
	}

	workspace, err := os.Getwd()
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
	hookEngine := hooks.New(loadHooks(workspace))

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
	mcpMgr := mcpclient.NewManager(mcpclient.LoadConfigs(workspace))
	if err := mcpMgr.ConnectAll(ctx); err != nil {
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
	gate := verify.NewGate(mode, runner, runner)

	dbPath := opts.dbPath
	if dbPath == "" {
		dbPath = session.DefaultPath()
	}
	store, err := session.Open(dbPath)
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
		fmt.Fprintln(os.Stderr, "[autoactivate] session start rules:\n"+d.Message)
	}
	// Apply the CLI `--activate` list now that the hook has wired the
	// state for this session id.
	for _, name := range act.Rules {
		act.Act.Activate(sess.ID, autoactivate.Rule{Name: name})
	}

	var ask agentloop.AskFunc
	if !headless {
		ask = terminalAsk
	}

	loop := &agentloop.Loop{
		Gate:       gate,
		LocalTool:  combinedTool(workspace, mcpMgr),
		LocalSpec:  combinedSpecs(mcpMgr),
		Workspace:  workspace,
		MaxTurns:   opts.maxTurns,
		SessionID:  sess.ID,
		Completion: completion,
		Hooks:      hookEngine,
		Perm:       perm,
		Ask:        ask,
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
			fmt.Fprintln(os.Stderr, "[autoactivate] per-turn rules:\n"+pd.Message)
		}
	}

	if headless {
		dispatchUserPrompt(opts.prompt)
		res, err := loop.Run(ctx, sess, opts.prompt)
		if err != nil {
			act.Act.EndSession(sess.ID)
			return err
		}
		act.Act.EndSession(sess.ID)
		return printResult(res, opts.jsonOut)
	}

	fmt.Printf("sin-code chat — session %s (verify=%s).", sess.ID, gate.Mode())
	if st, ok := act.Act.Snapshot(sess.ID); ok && len(st.ActiveRules.Names()) > 0 {
		fmt.Printf(" Active rules: %s", strings.Join(st.ActiveRules.Names(), ", "))
	}
	fmt.Println(" Type 'exit' to quit.")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for {
		fmt.Print("> ")
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
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		_ = printResult(res, opts.jsonOut)
	}
	act.Act.EndSession(sess.ID)
	return scanner.Err()
}

func printResult(res *agentloop.Result, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	fmt.Println(res.Summary)
	fmt.Printf("[session=%s verified=%v turns=%d]\n", res.SessionID, res.Verified, res.Turns)
	return nil
}

func terminalAsk(tc agentloop.ToolCall) bool {
	fmt.Printf("Permission required: tool %q with args %v — allow? [y/N] ", tc.Name, tc.Args)
	reader := bufio.NewReader(os.Stdin)
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

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
