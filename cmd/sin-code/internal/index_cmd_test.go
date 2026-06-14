// SPDX-License-Identifier: MIT
// Purpose: Unit tests for index CLI commands. (st-cov1)
package internal

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func captureIndexCmd(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.RunE(cmd, args)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

func TestIndexBuildStatusClear(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)

	// status on non-existent index
	out, err := captureIndexCmd(t, indexStatusCmd, []string{dir})
	if err != nil {
		t.Fatalf("indexStatusCmd failed: %v", err)
	}
	if !strings.Contains(out, "No index found") {
		t.Errorf("expected 'No index found', got %q", out)
	}

	// build
	out, err = captureIndexCmd(t, indexBuildCmd, []string{dir})
	if err != nil {
		t.Fatalf("indexBuildCmd failed: %v", err)
	}
	if !strings.Contains(out, "Indexed") {
		t.Errorf("expected 'Indexed' output, got %q", out)
	}

	// status
	out, err = captureIndexCmd(t, indexStatusCmd, []string{dir})
	if err != nil {
		t.Fatalf("indexStatusCmd failed: %v", err)
	}
	if !strings.Contains(out, "Files:") {
		t.Errorf("expected status output, got %q", out)
	}

	// refresh
	out, err = captureIndexCmd(t, indexRefreshCmd, []string{dir})
	if err != nil {
		t.Fatalf("indexRefreshCmd failed: %v", err)
	}
	if !strings.Contains(out, "Refreshed") {
		t.Errorf("expected 'Refreshed' output, got %q", out)
	}

	// clear
	out, err = captureIndexCmd(t, indexClearCmd, []string{dir})
	if err != nil {
		t.Fatalf("indexClearCmd failed: %v", err)
	}
	if !strings.Contains(out, "Index cleared") {
		t.Errorf("expected 'Index cleared', got %q", out)
	}

	// refresh with no existing index
	out, err = captureIndexCmd(t, indexRefreshCmd, []string{dir})
	if err != nil {
		t.Fatalf("indexRefreshCmd failed: %v", err)
	}
	if !strings.Contains(out, "No existing index") {
		t.Errorf("expected 'No existing index' message, got %q", out)
	}
}

func TestIndexBuild_DefaultRoot(t *testing.T) {
	oldDir, _ := os.Getwd()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644)
	os.Chdir(dir)
	defer os.Chdir(oldDir)

	out, err := captureIndexCmd(t, indexBuildCmd, []string{})
	if err != nil {
		t.Fatalf("indexBuildCmd default root failed: %v", err)
	}
	if !strings.Contains(out, "Indexed") {
		t.Errorf("expected 'Indexed' output, got %q", out)
	}
}
