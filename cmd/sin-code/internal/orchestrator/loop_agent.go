// SPDX-License-Identifier: MIT
// Purpose: LoopAgent — real LLM-backed agent using agentloop.Loop as its
// execution engine. Unlike LLMAgent (single chat call), LoopAgent runs a
// full PLAN→ACT→VERIFY→DONE loop with tool-use, isolated sessions, per-
// agent system prompt, per-agent verify gate (mandate M3), and cost
// tracking. Issue #287 Phase 1.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// LoopAgent is a real LLM-backed agent that uses agentloop.Loop as its
// execution engine. Each Run() creates an isolated session, loads the
// agent's system prompt, and runs the full tool-use loop with verify
// gate and cost tracking. Issue #287.
type LoopAgent struct {
	cfg       AgentConfig
	client    *llm.Client
	sessions  *session.Store
	gate      *verify.Gate
	hooks     *hooks.Engine
	perm      *permission.Engine
	localTool agentloop.LocalToolFunc
	localSpec []agentloop.ToolSpec
	maxTurns  int
	workspace string

	sessionOnce      sync.Once
	sessionErr       error
	preWarmedPrompt  string
	promptMu         sync.RWMutex
}

// LoopAgentOption configures a LoopAgent at construction time.
type LoopAgentOption func(*LoopAgent)

func WithSessionStore(s *session.Store) LoopAgentOption {
	return func(a *LoopAgent) { a.sessions = s }
}

func WithVerifyGate(g *verify.Gate) LoopAgentOption {
	return func(a *LoopAgent) { a.gate = g }
}

func WithHookEngine(h *hooks.Engine) LoopAgentOption {
	return func(a *LoopAgent) { a.hooks = h }
}

func WithTools(tool agentloop.LocalToolFunc, spec []agentloop.ToolSpec) LoopAgentOption {
	return func(a *LoopAgent) { a.localTool = tool; a.localSpec = spec }
}

func WithMaxTurns(n int) LoopAgentOption {
	return func(a *LoopAgent) { a.maxTurns = n }
}

func WithWorkspace(ws string) LoopAgentOption {
	return func(a *LoopAgent) { a.workspace = ws }
}

// NewLoopAgent creates a LoopAgent with the given config and LLM client.
// Options may override the session store, verify gate, hooks, tools,
// max turns, and workspace. Defaults are sensible for autonomous
// orchestrator use: verify mode "off", empty hooks, permission from
// config, 40 max turns, cwd workspace.
func NewLoopAgent(cfg AgentConfig, client *llm.Client, opts ...LoopAgentOption) *LoopAgent {
	a := &LoopAgent{cfg: cfg, client: client}
	for _, opt := range opts {
		opt(a)
	}
	if a.gate == nil {
		a.gate = verify.NewGate("off", nil, nil)
	}
	if a.hooks == nil {
		a.hooks = hooks.New(nil)
	}
	if a.perm == nil {
		a.perm = buildPermFromConfig(cfg)
	}
	if a.maxTurns == 0 {
		a.maxTurns = 40
	}
	if a.workspace == "" {
		a.workspace, _ = os.Getwd()
	}
	return a
}

// buildPermFromConfig creates a permission engine from the agent config's
// ToolsAllow (allow) and ToolsDeny (deny) lists. Unlisted tools default
// to Ask, which the loop resolves via the AskFunc (autonomous: always
// allow, but Deny is never bypassed).
func buildPermFromConfig(cfg AgentConfig) *permission.Engine {
	var rules []permission.Rule
	for _, t := range cfg.ToolsAllow {
		rules = append(rules, permission.Rule{Tool: t, Policy: "allow"})
	}
	for _, t := range cfg.ToolsDeny {
		rules = append(rules, permission.Rule{Tool: t, Policy: "deny"})
	}
	perm := permission.New(rules)
	perm.Yolo = true
	return perm
}

func (a *LoopAgent) Name() string        { return a.cfg.Name }
func (a *LoopAgent) Config() AgentConfig { return a.cfg }

// Run executes the task through a full agentloop.Loop with isolated
// session, system prompt, verify gate, and cost tracking. The result
// summary and usage info are written to the scratchpad.
func (a *LoopAgent) Run(ctx context.Context, task *Task, scratch *Scratchpad) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("agent %s: no LLM client (missing API key?)", a.cfg.Name)
	}

	systemPrompt, err := loadLoopSystemPromptHook(a.cfg)
	if err != nil {
		return "", fmt.Errorf("load system prompt: %w", err)
	}

	// Use pre-warmed prompt if available (issue #285 integration).
	a.promptMu.RLock()
	preWarmed := a.preWarmedPrompt
	a.promptMu.RUnlock()
	if preWarmed != "" {
		systemPrompt = preWarmed
	}

	priorInputs, _ := scratch.Read("inputs")
	var priorOutputs []string
	for k, v := range scratch.ReadAll() {
		if strings.HasPrefix(k, "outputs:") {
			priorOutputs = append(priorOutputs, fmt.Sprintf("[%s]\n%s", k, v.Content))
		}
	}

	userPrompt := buildLoopUserPrompt(task, priorInputs, priorOutputs)

	if primeCtx, perr := a.primeContext(task); perr == nil && primeCtx != "" {
		userPrompt += "\n\n## Relevant Project Memory\n" + primeCtx
	}

	scratch.Write(a.cfg.Name, "inputs", task.Description)

	model := a.cfg.Model
	if model != "" {
		model = llm.ResolveModel(model)
	}
	if model == "" {
		providerName := a.cfg.Provider
		if providerName == "" {
			providerName = inferProviderFromEnv()
		}
		if providerName == "" {
			providerName = "nim"
		}
		if prov, perr := llm.LookupProvider(providerName); perr == nil {
			model = prov.DefaultModel
		}
	}
	if model == "" {
		return "", fmt.Errorf("agent %s: no model configured", a.cfg.Name)
	}

	sessions, err := a.ensureSessions()
	if err != nil {
		return "", fmt.Errorf("session store: %w", err)
	}

	sess, err := sessions.StartOrResume("")
	if err != nil {
		return "", fmt.Errorf("start isolated session: %w", err)
	}

	maxTokens := a.cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	completion := agentloop.NewProviderCompletion(a.client, model, maxTokens, a.cfg.Temperature)

	loop := &agentloop.Loop{
		Gate:         a.gate,
		LocalTool:    a.localTool,
		LocalSpec:    a.localSpec,
		Workspace:    a.workspace,
		MaxTurns:     a.maxTurns,
		SessionID:    sess.ID,
		SystemPrompt: systemPrompt,
		Completion:   completion,
		Hooks:        a.hooks,
		Perm:         a.perm,
		Ask:          func(tc agentloop.ToolCall) bool { return true },
		MaxTokens:    a.cfg.MaxContext,
	}

	start := time.Now()
	result, err := loop.Run(ctx, sess, userPrompt)
	duration := time.Since(start)

	if err != nil {
		scratch.Write(a.cfg.Name, "error:"+task.ID, err.Error())
		return "", err
	}

	scratch.Write(a.cfg.Name, "outputs:"+task.ID, result.Summary)
	scratch.Write(a.cfg.Name, "usage:"+task.ID, fmt.Sprintf(
		"tokens=%d turns=%d verified=%v duration=%s model=%s",
		result.Tokens, result.Turns, result.Verified, duration.Round(time.Millisecond), model,
	))

	return result.Summary, nil
}

// ensureSessions lazily opens a session store on first call. Race-safe
// via sync.Once (mandate M7). Tests inject a store via WithSessionStore.
func (a *LoopAgent) ensureSessions() (*session.Store, error) {
	a.sessionOnce.Do(func() {
		if a.sessions != nil {
			return
		}
		a.sessions, a.sessionErr = sessionOpenHook(session.DefaultPath())
	})
	return a.sessions, a.sessionErr
}

// sessionOpenHook is a test seam for the session store factory.
var sessionOpenHook = func(path string) (*session.Store, error) {
	return session.Open(path)
}

// loadLoopSystemPromptHook loads the system prompt for a LoopAgent from
// the config's SystemFile, falling back to a generated default. Tests
// override it to simulate prompt-loading failures.
var loadLoopSystemPromptHook = func(cfg AgentConfig) (string, error) {
	if cfg.SystemFile == "" {
		return defaultLoopSystemPrompt(cfg), nil
	}
	candidates := []string{
		cfg.SystemFile,
		filepath.Join(".", cfg.SystemFile),
		filepath.Join(os.Getenv("HOME"), ".config", "sin-code", cfg.SystemFile),
	}
	if env := os.Getenv("SIN_AGENTS_DIR"); env != "" {
		candidates = append(candidates, filepath.Join(env, cfg.SystemFile))
	}
	for _, p := range candidates {
		if p != "" {
			data, err := os.ReadFile(p)
			if err == nil {
				return string(data), nil
			}
		}
	}
	return defaultLoopSystemPrompt(cfg), nil
}

func defaultLoopSystemPrompt(cfg AgentConfig) string {
	return fmt.Sprintf("You are %s, a specialized agent.\n\nType: %s\nDescription: %s\n\nRespond concisely and accurately. Use the available scratchpad context to inform your answer.",
		cfg.Name, cfg.Type, cfg.Description)
}

func buildLoopUserPrompt(task *Task, priorInputs string, priorOutputs []string) string {
	var b strings.Builder
	b.WriteString("## Task\n")
	fmt.Fprintf(&b, "ID: %s\n", task.ID)
	fmt.Fprintf(&b, "Type: %s\n", task.Type)
	fmt.Fprintf(&b, "Description: %s\n", task.Description)
	if task.AgentName != "" {
		fmt.Fprintf(&b, "Assigned Agent: %s\n", task.AgentName)
	}
	if task.ExpectedOutput != "" {
		fmt.Fprintf(&b, "Expected Output: %s\n", task.ExpectedOutput)
	}
	if priorInputs != "" {
		b.WriteString("\n## Prior Context (from scratchpad)\n")
		b.WriteString(priorInputs)
	}
	if len(priorOutputs) > 0 {
		b.WriteString("\n## Prior Outputs (from scratchpad)\n")
		b.WriteString(strings.Join(priorOutputs, "\n\n"))
	}
	return b.String()
}

func (a *LoopAgent) primeContext(task *Task) (string, error) {
	store, err := memoryOpenHook("")
	if err != nil {
		return "", err
	}
	defer store.Close()
	return store.Prime(task.Description, "", 5)
}

// PreWarm loads the system prompt and opens the session store without
// making an LLM call. This implements the PreWarmer interface (issue
// #285) so the PreWarmManager can pre-warm LoopAgent instances before
// their dependencies complete, reducing cold-start latency.
func (a *LoopAgent) PreWarm(ctx context.Context, task *Task) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	prompt, err := loadLoopSystemPromptHook(a.cfg)
	if err != nil {
		return err
	}

	a.promptMu.Lock()
	a.preWarmedPrompt = prompt
	a.promptMu.Unlock()

	_, err = a.ensureSessions()
	return err
}
