// SPDX-License-Identifier: MIT
package codegraph

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// writeFakeCodegraph installs a POSIX shell script at dir/name that prints
// stdout (shell-quoted) and exits with code.
func writeFakeCodegraph(t *testing.T, dir, name, stdout string, code int) string {
	t.Helper()
	body := fmt.Sprintf("#!/bin/sh\necho %q\nexit %d\n", stdout, code)
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFindCached(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeCodegraph(t, dir, "codegraph", "v1", 0)
	b := &Bridge{lookPath: func(string) (string, error) { return fake, nil }, candidates: nil}
	got1, err := b.Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got1 != fake {
		t.Errorf("Find = %q, want %q", got1, fake)
	}
	got2, err := b.Find()
	if err != nil {
		t.Fatalf("Find second: %v", err)
	}
	if got2 != fake {
		t.Errorf("Find second = %q, want cached %q", got2, fake)
	}
}

func TestFindStaleCache(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeCodegraph(t, dir, "codegraph", "v1", 0)
	missing := filepath.Join(dir, "missing")
	b := &Bridge{
		lookPath:   func(string) (string, error) { return fake, nil },
		candidates: nil,
		cached:     missing,
	}
	got, err := b.Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != fake {
		t.Errorf("Find = %q, want %q", got, fake)
	}
	if b.cached != fake {
		t.Errorf("cached = %q, want %q", b.cached, fake)
	}
}

func TestFindNilLookPath(t *testing.T) {
	// Cover the lp == nil fallback and the empty-candidate skip branch.
	t.Setenv("PATH", "")
	dir := t.TempDir()
	fake := writeFakeCodegraph(t, dir, "codegraph", "v1", 0)
	b := &Bridge{lookPath: nil, candidates: []string{"", fake}}
	got, err := b.Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != fake {
		t.Errorf("Find = %q, want %q", got, fake)
	}
}

func TestFindAllCandidatesMissing(t *testing.T) {
	b := &Bridge{
		lookPath: func(string) (string, error) { return "", errors.New("not found") },
		candidates: []string{
			filepath.Join(t.TempDir(), "a"),
			filepath.Join(t.TempDir(), "b"),
		},
	}
	if _, err := b.Find(); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Find err = %v, want ErrNotInstalled", err)
	}
}

func TestRunHappy(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeCodegraph(t, dir, "codegraph", "hello world", 0)
	b := &Bridge{lookPath: func(string) (string, error) { return fake, nil }, candidates: nil}
	out, err := b.Run(context.Background(), "", []string{"--version"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "hello world" {
		t.Errorf("Run = %q, want %q", out, "hello world")
	}
}

func TestRunTimeout(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "codegraph")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nsleep 5\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{lookPath: func(string) (string, error) { return p, nil }, candidates: nil}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := b.Run(ctx, ".", []string{"analyze"})
	if err == nil {
		t.Fatal("Run: expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("Run err = %q, want 'timed out'", err.Error())
	}
}

func TestRunError(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeCodegraph(t, dir, "codegraph", "boom", 1)
	b := &Bridge{lookPath: func(string) (string, error) { return fake, nil }, candidates: nil}
	_, err := b.Run(context.Background(), ".", []string{"bad"})
	if err == nil {
		t.Fatal("Run: expected error")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("Run err = %q, want command in error", err.Error())
	}
}

func TestRunNotInstalled(t *testing.T) {
	b := &Bridge{lookPath: func(string) (string, error) { return "", errors.New("not found") }, candidates: nil}
	_, err := b.Run(context.Background(), ".", []string{"--version"})
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("Run err = %v, want ErrNotInstalled", err)
	}
}

func TestAnalyzeHappy(t *testing.T) {
	dir := t.TempDir()
	out := `{"root":"original","nodes":[],"edges":[]}`
	fake := writeFakeCodegraph(t, dir, "codegraph", out, 0)
	b := &Bridge{lookPath: func(string) (string, error) { return fake, nil }, candidates: nil}
	g, err := b.Analyze(context.Background(), dir)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if g.Root != "original" {
		t.Errorf("Analyze root = %q, want %q", g.Root, "original")
	}
}

func TestAnalyzeEmptyPath(t *testing.T) {
	dir := t.TempDir()
	out := `{"root":"p","nodes":[],"edges":[]}`
	fake := writeFakeCodegraph(t, dir, "codegraph", out, 0)
	b := &Bridge{lookPath: func(string) (string, error) { return fake, nil }, candidates: nil}
	g, err := b.Analyze(context.Background(), "")
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if g.Root != "p" {
		t.Errorf("Analyze root = %q, want %q", g.Root, "p")
	}
}

func TestAnalyzeEmptyRoot(t *testing.T) {
	dir := t.TempDir()
	out := `{"nodes":[],"edges":[]}`
	fake := writeFakeCodegraph(t, dir, "codegraph", out, 0)
	b := &Bridge{lookPath: func(string) (string, error) { return fake, nil }, candidates: nil}
	g, err := b.Analyze(context.Background(), dir)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if g.Root != dir {
		t.Errorf("Analyze root = %q, want %q", g.Root, dir)
	}
}

func TestAnalyzeRunError(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeCodegraph(t, dir, "codegraph", "fail", 1)
	b := &Bridge{lookPath: func(string) (string, error) { return fake, nil }, candidates: nil}
	if _, err := b.Analyze(context.Background(), "."); err == nil {
		t.Fatal("Analyze: expected error")
	}
}

func TestAnalyzeParseError(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeCodegraph(t, dir, "codegraph", "not json", 0)
	b := &Bridge{lookPath: func(string) (string, error) { return fake, nil }, candidates: nil}
	if _, err := b.Analyze(context.Background(), "."); err == nil {
		t.Fatal("Analyze: expected error")
	}
}

func TestVersionHappy(t *testing.T) {
	dir := t.TempDir()
	fake := writeFakeCodegraph(t, dir, "codegraph", "1.2.3", 0)
	b := &Bridge{lookPath: func(string) (string, error) { return fake, nil }, candidates: nil}
	v, err := b.Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if v != "1.2.3" {
		t.Errorf("Version = %q, want %q", v, "1.2.3")
	}
}
