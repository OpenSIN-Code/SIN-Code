// SPDX-License-Identifier: MIT
// Purpose: install — coverage-composer tests for composer paths that
// were previously uncovered (48.8% pre-PR). TarGz missing member
// (with dest-empty assertion), zip-on-Windows, ChooseBinDir home-dir
// fallback failure path, Place atomic failure on read-only bin dir,
// and FetchLatest 403 + X-RateLimit-Remaining=0. All hermetic via
// t.TempDir() and httptest.NewServer; no GitHub, no network.
package install_test

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/install"
)

// ─────────────────────────────────────────────────────────────────────────────
// 1. ExtractBinary — TarGz missing member + empty / nonexistent archive
// ─────────────────────────────────────────────────────────────────────────────

func TestExtractBinary_TarGz_MissingMember(t *testing.T) {
	dir := t.TempDir()

	// 1a. Empty archivePath → error verbatim.
	_, err := install.ExtractBinary("", dir, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "install: empty archive path") {
		t.Fatalf("empty archivePath: expected empty-path error, got %v", err)
	}

	// 1b. Non-existent archivePath → wrapped os.Open error.
	missing := filepath.Join(dir, "does-not-exist.tar.gz")
	_, err = install.ExtractBinary(missing, dir, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil {
		t.Fatalf("nonexistent archive: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist.tar.gz") && !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("nonexistent archive: expected os.Open-wrapped error mentioning file, got %v", err)
	}

	// 1c. tar.gz present but no `sin-code` member → "no member named".
	tgz := buildTarGz(t, map[string]string{
		"sin-code-linux-amd64/README.md": "# SIN-Code — not the binary",
		"sin-code-linux-amd64/LICENSE":   "MIT",
	})
	dest := t.TempDir()
	got, err := install.ExtractBinary(tgz, dest, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil {
		t.Fatalf("expected no-member error, got nil (extracted=%q)", got)
	}
	if !strings.Contains(err.Error(), "no member named") {
		t.Fatalf("expected error containing \"no member named\", got %v", err)
	}
	if !strings.Contains(err.Error(), "sin-code") {
		t.Fatalf("expected error mentioning missing member name %q, got %v", "sin-code", err)
	}

	// Destination must be empty — composer must not write the README
	// or LICENSE as a side-effect when the binary is absent.
	entries, statErr := os.ReadDir(dest)
	if statErr != nil {
		t.Fatalf("ReadDir(dest): %v", statErr)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dest should be empty on no-member path, found: %v", names)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. ExtractBinary — Zip on Windows
// ─────────────────────────────────────────────────────────────────────────────

func TestExtractBinary_Zip_Windows(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Render mode bits differ on windows; the existing happy-path
		// coverage in install_test handles the live-windows path.
		t.Skip("windows-specific zip extraction covered by install_test under live OS")
	}

	dir := t.TempDir()
	zipPath := filepath.Join(dir, "sin-code-windows-amd64.zip")

	payload := []byte("MZ\x90\x00 fake windows exe payload for the installer test\n")

	// Build a minimal valid zip with `sin-code.exe` at the root plus
	// a README.md (must be ignored by composer just like the tar.gz
	// path — see goreleaser archives layout).
	zw, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zw.Close()
	zipW := zip.NewWriter(zw)
	for name, body := range map[string][]byte{
		"sin-code.exe":                   payload,
		"sin-code-windows-amd64/LICENSE": []byte("MIT"),
		"README.md":                      []byte("# SIN"),
	} {
		w, err := zipW.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipW.Close(); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	got, err := install.ExtractBinary(zipPath, dest, install.Platform{GOOS: "windows", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("ExtractBinary zip: %v", err)
	}
	if got == "" {
		t.Fatalf("ExtractBinary zip returned empty path")
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("Stat extracted: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("extracted file is not regular: mode=%v", info.Mode())
	}
	if info.Size() <= 0 {
		t.Fatalf("extracted file has zero size")
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("ReadFile extracted: %v", err)
	}
	if !bytes.Equal(body, payload) {
		t.Fatalf("extracted body mismatch: got %q want %q", string(body), string(payload))
	}

	// Composer must not have pulled the LICENSE or README into dest.
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		base := strings.ToLower(e.Name())
		if strings.Contains(base, "license") || strings.Contains(base, "readme") {
			t.Fatalf("composer extracted extra file %q (binary-only contract violated)", e.Name())
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. ChooseBinDir — fallback to $HOME fails (returns expected error)
// ─────────────────────────────────────────────────────────────────────────────

func TestChooseBinDir_FallbackToHomeLocalBin(t *testing.T) {
	// Happy path: SIN_CODE_BIN_DIR set + writable → returns that dir,
	// skips the home-dir candidates and the fallback branch.
	writableBin := filepath.Join(t.TempDir(), "my-bin")
	if err := os.MkdirAll(writableBin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIN_CODE_BIN_DIR", writableBin)
	// Even with HOME pointing at nonsense, the env override short-
	// circuits before the UserHomeDir call in the candidates loop,
	// proving the first branch dominates.
	t.Setenv("HOME", "")
	got, created, hint, err := install.ChooseBinDir()
	if err != nil {
		t.Fatalf("happy path ChooseBinDir: %v", err)
	}
	if got != writableBin {
		t.Fatalf("happy path dir: got %q want %q", got, writableBin)
	}
	if created {
		t.Errorf("happy path must not set `created` (dir pre-existed)")
	}
	if hint != "" {
		t.Errorf("happy path must not emit PATH hint, got %q", hint)
	}

	// Failure path: env override is empty AND os.UserHomeDir errors.
	// On Linux, os.UserHomeDir() reads $HOME and returns an error
	// when it is empty; that error propagates into both the
	// candidates loop (so $HOME/.local/bin never appends) and the
	// fallback branch (which then short-circuits with the same
	// error). On darwin and windows, Go falls back to a password-DB
	// / USERPROFILE lookup that usually succeeds even with HOME="",
	// so the error branch is unreachable there — skip is correct.
	if runtime.GOOS != "linux" {
		t.Skip("error branch only triggers on linux; positive branch above covers the contract on darwin/windows")
	}
	t.Setenv("SIN_CODE_BIN_DIR", "")
	t.Setenv("HOME", "")
	_, _, _, err = install.ChooseBinDir()
	if err == nil {
		t.Fatalf("expected error when $HOME unavailable, got nil")
	}
	if !strings.Contains(err.Error(), "could not determine $HOME") {
		t.Fatalf("expected error containing %q, got %v", "could not determine $HOME", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. Place — atomic failure on read-only bin dir; success is byte-identical
// ─────────────────────────────────────────────────────────────────────────────

func TestPlace_AtomicFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("0555 not enforced on windows; place-failure covered separately")
	}

	// 4a. Empty binDir → verbatim error.
	_, err := install.Place("anything", "", install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "install: empty bin dir") {
		t.Fatalf("empty binDir: expected empty-bin-dir error, got %v", err)
	}

	// 4b. Empty binaryPath → verbatim error.
	_, err = install.Place("", "/tmp", install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "install: empty binary path") {
		t.Fatalf("empty binaryPath: expected empty-binary-path error, got %v", err)
	}

	// 4c. Bin dir wholly read-only → MkdirAll wrapped error.
	root := t.TempDir()
	lockedRoot := filepath.Join(root, "locked")
	if err := os.MkdirAll(lockedRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(lockedRoot, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedRoot, 0o755) }) // so TempDir cleanup can rm it

	blockedDir := filepath.Join(lockedRoot, "cannot-create")
	src := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho src\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = install.Place(src, blockedDir, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil {
		t.Fatalf("expected MkdirAll error on read-only parent, got nil")
	}
	if !strings.Contains(err.Error(), "install: mkdir") {
		t.Fatalf("expected wrapped MkdirAll error pattern, got %v", err)
	}
	// Place must NOT have created the directory.
	if _, statErr := os.Stat(blockedDir); statErr == nil {
		t.Fatalf("blockedDir was created despite read-only parent (stat succeeded)")
	}

	// 4d. Success: byte-identical contents + executable mode at final.
	binDir := t.TempDir()
	final, err := install.Place(src, binDir, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("Place happy path: %v", err)
	}
	wantFinal := filepath.Join(binDir, "sin-code")
	if final != wantFinal {
		t.Fatalf("final path: got %q want %q", final, wantFinal)
	}
	got, err := os.ReadFile(final)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("placed body mismatch: got %q want %q", string(got), string(want))
	}
	info, err := os.Stat(final)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("placed mode: got %v want 0755", info.Mode().Perm())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. FetchLatest — 403 + X-RateLimit-Remaining: 0 short-circuits
// ─────────────────────────────────────────────────────────────────────────────

func TestFetchLatest_RateLimited(t *testing.T) {
	var requestCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		http.Error(w, "API rate limit exceeded for installation ID", http.StatusForbidden)
	}))
	defer ts.Close()

	c := install.NewHTTPClientWithBaseURLForTest(ts.URL)
	got, err := install.FetchLatestWithConfigForTest(context.Background(), c, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil {
		t.Fatalf("expected rate-limit error, got nil (release=%+v)", got)
	}
	if got != nil {
		t.Fatalf("expected nil release, got %+v", got)
	}
	if !strings.Contains(err.Error(), "install: GitHub API rate limit hit (unauthenticated)") {
		t.Fatalf("expected rate-limit error message, got %v", err)
	}
	// No retry, no fallback — assert the server saw exactly one hit.
	if requestCount != 1 {
		t.Fatalf("expected exactly 1 server hit (no retry / no fallback), got %d", requestCount)
	}
}
