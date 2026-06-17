// SPDX-License-Identifier: MIT
// Purpose: watch mode tests (mandate M7 race-safe).
package agentloop

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func writeWatchFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestWatchMode_StartStop(t *testing.T) {
	w := NewWatchMode([]string{"*.go"})
	if w.Active() {
		t.Fatal("should not be active before Start")
	}
	root := t.TempDir()
	w.SetRoot(root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx, func() {}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !w.Active() {
		t.Fatal("should be active after Start")
	}
	w.Stop()
	if w.Active() {
		t.Fatal("should not be active after Stop")
	}
}

func TestWatchMode_FileChangeDetection(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "main.go", "package main\n")
	w := NewWatchMode([]string{"*.go"})
	w.SetRoot(root)
	var calls atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx, func() { calls.Add(1) }); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	writeWatchFile(t, root, "main.go", "package main\nfunc main() {}\n")
	deadline := time.After(5 * time.Second)
	for {
		if calls.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("callback not fired after file change")
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	w.Stop()
}

func TestWatchMode_Debounce(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "main.go", "package main\n")
	w := NewWatchMode([]string{"*.go"})
	w.SetRoot(root)
	var calls atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx, func() { calls.Add(1) }); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	for i := 0; i < 5; i++ {
		writeWatchFile(t, root, "main.go", "package main\n// change\n")
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(1 * time.Second)
	w.Stop()
	got := calls.Load()
	if got < 1 {
		t.Fatalf("expected at least 1 callback, got %d", got)
	}
	if got > 3 {
		t.Errorf("debounce should limit callbacks, got %d (expected <=3)", got)
	}
}

func TestWatchMode_IgnorePatterns(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "main.go", "package main\n")
	os.MkdirAll(filepath.Join(root, "node_modules"), 0755)
	writeWatchFile(t, root, "node_modules/lib.go", "package lib\n")
	os.MkdirAll(filepath.Join(root, ".git"), 0755)
	writeWatchFile(t, root, ".git/hook.go", "package hook\n")
	os.MkdirAll(filepath.Join(root, "vendor"), 0755)
	writeWatchFile(t, root, "vendor/v.go", "package vendor\n")
	w := NewWatchMode([]string{"*.go"})
	w.SetRoot(root)
	w.initialScan()
	w.mu.Lock()
	for path := range w.snapshots {
		base := filepath.Base(filepath.Dir(path))
		if base == "node_modules" || base == ".git" || base == "vendor" {
			t.Errorf("ignored dir file should not be in snapshots: %s", path)
		}
	}
	w.mu.Unlock()
}

func TestWatchMode_ConcurrentAccess(t *testing.T) {
	w := NewWatchMode([]string{"*.go"})
	root := t.TempDir()
	w.SetRoot(root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			w.Active()
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		_ = w.Start(ctx, func() {})
	}
	for i := 0; i < 100; i++ {
		w.Stop()
	}
	<-done
}

func TestWatchMode_ContextCancellation(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "main.go", "package main\n")
	w := NewWatchMode([]string{"*.go"})
	w.SetRoot(root)
	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx, func() {}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	cancel()
	time.Sleep(300 * time.Millisecond)
	w.mu.Lock()
	started := w.started
	w.mu.Unlock()
	if started {
		w.Stop()
	}
}

func TestWatchMode_CallbackExecution(t *testing.T) {
	root := t.TempDir()
	writeWatchFile(t, root, "a.go", "package a\n")
	w := NewWatchMode([]string{"*.go"})
	w.SetRoot(root)
	var data atomic.Int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	callback := func() {
		data.Add(42)
	}
	if err := w.Start(ctx, callback); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	writeWatchFile(t, root, "a.go", "package a\n// modified\n")
	time.Sleep(2 * time.Second)
	w.Stop()
	if data.Load() == 0 {
		t.Fatal("callback was never executed")
	}
}

func TestWatchMode_ActiveState(t *testing.T) {
	w := NewWatchMode([]string{"*.py"})
	if w.Active() {
		t.Fatal("new watch should be inactive")
	}
	root := t.TempDir()
	w.SetRoot(root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx, func() {})
	if !w.Active() {
		t.Fatal("should be active after Start")
	}
	w.Stop()
	if w.Active() {
		t.Fatal("should be inactive after Stop")
	}
	w.Start(ctx, func() {})
	if !w.Active() {
		t.Fatal("should be active after second Start")
	}
	w.Stop()
}

func TestWatchMode_MatchesPattern(t *testing.T) {
	w := NewWatchMode([]string{"*.go", "*.py"})
	tests := []struct {
		name string
		want bool
	}{
		{"main.go", true},
		{"app.py", true},
		{"test.js", false},
		{"readme.md", false},
		{"foo_test.go", true},
	}
	for _, tt := range tests {
		if got := w.matchesPattern(tt.name); got != tt.want {
			t.Errorf("matchesPattern(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestWatchMode_StartIdempotent(t *testing.T) {
	w := NewWatchMode([]string{"*.go"})
	root := t.TempDir()
	w.SetRoot(root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx, func() {}); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := w.Start(ctx, func() {}); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	w.Stop()
}

func TestWatchMode_StopIdempotent(t *testing.T) {
	w := NewWatchMode([]string{"*.go"})
	w.Stop()
	w.Stop()
	root := t.TempDir()
	w.SetRoot(root)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx, func() {})
	w.Stop()
	w.Stop()
}
