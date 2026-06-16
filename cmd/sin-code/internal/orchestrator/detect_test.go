// SPDX-License-Identifier: MIT
// Purpose: tests for polyglot verification check detection.
package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectChecks_NodeRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	checks := DetectChecks(dir)
	var names []string
	for _, c := range checks {
		names = append(names, c.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "npm test") {
		t.Fatalf("expected npm checks for node repo, got %v", names)
	}
	if strings.Contains(joined, "go build") {
		t.Fatalf("should not pick Go checks for a pure node repo, got %v", names)
	}
}

func TestDetectChecks_EmptyFallsBackToGo(t *testing.T) {
	checks := DetectChecks(t.TempDir())
	if len(checks) == 0 || checks[0].Name != "go build" {
		t.Fatalf("expected Go fallback, got %v", checks)
	}
}

func TestDetectChecks_MonorepoCombines(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"go.mod", "package.json"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	checks := DetectChecks(dir)
	var names []string
	for _, c := range checks {
		names = append(names, c.Name)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "go build") || !strings.Contains(joined, "npm test") {
		t.Fatalf("expected combined Go+Node checks, got %v", names)
	}
}
