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

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
	"github.com/spf13/cobra"
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
  sin-code chat --yolo                   bypass 'ask' permissions (M4)`,
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

	workspace, err := os.Getwd()
	if err != nil {
		return err
	}
	hookEngine := hooks.New(loadHooks(workspace))

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

	var ask agentloop.AskFunc
	if !headless {
		ask = terminalAsk
	}

	loop := &agentloop.Loop{
		Gate:       gate,
		LocalTool:  combinedTool(mcpMgr),
		LocalSpec:  combinedSpecs(mcpMgr),
		Workspace:  workspace,
		MaxTurns:   opts.maxTurns,
		SessionID:  sess.ID,
		Completion: completion,
		Hooks:      hookEngine,
		Perm:       perm,
		Ask:        ask,
	}

	if headless {
		res, err := loop.Run(ctx, sess, opts.prompt)
		if err != nil {
			return err
		}
		return printResult(res, opts.jsonOut)
	}

	fmt.Printf("sin-code chat — session %s (verify=%s). Type 'exit' to quit.\n", sess.ID, gate.Mode())
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
		res, err := loop.Run(ctx, sess, line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		_ = printResult(res, opts.jsonOut)
	}
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
