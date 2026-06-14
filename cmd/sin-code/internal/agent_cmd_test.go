// SPDX-License-Identifier: MIT
// Purpose: Unit tests for agent subcommand helpers. (st-cov1)
package internal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchModels_HappyPath(t *testing.T) {
	 srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"gpt-4"},{"id":"gpt-3.5"}]}`))
	}))
	defer srv.Close()

	models, err := fetchModels(srv.URL, "")
	if err != nil {
		t.Fatalf("fetchModels failed: %v", err)
	}
	if len(models) != 2 || models[0] != "gpt-4" {
		t.Errorf("fetchModels = %v, want [gpt-4 gpt-3.5]", models)
	}
}

func TestFetchModels_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := fetchModels(srv.URL, ""); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestOpenAgentInEditor_SeedsAndOpens(t *testing.T) {
	oldEditor := os.Getenv("EDITOR")
	t.Setenv("EDITOR", "true")
	defer os.Setenv("EDITOR", oldEditor)

	dir := t.TempDir()
	t.Setenv("SIN_CODE_CONFIG_DIR", dir)

	if err := openAgentInEditor("test-agent"); err != nil {
		t.Fatalf("openAgentInEditor failed: %v", err)
	}

	cfgPath := filepath.Join(dir, "sin-code", "agents", "test-agent", "agent.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("expected config file at %s: %v", cfgPath, err)
	}
}

func TestOpenAgentInEditor_MissingEditorFails(t *testing.T) {
	oldEditor := os.Getenv("EDITOR")
	t.Setenv("EDITOR", "nonexistent-binary-for-test-xyz")
	defer os.Setenv("EDITOR", oldEditor)

	dir := t.TempDir()
	t.Setenv("SIN_CODE_CONFIG_DIR", dir)

	if err := openAgentInEditor("another-agent"); err == nil {
		t.Fatal("expected error for missing editor binary")
	}
}

func TestOpenAgentInEditor_DefaultEditor(t *testing.T) {
	oldEditor := os.Getenv("EDITOR")
	t.Setenv("EDITOR", "")
	defer os.Setenv("EDITOR", oldEditor)

	binDir := t.TempDir()
	fakeVim := filepath.Join(binDir, "vim")
	if err := os.WriteFile(fakeVim, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake vim: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir)
	defer os.Setenv("PATH", oldPath)

	dir := t.TempDir()
	t.Setenv("SIN_CODE_CONFIG_DIR", dir)

	if err := openAgentInEditor("default-editor-agent"); err != nil {
		t.Fatalf("openAgentInEditor with default editor failed: %v", err)
	}
}

func TestOpenAgentInEditor_InvalidName(t *testing.T) {
	if err := openAgentInEditor("invalid/name"); err == nil {
		t.Fatal("expected error for invalid agent name")
	}
}

func TestApplyAgentEdits_InvalidName(t *testing.T) {
	if err := applyAgentEdits("invalid/name", []string{"model=gpt-4"}); err == nil {
		t.Fatal("expected error for invalid agent name")
	}
}

func TestOpenAgentInEditor_MkdirError(t *testing.T) {
	oldEditor := os.Getenv("EDITOR")
	t.Setenv("EDITOR", "true")
	defer os.Setenv("EDITOR", oldEditor)

	dir := t.TempDir()
	readOnly := filepath.Join(dir, "readonly")
	if err := os.Mkdir(readOnly, 0o555); err != nil {
		t.Fatalf("mkdir readonly: %v", err)
	}
	t.Setenv("SIN_CODE_CONFIG_DIR", readOnly)

	if err := openAgentInEditor("mkdir-error-agent"); err == nil {
		t.Fatal("expected error when mkdir fails")
	}
}

func TestApplyAgentEdits_MkdirError(t *testing.T) {
	dir := t.TempDir()
	readOnly := filepath.Join(dir, "readonly")
	if err := os.Mkdir(readOnly, 0o555); err != nil {
		t.Fatalf("mkdir readonly: %v", err)
	}
	t.Setenv("SIN_CODE_CONFIG_DIR", readOnly)

	if err := applyAgentEdits("mkdir-error-agent", []string{"model=gpt-4"}); err == nil {
		t.Fatal("expected error when mkdir fails")
	}
}

func TestApplyAgentEdits_CreateError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SIN_CODE_CONFIG_DIR", dir)
	agentDirPath := filepath.Join(dir, "sin-code", "agents", "create-error-agent")
	if err := os.MkdirAll(agentDirPath, 0o755); err != nil {
		t.Fatalf("mkdir agent dir: %v", err)
	}
	// Create agent.toml as a directory so os.Create fails.
	if err := os.Mkdir(filepath.Join(agentDirPath, "agent.toml"), 0o755); err != nil {
		t.Fatalf("mkdir agent.toml: %v", err)
	}

	if err := applyAgentEdits("create-error-agent", []string{"model=gpt-4"}); err == nil {
		t.Fatal("expected error when create fails")
	}
}
