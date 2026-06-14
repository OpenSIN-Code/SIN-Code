// SPDX-License-Identifier: MIT
package codegraph

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseGraph(t *testing.T) {
	in := `{
	  "root": "/repo",
	  "nodes": [
	    {"id": "n1", "kind": "function", "name": "main", "file": "main.go", "line": 10, "lang": "go"},
	    {"id": "n2", "kind": "function", "name": "helper", "file": "util.go", "line": 3, "lang": "go"}
	  ],
	  "edges": [
	    {"from": "n1", "to": "n2", "kind": "calls"}
	  ]
	}`
	g, err := ParseGraph([]byte(in))
	if err != nil {
		t.Fatalf("ParseGraph: %v", err)
	}
	if g.Root != "/repo" {
		t.Errorf("root = %q, want /repo", g.Root)
	}
	if len(g.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(g.Nodes))
	}
	if g.Nodes[0].Name != "main" || g.Nodes[0].Lang != "go" {
		t.Errorf("node[0] = %+v", g.Nodes[0])
	}
	if len(g.Edges) != 1 || g.Edges[0].Kind != "calls" {
		t.Errorf("edges = %+v", g.Edges)
	}
}

func TestParseGraphErrors(t *testing.T) {
	if _, err := ParseGraph([]byte("  ")); err == nil {
		t.Error("expected error for empty input")
	}
	if _, err := ParseGraph([]byte("not json")); err == nil {
		t.Error("expected error for invalid json")
	}
}

func TestFindNotInstalled(t *testing.T) {
	b := &Bridge{
		lookPath:   func(string) (string, error) { return "", errors.New("not found") },
		candidates: []string{filepath.Join(t.TempDir(), "nope")},
	}
	if _, err := b.Find(); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Find err = %v, want ErrNotInstalled", err)
	}
	if b.Available() {
		t.Error("Available() = true, want false")
	}
}

func TestFindViaCandidate(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "codegraph")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{
		lookPath:   func(string) (string, error) { return "", errors.New("not on PATH") },
		candidates: []string{fake},
	}
	got, err := b.Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != fake {
		t.Errorf("Find = %q, want %q", got, fake)
	}
	if !b.Available() {
		t.Error("Available() = false, want true")
	}
}

func TestFindPrefersPATH(t *testing.T) {
	dir := t.TempDir()
	onPath := filepath.Join(dir, "path-codegraph")
	if err := os.WriteFile(onPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{
		lookPath:   func(string) (string, error) { return onPath, nil },
		candidates: []string{"/should/not/be/used"},
	}
	got, err := b.Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != onPath {
		t.Errorf("Find = %q, want PATH hit %q", got, onPath)
	}
}

func TestAnalyzeNotInstalled(t *testing.T) {
	b := &Bridge{
		lookPath:   func(string) (string, error) { return "", errors.New("not found") },
		candidates: nil,
	}
	if _, err := b.Analyze(context.Background(), "."); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Analyze err = %v, want ErrNotInstalled", err)
	}
}

func TestRunNoArgs(t *testing.T) {
	if _, err := New().Run(context.Background(), ".", nil); err == nil {
		t.Error("expected error for empty args")
	}
}
