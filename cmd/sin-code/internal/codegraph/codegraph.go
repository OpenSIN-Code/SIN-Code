// SPDX-License-Identifier: MIT
// Purpose: bridge to CodeGraph (https://github.com/codegraph-ai/codegraph),
// an external multi-language static-analysis engine exposed to SIN-Code as
// an MCP tool (issue #126). Follows the Bridged-External-Contract shared
// with gh/vane/dox/rtk: CodeGraph is NEVER vendored — we shell out to the
// user's installed binary and fail with a clear install hint when missing.
//
// Unlike the rtk bridge (which returns filtered text), CodeGraph emits a
// structured graph of symbols/edges, so Analyze parses the JSON envelope.
//
// Docs: docs/codegraph-integration.md
package codegraph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotInstalled is returned when no codegraph binary can be located.
var ErrNotInstalled = errors.New("codegraph not found in PATH or standard locations; install from " +
	"https://github.com/codegraph-ai/codegraph (curl -fsSL https://raw.githubusercontent.com/codegraph-ai/codegraph/main/install.sh | sh  •  or  cargo install codegraph)")

// Node is a single symbol in the code graph (function, type, module, ...).
type Node struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // function|type|method|module|variable|...
	Name  string `json:"name"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Lang  string `json:"lang"`
}

// Edge is a directed relationship between two nodes (calls, imports, ...).
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // calls|imports|implements|references|...
}

// Graph is the structured analysis envelope returned by `codegraph analyze --json`.
type Graph struct {
	Root  string `json:"root"`
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Bridge locates and invokes the codegraph binary. The zero value is NOT
// ready; use New. Discovery fields are injectable for tests.
type Bridge struct {
	cached     string
	lookPath   func(string) (string, error)
	candidates []string
}

// New builds a Bridge with the production binary-discovery strategy:
// PATH first, then common Cargo/Homebrew/system locations.
func New() *Bridge {
	home, _ := os.UserHomeDir()
	return &Bridge{
		lookPath: exec.LookPath,
		candidates: []string{
			"/usr/local/bin/codegraph",
			"/opt/homebrew/bin/codegraph",
			filepath.Join(home, ".cargo", "bin", "codegraph"),
			filepath.Join(home, ".local", "bin", "codegraph"),
		},
	}
}

// Find returns the absolute path to the codegraph binary, caching the
// result. It returns ErrNotInstalled when nothing is found.
func (b *Bridge) Find() (string, error) {
	if b.cached != "" {
		if _, err := os.Stat(b.cached); err == nil {
			return b.cached, nil
		}
		b.cached = "" // stale cache, re-resolve
	}
	lp := b.lookPath
	if lp == nil {
		lp = exec.LookPath
	}
	if p, err := lp("codegraph"); err == nil && p != "" {
		b.cached = p
		return p, nil
	}
	for _, c := range b.candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			b.cached = c
			return c, nil
		}
	}
	return "", ErrNotInstalled
}

// Available reports whether codegraph can be located (no error returned).
func (b *Bridge) Available() bool {
	_, err := b.Find()
	return err == nil
}

// Run executes `codegraph <args...>` in workdir and returns combined
// stdout+stderr. ctx governs cancellation/timeout.
func (b *Bridge) Run(ctx context.Context, workdir string, args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("codegraph: no command given")
	}
	bin, err := b.Find()
	if err != nil {
		return "", err
	}
	if workdir == "" {
		workdir = "."
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = workdir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("codegraph: command timed out: %w", ctx.Err())
	}
	if runErr != nil {
		return out, fmt.Errorf("codegraph %s: %w", strings.Join(args, " "), runErr)
	}
	return out, nil
}

// Analyze runs `codegraph analyze --json <path>` and decodes the graph.
// It is the primary entry point used by the MCP tool surface.
func (b *Bridge) Analyze(ctx context.Context, path string) (*Graph, error) {
	if path == "" {
		path = "."
	}
	out, err := b.Run(ctx, path, []string{"analyze", "--json", "."})
	if err != nil {
		return nil, err
	}
	g, err := ParseGraph([]byte(out))
	if err != nil {
		return nil, fmt.Errorf("codegraph analyze: %w", err)
	}
	if g.Root == "" {
		g.Root = path
	}
	return g, nil
}

// ParseGraph decodes a CodeGraph JSON envelope. It is split out from
// Analyze so it can be unit-tested without the external binary.
func ParseGraph(data []byte) (*Graph, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, errors.New("empty analysis output")
	}
	var g Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("invalid graph json: %w", err)
	}
	return &g, nil
}

// Version returns `codegraph --version`.
func (b *Bridge) Version(ctx context.Context) (string, error) {
	return b.Run(ctx, ".", []string{"--version"})
}
