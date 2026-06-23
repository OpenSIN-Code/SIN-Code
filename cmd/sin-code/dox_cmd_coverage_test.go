// SPDX-License-Identifier: MIT
// Purpose: coverage tests for dox_cmd.go — exercises every RunE branch and
// helper function using package-level hooks.
// Docs: dox.doc.md
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dox"
)

type errWriter struct{ err error }

func (e errWriter) Write(p []byte) (int, error) { return 0, e.err }

func runDoxCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewDoxCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func saveDoxHooks(t *testing.T) {
	t.Helper()
	origDoxInjectRootHook := doxInjectRootHook
	origDoxScaffoldHook := doxScaffoldHook
	origDoxCheckHook := doxCheckHook
	origDoxRenderTreeHook := doxRenderTreeHook
	origDoxFilepathAbsHook := doxFilepathAbsHook
	origDoxOsMkdirAllHook := doxOsMkdirAllHook
	origDoxOsWriteFileHook := doxOsWriteFileHook
	origDoxOsStatHook := doxOsStatHook
	t.Cleanup(func() {
		doxInjectRootHook = origDoxInjectRootHook
		doxScaffoldHook = origDoxScaffoldHook
		doxCheckHook = origDoxCheckHook
		doxRenderTreeHook = origDoxRenderTreeHook
		doxFilepathAbsHook = origDoxFilepathAbsHook
		doxOsMkdirAllHook = origDoxOsMkdirAllHook
		doxOsWriteFileHook = origDoxOsWriteFileHook
		doxOsStatHook = origDoxOsStatHook
	})
}

func TestDoxInitDefault(t *testing.T) {
	dir := t.TempDir()
	_, err := runDoxCmd(t, "init", dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, dox.AgentsFileName)); err != nil {
		t.Fatal(err)
	}
}

func TestDoxInitForce(t *testing.T) {
	dir := t.TempDir()
	agentsPath := filepath.Join(dir, dox.AgentsFileName)
	if err := os.WriteFile(agentsPath, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runDoxCmd(t, "init", dir, "--force")
	if err != nil {
		t.Fatal(err)
	}
}

func TestDoxInitAbsError(t *testing.T) {
	saveDoxHooks(t)
	doxFilepathAbsHook = func(string) (string, error) { return "", errors.New("abs boom") }
	_, err := runDoxCmd(t, "init", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "abs boom") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestDoxInitMkdirAllError(t *testing.T) {
	saveDoxHooks(t)
	doxOsMkdirAllHook = func(string, os.FileMode) error { return errors.New("mkdir boom") }
	_, err := runDoxCmd(t, "init", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "mkdir boom") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestDoxInitWriteSeedError(t *testing.T) {
	saveDoxHooks(t)
	doxOsWriteFileHook = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	_, err := runDoxCmd(t, "init", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "write boom") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestDoxInitStatNonNotExist(t *testing.T) {
	saveDoxHooks(t)
	// A non-NotExist stat error causes the code to try seeding the file.
	// Make the write fail so the error propagates.
	doxOsStatHook = func(string) (os.FileInfo, error) { return nil, errors.New("stat boom") }
	doxOsWriteFileHook = func(string, []byte, os.FileMode) error { return errors.New("stat boom") }
	_, err := runDoxCmd(t, "init", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "stat boom") {
		t.Fatalf("expected stat error, got %v", err)
	}
}

func TestDoxInitInjectRootError(t *testing.T) {
	saveDoxHooks(t)
	doxInjectRootHook = func(string, string) error { return errors.New("inject boom") }
	_, err := runDoxCmd(t, "init", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "inject boom") {
		t.Fatalf("expected inject error, got %v", err)
	}
}

func TestDoxInitJSON(t *testing.T) {
	dir := t.TempDir()
	out, err := runDoxCmd(t, "init", dir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["agents_path"] != filepath.Join(dir, dox.AgentsFileName) {
		t.Errorf("unexpected agents_path: %v", result["agents_path"])
	}
}

func TestDoxInitJSONEncodeError(t *testing.T) {
	saveDoxHooks(t)
	doxInjectRootHook = func(string, string) error { return nil }
	cmd := NewDoxCmd()
	cmd.SetArgs([]string{"init", t.TempDir(), "--json"})
	setOutAll(cmd, errWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestDoxNewDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dox.AgentsFileName), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runDoxCmd(t, "new", "child", "--parent", dir, "--title", "Child Node")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "scaffolded") {
		t.Errorf("expected scaffolded output, got %q", out.String())
	}
}

func TestDoxNewParentDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dox.AgentsFileName), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(cwd)
	out, err := runDoxCmd(t, "new", "child2")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "scaffolded") {
		t.Errorf("expected scaffolded output, got %q", out.String())
	}
}
func TestDoxNewParentEmptyFlag(t *testing.T) {
	saveDoxHooks(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dox.AgentsFileName), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(cwd)
	// Explicitly pass an empty parent flag so the fallback `parent = "."` branch runs.
	out, err := runDoxCmd(t, "new", "child3", "--parent", "", "--title", "Child Node")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "scaffolded") {
		t.Errorf("expected scaffolded output, got %q", out.String())
	}
}

func TestDoxNewAbsError(t *testing.T) {
	saveDoxHooks(t)
	doxFilepathAbsHook = func(string) (string, error) { return "", errors.New("abs boom") }
	_, err := runDoxCmd(t, "new", "child", "--parent", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "abs boom") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestDoxNewScaffoldError(t *testing.T) {
	saveDoxHooks(t)
	doxScaffoldHook = func(string, string, string) (string, error) { return "", errors.New("scaffold boom") }
	_, err := runDoxCmd(t, "new", "child", "--parent", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "scaffold boom") {
		t.Fatalf("expected scaffold error, got %v", err)
	}
}

func TestDoxNewJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dox.AgentsFileName), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runDoxCmd(t, "new", "child", "--parent", dir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["name"] != "child" {
		t.Errorf("unexpected name: %v", result["name"])
	}
}

func TestDoxNewJSONEncodeError(t *testing.T) {
	saveDoxHooks(t)
	doxScaffoldHook = func(string, string, string) (string, error) { return "/tmp/child", nil }
	cmd := NewDoxCmd()
	cmd.SetArgs([]string{"new", "child", "--parent", t.TempDir(), "--json"})
	setOutAll(cmd, errWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestDoxCheckDefaultHealthy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dox.AgentsFileName), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runDoxCmd(t, "check", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "healthy") {
		t.Errorf("expected healthy output, got %q", out.String())
	}
}

func TestDoxCheckWithArgs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dox.AgentsFileName), []byte("# Root\n\nTODO: fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runDoxCmd(t, "check", dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "WARN") {
		t.Errorf("expected WARN output, got %q", out.String())
	}
}

func TestDoxCheckError(t *testing.T) {
	saveDoxHooks(t)
	doxCheckHook = func(string) ([]dox.Finding, error) { return nil, errors.New("check boom") }
	_, err := runDoxCmd(t, "check", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "check boom") {
		t.Fatalf("expected check error, got %v", err)
	}
}

func TestDoxCheckErrorFindings(t *testing.T) {
	saveDoxHooks(t)
	doxCheckHook = func(string) ([]dox.Finding, error) {
		return []dox.Finding{{Path: "/a", Kind: "broken", Severity: "error", Message: "m"}}, nil
	}
	_, err := runDoxCmd(t, "check", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "1 error(s) found") {
		t.Fatalf("expected error findings, got %v", err)
	}
}

func TestDoxCheckJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dox.AgentsFileName), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runDoxCmd(t, "check", dir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["healthy"] != true {
		t.Errorf("expected healthy true, got %v", result["healthy"])
	}
}

func TestDoxCheckJSONEncodeError(t *testing.T) {
	saveDoxHooks(t)
	doxCheckHook = func(string) ([]dox.Finding, error) { return nil, nil }
	cmd := NewDoxCmd()
	cmd.SetArgs([]string{"check", "/tmp", "--json"})
	setOutAll(cmd, errWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestDoxTreeDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dox.AgentsFileName), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runDoxCmd(t, "tree", dir)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() == "" {
		t.Error("expected non-empty tree output")
	}
}

func TestDoxTreeRenderError(t *testing.T) {
	saveDoxHooks(t)
	doxRenderTreeHook = func(string) (string, error) { return "", errors.New("tree boom") }
	_, err := runDoxCmd(t, "tree", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "tree boom") {
		t.Fatalf("expected tree error, got %v", err)
	}
}

func TestDoxTreeJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, dox.AgentsFileName), []byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runDoxCmd(t, "tree", dir, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["tree"]; !ok {
		t.Errorf("expected tree field, got %v", result)
	}
}

func TestDoxTreeJSONEncodeError(t *testing.T) {
	saveDoxHooks(t)
	doxRenderTreeHook = func(string) (string, error) { return "tree", nil }
	cmd := NewDoxCmd()
	cmd.SetArgs([]string{"tree", "/tmp", "--json"})
	setOutAll(cmd, errWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}

func TestDoxHelpers(t *testing.T) {
	fs := []dox.Finding{
		{Path: "/b", Kind: "warn", Severity: "warn", Message: "w"},
		{Path: "/a", Kind: "error", Severity: "error", Message: "e"},
		{Path: "/a", Kind: "warn2", Severity: "warn", Message: "w2"},
	}
	sortFindings(fs)
	if fs[0].Severity != "error" || fs[1].Path != "/a" || fs[2].Path != "/b" {
		t.Errorf("unexpected sort order: %+v", fs)
	}
	if !hasErrors(fs) {
		t.Error("expected hasErrors true")
	}
	if countErrors(fs) != 1 {
		t.Errorf("expected 1 error, got %d", countErrors(fs))
	}
	if hasErrors(nil) {
		t.Error("expected hasErrors false for nil")
	}
	if countErrors(nil) != 0 {
		t.Errorf("expected 0 errors for nil, got %d", countErrors(nil))
	}

	if !lessFinding(dox.Finding{Severity: "error"}, dox.Finding{Severity: "warn"}) {
		t.Error("expected error < warn")
	}
	if lessFinding(dox.Finding{Severity: "warn"}, dox.Finding{Severity: "error"}) {
		t.Error("expected warn > error")
	}
	if !lessFinding(dox.Finding{Severity: "warn", Path: "/a"}, dox.Finding{Severity: "warn", Path: "/b"}) {
		t.Error("expected path sort")
	}
	if !lessFinding(dox.Finding{Severity: "warn", Path: "/a", Kind: "a"}, dox.Finding{Severity: "warn", Path: "/a", Kind: "b"}) {
		t.Error("expected kind sort")
	}
}

func TestDoxCheckPrintFindings(t *testing.T) {
	saveDoxHooks(t)
	doxCheckHook = func(string) ([]dox.Finding, error) {
		return []dox.Finding{
			{Path: "/a", Kind: "warn", Severity: "warn", Message: "w"},
			{Path: "/b", Kind: "broken", Severity: "error", Message: "e"},
		}, nil
	}
	out, err := runDoxCmd(t, "check", "/tmp")
	if err == nil || !strings.Contains(err.Error(), "1 error(s) found") {
		t.Fatalf("expected error return, got %v", err)
	}
	if !strings.Contains(out.String(), "WARN") || !strings.Contains(out.String(), "ERR") {
		t.Errorf("expected warn and error output, got %q", out.String())
	}
}

func TestDoxCheckJSONPrintErrorPath(t *testing.T) {
	saveDoxHooks(t)
	doxCheckHook = func(string) ([]dox.Finding, error) {
		return []dox.Finding{{Path: "/a", Kind: "k", Severity: "error", Message: "m"}}, nil
	}
	cmd := NewDoxCmd()
	cmd.SetArgs([]string{"check", "/tmp", "--json"})
	setOutAll(cmd, errWriter{err: errors.New("encode boom")})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "encode boom") {
		t.Fatalf("expected encode error, got %v", err)
	}
}
