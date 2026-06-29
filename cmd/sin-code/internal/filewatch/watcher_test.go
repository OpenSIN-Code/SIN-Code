// SPDX-License-Identifier: MIT
// Purpose: tests for the filewatch package (issue #486). Covers pattern
// matching (basename + path-anchored), skip-dir filtering, event
// filtering, debounce reset, and a full end-to-end watch+trigger flow
// against a real temp directory. All tests are race-clean (M7) and
// environment-independent.
package filewatch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// safeBuf is a concurrency-safe strings.Builder wrapper for tests. The
// watcher's runCommand writes from the timer goroutine while the test
// goroutine reads — a plain strings.Builder would race (M7).
type safeBuf struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}
func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestMatchRule_Basename covers patterns without a slash — they are
// matched against the file's basename at any depth.
func TestMatchRule_Basename(t *testing.T) {
	root := "/proj"
	rules := []Rule{
		{Pattern: "*.go", Command: "go test"},
		{Pattern: "*.ts", Command: "npm test"},
	}
	cases := []struct {
		name string
		path string
		want bool
		cmd  string
	}{
		{"deep go file", "/proj/internal/foo/bar.go", true, "go test"},
		{"root go file", "/proj/main.go", true, "go test"},
		{"ts file", "/proj/src/x.ts", true, "npm test"},
		{"no match py", "/proj/x.py", false, ""},
		{"no match md", "/proj/README.md", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, ok := MatchRule(root, c.path, rules)
			if ok != c.want {
				t.Fatalf("MatchRule(%q) = %v, want %v", c.path, ok, c.want)
			}
			if ok && r.Command != c.cmd {
				t.Fatalf("MatchRule command = %q, want %q", r.Command, c.cmd)
			}
		})
	}
}

// TestMatchRule_PathAnchored covers patterns containing a slash — they
// are matched against the path relative to the watch root (forward
// slashes).
func TestMatchRule_PathAnchored(t *testing.T) {
	root := "/proj"
	rules := []Rule{
		{Pattern: "src/*.py", Command: "pytest"},
		{Pattern: "*.go", Command: "go test"},
	}
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"anchored match", "/proj/src/app.py", true},
		{"wrong dir", "/proj/tests/app.py", false},
		{"too deep", "/proj/src/sub/app.py", false}, // filepath.Match does not cross '/'
		{"basename still works", "/proj/deep/x.go", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := MatchRule(root, c.path, rules)
			if ok != c.want {
				t.Fatalf("MatchRule(%q) = %v, want %v", c.path, ok, c.want)
			}
		})
	}
}

// TestSkipDir verifies the closed set of skipped directory basenames.
func TestSkipDir(t *testing.T) {
	skipped := []string{".git", "node_modules", "vendor", ".sin-code", "dist", "build"}
	for _, d := range skipped {
		if !SkipDir(d) {
			t.Errorf("SkipDir(%q) = false, want true", d)
		}
	}
	notSkipped := []string{"src", "cmd", "internal", "tests", "mybuild"}
	for _, d := range notSkipped {
		if SkipDir(d) {
			t.Errorf("SkipDir(%q) = true, want false", d)
		}
	}
}

// TestIsWriteOrCreate verifies that only Write|Create events trigger a
// run; Remove/Rename/Chmod are ignored (issue #486).
func TestIsWriteOrCreate(t *testing.T) {
	cases := []struct {
		op   fsnotify.Op
		want bool
	}{
		{fsnotify.Write, true},
		{fsnotify.Create, true},
		{fsnotify.Write | fsnotify.Create, true},
		{fsnotify.Remove, false},
		{fsnotify.Rename, false},
		{fsnotify.Chmod, false},
		{fsnotify.Remove | fsnotify.Write, true}, // Write bit set
	}
	for _, c := range cases {
		ev := fsnotify.Event{Name: "/x", Op: c.op}
		if got := IsWriteOrCreate(ev); got != c.want {
			t.Errorf("IsWriteOrCreate(op=%v) = %v, want %v", c.op, got, c.want)
		}
	}
}

// TestDebounceReset confirms that rapid-fire events within the debounce
// window update the pending rule so the LAST match wins when the timer
// fires (time.Timer.Reset does not rebind closure variables, so
// pendingRule must be stored separately). No real command is executed
// — the timer is stopped before it fires.
func TestDebounceReset(t *testing.T) {
	w := &Watcher{
		debounce: 500 * time.Millisecond, // long enough to inspect before fire
		out:      &strings.Builder{},
		err:      &strings.Builder{},
	}
	w.debouncedRun(Rule{Pattern: "first", Command: "a"})
	w.debouncedRun(Rule{Pattern: "second", Command: "b"})
	w.debouncedRun(Rule{Pattern: "third", Command: "c"})

	// Stop the timer so no command executes.
	if w.timer == nil || !w.timer.Stop() {
		// timer already fired (unlikely with 500ms); drain if needed
	}
	w.pendingMu.Lock()
	fired := w.pendingRule
	w.pendingMu.Unlock()
	if fired.Pattern != "third" {
		t.Errorf("debounce: pending rule = %q, want %q (last match wins)", fired.Pattern, "third")
	}
}

// TestWatcherEndToEnd creates a real temp workspace, starts a watcher
// with a small debounce, writes a matching file, and asserts the
// triggered command executes. Uses `sh -c echo` (or cmd /c echo on
// Windows) which is universally available.
func TestWatcherEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end watcher test skipped in -short mode")
	}
	tmp := t.TempDir()
	// Create a watched subdirectory before starting.
	sub := filepath.Join(tmp, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var out safeBuf
	var errw safeBuf
	rules := []Rule{{Pattern: "*.go", Command: "echo hello-from-watch"}}
	w, err := New(tmp, rules, 200, &out, &errw)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Stop()

	// Give the watcher a moment to install kernel watches.
	time.Sleep(50 * time.Millisecond)

	target := filepath.Join(sub, "trigger.go")
	if err := os.WriteFile(target, []byte("package x\n"), 0o644); err != nil {
		t.Fatalf("write trigger: %v", err)
	}

	// Wait for debounce (200ms) + command exec.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "hello-from-watch") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("command output not observed within timeout.\nout=%s\nerr=%s", out.String(), errw.String())
}

// TestWatcherSkipDirNotWatched verifies that writing a file inside a
// skipped directory does NOT trigger the command (the dir was never
// watched).
func TestWatcherSkipDirNotWatched(t *testing.T) {
	if testing.Short() {
		t.Skip("skip-dir watcher test skipped in -short mode")
	}
	tmp := t.TempDir()
	// Pre-create a skipped dir with a nested file-watching expectation.
	gitDir := filepath.Join(tmp, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var out safeBuf
	var errw safeBuf
	rules := []Rule{{Pattern: "*.go", Command: "echo should-not-fire"}}
	w, err := New(tmp, rules, 100, &out, &errw)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	defer w.Stop()

	time.Sleep(50 * time.Millisecond)

	// Write a .go file inside the skipped .git dir.
	target := filepath.Join(gitDir, "config.go")
	if err := os.WriteFile(target, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	time.Sleep(300 * time.Millisecond)
	if strings.Contains(out.String(), "should-not-fire") {
		t.Fatalf("command fired for file in skipped dir; out=%s", out.String())
	}
}

// TestNewNegativeDebounce verifies that a negative debounce falls back
// to the default rather than panicking.
func TestNewNegativeDebounce(t *testing.T) {
	w, err := New(t.TempDir(), []Rule{{Pattern: "*.go", Command: "x"}}, -1, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()
	if w.debounce != time.Duration(DefaultDebounce)*time.Millisecond {
		t.Errorf("debounce = %v, want default %v", w.debounce, DefaultDebounce)
	}
}

// TestStopIdempotent verifies Stop can be called multiple times without
// panicking, even before Watch was called.
func TestStopIdempotent(t *testing.T) {
	w, err := New(t.TempDir(), nil, 100, nil, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Stop before Watch — should be a no-op.
	w.Stop()
	// Start then stop twice.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	w.Stop()
	w.Stop() // second stop must not panic or hang
}

// TestShellCommand confirms the right interpreter is selected per OS.
func TestShellCommand(t *testing.T) {
	cmd := shellCommand("echo hi")
	if runtime.GOOS == "windows" {
		if cmd.Args[0] != "cmd" {
			t.Errorf("windows: cmd = %q, want cmd", cmd.Args[0])
		}
	} else {
		if cmd.Args[0] != "sh" {
			t.Errorf("unix: cmd = %q, want sh", cmd.Args[0])
		}
	}
}
