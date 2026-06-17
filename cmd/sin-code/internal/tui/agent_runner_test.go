// SPDX-License-Identifier: MIT
// Purpose: tests for the AgentRunner (issue #53). Covers kind routing,
// ask interaction via channel, headless mode, race-clean event
// streaming, and busy/closed safety.
package tui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// newTestRunner builds a runner inside a fresh temp workspace. By
// default AskTimeout=-1 (block forever) so tests reply on AskReply.
func newTestRunner(t *testing.T, cfg Config) *AgentRunner {
	t.Helper()
	if cfg.Workspace == "" {
		cfg.Workspace = t.TempDir()
	}
	if cfg.AskTimeout == 0 {
		cfg.AskTimeout = -1
	}
	cfg.SkipMCP = true
	r, err := NewAgentRunner(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewAgentRunner: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// stubCompletion returns a Completion func that yields the supplied
// sequence in order, then a "done" completion (no tool calls) so the
// loop terminates.
func stubCompletion(seq ...*agentloop.Completion) CompletionFunc {
	var idx atomic.Int32
	return func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		i := int(idx.Add(1) - 1)
		if i < len(seq) {
			return seq[i], nil
		}
		return &agentloop.Completion{
			Text: "done",
			Raw:  session.Message{Role: "assistant", Content: "done"},
		}, nil
	}
}

// toolCallJSON returns a JSON-encoded ToolCalls payload for a tool.
func toolCallJSON(name string) []byte {
	return []byte(`[{"function":{"name":"` + name + `"}}]`)
}

func findEvent(events []AgentEvent, kind EventKind) *AgentEvent {
	for i := range events {
		if events[i].Kind == kind {
			return &events[i]
		}
	}
	return nil
}

func collectEvents(r *AgentRunner, dur time.Duration) []AgentEvent {
	out := []AgentEvent{}
	deadline := time.NewTimer(dur)
	defer deadline.Stop()
	for {
		select {
		case ev, ok := <-r.Events:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline.C:
			return out
		}
	}
}

// ── Kind routing ───────────────────────────────────────────────────

func TestEventKindString(t *testing.T) {
	cases := map[EventKind]string{
		EventTurn:   "turn",
		EventTool:   "tool",
		EventVerify: "verify",
		EventDone:   "done",
		EventError:  "error",
		EventAsk:    "ask",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("EventKind(%d).String() = %q, want %q", int(k), got, want)
		}
	}
	if got := EventKind(0).String(); !strings.HasPrefix(got, "event-") {
		t.Errorf("unknown kind String() = %q, want event-*", got)
	}
	if got := EventKind(99).String(); !strings.HasPrefix(got, "event-") {
		t.Errorf("unknown kind String() = %q, want event-*", got)
	}
}

func TestSubmitEmitsTurnAndDone(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	r.SetCompletion(stubCompletion(
		&agentloop.Completion{Text: "all good", Raw: session.Message{Role: "assistant", Content: "all good"}},
	))
	done, err := r.Submit(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Submit did not complete in 2s")
	}
	events := collectEvents(r, 500*time.Millisecond)
	if findEvent(events, EventTurn) == nil {
		t.Errorf("missing EventTurn, got %d events", len(events))
	}
	doneEv := findEvent(events, EventDone)
	if doneEv == nil {
		t.Fatalf("missing EventDone, got %d events: %+v", len(events), events)
	}
	if doneEv.Result != "all good" {
		t.Errorf("EventDone.Result = %q, want %q", doneEv.Result, "all good")
	}
}

// ── Ask interaction via channel ────────────────────────────────────

func TestAskAllowViaChannel(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir(), AskTimeout: -1})
	r.SetCompletion(stubCompletion(
		&agentloop.Completion{
			Text:      "",
			ToolCalls: []agentloop.ToolCall{{ID: "t1", Name: "sin_bash", Args: map[string]any{"command": "echo hi"}}},
			Raw:       session.Message{Role: "assistant", Content: "", ToolCalls: toolCallJSON("sin_bash")},
		},
		&agentloop.Completion{Text: "done after ask", Raw: session.Message{Role: "assistant", Content: "done after ask"}},
	))
	r.Loop().LocalTool = func(ctx context.Context, name string, args map[string]any) (string, error) { return "tool-out", nil }
	r.Loop().LocalSpec = []agentloop.ToolSpec{{Name: "sin_bash", Description: "stub"}}

	done, err := r.Submit(context.Background(), "do the thing")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case ev := <-r.Events:
			if ev.Kind == EventAsk {
				if ev.AskReply == nil {
					t.Error("EventAsk missing AskReply channel")
				}
				if ev.ToolName != "sin_bash" {
					t.Errorf("EventAsk ToolName = %q, want %q", ev.ToolName, "sin_bash")
				}
				r.AnswerAsk(true)
				goto saw
			}
		case <-deadline.C:
			t.Fatal("did not receive EventAsk within 2s")
		}
	}
saw:
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Submit did not complete in 2s")
	}
}

func TestAskDenyViaChannel(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir(), AskTimeout: -1})
	r.SetCompletion(stubCompletion(
		&agentloop.Completion{
			Text:      "",
			ToolCalls: []agentloop.ToolCall{{ID: "t1", Name: "sin_bash", Args: map[string]any{"command": "rm -rf /"}}},
			Raw:       session.Message{Role: "assistant", Content: "", ToolCalls: toolCallJSON("sin_bash")},
		},
		&agentloop.Completion{Text: "denied and done", Raw: session.Message{Role: "assistant", Content: "denied and done"}},
	))
	r.Loop().LocalTool = func(ctx context.Context, name string, args map[string]any) (string, error) {
		return "should-not-reach", nil
	}
	r.Loop().LocalSpec = []agentloop.ToolSpec{{Name: "sin_bash", Description: "stub"}}

	done, err := r.Submit(context.Background(), "deny please")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case ev := <-r.Events:
			if ev.Kind == EventAsk {
				r.AnswerAsk(false)
				goto saw
			}
		case <-deadline.C:
			t.Fatal("did not receive EventAsk within 2s")
		}
	}
saw:
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Submit did not complete in 2s")
	}
	events := collectEvents(r, 500*time.Millisecond)
	var sawDenied bool
	for _, ev := range events {
		if ev.Kind == EventTool && strings.Contains(ev.Detail, "DENIED by user") {
			sawDenied = true
		}
	}
	if !sawDenied {
		t.Errorf("expected a DENIED-by-user tool event; got %+v", events)
	}
}

func TestAskTimeoutAutoDenies(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir(), AskTimeout: 50 * time.Millisecond, AutoApprove: false})
	r.SetCompletion(stubCompletion(
		&agentloop.Completion{
			Text:      "",
			ToolCalls: []agentloop.ToolCall{{ID: "t", Name: "sin_bash", Args: map[string]any{}}},
			Raw:       session.Message{Role: "assistant", Content: "", ToolCalls: toolCallJSON("sin_bash")},
		},
		&agentloop.Completion{Text: "after-timeout", Raw: session.Message{Role: "assistant", Content: "after-timeout"}},
	))
	r.Loop().LocalTool = func(ctx context.Context, name string, args map[string]any) (string, error) { return "out", nil }
	r.Loop().LocalSpec = []agentloop.ToolSpec{{Name: "sin_bash", Description: "stub"}}
	done, err := r.Submit(context.Background(), "x")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Submit did not complete in 3s")
	}
	events := collectEvents(r, 500*time.Millisecond)
	var sawAsk bool
	for _, ev := range events {
		if ev.Kind == EventAsk {
			sawAsk = true
		}
	}
	if !sawAsk {
		t.Errorf("expected EventAsk to fire; got %+v", events)
	}
}

func TestAskTimeoutAutoApproves(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir(), AskTimeout: 50 * time.Millisecond, AutoApprove: true})
	r.SetCompletion(stubCompletion(
		&agentloop.Completion{
			Text:      "",
			ToolCalls: []agentloop.ToolCall{{ID: "t", Name: "sin_bash", Args: map[string]any{}}},
			Raw:       session.Message{Role: "assistant", Content: "", ToolCalls: toolCallJSON("sin_bash")},
		},
		&agentloop.Completion{Text: "ok", Raw: session.Message{Role: "assistant", Content: "ok"}},
	))
	r.Loop().LocalTool = func(ctx context.Context, name string, args map[string]any) (string, error) { return "out", nil }
	r.Loop().LocalSpec = []agentloop.ToolSpec{{Name: "sin_bash", Description: "stub"}}
	done, err := r.Submit(context.Background(), "x")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Submit did not complete in 3s")
	}
}

// ── Headless mode ──────────────────────────────────────────────────

func TestHeadlessNoAskEvent(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir(), Headless: true})
	r.SetCompletion(stubCompletion(
		&agentloop.Completion{Text: "no ask in headless", Raw: session.Message{Role: "assistant", Content: "no ask in headless"}},
	))
	done, err := r.Submit(context.Background(), "headless prompt")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Submit did not complete in 3s")
	}
	events := collectEvents(r, 500*time.Millisecond)
	for _, ev := range events {
		if ev.Kind == EventAsk {
			t.Errorf("headless mode emitted EventAsk: %+v", ev)
		}
	}
}

// ── Error path ─────────────────────────────────────────────────────

func TestSubmitErrorEmitsEventError(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	r.SetCompletion(func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil, errors.New("provider is down")
	})
	done, err := r.Submit(context.Background(), "fail me")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Submit did not complete in 3s")
	}
	events := collectEvents(r, 500*time.Millisecond)
	errEv := findEvent(events, EventError)
	if errEv == nil {
		t.Fatalf("missing EventError; got %+v", events)
	}
	if errEv.Err == nil || !strings.Contains(errEv.Err.Error(), "provider is down") {
		t.Errorf("EventError.Err = %v, want substring 'provider is down'", errEv.Err)
	}
}

// ── Busy / closed safety ───────────────────────────────────────────

func TestSubmitBusyWhenInFlight(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	hold := make(chan struct{})
	r.SetCompletion(func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		<-hold
		return &agentloop.Completion{Text: "released", Raw: session.Message{Role: "assistant", Content: "released"}}, nil
	})
	first, err := r.Submit(context.Background(), "first")
	if err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	_, err = r.Submit(context.Background(), "second")
	if !errors.Is(err, ErrBusy) {
		t.Errorf("second Submit err = %v, want ErrBusy", err)
	}
	close(hold)
	select {
	case <-first:
	case <-time.After(2 * time.Second):
		t.Fatal("first Submit never completed")
	}
}

func TestSubmitAfterCloseReturnsErrClosed(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := r.Submit(context.Background(), "x")
	if !errors.Is(err, ErrClosed) {
		t.Errorf("Submit after Close err = %v, want ErrClosed", err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	for i := 0; i < 3; i++ {
		if err := r.Close(); err != nil {
			t.Errorf("Close #%d: %v", i, err)
		}
	}
}

func TestAnswerAskNoOpWhenNoAskPending(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	r.AnswerAsk(true)
	r.AnswerAsk(false)
}

// ── Race-clean event streaming ─────────────────────────────────────

func TestEventsStreamRaceClean(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir(), AskTimeout: -1})
	seq := []*agentloop.Completion{
		{Text: "", ToolCalls: []agentloop.ToolCall{{ID: "t", Name: "sin_bash", Args: map[string]any{}}}, Raw: session.Message{Role: "assistant", Content: "", ToolCalls: toolCallJSON("sin_bash")}},
		{Text: "ok", Raw: session.Message{Role: "assistant", Content: "ok"}},
	}
	r.SetCompletion(stubCompletion(seq...))
	r.Loop().LocalTool = func(ctx context.Context, name string, args map[string]any) (string, error) { return "out", nil }
	r.Loop().LocalSpec = []agentloop.ToolSpec{{Name: "sin_bash", Description: "stub"}}
	var wg sync.WaitGroup
	var askCount, doneCount atomic.Int32
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			case ev, ok := <-r.Events:
				if !ok {
					return
				}
				switch ev.Kind {
				case EventAsk:
					askCount.Add(1)
					if ev.AskReply != nil {
						go func(ch chan bool) { ch <- true }(ev.AskReply)
					}
				case EventDone:
					doneCount.Add(1)
				}
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			done, err := r.Submit(context.Background(), "x")
			if err != nil && !errors.Is(err, ErrBusy) {
				t.Errorf("Submit #%d: %v", i, err)
				return
			}
			if done == nil {
				return
			}
			<-done
		}
		time.Sleep(50 * time.Millisecond)
		_ = r.Close()
		close(stop)
	}()
	wg.Wait()
	if askCount.Load() == 0 {
		t.Errorf("expected at least 1 ask event, got 0")
	}
	if doneCount.Load() == 0 {
		t.Errorf("expected at least 1 done event, got 0")
	}
}

// ── Session / loop plumbing ────────────────────────────────────────

func TestSessionIDStable(t *testing.T) {
	ws := t.TempDir()
	r := newTestRunner(t, Config{Workspace: ws})
	if r.SessionID() == "" {
		t.Fatal("SessionID empty after NewAgentRunner")
	}
}

func TestSubmitSyncReturnsResult(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	r.SetCompletion(func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return &agentloop.Completion{Text: "sync ok", Raw: session.Message{Role: "assistant", Content: "sync ok"}}, nil
	})
	res, err := r.SubmitSync(context.Background(), "sync me")
	if err != nil {
		t.Fatalf("SubmitSync: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	if res.Summary != "sync ok" {
		t.Errorf("res.Summary = %q, want %q", res.Summary, "sync ok")
	}
}

func TestSubmitSyncBusy(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	defer r.Close()
	hold := make(chan struct{})
	r.SetCompletion(func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		<-hold
		return &agentloop.Completion{Text: "x", Raw: session.Message{Role: "assistant", Content: "x"}}, nil
	})
	done, err := r.Submit(context.Background(), "first")
	if err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	_, err = r.SubmitSync(context.Background(), "second")
	if !errors.Is(err, ErrBusy) {
		t.Errorf("SubmitSync err = %v, want ErrBusy", err)
	}
	close(hold)
	<-done // wait for async submit to finish before cleanup
}

func TestLoopIsWiredFromBuilder(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	if r.Loop() == nil {
		t.Fatal("Loop() is nil")
	}
	if r.Loop().Ask == nil {
		t.Error("Loop.Ask not wired (should be bridgeAsk)")
	}
	stub := func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return &agentloop.Completion{Text: "x", Raw: session.Message{Role: "assistant", Content: "x"}}, nil
	}
	r.SetCompletion(stub)
	if r.Loop().Completion == nil {
		t.Error("Completion nil after SetCompletion")
	}
}

func TestNewAgentRunnerCreatesSinCodeDir(t *testing.T) {
	// Issue #62 / #265 — runtime DBs (.sin-code/sessions.db, lessons.db)
	// must NOT live inside the workspace; they live under
	// DBHome/workspaces/<sha256-prefix12(abs(ws))>/… instead. The
	// workspace itself must remain untouched.
	ws := filepath.Join(t.TempDir(), "fresh", "workspace")
	r := newTestRunner(t, Config{Workspace: ws})
	if _, err := os.Stat(filepath.Join(ws, ".sin-code")); err == nil {
		t.Error(".sin-code should NOT exist under workspace after migration")
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected stat err: %v", err)
	}
	if r.SessionID() == "" {
		t.Error("SessionID empty after fresh init")
	}
}

func TestEventsChannel(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	if r.EventsChannel() == nil {
		t.Fatal("EventsChannel() returned nil")
	}
	if r.EventsChannel() != r.Events {
		t.Error("EventsChannel() returns wrong channel")
	}
}

func TestSetCompletionNilLoop(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	_ = r.Close()
	r.SetCompletion(func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return &agentloop.Completion{Text: "x", Raw: session.Message{Role: "assistant", Content: "x"}}, nil
	})
}

// TestNewAgentRunner_UsesDBHomeWhenSet — explicit DBHome overrides
// UserConfigDir so tests can hermetic-creep each run (issue #62 / #265).
func TestNewAgentRunner_UsesDBHomeWhenSet(t *testing.T) {
	ws := t.TempDir()
	home := filepath.Join(t.TempDir(), "home")
	r, err := NewAgentRunner(context.Background(), Config{Workspace: ws, DBHome: home, SkipMCP: true})
	if err != nil {
		t.Fatalf("NewAgentRunner: %v", err)
	}
	defer r.Close()
	keyPath, err := filepath.Abs(ws)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(home, "workspaces", shortSHA256Helper(keyPath))
	wantSessions := filepath.Join(wantDir, "sessions.db")
	wantLessons := filepath.Join(wantDir, "lessons.db")
	if _, err := os.Stat(wantSessions); err != nil {
		t.Errorf("sessions.db should exist at %s; got err %v", wantSessions, err)
	}
	if _, err := os.Stat(wantLessons); err != nil {
		t.Errorf("lessons.db should exist at %s; got err %v", wantLessons, err)
	}
	// The workspace itself must remain untouched.
	if _, err := os.Stat(filepath.Join(ws, ".sin-code")); err == nil {
		t.Error(".sin-code must NOT exist under workspace")
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected stat err: %v", err)
	}
}

// TestNewAgentRunner_DefaultsAreUserScoped — when DBHome is empty,
// sessions.db / lessons.db land under UserConfigDir/sin-code/workspaces/<key>/…
// (never in the workspace). Heroines of issue #62 / #265: this is what
// prevents accidental commits.
func TestNewAgentRunner_DefaultsAreUserScoped(t *testing.T) {
	ws := t.TempDir()
	home := filepath.Join(t.TempDir(), "user-cfg")
	oldGetwd := osGetwd
	osGetwd = func() (string, error) { return ws, nil }
	defer func() { osGetwd = oldGetwd }()
	oldUserConfigDir := userConfigDir
	userConfigDir = func() (string, error) { return home, nil }
	defer func() { userConfigDir = oldUserConfigDir }()

	r, err := NewAgentRunner(context.Background(), Config{Workspace: "" /* defaults to ws */, SkipMCP: true})
	if err != nil {
		t.Fatalf("NewAgentRunner: %v", err)
	}
	defer r.Close()
	keyPath, _ := filepath.Abs(ws)
	wantSessions := filepath.Join(home, "sin-code", "workspaces", shortSHA256Helper(keyPath), "sessions.db")
	if _, err := os.Stat(wantSessions); err != nil {
		t.Errorf("sessions.db should exist at %s; got err %v", wantSessions, err)
	}
	// Workspace must remain untouched.
	if _, err := os.Stat(filepath.Join(ws, ".sin-code")); err == nil {
		t.Error(".sin-code must NOT exist under workspace")
	} else if !os.IsNotExist(err) {
		t.Errorf("unexpected stat err: %v", err)
	}
}

// shortSHA256Helper mirrors dbhome.shortSHA256 but inlined to avoid
// an internal-package-only path collision in the test binary.
func shortSHA256Helper(s string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(s)))
	return hex.EncodeToString(sum[:])[:12]
}

// ── parseToolCalls ─────────────────────────────────────────────────

func TestParseToolCalls(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		want []string
	}{
		{"empty", nil, nil},
		{"blank", []byte(""), nil},
		{"garbage", []byte("not json"), nil},
		{"single", []byte(`[{"function":{"name":"sin_read"}}]`), []string{"sin_read"}},
		{"multi", []byte(`[{"function":{"name":"a"}},{"function":{"name":"b"}}]`), []string{"a", "b"}},
		{"empty-name", []byte(`[{"function":{"name":""}}]`), nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseToolCalls(c.raw)
			if len(got) != len(c.want) {
				t.Fatalf("parseToolCalls(%q) = %v, want %v", c.raw, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("parseToolCalls(%q)[%d] = %q, want %q", c.raw, i, got[i], c.want[i])
				}
			}
		})
	}
}

// ── truncate ───────────────────────────────────────────────────────

func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"", 10, ""},
		{"abc", 0, ""},
		{"abc", -1, ""},
		{"abc", 10, "abc"},
		{"abcdef", 3, "abc…"},
		{"abcdef", 6, "abcdef"},
	}
	for _, c := range cases {
		got := truncate(c.in, c.n)
		if got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

// ── Construction error paths ───────────────────────────────────────

func TestNewAgentRunnerWorkspaceEmpty(t *testing.T) {
	tmp := t.TempDir()
	orig := osGetwd
	osGetwd = func() (string, error) { return tmp, nil }
	t.Cleanup(func() { osGetwd = orig })
	r, err := NewAgentRunner(context.Background(), Config{Workspace: "", SkipMCP: true})
	if err != nil {
		t.Fatalf("NewAgentRunner: %v", err)
	}
	if r.SessionID() == "" {
		t.Error("SessionID empty after default workspace fallback")
	}
	_ = r.Close()
}

func TestNewAgentRunnerWorkspaceGetwdError(t *testing.T) {
	orig := osGetwd
	osGetwd = func() (string, error) { return "", errors.New("getwd failed") }
	t.Cleanup(func() { osGetwd = orig })
	_, err := NewAgentRunner(context.Background(), Config{Workspace: "", SkipMCP: true})
	if err == nil || !strings.Contains(err.Error(), "resolve workspace") {
		t.Errorf("err = %v, want 'resolve workspace'", err)
	}
}

func TestNewAgentRunnerSessionOpenError(t *testing.T) {
	orig := sessionOpen
	sessionOpen = func(path string) (*session.Store, error) { return nil, errors.New("open failed") }
	t.Cleanup(func() { sessionOpen = orig })
	_, err := NewAgentRunner(context.Background(), Config{Workspace: t.TempDir(), SkipMCP: true})
	if err == nil || !strings.Contains(err.Error(), "open sessions") {
		t.Errorf("err = %v, want 'open sessions'", err)
	}
}

func TestNewAgentRunnerStartOrResumeError(t *testing.T) {
	orig := storeStartOrResume
	storeStartOrResume = func(s *session.Store, id string) (*session.Session, error) { return nil, errors.New("sor failed") }
	t.Cleanup(func() { storeStartOrResume = orig })
	_, err := NewAgentRunner(context.Background(), Config{Workspace: t.TempDir(), SkipMCP: true})
	if err == nil || !strings.Contains(err.Error(), "start session") {
		t.Errorf("err = %v, want 'start session'", err)
	}
}

func TestNewAgentRunnerLoopBuilderError(t *testing.T) {
	orig := loopbuilderBuild
	loopbuilderBuild = func(ctx context.Context, cfg loopbuilder.Config, memStore *lessons.Store) (*agentloop.Loop, func() error, error) {
		return nil, nil, errors.New("build failed")
	}
	t.Cleanup(func() { loopbuilderBuild = orig })
	_, err := NewAgentRunner(context.Background(), Config{Workspace: t.TempDir(), SkipMCP: true})
	if err == nil || !strings.Contains(err.Error(), "build loop") {
		t.Errorf("err = %v, want 'build loop'", err)
	}
}

// ── Ask edge cases ─────────────────────────────────────────────────

func TestBridgeAskClosedFirstSelect(t *testing.T) {
	r := &AgentRunner{cfg: Config{AskTimeout: -1}, Events: make(chan AgentEvent), closed: make(chan struct{})}
	close(r.closed)
	if got := r.bridgeAsk(agentloop.ToolCall{Name: "x"}); got != false {
		t.Errorf("bridgeAsk = %v, want false", got)
	}
}

func TestBridgeAskReplyViaChannel(t *testing.T) {
	r := &AgentRunner{cfg: Config{AskTimeout: 100 * time.Millisecond}, Events: make(chan AgentEvent, 1), closed: make(chan struct{})}
	done := make(chan bool, 1)
	go func() { done <- r.bridgeAsk(agentloop.ToolCall{Name: "x"}) }()
	time.Sleep(20 * time.Millisecond)
	r.askMu.Lock()
	ch := r.askReply
	r.askMu.Unlock()
	if ch == nil {
		t.Fatal("askReply not set")
	}
	ch <- true
	select {
	case got := <-done:
		if !got {
			t.Errorf("bridgeAsk = %v, want true", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bridgeAsk did not return")
	}
}

func TestBridgeAskClosedSecondSelect(t *testing.T) {
	r := &AgentRunner{cfg: Config{AskTimeout: 10 * time.Second}, Events: make(chan AgentEvent, 1), closed: make(chan struct{})}
	done := make(chan bool, 1)
	go func() { done <- r.bridgeAsk(agentloop.ToolCall{Name: "x"}) }()
	time.Sleep(20 * time.Millisecond)
	close(r.closed)
	select {
	case got := <-done:
		if got {
			t.Errorf("bridgeAsk = %v, want false", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("bridgeAsk did not return")
	}
}

func TestAnswerAskDefaultWhenBlocked(t *testing.T) {
	r := &AgentRunner{askReply: make(chan bool)} // unbuffered, no receiver
	r.AnswerAsk(true)                            // should hit default case, not block
}

func TestSubmitSyncAfterCloseReturnsErrClosed(t *testing.T) {
	r := newTestRunner(t, Config{Workspace: t.TempDir()})
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_, err := r.SubmitSync(context.Background(), "x")
	if !errors.Is(err, ErrClosed) {
		t.Errorf("SubmitSync err = %v, want ErrClosed", err)
	}
}

// ── Emit edge cases ────────────────────────────────────────────────

func TestEmitClosedAndContextDone(t *testing.T) {
	t.Run("closed", func(t *testing.T) {
		r := &AgentRunner{Events: make(chan AgentEvent), closed: make(chan struct{})}
		close(r.closed)
		r.emit(context.Background(), EventTurn, "detail", "", "", nil)
	})
	t.Run("ctx done", func(t *testing.T) {
		r := &AgentRunner{Events: make(chan AgentEvent)} // unbuffered, no receiver
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r.emit(ctx, EventTurn, "detail", "", "", nil)
	})
}

// ── Nil-safe helpers ───────────────────────────────────────────────

func TestSessionIDNil(t *testing.T) {
	r := &AgentRunner{}
	if got := r.SessionID(); got != "" {
		t.Errorf("SessionID() = %q, want empty", got)
	}
}

func TestSetCompletionNilLoopGuard(t *testing.T) {
	r := &AgentRunner{}
	r.SetCompletion(func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil, nil
	})
}

func TestEmitSessionHistoryNilSession(t *testing.T) {
	r := &AgentRunner{}
	r.emitSessionHistory(context.Background())
}

// ── Close error propagation ────────────────────────────────────────

func TestCloseCleanupError(t *testing.T) {
	r, err := NewAgentRunner(context.Background(), Config{Workspace: t.TempDir(), SkipMCP: true})
	if err != nil {
		t.Fatalf("NewAgentRunner: %v", err)
	}
	r.cleanup = func() error { return errors.New("cleanup failed") }
	if err := r.Close(); err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Errorf("Close err = %v, want 'cleanup failed'", err)
	}
}

func TestCloseStoreCloseError(t *testing.T) {
	orig := storeClose
	defer func() { storeClose = orig }()
	storeClose = func(s *session.Store) error { return errors.New("store close failed") }
	r, err := NewAgentRunner(context.Background(), Config{Workspace: t.TempDir(), SkipMCP: true})
	if err != nil {
		t.Fatalf("NewAgentRunner: %v", err)
	}
	r.cleanup = nil // ensure firstErr is available for the store error
	if err := r.Close(); err == nil || !strings.Contains(err.Error(), "store close failed") {
		t.Errorf("Close err = %v, want 'store close failed'", err)
	}
}

// sessionWithHistory creates a real session seeded with the supplied messages.
// The caller is responsible for closing the returned store when done.
func sessionWithHistory(t *testing.T, msgs []session.Message) (*session.Store, *session.Session) {
	t.Helper()
	ws := t.TempDir()
	store, err := session.Open(filepath.Join(ws, "sessions.db"))
	if err != nil {
		t.Fatalf("session.Open: %v", err)
	}
	sess, err := store.StartOrResume("")
	if err != nil {
		_ = store.Close()
		t.Fatalf("StartOrResume: %v", err)
	}
	if err := sess.SaveHistory(msgs); err != nil {
		_ = store.Close()
		t.Fatalf("SaveHistory: %v", err)
	}
	return store, sess
}

func TestEmitSessionHistoryBranches(t *testing.T) {
	store, sess := sessionWithHistory(t, []session.Message{
		{Role: "tool", Content: "orphan tool result with no assistant call"},
		{Role: "tool", Content: strings.Repeat("x", 100)},
		{Role: "user", Content: "VERIFICATION PASSED summary text"},
		{Role: "user", Content: "VERIFICATION FAILED another summary"},
		{Role: "user", Content: "VERIFICATION BLOCKED — permission denied"},
	})
	defer store.Close()

	r := &AgentRunner{Events: make(chan AgentEvent, 64)}
	r.sess = sess
	r.emitSessionHistory(context.Background())

	events := collectEvents(r, 500*time.Millisecond)
	var orphanResult, longContent, passed, failed, blocked bool
	for _, ev := range events {
		if ev.Kind == EventTool && ev.Detail == "tool result" {
			orphanResult = true
		}
		if ev.Kind == EventTool && ev.Detail == "tool result" && ev.ToolName == "" {
			// long content branch also emits "tool result" with empty name
			longContent = true
		}
		if ev.Kind == EventVerify && ev.Result == "summary text" {
			passed = true
		}
		if ev.Kind == EventVerify && ev.Result == "another summary" {
			failed = true
		}
		if ev.Kind == EventVerify && ev.Result == "permission denied" {
			blocked = true
		}
	}
	if !orphanResult {
		t.Errorf("expected orphan tool result event, got %+v", events)
	}
	if !longContent {
		t.Errorf("expected long-content tool result event, got %+v", events)
	}
	if !passed {
		t.Errorf("expected VERIFICATION PASSED event, got %+v", events)
	}
	if !failed {
		t.Errorf("expected VERIFICATION FAILED event, got %+v", events)
	}
	if !blocked {
		t.Errorf("expected VERIFICATION BLOCKED event, got %+v", events)
	}
}
