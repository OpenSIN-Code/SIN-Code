// SPDX-License-Identifier: MIT
// Purpose: tests for issue #154 — language-agnostic verify predicates
// (polyglot detection). DetectChecks picks the right checks based
// on ecosystem marker files in the workspace.
package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMarker(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectChecks_GoOnly(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "go.mod")
	got := DetectChecks(dir)
	if len(got) != 3 {
		t.Fatalf("expected 3 Go checks, got %d", len(got))
	}
	for _, c := range got {
		if c.Name != "go build" && c.Name != "go test" && c.Name != "go vet" {
			t.Errorf("unexpected check name: %s", c.Name)
		}
	}
}

func TestDetectChecks_PythonOnly(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "pyproject.toml")
	got := DetectChecks(dir)
	if len(got) != 2 {
		t.Fatalf("expected 2 Python checks, got %d", len(got))
	}
	names := map[string]bool{}
	for _, c := range got {
		names[c.Name] = true
	}
	if !names["pytest"] || !names["ruff"] {
		t.Errorf("expected pytest + ruff, got %v", names)
	}
}

func TestDetectChecks_RustOnly(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "Cargo.toml")
	got := DetectChecks(dir)
	if len(got) != 3 {
		t.Fatalf("expected 3 Rust checks, got %d", len(got))
	}
}

func TestDetectChecks_NodeOnly(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "package.json")
	got := DetectChecks(dir)
	if len(got) != 3 {
		t.Fatalf("expected 3 Node checks, got %d", len(got))
	}
}

func TestDetectChecks_PolyglotMonorepo(t *testing.T) {
	dir := t.TempDir()
	writeMarker(t, dir, "go.mod")
	web := filepath.Join(dir, "web")
	if err := os.Mkdir(web, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMarker(t, web, "package.json")
	got := DetectChecks(dir)
	// 3 Go (root, weight 2) + 3 Node (subdir, weight 1) = 6 distinct
	if len(got) != 6 {
		t.Errorf("expected 6 checks for Go+Node monorepo, got %d", len(got))
	}
}

func TestDetectChecks_NoMarkersFallback(t *testing.T) {
	dir := t.TempDir()
	got := DetectChecks(dir)
	// Empty workspace falls back to DefaultGoChecks (3 checks).
	if len(got) != 3 {
		t.Errorf("expected 3 fallback checks, got %d", len(got))
	}
}

func TestDetectChecks_EmptyWorkspace(t *testing.T) {
	got := DetectChecks("")
	if len(got) != 3 {
		t.Errorf("empty workspace should fall back to DefaultGoChecks, got %d", len(got))
	}
}

func TestDetectChecks_Dedupe(t *testing.T) {
	// Two directories with the same marker must not produce duplicate
	// checks (the matched map dedupes by name+marker).
	dir := t.TempDir()
	writeMarker(t, dir, "pyproject.toml")
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.MkdirAll(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMarker(t, a, "pyproject.toml")
	writeMarker(t, b, "pyproject.toml")
	got := DetectChecks(dir)
	if len(got) != 2 {
		t.Errorf("expected 2 dedup'd checks (pytest + ruff), got %d", len(got))
	}
}
