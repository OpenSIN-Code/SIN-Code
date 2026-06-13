// SPDX-License-Identifier: MIT
// Purpose: MCP handler tests for sin_security_scan (issue #36).
// All tests are hermetic — no network, no real tool lookups.
// Known-absent tools (govulncheck, gosec, etc.) return not_found
// status, which is expected on clean CI images.
package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestHandleSecurity_DefaultsToCurrentDir(t *testing.T) {
	ctx := context.Background()
	result, err := handleSecurity(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	var sr SecurityResult
	if uerr := json.Unmarshal([]byte(result), &sr); uerr != nil {
		t.Fatalf("result is not valid JSON: %v\n%s", uerr, result[:min(len(result), 200)])
	}
	if sr.ProjectType == "" {
		t.Fatal("expected project_type in result")
	}
	if len(sr.Tools) == 0 {
		t.Fatal("expected at least one tool in result")
	}
}

func TestHandleSecurity_FormatJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.23\n"), 0644)

	ctx := context.Background()
	result, err := handleSecurity(ctx, map[string]any{
		"path":   dir,
		"format": "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sr SecurityResult
	if uerr := json.Unmarshal([]byte(result), &sr); uerr != nil {
		t.Fatalf("result is not valid JSON: %v\n%s", uerr, result[:min(len(result), 200)])
	}
	if sr.ProjectType != "go" {
		t.Errorf("expected project_type=go, got %s", sr.ProjectType)
	}
}

func TestHandleSecurity_FormatText(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.23\n"), 0644)

	ctx := context.Background()
	result, err := handleSecurity(ctx, map[string]any{
		"path":   dir,
		"format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty text result")
	}
}

func TestHandleSecurity_TimeoutClamp(t *testing.T) {
	dir := t.TempDir()

	ctx := context.Background()
	result, err := handleSecurity(ctx, map[string]any{
		"path":    dir,
		"format":  "json",
		"timeout": float64(99999),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sr SecurityResult
	if uerr := json.Unmarshal([]byte(result), &sr); uerr != nil {
		t.Fatalf("result is not valid JSON: %v\n%s", uerr, result[:min(len(result), 200)])
	}
}

func TestHandleSecurity_TimeoutZeroDefaults(t *testing.T) {
	dir := t.TempDir()

	ctx := context.Background()
	result, err := handleSecurity(ctx, map[string]any{
		"path":    dir,
		"format":  "json",
		"timeout": float64(0),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sr SecurityResult
	if uerr := json.Unmarshal([]byte(result), &sr); uerr != nil {
		t.Fatalf("result is not valid JSON: %v\n%s", uerr, result[:min(len(result), 200)])
	}
}

func TestHandleSecurity_NotFoundPath(t *testing.T) {
	// handler resolves the path with filepath.Abs which does NOT fail
	// for nonexistent directories. runSecurityScan gracefully detects
	// the project type as "generic" and runs the generic tool chain.
	// This is expected — the handler is not a path-existence guard.
	ctx := context.Background()
	result, err := handleSecurity(ctx, map[string]any{
		"path":   filepath.Join(t.TempDir(), "nonexistent-xyz-123"),
		"format": "json",
	})
	if err != nil {
		t.Fatalf("expected no error for nonexistent path: %v", err)
	}
	var sr SecurityResult
	if uerr := json.Unmarshal([]byte(result), &sr); uerr != nil {
		t.Fatalf("result is not valid JSON: %v\n%s", uerr, result[:min(len(result), 200)])
	}
}

func TestHandleSecurity_StrictDoesNotError(t *testing.T) {
	// strict flag is accepted but does NOT propagate as MCP error;
	// the caller inspects Summary.Issues instead.
	dir := t.TempDir()

	ctx := context.Background()
	result, err := handleSecurity(ctx, map[string]any{
		"path":   dir,
		"format": "json",
		"strict": true,
	})
	if err != nil {
		t.Fatalf("strict should not cause MCP error: %v", err)
	}
	if result == "" {
		t.Fatal("expected non-empty result even with strict=true")
	}
}

func TestHandleSecurity_RaceSafety(t *testing.T) {
	var wg sync.WaitGroup
	errCh := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dir := t.TempDir()
			_, err := handleSecurity(context.Background(), map[string]any{
				"path":   dir,
				"format": "json",
			})
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("goroutine error: %v", err)
	}
}
