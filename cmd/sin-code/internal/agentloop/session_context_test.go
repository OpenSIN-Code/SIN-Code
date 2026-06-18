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

func TestSessionContextBuilder_Build_AllSources(t *testing.T) {
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"always run tests", "prefer sin_edit"}},
		&mockMemoryReader{items: []string{"user likes terse output", "project uses Go"}},
		&mockGoalReader{items: []string{"fix issue #375", "ship v3.23"}},
	)
	out, err := b.Build(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, want := range []string{"## Lessons", "## Memories", "## Goals", "always run tests", "user likes terse output", "fix issue #375"} {
		if !strings.Contains(out, want) {
			t.Fatalf("Build output missing %q:\n%s", want, out)
		}
	}
}

func TestSessionContextBuilder_Build_NilStores(t *testing.T) {
	b := NewSessionContextBuilder(nil, nil, nil)
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
	)
	if _, err := b.Build(context.Background()); err != want {
		t.Fatalf("expected memory error, got %v", err)
	}
}

func TestSessionContextBuilder_Build_GoalError(t *testing.T) {
	want := errors.New("goals down")
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"ok"}},
		&mockMemoryReader{items: []string{"ok"}},
		&mockGoalReader{err: want},
	)
	if _, err := b.Build(context.Background()); err != want {
		t.Fatalf("expected goal error, got %v", err)
	}
}

func TestSessionContextBuilder_Build_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := NewSessionContextBuilder(
		&mockLessonsReader{items: []string{"ok"}},
		nil,
		nil,
	)
	if _, err := b.Build(ctx); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSessionContextBuilder_Format(t *testing.T) {
	b := NewSessionContextBuilder(nil, nil, nil)
	out := b.Format(
		[]string{"l1", "l2"},
		[]string{"m1"},
		[]string{"g1", "g2", "g3"},
	)
	if !strings.Contains(out, "## Lessons\n- l1\n- l2\n") {
		t.Fatalf("lessons section wrong:\n%s", out)
	}
	if !strings.Contains(out, "## Memories\n- m1\n") {
		t.Fatalf("memories section wrong:\n%s", out)
	}
	if !strings.Contains(out, "## Goals\n- g1\n- g2\n- g3\n") {
		t.Fatalf("goals section wrong:\n%s", out)
	}
}

func TestSessionContextBuilder_Format_Empty(t *testing.T) {
	b := NewSessionContextBuilder(nil, nil, nil)
	if got := b.Format(nil, nil, nil); got != "" {
		t.Fatalf("Format(nil,nil,nil) = %q, want empty", got)
	}
	if got := b.Format([]string{}, []string{}, []string{}); got != "" {
		t.Fatalf("Format(empty slices) = %q, want empty", got)
	}
}
