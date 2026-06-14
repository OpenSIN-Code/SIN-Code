// SPDX-License-Identifier: MIT
// Purpose: bridge to rtk (Rust Token Killer, https://github.com/rtk-ai/rtk),
// an external CLI proxy that filters command output to cut LLM token usage
// by 60-90% (issue #123). Follows the Bridged-External-Contract shared with
// gh/vane/dox: rtk is NEVER vendored — we shell out to the user's installed
// binary and fail with a clear install hint when it is missing.
//
// Docs: rtk.doc.md
package rtk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotInstalled is returned when no rtk binary can be located. The
// message carries the canonical install instructions.
var ErrNotInstalled = errors.New("rtk not found in PATH or standard locations; install from https://github.com/rtk-ai/rtk " +
	"(curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/main/install.sh | sh  •  or  brew install rtk)")

// Bridge locates and invokes the rtk binary. The zero value is NOT
// ready; use New. Fields are exported-via-constructor so tests can
// inject a fake lookPath and candidate list.
type Bridge struct {
	cached     string
	lookPath   func(string) (string, error)
	candidates []string
}

// New builds a Bridge with the production binary-discovery strategy:
// PATH first, then the common Cargo/Homebrew/system locations.
func New() *Bridge {
	home, _ := os.UserHomeDir()
	return &Bridge{
		lookPath: exec.LookPath,
		candidates: []string{
			"/usr/local/bin/rtk",
			"/opt/homebrew/bin/rtk", // Homebrew on Apple Silicon
			filepath.Join(home, ".cargo", "bin", "rtk"),
			filepath.Join(home, ".local", "bin", "rtk"),
		},
	}
}

// Find returns the absolute path to the rtk binary, caching the
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
	if p, err := lp("rtk"); err == nil && p != "" {
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

// Available reports whether rtk can be located (no error returned).
func (b *Bridge) Available() bool {
	_, err := b.Find()
	return err == nil
}

// Run executes `rtk <args...>` in workdir and returns the combined
// (filtered) stdout+stderr. A non-zero exit surfaces as an error that
// still carries the captured output for debugging. The provided ctx
// governs cancellation/timeout — callers set the deadline.
func (b *Bridge) Run(ctx context.Context, workdir string, args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("rtk: no command given")
	}
	bin, err := b.Find()
	if err != nil {
		return "", err
	}
	if workdir == "" {
		workdir = "."
	}
	cmd := exec.CommandContext(ctx, bin, args...) // #nosec G204
	cmd.Dir = workdir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	out := strings.TrimSpace(buf.String())
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("rtk: command timed out: %w", ctx.Err())
	}
	if runErr != nil {
		return out, fmt.Errorf("rtk %s: %w", strings.Join(args, " "), runErr)
	}
	return out, nil
}

// Version returns the output of `rtk --version`, or an error if rtk is
// missing or the call fails.
func (b *Bridge) Version(ctx context.Context) (string, error) {
	return b.Run(ctx, ".", []string{"--version"})
}
