// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for codegraph_cmd.go.
// Docs: codegraph_cmd.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/codegraph"
)

// fakeCodeGraphBridge implements codegraphBridge for tests.
type fakeCodeGraphBridge struct {
	graph       *codegraph.Graph
	analyzeErr  error
	findPath    string
	findErr     error
	version     string
	versionErr  error
	analyzePath string
}

func (f *fakeCodeGraphBridge) Analyze(ctx context.Context, path string) (*codegraph.Graph, error) {
	f.analyzePath = path
	return f.graph, f.analyzeErr
}

func (f *fakeCodeGraphBridge) Find() (string, error) { return f.findPath, f.findErr }

func (f *fakeCodeGraphBridge) Version(ctx context.Context) (string, error) {
	return f.version, f.versionErr
}

func resetCodeGraphHooks(t *testing.T) {
	t.Helper()
	orig := codegraphHookVars
	t.Cleanup(func() { codegraphHookVars = orig })
}

func runCodeGraphCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewCodeGraphCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestCodeGraphAnalyzeHuman(t *testing.T) {
	resetCodeGraphHooks(t)
	bridge := &fakeCodeGraphBridge{graph: &codegraph.Graph{Root: "/repo", Nodes: []codegraph.Node{{}, {}}, Edges: []codegraph.Edge{{}}}}
	codegraphHookVars.newBridge = func() codegraphBridge { return bridge }
	out, err := runCodeGraphCmd(t, "analyze", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if bridge.analyzePath != "/repo" {
		t.Errorf("expected path /repo, got %q", bridge.analyzePath)
	}
	if !strings.Contains(out.String(), "CodeGraph: /repo") || !strings.Contains(out.String(), "nodes: 2") || !strings.Contains(out.String(), "edges: 1") {
		t.Errorf("expected summary output, got %q", out.String())
	}
}

func TestCodeGraphAnalyzeDefaultPath(t *testing.T) {
	resetCodeGraphHooks(t)
	bridge := &fakeCodeGraphBridge{graph: &codegraph.Graph{Root: "."}}
	codegraphHookVars.newBridge = func() codegraphBridge { return bridge }
	out, err := runCodeGraphCmd(t, "analyze")
	if err != nil {
		t.Fatal(err)
	}
	if bridge.analyzePath != "." {
		t.Errorf("expected path ., got %q", bridge.analyzePath)
	}
	if !strings.Contains(out.String(), "CodeGraph: .") {
		t.Errorf("expected summary output, got %q", out.String())
	}
	_ = out
}

func TestCodeGraphAnalyzeJSON(t *testing.T) {
	resetCodeGraphHooks(t)
	bridge := &fakeCodeGraphBridge{graph: &codegraph.Graph{Root: "/repo", Nodes: []codegraph.Node{{ID: "n"}}, Edges: []codegraph.Edge{{}}}}
	codegraphHookVars.newBridge = func() codegraphBridge { return bridge }
	out, err := runCodeGraphCmd(t, "analyze", "--json", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	var result codegraph.Graph
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Root != "/repo" {
		t.Errorf("unexpected root: %q", result.Root)
	}
}

func TestCodeGraphAnalyzeError(t *testing.T) {
	resetCodeGraphHooks(t)
	codegraphHookVars.newBridge = func() codegraphBridge { return &fakeCodeGraphBridge{analyzeErr: errors.New("analyze boom")} }
	_, err := runCodeGraphCmd(t, "analyze", "/repo")
	if err == nil || !strings.Contains(err.Error(), "analyze boom") {
		t.Fatalf("expected analyze error, got %v", err)
	}
}

func TestCodeGraphDoctorSuccess(t *testing.T) {
	resetCodeGraphHooks(t)
	codegraphHookVars.newBridge = func() codegraphBridge {
		return &fakeCodeGraphBridge{findPath: "/usr/bin/codegraph", version: "v2.0"}
	}
	out, err := runCodeGraphCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "codegraph: OK") || !strings.Contains(out.String(), "/usr/bin/codegraph") || !strings.Contains(out.String(), "v2.0") {
		t.Errorf("expected doctor output, got %q", out.String())
	}
}

func TestCodeGraphDoctorFindError(t *testing.T) {
	resetCodeGraphHooks(t)
	codegraphHookVars.newBridge = func() codegraphBridge { return &fakeCodeGraphBridge{findErr: errors.New("not found")} }
	out, err := runCodeGraphCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected find error, got %v", err)
	}
	if !strings.Contains(out.String(), "codegraph: NOT installed") {
		t.Errorf("expected not installed message, got %q", out.String())
	}
}

func TestCodeGraphDoctorVersionError(t *testing.T) {
	resetCodeGraphHooks(t)
	codegraphHookVars.newBridge = func() codegraphBridge {
		return &fakeCodeGraphBridge{findPath: "/usr/bin/codegraph", versionErr: errors.New("version boom")}
	}
	out, err := runCodeGraphCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "version:") {
		t.Errorf("expected no version line, got %q", out.String())
	}
	if !strings.Contains(out.String(), "codegraph: OK") {
		t.Errorf("expected ok output, got %q", out.String())
	}
}

func TestCodeGraphDoctorNoVersion(t *testing.T) {
	resetCodeGraphHooks(t)
	codegraphHookVars.newBridge = func() codegraphBridge { return &fakeCodeGraphBridge{findPath: "/usr/bin/codegraph"} }
	out, err := runCodeGraphCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "version:") {
		t.Errorf("expected no version line, got %q", out.String())
	}
	_ = out
}

func TestCodeGraphDefaultHooks(t *testing.T) {
	resetCodeGraphHooks(t)
	b := codegraphHookVars.newBridge()
	if b == nil {
		t.Fatal("expected non-nil bridge from default hook")
	}
}
