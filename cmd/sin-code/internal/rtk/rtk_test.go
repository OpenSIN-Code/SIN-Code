// SPDX-License-Identifier: MIT
package rtk

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestFindNotInstalled(t *testing.T) {
	b := &Bridge{
		lookPath:   func(string) (string, error) { return "", errors.New("not found") },
		candidates: []string{"/nonexistent/rtk"},
	}
	if _, err := b.Find(); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("expected ErrNotInstalled, got %v", err)
	}
	if b.Available() {
		t.Error("Available should be false when rtk is missing")
	}
}

func TestFindViaLookPath(t *testing.T) {
	b := &Bridge{
		lookPath: func(string) (string, error) { return "/usr/bin/rtk", nil },
	}
	got, err := b.Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != "/usr/bin/rtk" {
		t.Errorf("Find = %q, want /usr/bin/rtk", got)
	}
}

func TestFindViaCandidate(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "rtk")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{
		lookPath:   func(string) (string, error) { return "", errors.New("not in PATH") },
		candidates: []string{filepath.Join(dir, "missing"), fake},
	}
	got, err := b.Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != fake {
		t.Errorf("Find = %q, want %q", got, fake)
	}
}

func TestRunNoArgs(t *testing.T) {
	if _, err := New().Run(context.Background(), ".", nil); err == nil {
		t.Error("expected error for empty args")
	}
}

func TestRunNotInstalled(t *testing.T) {
	b := &Bridge{
		lookPath:   func(string) (string, error) { return "", errors.New("nope") },
		candidates: nil,
	}
	if _, err := b.Run(context.Background(), ".", []string{"git", "status"}); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("expected ErrNotInstalled, got %v", err)
	}
}

// TestRunWithFakeBinary points the bridge at a tiny shell script that
// behaves like a trivial rtk: it echoes its args. Skips on platforms
// without /bin/sh.
func TestRunWithFakeBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake not portable to windows")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "rtk")
	script := "#!/bin/sh\necho \"FILTERED: $*\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{cached: fake}
	out, err := b.Run(context.Background(), dir, []string{"git", "status"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "FILTERED: git status" {
		t.Errorf("Run output = %q, want %q", out, "FILTERED: git status")
	}
}

func TestRunTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake not portable to windows")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "rtk")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{cached: fake}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := b.Run(ctx, dir, []string{"slow"}); err == nil {
		t.Error("expected timeout error")
	}
}
