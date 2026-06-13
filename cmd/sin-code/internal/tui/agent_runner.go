// SPDX-License-Identifier: MIT
// Purpose: AgentRunner — async wrapper around agentloop.Loop that emits
// AgentEvents on a channel for the TUI render loop to consume. Builds
// the fully wired loop via loopbuilder.Build so all mandates
// (M2/M3/M4/M7) apply uniformly with `sin-code chat`.
//
// Issue #53: TUI v3.3.1 — embed the agent loop directly. The TUI used
// to print "sin-code mcp call ..." hints; the AgentRunner now actually
// runs the loop and streams turn/tool/verify/done/ask events back so
// the palette entries (websearch, browser, scheduler, ...) execute
// live.
package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// EventKind enumerates the discrete signals the runner emits.
type EventKind int

const (
	EventTurn   EventKind = iota + 1
	EventTool
	EventVerify
	EventDone
	EventError
	EventAsk
)

// String returns a stable lower-case identifier.
func (k EventKind) String() string {
	switch k {
	case EventTurn:
		return "turn"
	case EventTool:
		return "tool"
	case EventVerify:
		return "verify"
	case EventDone:
		return "done"
	case EventError:
		return "error"
	case EventAsk:
		return "ask"
	default:
		return fmt.Sprintf("event-%d", int(k))
	}
}

// AgentEvent is a single message from the runner to the TUI.
type AgentEvent struct {
	Kind     EventKind
	Detail   string
	Result   string
	ToolName string
	Err      error
	AskReply chan bool
}

// Config bundles the construction-time knobs the TUI passes when
// building a runner. Zero values are sensible defaults.
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
	AutoApprove bool
	AskTimeout  time.Duration
	ToolFactory func(*mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec)
	SkipMCP     bool
}

// AgentRunner owns one configured loop + session.
type AgentRunner struct {
	cfg       Config
	loop      *agentloop.Loop
	cleanup   func() error
	store     *session.Store
	sess      *session.Session
	lessons   *lessons.Store
	Events    chan AgentEvent
	inflight  atomic.Bool
	closeOnce sync.Once
	closed    chan struct{}
	askMu     sync.Mutex
	askReply  chan bool
}

// ErrBusy is returned by Submit when a prompt is already running.
var ErrBusy = errors.New("agentrunner: prompt already in flight")

// ErrClosed is returned by Submit/Close when the runner has been closed.
var ErrClosed = errors.New("agentrunner: closed")

const (
	askBuffer          = 1
	defaultAskTimeout  = 30 * time.Second
)

// NewAgentRunner builds a runner with a fully wired loop, opens the
// session store, and starts a new resumable session.
func NewAgentRunner(ctx context.Context, cfg Config) (*AgentRunner, error) {
	if cfg.Workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("agentrunner: resolve workspace: %w", err)
		}
		cfg.Workspace = wd
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 80
	}
	if cfg.AskTimeout == 0 {
		cfg.AskTimeout = defaultAskTimeout
	}
	dbPath := filepath.Join(cfg.Workspace, ".sin-code", "sessions.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("agentrunner: mkdir sessions: %w", err)
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("agentrunner: open sessions: %w", err)
	}
	sess, err := store.StartOrResume(cfg.SessionID)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("agentrunner: start session: %w", err)
	}
	lessonPath := filepath.Join(cfg.Workspace, ".sin-code", "lessons.db")
	memStore, _ := lessons.Open(lessonPath) //nolint:errcheck

	runner := &AgentRunner{
		cfg:     cfg,
		store:   store,
		sess:    sess,
		lessons: memStore,
		Events:  make(chan AgentEvent, 64),
		closed:  make(chan struct{}),
	}
	loop, cleanup, err := loopbuilder.Build(ctx, loopbuilder.Config{
		Workspace:   cfg.Workspace,
		SessionID:   sess.ID,
		AgentName:   cfg.AgentName,
		Model:       cfg.Model,
		BaseURL:     cfg.BaseURL,
		MaxTurns:    cfg.MaxTurns,
		VerifyMode:  cfg.VerifyMode,
		VerifyCmd:   cfg.VerifyCmd,
		Yolo:        cfg.Yolo,
		Headless:    cfg.Headless,
		ToolFactory: cfg.ToolFactory,
		SkipMCP:     cfg.SkipMCP,
	}, memStore)
	if err != nil {
		_ = store.Close()
		if memStore != nil {
			_ = memStore.Close()
		}
		return nil, fmt.Errorf("agentrunner: build loop: %w", err)
	}
	loop.Ask = runner.bridgeAsk
	runner.loop = loop
	runner.cleanup = cleanup
	return runner, nil
}

// bridgeAsk is the agentloop.AskFunc installed on the loop. It
// packages the tool call into an AgentEvent and parks the goroutine
// on a reply channel. A timeout (default 30s, configurable) falls
// back to cfg.AutoApprove; if AutoApprove is false and no reply
// arrives, the tool is denied (M4: ask→deny in headless unless
// --yolo).
func (r *AgentRunner) bridgeAsk(tc agentloop.ToolCall) bool {
	reply := make(chan bool, askBuffer)
	r.askMu.Lock()
	r.askReply = reply
	r.askMu.Unlock()
	select {
	case r.Events <- AgentEvent{
		Kind:     EventAsk,
		Detail:   fmt.Sprintf("permission required: tool %q with args %v", tc.Name, tc.Args),
		ToolName: tc.Name,
		AskReply: reply,
	}:
	case <-r.closed:
		return false
	}
	if r.cfg.AskTimeout < 0 {
		return <-reply
	}
	timer := time.NewTimer(r.cfg.AskTimeout)
	defer timer.Stop()
	select {
	case ok := <-reply:
		return ok
	case <-timer.C:
		return r.cfg.AutoApprove
	case <-r.closed:
		return false
	}
}

// AnswerAsk is the public entry point for the TUI to reply to an ask.
func (r *AgentRunner) AnswerAsk(allow bool) {
	r.askMu.Lock()
	ch := r.askReply
	r.askReply = nil
	r.askMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- allow:
	default:
	}
}

// Submit runs a single prompt on the loop asynchronously.
func (r *AgentRunner) Submit(ctx context.Context, prompt string) (<-chan struct{}, error) {
	select {
	case <-r.closed:
		return nil, ErrClosed
	default:
	}
	if !r.inflight.CompareAndSwap(false, true) {
		return nil, ErrBusy
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer r.inflight.Store(false)
		r.runOnce(ctx, prompt)
	}()
	return done, nil
}

// SubmitSync runs a prompt to completion on the calling goroutine.
func (r *AgentRunner) SubmitSync(ctx context.Context, prompt string) (*agentloop.Result, error) {
	select {
	case <-r.closed:
		return nil, ErrClosed
	default:
	}
	if !r.inflight.CompareAndSwap(false, true) {
		return nil, ErrBusy
	}
	defer r.inflight.Store(false)
	return r.loop.Run(ctx, r.sess, prompt)
}

func (r *AgentRunner) runOnce(ctx context.Context, prompt string) {
	r.emit(ctx, EventTurn, fmt.Sprintf("turn start: %q", truncate(prompt, 80)), "", "", nil)
	res, err := r.loop.Run(ctx, r.sess, prompt)
	r.emitSessionHistory(ctx)
	if err != nil {
		r.emit(ctx, EventError, err.Error(), "", "", err)
		return
	}
	r.emit(ctx, EventDone, "verified", res.Summary, "", nil)
}

func (r *AgentRunner) emit(ctx context.Context, kind EventKind, detail, result, toolName string, err error) {
	ev := AgentEvent{
		Kind:     kind,
		Detail:   detail,
		Result:   result,
		ToolName: toolName,
		Err:      err,
	}
	select {
	case r.Events <- ev:
	case <-r.closed:
	case <-ctx.Done():
	}
}

// Close releases all resources. Safe to call multiple times.
func (r *AgentRunner) Close() error {
	var firstErr error
	r.closeOnce.Do(func() {
		close(r.closed)
		r.AnswerAsk(false)
		if r.cleanup != nil {
			if err := r.cleanup(); err != nil {
				firstErr = err
			}
		}
		if r.store != nil {
			if err := r.store.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if r.lessons != nil {
			_ = r.lessons.Close()
		}
	})
	return firstErr
}

// SessionID returns the session ID the runner is operating on.
func (r *AgentRunner) SessionID() string {
	if r.sess == nil {
		return ""
	}
	return r.sess.ID
}

// EventsChannel returns the event channel for embedding into a tea.Cmd.
func (r *AgentRunner) EventsChannel() <-chan AgentEvent { return r.Events }

// CompletionFunc is the loop's Completion signature, exposed as a
// named type for callers that want to plug in a custom LLM provider.
type CompletionFunc func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error)

// SetCompletion overrides the loop's Completion function.
func (r *AgentRunner) SetCompletion(c CompletionFunc) {
	if r.loop == nil {
		return
	}
	r.loop.Completion = func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return c(ctx, history, tools)
	}
}

// Loop exposes the underlying loop for advanced use (tests).
func (r *AgentRunner) Loop() *agentloop.Loop { return r.loop }

// emitSessionHistory replays the session's post-run message log and
// synthesizes per-tool and per-verify AgentEvents.
func (r *AgentRunner) emitSessionHistory(ctx context.Context) {
	if r.sess == nil {
		return
	}
	msgs := r.sess.History()
	type pending struct {
		name string
	}
	stack := []pending{}
	for _, m := range msgs {
		switch m.Role {
		case "assistant":
			calls := parseToolCalls(m.ToolCalls)
			for _, c := range calls {
				stack = append(stack, pending{name: c})
				r.emit(ctx, EventTool, "tool: "+c, "", c, nil)
			}
		case "tool":
			name := ""
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				name = top.name
			}
			detail := "tool result"
			if name != "" {
				detail = "tool result: " + name
			}
			if m.Content != "" && len(m.Content) <= 80 {
				detail += " — " + m.Content
			}
			r.emit(ctx, EventTool, detail, "", name, nil)
		case "user":
			switch {
			case strings.HasPrefix(m.Content, "VERIFICATION PASSED"):
				r.emit(ctx, EventVerify, m.Content, strings.TrimPrefix(m.Content, "VERIFICATION PASSED "), "", nil)
			case strings.HasPrefix(m.Content, "VERIFICATION FAILED"):
				r.emit(ctx, EventVerify, m.Content, strings.TrimPrefix(m.Content, "VERIFICATION FAILED "), "", nil)
			case strings.HasPrefix(m.Content, "VERIFICATION BLOCKED"):
				r.emit(ctx, EventVerify, m.Content, strings.TrimPrefix(m.Content, "VERIFICATION BLOCKED — "), "", nil)
			}
		}
	}
}

// parseToolCalls extracts tool names from a JSON-encoded tool_calls
// payload (OpenAI-compatible: array of {function: {name}} objects).
func parseToolCalls(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var calls []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil
	}
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		if c.Function.Name != "" {
			out = append(out, c.Function.Name)
		}
	}
	return out
}

// truncate clips s to at most n bytes, appending an ellipsis.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
