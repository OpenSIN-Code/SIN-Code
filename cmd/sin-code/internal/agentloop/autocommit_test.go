// SPDX-License-Identifier: MIT
// Purpose: tests for auto-commit after verified task (issue #487).
// M3: commit fires ONLY after verification passes — never before.
// M7: all tests pass under -race.
package agentloop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// writeFile writes content to path (test helper).
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitInit creates a temporary git repo and returns its path.
func gitInit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	} {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			t.Fatalf("git init step %v: %v", args, err)
		}
	}
	return dir
}

// gitCommitCount returns the number of commits in the repo.
func gitCommitCount(t *testing.T, dir string) int {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		return 0
	}
	return parseInt(strings.TrimSpace(string(out)))
}

// gitLatestMessage returns the latest commit message.
func gitLatestMessage(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "log", "-1", "--pretty=%s").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// TestAutoCommit_FiresAfterVerifyPass verifies that a git commit is
// created after the verification gate passes when AutoCommit is true.
func TestAutoCommit_FiresAfterVerifyPass(t *testing.T) {
	ws := gitInit(t)
	// Create a file so there is something to commit.
	if err := exec.Command("git", "-C", ws, "commit", "--allow-empty", "-m", "initial").Run(); err != nil {
		t.Fatal(err)
	}
	// Write a file to create a dirty tree.
	writeFile(t, filepath.Join(ws, "hello.txt"), "hello world")

	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	before := gitCommitCount(t, ws)
	loop := &Loop{
		Gate:       gate,
		Workspace:  ws,
		AutoCommit: true,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	res, err := loop.Run(context.Background(), s, "fix the bug")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Verified {
		t.Fatal("expected verified=true")
	}
	after := gitCommitCount(t, ws)
	if after != before+1 {
		t.Fatalf("expected %d commits after, got %d (before=%d)", before+1, after, before)
	}
	msg := gitLatestMessage(t, ws)
	if !strings.HasPrefix(msg, "fix:") {
		t.Fatalf("expected commit prefix 'fix:', got %q", msg)
	}
}

// TestAutoCommit_DoesNotFireWhenDisabled verifies no commit is created
// when AutoCommit is false (the default).
func TestAutoCommit_DoesNotFireWhenDisabled(t *testing.T) {
	ws := gitInit(t)
	if err := exec.Command("git", "-C", ws, "commit", "--allow-empty", "-m", "initial").Run(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(ws, "hello.txt"), "hello world")

	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	before := gitCommitCount(t, ws)
	loop := &Loop{
		Gate:       gate,
		Workspace:  ws,
		AutoCommit: false,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "add a feature")
	if err != nil {
		t.Fatal(err)
	}
	after := gitCommitCount(t, ws)
	if after != before {
		t.Fatalf("expected no new commit, got %d (before=%d)", after, before)
	}
}

// TestAutoCommit_DoesNotFireOnVerifyFail verifies that no commit is
// created when verification fails (M3: only after verify passes).
func TestAutoCommit_DoesNotFireOnVerifyFail(t *testing.T) {
	ws := gitInit(t)
	if err := exec.Command("git", "-C", ws, "commit", "--allow-empty", "-m", "initial").Run(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(ws, "hello.txt"), "hello world")

	s := setupSession(t)
	calls := 0
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) {
			calls++
			if calls < 3 {
				return false, "tests-fail", nil
			}
			return true, "ok", nil
		}, nil)

	before := gitCommitCount(t, ws)
	loop := &Loop{
		Gate:       gate,
		Workspace:  ws,
		AutoCommit: true,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "fix the bug")
	if err != nil {
		t.Fatal(err)
	}
	after := gitCommitCount(t, ws)
	if after != before+1 {
		t.Fatalf("expected exactly 1 commit after verify passes, got %d (before=%d)", after, before)
	}
}

// TestAutoCommit_SkipsCleanTree verifies that auto-commit is a no-op
// when the working tree is clean (nothing to commit).
func TestAutoCommit_SkipsCleanTree(t *testing.T) {
	ws := gitInit(t)
	if err := exec.Command("git", "-C", ws, "commit", "--allow-empty", "-m", "initial").Run(); err != nil {
		t.Fatal(err)
	}

	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	before := gitCommitCount(t, ws)
	loop := &Loop{
		Gate:       gate,
		Workspace:  ws,
		AutoCommit: true,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "fix the bug")
	if err != nil {
		t.Fatal(err)
	}
	after := gitCommitCount(t, ws)
	if after != before {
		t.Fatalf("expected no new commit on clean tree, got %d (before=%d)", after, before)
	}
}

// TestAutoCommit_UsesExplicitPrefix verifies that CommitPrefix is used
// verbatim when set, overriding auto-detection.
func TestAutoCommit_UsesExplicitPrefix(t *testing.T) {
	ws := gitInit(t)
	if err := exec.Command("git", "-C", ws, "commit", "--allow-empty", "-m", "initial").Run(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(ws, "hello.txt"), "hello world")

	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	loop := &Loop{
		Gate:         gate,
		Workspace:    ws,
		AutoCommit:   true,
		CommitPrefix: "docs",
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "update readme", Raw: session.Message{Role: "assistant", Content: "update readme"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "fix the bug")
	if err != nil {
		t.Fatal(err)
	}
	msg := gitLatestMessage(t, ws)
	if !strings.HasPrefix(msg, "docs:") {
		t.Fatalf("expected commit prefix 'docs:', got %q", msg)
	}
}

// TestAutoCommit_FiresCommitPostHook verifies that the commit.post hook
// is fired after a successful auto-commit by using a command hook that
// creates a marker file.
func TestAutoCommit_FiresCommitPostHook(t *testing.T) {
	ws := gitInit(t)
	if err := exec.Command("git", "-C", ws, "commit", "--allow-empty", "-m", "initial").Run(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(ws, "hello.txt"), "hello world")

	markerPath := filepath.Join(ws, ".commit-post-fired")
	hookList := []hooks.Hook{
		{
			Event:   hooks.CommitPost,
			Type:    "command",
			Command: "touch " + markerPath,
		},
	}
	hookEngine := hooks.New(hookList)

	s := setupSession(t)
	gate := verify.NewGate("poc",
		func(ctx context.Context, ws string) (bool, string, error) { return true, "ok", nil },
		nil)

	loop := &Loop{
		Gate:       gate,
		Workspace:  ws,
		AutoCommit: true,
		Hooks:      hookEngine,
		Completion: func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	_, err := loop.Run(context.Background(), s, "fix the bug")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Fatal("expected commit.post hook marker file to exist")
	}
}

// TestDetectCommitPrefix verifies the prefix detection logic.
func TestDetectCommitPrefix(t *testing.T) {
	tests := []struct {
		prompt string
		want   string
	}{
		{"fix the bug in parser", "fix"},
		{"add new feature for auth", "feat"},
		{"refactor the module", "refactor"},
		{"update documentation", "docs"},
		{"add test for handler", "test"},
		{"fix security vulnerability", "security"},
		{"optimize performance", "perf"},
		{"cleanup and chore", "chore"},
		{"format style", "style"},
		{"update ci pipeline", "ci"},
		{"update build system", "build"},
		{"random task", "feat"},
	}
	for _, tt := range tests {
		got := detectCommitPrefix(tt.prompt)
		if got != tt.want {
			t.Errorf("detectCommitPrefix(%q) = %q, want %q", tt.prompt, got, tt.want)
		}
	}
}

// TestBuildCommitMessage verifies message construction.
func TestBuildCommitMessage(t *testing.T) {
	got := buildCommitMessage("feat", "add user auth", "added login flow")
	if got != "feat: added login flow" {
		t.Fatalf("got %q", got)
	}
	// Falls back to prompt when summary is empty.
	got = buildCommitMessage("fix", "fix the bug", "")
	if got != "fix: fix the bug" {
		t.Fatalf("got %q", got)
	}
	// Truncates long messages (full line including prefix > 72 chars).
	long := strings.Repeat("a", 100)
	got = buildCommitMessage("feat", "prompt", long)
	if len(got) > 72 {
		t.Fatalf("expected truncation, got len=%d: %q", len(got), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncation suffix, got %q", got)
	}
}
