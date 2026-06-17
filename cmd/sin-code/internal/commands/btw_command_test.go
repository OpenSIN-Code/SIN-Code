// SPDX-License-Identifier: MIT
// Purpose: /btw built-in command tests (issue #276).
package commands

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

type fakeSideLLM struct {
	resp   string
	err    error
	calls  atomic.Int64
	delay  time.Duration
	mu     sync.Mutex
	gotSys string
	gotUsr string
}

func (f *fakeSideLLM) Complete(ctx context.Context, system, user string) (string, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.gotSys = system
	f.gotUsr = user
	f.mu.Unlock()
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.err != nil {
		return "", f.err
	}
	return f.resp, nil
}

func (f *fakeSideLLM) lastUser() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotUsr
}

func (f *fakeSideLLM) lastSystem() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotSys
}

func newTestSession(t *testing.T) *session.Session {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "btw.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	return sess
}

func TestBTWCommand_NameAndDescription(t *testing.T) {
	c := NewBTWCommand(nil, "")
	if c.Name() != "btw" {
		t.Errorf("Name: %q", c.Name())
	}
	if c.Description() != "Ask a side question without breaking context" {
		t.Errorf("Description: %q", c.Description())
	}
}

func TestBTWCommand_ValidQuestion(t *testing.T) {
	llm := &fakeSideLLM{resp: "Patrick Collison"}
	c := NewBTWCommand(llm, "")
	out, err := c.Execute(context.Background(), "who is the CEO of Stripe?", newTestSession(t))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(out, "BTW: ") {
		t.Errorf("expected BTW prefix, got %q", out)
	}
	if !strings.Contains(out, "Patrick Collison") {
		t.Errorf("expected answer in output, got %q", out)
	}
	if llm.lastUser() != "who is the CEO of Stripe?" {
		t.Errorf("user message passed through: %q", llm.lastUser())
	}
	if llm.lastSystem() != DefaultBtwSystemPrompt {
		t.Errorf("default system prompt used: %q", llm.lastSystem())
	}
}

func TestBTWCommand_EmptyArgsReturnsUsage(t *testing.T) {
	c := NewBTWCommand(&fakeSideLLM{}, "")
	out, err := c.Execute(context.Background(), "   ", newTestSession(t))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out, "usage:") {
		t.Errorf("expected usage hint, got %q", out)
	}
}

func TestBTWCommand_NoLLMClient(t *testing.T) {
	c := NewBTWCommand(nil, "")
	_, err := c.Execute(context.Background(), "any question", newTestSession(t))
	if err == nil {
		t.Fatal("expected error when no LLM client")
	}
	var e ErrNoLLM
	if !errors.As(err, &e) {
		t.Errorf("expected ErrNoLLM, got %T: %v", err, err)
	}
	if e.CommandName != "btw" {
		t.Errorf("CommandName: %q", e.CommandName)
	}
}

func TestBTWCommand_DoesNotModifyHistory(t *testing.T) {
	sess := newTestSession(t)
	msgs := []session.Message{
		{Role: "user", Content: "implement auth"},
		{Role: "assistant", Content: "working on it"},
	}
	if err := sess.SaveHistory(msgs); err != nil {
		t.Fatalf("save: %v", err)
	}
	before := sess.History()
	c := NewBTWCommand(&fakeSideLLM{resp: "answer"}, "")
	if _, err := c.Execute(context.Background(), "side q", sess); err != nil {
		t.Fatalf("err: %v", err)
	}
	after := sess.History()
	if len(after) != len(before) {
		t.Errorf("history length changed: before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		if !reflect.DeepEqual(before[i], after[i]) {
			t.Errorf("history[%d] changed: before=%+v after=%+v", i, before[i], after[i])
		}
	}
}

func TestBTWCommand_ContextCancellation(t *testing.T) {
	llm := &fakeSideLLM{resp: "late", delay: 200 * time.Millisecond}
	c := NewBTWCommand(llm, "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := c.Execute(ctx, "question", newTestSession(t))
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestBTWCommand_LongQuestionTruncation(t *testing.T) {
	llm := &fakeSideLLM{resp: "ok"}
	c := NewBTWCommand(llm, "")
	c.maxLen = 10
	long := strings.Repeat("x", 500)
	_, err := c.Execute(context.Background(), long, newTestSession(t))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(llm.lastUser()) != 10 {
		t.Errorf("expected truncation to 10 runes, got %d", len(llm.lastUser()))
	}
}

func TestBTWCommand_ConcurrentExecution(t *testing.T) {
	llm := &fakeSideLLM{resp: "ok"}
	c := NewBTWCommand(llm, "")
	c.maxPerSess = 0
	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := c.Execute(context.Background(), "q", newTestSession(t))
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent err: %v", err)
	}
	if got := llm.calls.Load(); got != n {
		t.Errorf("LLM call count: want %d, got %d", n, got)
	}
}

func TestBTWCommand_PerSessionLimit(t *testing.T) {
	llm := &fakeSideLLM{resp: "ok"}
	c := NewBTWCommand(llm, "")
	c.maxPerSess = 2
	sess := newTestSession(t)
	for i := 0; i < 2; i++ {
		if _, err := c.Execute(context.Background(), "q", sess); err != nil {
			t.Fatalf("call %d err: %v", i, err)
		}
	}
	if _, err := c.Execute(context.Background(), "q", sess); err == nil {
		t.Fatal("expected limit error on 3rd call")
	}
}

func TestBTWCommand_CustomSystemPrompt(t *testing.T) {
	llm := &fakeSideLLM{resp: "ok"}
	c := NewBTWCommand(llm, "custom system prompt")
	if _, err := c.Execute(context.Background(), "q", newTestSession(t)); err != nil {
		t.Fatalf("err: %v", err)
	}
	if llm.lastSystem() != "custom system prompt" {
		t.Errorf("custom system prompt: %q", llm.lastSystem())
	}
}
