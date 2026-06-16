// SPDX-License-Identifier: MIT
// Purpose: tests for the autodev bridge package. All tests are hermetic
// — no real autodev binary is invoked, no real network. Each test that
// would normally shell out to `autodev` rewrites $PATH via t.Setenv to
// point at t.TempDir(), where a POSIX shell-script shim is installed
// (mode 0o755, exec.LookPath-resolvable). Stdlib-only imports per
// package contract.
// Docs: autodev.doc.md
package autodev

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ── Test helpers ──────────────────────────────────────────────────────

// writeFakeBinary installs a POSIX shell script `name` at
// filepath.Join(dir, name) that prints `stdout` then exits `code`.
// Mode 0o755 so exec.LookPath() finds it through PATH.
func writeFakeBinary(t *testing.T, dir, name, stdout string, code int) string {
	t.Helper()
	body := fmt.Sprintf("#!/bin/sh\necho %q\nexit %d\n", stdout, code)
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary %s: %v", p, err)
	}
	return p
}

// prependPath returns a new PATH string with dir at the front so the
// fake binary shadows the real one for the duration of this test.
func prependPath(dir string) string {
	if cur := os.Getenv("PATH"); cur != "" {
		return dir + string(os.PathListSeparator) + cur
	}
	return dir
}

// ── DefaultBin / DefaultMCPBin ────────────────────────────────────────

func TestDefaultBin_ReturnsDefault(t *testing.T) {
	t.Setenv("AUTODEV_BIN", "")
	if got := DefaultBin(); got != "autodev" {
		t.Errorf("DefaultBin() = %q, want %q", got, "autodev")
	}
}

func TestDefaultBin_EnvOverrideTrimmed(t *testing.T) {
	t.Setenv("AUTODEV_BIN", "/opt/local/bin/my-autodev")
	if got := DefaultBin(); got != "/opt/local/bin/my-autodev" {
		t.Errorf("DefaultBin() = %q, want literal override", got)
	}
	t.Setenv("AUTODEV_BIN", "   /tmp/x   ")
	if got := DefaultBin(); got != "/tmp/x" {
		t.Errorf("DefaultBin() = %q, want trimmed override", got)
	}
}

func TestDefaultMCPBin_EnvAndDefault(t *testing.T) {
	t.Setenv("AUTODEV_MCP_BIN", "")
	if got := DefaultMCPBin(); got != "autodev-mcp" {
		t.Errorf("DefaultMCPBin() = %q, want %q", got, "autodev-mcp")
	}
	t.Setenv("AUTODEV_MCP_BIN", "/usr/local/bin/autodev-mcp")
	if got := DefaultMCPBin(); got != "/usr/local/bin/autodev-mcp" {
		t.Errorf("DefaultMCPBin() = %q, want override", got)
	}
}

// ── IsInstalled ───────────────────────────────────────────────────────

func TestIsInstalled_HappyPaths(t *testing.T) {
	dir := t.TempDir()
	p := writeFakeBinary(t, dir, "autodev-mcp-fake", "x", 0)
	if !IsInstalled(p) {
		t.Errorf("IsInstalled(absolute %q) = false, want true", p)
	}
	t.Setenv("PATH", prependPath(dir))
	if !IsInstalled("autodev-mcp-fake") {
		t.Errorf("IsInstalled(PATH-resident name) = false, want true")
	}
}

func TestIsInstalled_RejectsEmptyAndMissing(t *testing.T) {
	if IsInstalled("") {
		t.Error("IsInstalled(\"\") = true, want false")
	}
	if IsInstalled("   ") {
		t.Error("IsInstalled(\"   \") = true, want false (whitespace)")
	}
	t.Setenv("PATH", t.TempDir())
	if IsInstalled("this-binary-really-does-not-exist-anywhere-xyzzy") {
		t.Error("IsInstalled(missing) = true, want false")
	}
}

// ── ResolveAutodevMCPBin ──────────────────────────────────────────────

func TestResolveAutodevMCPBin_OK(t *testing.T) {
	dir := t.TempDir()
	writeFakeBinary(t, dir, "autodev-mcp-fake", "ok", 0)
	t.Setenv("AUTODEV_MCP_BIN", "autodev-mcp-fake")
	t.Setenv("PATH", prependPath(dir))
	if err := ResolveAutodevMCPBin(); err != nil {
		t.Errorf("ResolveAutodevMCPBin() = %v, want nil", err)
	}
}

func TestResolveAutodevMCPBin_NotInstalledWrapsErr(t *testing.T) {
	t.Setenv("AUTODEV_MCP_BIN", "definitely-not-installed-xyzzy")
	t.Setenv("PATH", t.TempDir())
	err := ResolveAutodevMCPBin()
	if err == nil {
		t.Fatal("ResolveAutodevMCPBin() = nil err, want non-nil")
	}
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("err = %v, does not wrap ErrNotInstalled", err)
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("err = %v, does not wrap exec.ErrNotFound", err)
	}
}

// ── Version (fake binary paths — the bridged subprocess contract) ─────

func TestVersion_FakeBinary_HappyPath(t *testing.T) {
	// The shim writes only stdout, exits 0. versionWith trims and
	// returns; no panic, no error wrapping.
	dir := t.TempDir()
	writeFakeBinary(t, dir, "autodev-fake", "v0.4.0\n", 0)
	t.Setenv("AUTODEV_BIN", "autodev-fake")
	t.Setenv("PATH", prependPath(dir))

	got, err := Version()
	if err != nil {
		t.Fatalf("Version() err = %v, want nil", err)
	}
	if got != "v0.4.0" {
		t.Errorf("Version() = %q, want %q (trimmed)", got, "v0.4.0")
	}
}

func TestVersion_FakeBinary_ExitNonZero_IsErrorPath(t *testing.T) {
	// Fake-process error path (runcount #1 in spec): shim exits 1 with
	// stderr populated. versionWith must surface a wrapped error
	// carrying the stderr payload and an empty return string.
	dir := t.TempDir()
	writeFakeBinary(t, dir, "autodev-broken", "ignored-on-stdout", 1)
	t.Setenv("AUTODEV_BIN", "autodev-broken")
	t.Setenv("PATH", prependPath(dir))

	got, err := Version()
	if err == nil {
		t.Fatal("Version() err = nil, want non-nil on exit 1")
	}
	if got != "" {
		t.Errorf("Version() = %q, want empty string on error", got)
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("Version() err %q missing exec.ExitError wrap", err.Error())
	}
}

func TestVersion_BinaryMissing_IsErrorPath(t *testing.T) {
	// Fake-process error path (run #2): bin name unresolvable. Surfaced
	// as wrapped exec error, not as a panic.
	t.Setenv("AUTODEV_BIN", "autodev-ghost")
	t.Setenv("PATH", t.TempDir())

	got, err := Version()
	if err == nil {
		t.Fatal("Version() err = nil, want non-nil on missing bin")
	}
	if got != "" {
		t.Errorf("Version() = %q, want empty on missing bin", got)
	}
}

func TestVersion_PropagatesContextDeadline(t *testing.T) {
	// Driving versionWith directly: a cancel-already ctx should not
	// block — it returns quickly. We do not assert specific error text
	// (varies by OS), only that the call returns within a few seconds.
	dir := t.TempDir()
	writeFakeBinary(t, dir, "autodev-fake", "v0.4.0", 0)
	t.Setenv("AUTODEV_BIN", "autodev-fake")
	t.Setenv("PATH", prependPath(dir))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already-deadline before the call
	if _, err := versionWith(ctx); err == nil {
		t.Errorf("versionWith(cancelled ctx) = (value, nil), want err on cancelled ctx")
	}
}

func TestResolveAutodevBin_OK(t *testing.T) {
	dir := t.TempDir()
	writeFakeBinary(t, dir, "autodev-fake", "ok", 0)
	t.Setenv("AUTODEV_BIN", "autodev-fake")
	t.Setenv("PATH", prependPath(dir))
	if err := ResolveAutodevBin(); err != nil {
		t.Errorf("ResolveAutodevBin() = %v, want nil", err)
	}
}

func TestResolveAutodevBin_NotInstalled(t *testing.T) {
	t.Setenv("AUTODEV_BIN", "definitely-not-installed-xyzzy")
	t.Setenv("PATH", t.TempDir())
	err := ResolveAutodevBin()
	if err == nil {
		t.Fatal("ResolveAutodevBin() = nil, want non-nil")
	}
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("err = %v, does not wrap ErrNotInstalled", err)
	}
}

func TestResolve_EmptyBin(t *testing.T) {
	// Direct coverage of the shared resolve() empty-name branch.
	err := resolve("")
	if err == nil {
		t.Fatal("resolve(\"\") = nil, want non-nil")
	}
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("err = %v, does not wrap ErrNotInstalled", err)
	}
}

func TestVersion_EmptyStdout(t *testing.T) {
	// Successful exit with no output -> explicit empty-stdout error.
	dir := t.TempDir()
	writeFakeBinary(t, dir, "autodev-empty", "", 0)
	t.Setenv("AUTODEV_BIN", "autodev-empty")
	t.Setenv("PATH", prependPath(dir))

	got, err := Version()
	if err == nil {
		t.Fatal("Version() err = nil, want non-nil on empty stdout")
	}
	if got != "" {
		t.Errorf("Version() = %q, want empty string", got)
	}
	if !strings.Contains(err.Error(), "returned empty stdout") {
		t.Errorf("Version() err %q missing expected text", err.Error())
	}
}

func TestVersion_NoOutputError(t *testing.T) {
	// Non-zero exit with no stdout/stderr -> the wrapped exec error is
	// used as the diagnostic payload.
	dir := t.TempDir()
	writeFakeBinary(t, dir, "autodev-silent", "", 1)
	t.Setenv("AUTODEV_BIN", "autodev-silent")
	t.Setenv("PATH", prependPath(dir))

	_, err := Version()
	if err == nil {
		t.Fatal("Version() err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("Version() err %q missing wrapped exec error", err.Error())
	}
}
