// SPDX-License-Identifier: MIT
// Purpose: coverage tests for rules_cmd.go.
package internal

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureRulesCmd(t *testing.T, args []string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := RulesCmd.Execute()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

func TestEncodeJSON(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := encodeJSON(map[string]string{"key": "value"})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("encodeJSON: %v", err)
	}
	if !strings.Contains(string(out), `"key"`) {
		t.Errorf("expected JSON output, got %q", out)
	}
}

func setupRulesDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".sin-code", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: test-rule
description: a test rule
paths:
  - "*.go"
---
Always test your code.
`
	if err := os.WriteFile(filepath.Join(rulesDir, "test-rule.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRulesListCmd_JSON(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	rulesWorkspace = dir
	oldFormat := rulesFormat
	rulesFormat = "json"
	defer func() {
		rulesWorkspace = oldWorkspace
		rulesFormat = oldFormat
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := rulesListCmd.RunE(rulesListCmd, []string{})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("rulesListCmd: %v", err)
	}
	if !strings.Contains(string(out), `"Name"`) {
		t.Errorf("expected JSON Name field, got %q", out)
	}
}

func TestRulesShowCmd_JSON(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	rulesWorkspace = dir
	oldFormat := rulesFormat
	rulesFormat = "json"
	defer func() {
		rulesWorkspace = oldWorkspace
		rulesFormat = oldFormat
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := rulesShowCmd.RunE(rulesShowCmd, []string{"test-rule"})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("rulesShowCmd: %v", err)
	}
	if !strings.Contains(string(out), `"Name"`) {
		t.Errorf("expected JSON Name field, got %q", out)
	}
}

func TestRulesPathCmd_JSON(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	rulesWorkspace = dir
	oldFormat := rulesFormat
	rulesFormat = "json"
	defer func() {
		rulesWorkspace = oldWorkspace
		rulesFormat = oldFormat
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := rulesPathCmd.RunE(rulesPathCmd, []string{"/tmp/main.go"})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("rulesPathCmd: %v", err)
	}
	if !strings.Contains(string(out), `"path"`) {
		t.Errorf("expected JSON path field, got %q", out)
	}
	if !strings.Contains(string(out), `"rules"`) {
		t.Errorf("expected JSON rules field, got %q", out)
	}
}

func TestRulesWhereCmd(t *testing.T) {
	dir := t.TempDir()
	oldWorkspace := rulesWorkspace
	rulesWorkspace = dir
	defer func() { rulesWorkspace = oldWorkspace }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := rulesWhereCmd.RunE(rulesWhereCmd, []string{})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("rulesWhereCmd: %v", err)
	}
	if !strings.Contains(string(out), ".sin-code/rules/") {
		t.Errorf("expected rules directory, got %q", out)
	}
}
