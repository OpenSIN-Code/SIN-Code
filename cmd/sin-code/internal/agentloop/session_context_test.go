// SPDX-License-Identifier: MIT
// Purpose: unit tests for SessionContextBuilder (issue #379).
package agentloop

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type mockLessonsReader struct {
	items []string
	err   error
}

func (m *mockLessonsReader) Recent(n int) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	if n >= len(m.items) {
		return m.items, nil
	}
	return m.items[:n], nil
}

type mockMemoryReader struct {
	items []string
	err   error
}

func (m *mockMemoryReader) Query(q string, n int) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	if n >= len(m.items) {
		return m.items, nil
	}
	return m.items[:n], nil
}

type mockGoalReader struct {
	items []string
	err   error
}

func (m *mockGoalReader) Active() ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.items, nil
}

type mockTodoReader struct {
	items []string
	err   error
}

func (m *mockTodoReader) Open(blockedOnly bool) ([]string, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.items, nil
}

type mockSessionSummaryReader struct {
	summary string
	err     error
}

func (m *mockSessionSummaryReader) Summary(sessionID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.summary, nil
}

type mockAutoMemoryReader struct {
	data []byte
	err  error
}

func (m *mockAutoMemoryReader) IndexBytes() ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.data, nil
}

func TestSessionContextBuilder_Build_AllSources(t *testing.T) {
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"always run tests", "prefer sin_edit"}},
		&mockMemoryReader{items: []string{"user likes terse output", "project uses Go"}},
		&mockGoalReader{items: []string{"fix issue #375", "ship v3.23"}},
		&mockTodoReader{items: []string{"todo #1: write tests", "todo #2: fix bug"}},
		&mockSessionSummaryReader{summary: "Previous session: fixed login bug"},
		&mockAutoMemoryReader{data: []byte("## Project Context\n- uses Go\n- uses SQLite")},
	)
	out, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, want := range []string{
		"## Session Summary",
		"## Auto Memory",
		"## Lessons",
		"## Memories",
		"## Goals",
		"## Open Todos",
		"always run tests",
		"user likes terse output",
		"fix issue #375",
		"todo #1: write tests",
		"Previous session: fixed login bug",
		"uses Go",
		"uses SQLite",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("Build output missing %q:\n%s", want, out)
		}
	}
}

func TestSessionContextBuilder_Build_NilStores(t *testing.T) {
	b := NewSessionContextBuilder(nil, nil, nil, nil, nil, nil)
	out, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output for all-nil stores, got %q", out)
	}
}

func TestSessionContextBuilder_Build_LessonsError(t *testing.T) {
	want := errors.New("lessons down")
	b := NewSessionContextBuilder(
		&mockLessonsReader{err: want},
		&mockMemoryReader{items: []string{"x"}},
		&mockGoalReader{items: []string{"y"}},
		&mockTodoReader{items: []string{"z"}},
		&mockSessionSummaryReader{summary: "s"},
		&mockAutoMemoryReader{data: []byte("m")},
	)
	if _, err := b.Build(context.Background()); err != want {
		t.Fatalf("expected lessons error, got %v", err)
	}
}

func TestSessionContextBuilder_Build_MemoryError(t *testing.T) {
	want := errors.New("memory down")
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"ok"}},
		&mockMemoryReader{err: want},
		&mockGoalReader{items: []string{"y"}},
		&mockTodoReader{items: []string{"z"}},
		&mockSessionSummaryReader{summary: "s"},
		&mockAutoMemoryReader{data: []byte("m")},
	)
	if _, err := b.Build(context.Background()); err != want {
		t.Fatalf("expected memory error, got %v", err)
	}
}

func TestSessionContextBuilder_Build_GoalError(t *testing.T) {
	want := errors.New("goals down")
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"ok"}},
		&mockMemoryReader{items: []string{"x"}},
		&mockGoalReader{err: want},
		&mockTodoReader{items: []string{"z"}},
		&mockSessionSummaryReader{summary: "s"},
		&mockAutoMemoryReader{data: []byte("m")},
	)
	if _, err := b.Build(context.Background()); err != want {
		t.Fatalf("expected goal error, got %v", err)
	}
}

func TestSessionContextBuilder_Build_TodoError(t *testing.T) {
	want := errors.New("todos down")
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"ok"}},
		&mockMemoryReader{items: []string{"x"}},
		&mockGoalReader{items: []string{"y"}},
		&mockTodoReader{err: want},
		&mockSessionSummaryReader{summary: "s"},
		&mockAutoMemoryReader{data: []byte("m")},
	)
	if _, err := b.Build(context.Background()); err != want {
		t.Fatalf("expected todo error, got %v", err)
	}
}

func TestSessionContextBuilder_Build_SessionError(t *testing.T) {
	want := errors.New("session down")
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"ok"}},
		&mockMemoryReader{items: []string{"x"}},
		&mockGoalReader{items: []string{"y"}},
		&mockTodoReader{items: []string{"z"}},
		&mockSessionSummaryReader{err: want},
		&mockAutoMemoryReader{data: []byte("m")},
	)
	if _, err := b.Build(context.Background()); err != want {
		t.Fatalf("expected session error, got %v", err)
	}
}

func TestSessionContextBuilder_Build_AutoMemoryError(t *testing.T) {
	want := errors.New("automem down")
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"ok"}},
		&mockMemoryReader{items: []string{"x"}},
		&mockGoalReader{items: []string{"y"}},
		&mockTodoReader{items: []string{"z"}},
		&mockSessionSummaryReader{summary: "s"},
		&mockAutoMemoryReader{err: want},
	)
	if _, err := b.Build(context.Background()); err != want {
		t.Fatalf("expected auto memory error, got %v", err)
	}
}

func TestSessionContextBuilder_Build_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"ok"}},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if _, err := b.Build(ctx); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSessionContextBuilder_Build_Truncation(t *testing.T) {
	longItem := strings.Repeat("x", 3000)
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{longItem}},
		nil,
		nil,
		nil,
		nil,
		nil,
	).WithConfig(SessionContextConfig{
		MaxPreambleChars: 100,
		IncludeLessons:   true,
	})
	out, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(out, "[...truncated]") {
		t.Fatalf("truncation marker missing: %s", out)
	}
	// The truncation happens after Format which adds headers, so total length
	// includes header. Just verify it's roughly in the expected ballpark.
	if len(out) < 100 || len(out) > 200 {
		t.Fatalf("output length %d unexpected, want ~100-200: %s", len(out), out)
	}
}

func TestSessionContextBuilder_Build_ConfigDisabled(t *testing.T) {
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"should not appear"}},
		nil,
		nil,
		nil,
		nil,
		nil,
	).WithConfig(SessionContextConfig{
		IncludeLessons: false,
	})
	out, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.Contains(out, "should not appear") {
		t.Fatalf("disabled source should not appear: %s", out)
	}
	if out != "" {
		t.Fatalf("expected empty output with all sources disabled: %q", out)
	}
}

func TestSessionContextBuilder_Format(t *testing.T) {
	b := NewSessionContextBuilder(nil, nil, nil, nil, nil, nil)
	out := b.Format(
		[]string{"l1", "l2"},
		[]string{"m1"},
		[]string{"g1", "g2", "g3"},
		[]string{"t1"},
		[]string{"session summary"},
		[]string{"auto mem"},
	)
	for _, want := range []string{
		"## Session Summary",
		"## Auto Memory",
		"## Lessons",
		"## Memories",
		"## Goals",
		"## Open Todos",
		"- l1",
		"- l2",
		"- m1",
		"- g1",
		"- g2",
		"- g3",
		"- t1",
		"- session summary",
		"- auto mem",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("Format output missing %q:\n%s", want, out)
		}
	}
}

func TestSessionContextBuilder_Format_Empty(t *testing.T) {
	b := NewSessionContextBuilder(nil, nil, nil, nil, nil, nil)
	if got := b.Format(nil, nil, nil, nil, nil, nil); got != "" {
		t.Fatalf("Format(nil,...) = %q, want empty", got)
	}
	if got := b.Format([]string{}, []string{}, []string{}, []string{}, []string{}, []string{}); got != "" {
		t.Fatalf("Format(empty slices) = %q, want empty", got)
	}
}
