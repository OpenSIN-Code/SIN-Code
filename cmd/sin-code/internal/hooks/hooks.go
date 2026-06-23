// SPDX-License-Identifier: MIT
// Purpose: SIN-Code hook engine: deterministic, user-configured automation
// fired at lifecycle events of the agent loop. Hooks are NOT LLM-decided —
// they always run when their event fires (mandate C7, AGENTS.md §8).
//
// Hook types:
//
//	command — shell command; event JSON on stdin; exit 0 = continue,
//	          exit 2 = BLOCK (stdout fed back to the agent), else warn.
//	webhook — HTTP POST of the event JSON (fire-and-forget unless blocking).
//	prompt  — injects static text into the next agent turn.
//	listener — programmatic Go PostListener (issue #376); fires only on
//	           tool.post and merges its return into PromptInjects.
//
// Only *.pre events and permission.ask honor blocking.
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

const (
	SessionStart  = "session.start"
	SessionResume = "session.resume"
	SessionEnd    = "session.end"

	TurnStart = "turn.start"
	TurnEnd   = "turn.end"

	ToolPre    = "tool.pre"
	ToolPost   = "tool.post"
	ToolDenied = "tool.denied"
	ToolError  = "tool.error"

	PermissionAsk = "permission.ask"

	VerifyPre  = "verify.pre"
	VerifyPass = "verify.pass"
	VerifyFail = "verify.fail"

	// Stop-gate: completion authority decoupled from the worker. StopEval
	// fires whenever the gate is consulted; StopContinue fires when the gate
	// rejects a proposed completion and forces the loop to keep working.
	StopEval     = "stop.eval"
	StopContinue = "stop.continue"
	// StopStalled fires when the stop-gate returns identical open criteria
	// StallThreshold turns in a row (no-progress escalation).
	StopStalled = "stop.stalled"
	// Token budget lifecycle (issue #151).
	BudgetWarn      = "budget.warn"
	BudgetExhausted = "budget.exhausted"
	// BudgetExceeded fires when a single LLM turn crosses the
	// per-turn cap (issue #375). Distinct from BudgetExhausted which
	// fires once per run when the cumulative cap is hit. Per-turn
	// is the granular signal; per-run is the gross one.
	BudgetExceeded = "budget.exceeded"
	// ReflectIssues fires when the self-reflection pass finds problems the
	// worker must fix before completion is evaluated.
	ReflectIssues = "reflect.issues"

	AgentSpawn       = "agent.spawn"
	AgentComplete    = "agent.complete"
	CriticReject     = "critic.reject"
	AdversaryFinding = "adversary.finding"
	GovernorBlock    = "governor.block"

	MemoryWrite   = "memory.write"
	MemoryCompact = "memory.compact"
	MemoryPrime   = "memory.prime"

	CommitPre  = "commit.pre"
	CommitPost = "commit.post"
	PushPre    = "push.pre"

	TaskComplete  = "task.complete"
	TaskAbort     = "task.abort"
	CompactionPre = "compaction.pre"

	// Autonomy lifecycle (daemon mode).
	GoalEnqueued  = "goal.enqueued"
	GoalStarted   = "goal.started"
	GoalVerified  = "goal.verified"
	GoalExhausted = "goal.exhausted"
	TriggerFired  = "trigger.fired"

	// Skill lifecycle.
	SkillInstalled = "skill.installed"
	SkillFailed    = "skill.failed"

	// Fusion lifecycle (issue #290).
	FusionDispatch = "fusion.dispatch"

	// LoopDetected fires when the LoopDetector refuses dispatch
	// because the worker entered a repeated tool-call sequence
	// (issue #377). The data payload carries pattern_length,
	// repeats and tool name so telemetry / UI / audit pipelines can
	// surface the stall. Not blockable — the loop has already been
	// broken by the dispatch site.
	LoopDetected = "loop.detected"
)

// blockable events: a blocking hook result is honored only for these.
var blockable = map[string]bool{
	ToolPre: true, VerifyPre: true, PermissionAsk: true,
	CommitPre: true, PushPre: true, CompactionPre: true,
	GoalStarted: true,
}

type Hook struct {
	Event   string `json:"event"`
	Matcher string `json:"matcher,omitempty"`
	Type    string `json:"type"`
	Command string `json:"command,omitempty"`
	URL     string `json:"url,omitempty"`
	Text    string `json:"text,omitempty"`
	Timeout int    `json:"timeout_seconds,omitempty"`
}

type Payload struct {
	Event     string         `json:"event"`
	SessionID string         `json:"session_id"`
	Workspace string         `json:"workspace"`
	Name      string         `json:"name,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp string         `json:"timestamp"`
}

type Result struct {
	Blocked       bool
	BlockReason   string
	PromptInjects []string
}

type Engine struct {
	hooks         []Hook
	client        *http.Client
	postListeners []PostListener
}

// PostListener is a programmatic listener registered via
// Engine.RegisterPostListener. It fires on tool.post events only and
// returns messages that are merged into Result.PromptInjects (and
// emitted to stderr for human visibility). It can never block — the
// tool.post event intentionally does not honor blocking so post-write
// automation stays advisory. Listeners MUST return promptly (≤ a few
// seconds) and MUST NOT panic; both are enforced by convention here,
// and callers should wrap long work in their own context with timeout.
//
// Use case: auto-lint, auto-test, auto-format, post-edit notes (issue
// #376). They are opt-in via config (agentloop.auto_lint,
// agentloop.auto_test) so legacy callers see no behavior change.
type PostListener func(ctx context.Context, p Payload) []string

func New(hooks []Hook) *Engine {
	return &Engine{hooks: hooks, client: &http.Client{Timeout: 15 * time.Second}}
}

// RegisterPostListener appends a programmatic listener that fires on
// every tool.post event. Multiple listeners are invoked in the order
// they were registered; their return values are concatenated into
// Result.PromptInjects. Calling RegisterPostListener on a nil Engine
// is a no-op (defence-in-depth for tests that skip Engine.New).
func (e *Engine) RegisterPostListener(fn PostListener) {
	if e == nil || fn == nil {
		return
	}
	e.postListeners = append(e.postListeners, fn)
}

// Fire runs all hooks matching the event (and matcher) sequentially.
// Hooks never crash the agent: errors degrade to warnings on stderr.
//
// For tool.post, registered PostListeners (programmatic Go funcs, not
// user-config files) also fire in registration order. Their return is
// merged into PromptInjects so the agent sees lint/test feedback in
// the next turn. PostListeners never block — tool.post is not in the
// blockable set by design.
func (e *Engine) Fire(ctx context.Context, p Payload) Result {
	p.Timestamp = time.Now().UTC().Format(time.RFC3339)
	var res Result
	for _, h := range e.hooks {
		if !match(h.Event, p.Event) || !matchName(h.Matcher, p.Name) {
			continue
		}
		switch h.Type {
		case "prompt":
			res.PromptInjects = append(res.PromptInjects, h.Text)
		case "webhook":
			e.fireWebhook(ctx, h, p)
		case "command":
			blocked, reason := e.fireCommand(ctx, h, p)
			if blocked && blockable[p.Event] {
				res.Blocked = true
				res.BlockReason = reason
				return res
			}
		default:
			fmt.Fprintf(os.Stderr, "warn: hook with unknown type %q ignored\n", h.Type)
		}
	}
	if p.Event == ToolPost {
		for _, fn := range e.postListeners {
			msgs := safeInvokePostListener(fn, ctx, p)
			if len(msgs) > 0 {
				res.PromptInjects = append(res.PromptInjects, msgs...)
			}
		}
	}
	return res
}

func (e *Engine) fireCommand(ctx context.Context, h Hook, p Payload) (blocked bool, reason string) {
	timeout := time.Duration(h.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, _ := json.Marshal(p)
	cmd := exec.CommandContext(cctx, "sh", "-c", h.Command)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Dir = p.Workspace
	cmd.Env = append(os.Environ(),
		"SIN_HOOK_EVENT="+p.Event,
		"SIN_HOOK_TOOL_NAME="+p.Name,
		"SIN_SESSION_ID="+p.SessionID,
	)
	if path, ok := p.Data["path"].(string); ok {
		cmd.Env = append(cmd.Env, "SIN_HOOK_DATA_PATH="+path)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	err := cmd.Run()
	if err == nil {
		return false, ""
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 2 {
		return true, strings.TrimSpace(stdout.String())
	}
	fmt.Fprintf(os.Stderr, "warn: hook %q on %s failed: %v (%s)\n",
		h.Command, p.Event, err, strings.TrimSpace(stderr.String()))
	return false, ""
}

func (e *Engine) fireWebhook(ctx context.Context, h Hook, p Payload) {
	payload, _ := json.Marshal(p)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: webhook hook %s on %s failed: %v\n", h.URL, p.Event, err)
		return
	}
	_ = resp.Body.Close()
}

func match(pattern, event string) bool {
	ok, _ := path.Match(pattern, event)
	return ok || pattern == event
}

func matchName(pattern, name string) bool {
	if pattern == "" {
		return true
	}
	ok, _ := path.Match(strings.ToLower(pattern), strings.ToLower(name))
	return ok
}

// safeInvokePostListener calls a registered PostListener, recovering
// from any panic so one misbehaving listener cannot crash the agent
// loop. Listeners are advisory — silence on panic is the correct
// policy because the tool's primary work already succeeded.
func safeInvokePostListener(fn PostListener, ctx context.Context, p Payload) (out []string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "warn: post-listener panicked on %s: %v\n", p.Name, r)
			out = nil
		}
	}()
	if fn == nil {
		return nil
	}
	return fn(ctx, p)
}
