// SPDX-License-Identifier: MIT
// Purpose: tests for the AgentRunner (issue #53). Covers kind routing,
// ask interaction via channel, headless mode, race-clean event
// streaming, and busy/closed safety.
package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
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
	r.Loop().LocalTool = func(ctx context.Context, name string, args map[string]any) (string, error) { return "should-not-reach", nil }
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
	hold := make(chan struct{})
	r.SetCompletion(func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		<-hold
		return &agentloop.Completion{Text: "x", Raw: session.Message{Role: "assistant", Content: "x"}}, nil
	})
	_, err := r.Submit(context.Background(), "first")
	if err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	_, err = r.SubmitSync(context.Background(), "second")
	if !errors.Is(err, ErrBusy) {
		t.Errorf("SubmitSync err = %v, want ErrBusy", err)
	}
	close(hold)
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
	ws := filepath.Join(t.TempDir(), "fresh", "workspace")
	r := newTestRunner(t, Config{Workspace: ws})
	if _, err := os.Stat(filepath.Join(ws, ".sin-code")); err != nil {
		t.Errorf("expected .sin-code dir under workspace, got: %v", err)
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

func TestNewAgentRunnerErrorPaths(t *testing.T) {
	ws := t.TempDir()
	fakeFile := filepath.Join(ws, "fake")
	if err := os.WriteFile(fakeFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := NewAgentRunner(context.Background(), Config{Workspace: fakeFile})
	if err == nil {
		t.Error("expected error when workspace is a file")
	}
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
