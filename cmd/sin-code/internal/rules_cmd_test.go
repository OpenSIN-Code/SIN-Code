// SPDX-License-Identifier: MIT
// Purpose: coverage tests for rules_cmd.go.
package internal

import (
	"errors"
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

func TestRulesListCmd_Text(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	oldFormat := rulesFormat
	rulesWorkspace = dir
	rulesFormat = "text"
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
	if !strings.Contains(string(out), "1 rules") {
		t.Errorf("expected rule count, got %q", out)
	}
}

func TestRulesListCmd_NoRules(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".sin-code", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := rulesWorkspace
	oldFormat := rulesFormat
	rulesWorkspace = dir
	rulesFormat = "text"
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
	if !strings.Contains(string(out), "no rules") {
		t.Errorf("expected no rules, got %q", out)
	}
}

func TestRulesListCmd_MultipleKinds(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".sin-code", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"always.md": "---\nname: always-rule\nalways_on: true\n---\nAlways.",
		"scoped.md": "---\nname: scoped-rule\npaths:\n  - '*.go'\n---\nScoped.",
		"unscoped.md": "---\nname: unscoped-rule\n---\nUnscoped.",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(rulesDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldWorkspace := rulesWorkspace
	oldFormat := rulesFormat
	rulesWorkspace = dir
	rulesFormat = "text"
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
	for _, label := range []string{"[always]", "[unscpd]", "[1 globs]"} {
		if !strings.Contains(string(out), label) {
			t.Errorf("expected %s in output, got %q", label, out)
		}
	}
}

func TestRulesListCmd_AbsError(t *testing.T) {
	oldAbs := rulesAbs
	rulesAbs = func(string) (string, error) { return "", errors.New("abs error") }
	defer func() { rulesAbs = oldAbs }()

	err := rulesListCmd.RunE(rulesListCmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "abs error") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestRulesListCmd_LoadError(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".sin-code", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "bad.md"), []byte("---\ninvalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := rulesWorkspace
	rulesWorkspace = dir
	defer func() { rulesWorkspace = oldWorkspace }()

	err := rulesListCmd.RunE(rulesListCmd, []string{})
	if err == nil {
		t.Fatalf("expected load error")
	}
}

func TestRulesShowCmd_Text(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	oldFormat := rulesFormat
	rulesWorkspace = dir
	rulesFormat = "text"
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
	if !strings.Contains(string(out), "# test-rule") {
		t.Errorf("expected rule header, got %q", out)
	}
}

func TestRulesShowCmd_AlwaysOn(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".sin-code", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: always-rule\nalways_on: true\n---\nAlways apply."
	if err := os.WriteFile(filepath.Join(rulesDir, "always-rule.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := rulesWorkspace
	oldFormat := rulesFormat
	rulesWorkspace = dir
	rulesFormat = "text"
	defer func() {
		rulesWorkspace = oldWorkspace
		rulesFormat = oldFormat
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := rulesShowCmd.RunE(rulesShowCmd, []string{"always-rule"})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("rulesShowCmd: %v", err)
	}
	if !strings.Contains(string(out), "(always-on)") {
		t.Errorf("expected always-on label, got %q", out)
	}
}

func TestRulesShowCmd_NotFound(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	rulesWorkspace = dir
	defer func() { rulesWorkspace = oldWorkspace }()

	err := rulesShowCmd.RunE(rulesShowCmd, []string{"missing"})
	if err == nil || !strings.Contains(err.Error(), "no such rule") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestRulesShowCmd_AbsError(t *testing.T) {
	oldAbs := rulesAbs
	rulesAbs = func(string) (string, error) { return "", errors.New("abs error") }
	defer func() { rulesAbs = oldAbs }()

	err := rulesShowCmd.RunE(rulesShowCmd, []string{"test-rule"})
	if err == nil || !strings.Contains(err.Error(), "abs error") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestRulesShowCmd_LoadError(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".sin-code", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "bad.md"), []byte("---\ninvalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := rulesWorkspace
	rulesWorkspace = dir
	defer func() { rulesWorkspace = oldWorkspace }()

	err := rulesShowCmd.RunE(rulesShowCmd, []string{"bad"})
	if err == nil {
		t.Fatalf("expected load error")
	}
}

func TestRulesPathCmd_Text(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	oldFormat := rulesFormat
	rulesWorkspace = dir
	rulesFormat = "text"
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
	if !strings.Contains(string(out), "matches") {
		t.Errorf("expected matches, got %q", out)
	}
}

func TestRulesPathCmd_NoMatch(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	oldFormat := rulesFormat
	rulesWorkspace = dir
	rulesFormat = "text"
	defer func() {
		rulesWorkspace = oldWorkspace
		rulesFormat = oldFormat
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := rulesPathCmd.RunE(rulesPathCmd, []string{"/tmp/main.txt"})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("rulesPathCmd: %v", err)
	}
	if !strings.Contains(string(out), "no rules match") {
		t.Errorf("expected no match, got %q", out)
	}
}

func TestRulesPathCmd_Relative(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	oldFormat := rulesFormat
	rulesWorkspace = dir
	rulesFormat = "json"
	defer func() {
		rulesWorkspace = oldWorkspace
		rulesFormat = oldFormat
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := rulesPathCmd.RunE(rulesPathCmd, []string{"main.go"})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("rulesPathCmd: %v", err)
	}
	if !strings.Contains(string(out), `"path"`) {
		t.Errorf("expected path field, got %q", out)
	}
}

func TestRulesPathCmd_AbsError(t *testing.T) {
	oldAbs := rulesAbs
	rulesAbs = func(string) (string, error) { return "", errors.New("abs error") }
	defer func() { rulesAbs = oldAbs }()

	err := rulesPathCmd.RunE(rulesPathCmd, []string{"/tmp/main.go"})
	if err == nil || !strings.Contains(err.Error(), "abs error") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestRulesPathCmd_LoadError(t *testing.T) {
	dir := t.TempDir()
	rulesDir := filepath.Join(dir, ".sin-code", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "bad.md"), []byte("---\ninvalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldWorkspace := rulesWorkspace
	rulesWorkspace = dir
	defer func() { rulesWorkspace = oldWorkspace }()

	err := rulesPathCmd.RunE(rulesPathCmd, []string{"/tmp/main.go"})
	if err == nil {
		t.Fatalf("expected load error")
	}
}

func TestRulesPathCmd_GetwdError(t *testing.T) {
	dir := setupRulesDir(t)
	oldWorkspace := rulesWorkspace
	oldFormat := rulesFormat
	oldGetwd := rulesGetwd
	rulesWorkspace = dir
	rulesFormat = "json"
	rulesGetwd = func() (string, error) { return "", errors.New("getwd error") }
	defer func() {
		rulesWorkspace = oldWorkspace
		rulesFormat = oldFormat
		rulesGetwd = oldGetwd
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := rulesPathCmd.RunE(rulesPathCmd, []string{"main.go"})
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("rulesPathCmd: %v", err)
	}
	if !strings.Contains(string(out), `"path"`) {
		t.Errorf("expected path field, got %q", out)
	}
}

func TestRulesWhereCmd_AbsError(t *testing.T) {
	oldAbs := rulesAbs
	rulesAbs = func(string) (string, error) { return "", errors.New("abs error") }
	defer func() { rulesAbs = oldAbs }()

	err := rulesWhereCmd.RunE(rulesWhereCmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "abs error") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestEncodeJSON_Error(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := encodeJSON(make(chan int))
	w.Close()
	os.Stdout = old
	io.ReadAll(r)
	if err == nil {
		t.Fatalf("expected JSON encode error")
	}
}
