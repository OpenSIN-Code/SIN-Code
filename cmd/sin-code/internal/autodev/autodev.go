// SPDX-License-Identifier: MIT
// Purpose: autodev bridge — stdlib-only binary discovery + version probe
// for the OpenSIN-Code/autodev-cli (Python, MIT, v0.4.0) CLI/MCP. The
// bridge itself is the runtime subprocess boundary; Python sources are
// never vendored (M2: single static Go binary, CGO_ENABLED=0, and
// "NOT a place to vendor tool implementations that live in their own
// repos" — AGENTS.md §2). Stdlib-only imports: os, context, os/exec,
// errors, fmt, strings. No modelcontextprotocol/go-sdk here — that
// dependency belongs to mcpclient/registry.go callers, not the bridge.
// Docs: autodev.doc.md
package autodev

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Env names for binary overrides (highest priority). Set these to point
// the bridge at locally-built / pinned copies without rebuilding sin-code.
const (
	envBin    = "AUTODEV_BIN"     // overrides DefaultBin()
	envMCPBin = "AUTODEV_MCP_BIN" // overrides DefaultMCPBin()
)

// DefaultBin returns the autodev CLI binary path the bridge will
// invoke, honoring $AUTODEV_BIN when set and non-empty (trimmed).
// Falls back to "autodev" (PATH-resolved at exec time).
func DefaultBin() string {
	if v := strings.TrimSpace(os.Getenv(envBin)); v != "" {
		return v
	}
	return "autodev"
}

// DefaultMCPBin returns the autodev-mcp stdio server binary, honoring
// $AUTODEV_MCP_BIN. Falls back to "autodev-mcp".
func DefaultMCPBin() string {
	if v := strings.TrimSpace(os.Getenv(envMCPBin)); v != "" {
		return v
	}
	return "autodev-mcp"
}

// IsInstalled reports whether bin resolves on PATH (or as an absolute
// path). Safe on empty / whitespace-only input (returns false).
func IsInstalled(bin string) bool {
	if strings.TrimSpace(bin) == "" {
		return false
	}
	_, err := exec.LookPath(bin)
	return err == nil
}

// ErrNotInstalled is returned by ResolveAutodevBin / ResolveAutodevMCPBin
// when the resolved binary is missing. Wrapped exec.ErrNotFound so
// callers can errors.Is() either layer.
var ErrNotInstalled = errors.New("autodev: binary not installed")

// ResolveAutodevBin returns ErrNotInstalled (wrapping exec.ErrNotFound)
// when DefaultBin() does not resolve on PATH or as an absolute path.
func ResolveAutodevBin() error { return resolve(DefaultBin()) }

// ResolveAutodevMCPBin returns ErrNotInstalled when DefaultMCPBin() is
// absent. Identical semantics to ResolveAutodevBin; split into two
// functions so callers can tick the right install gate independently.
func ResolveAutodevMCPBin() error { return resolve(DefaultMCPBin()) }

// Version shells out to `autodev --version` (per the upstream bridge
// contract) and returns the trimmed stdout on clean exit. Any non-zero
// exit code, non-empty stderr, or empty stdout becomes an error so the
// caller can branch on upstream presence. Stdlib-only — no JSON, no
// regex, no side imports.
func Version() (string, error) {
	return versionWith(context.Background())
}

// versionWith is the context-aware test seam. Production code calls
// Version(); tests inject deadlines or background as needed.
func versionWith(ctx context.Context) (string, error) {
	bin := DefaultBin()
	cmd := exec.CommandContext(ctx, bin, "--version")
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Prefer upstream's stderr (rich diagnostics) over the wrapped
		// exit error so the operator sees WHY upstream rejected the
		// flag (e.g. "No such option --version" until upstream lands
		// one — tracked upstream).
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("autodev: %s --version failed: %w: %s", bin, err, msg)
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("autodev: %s --version returned empty stdout", bin)
	}
	return out, nil
}

// resolve is the shared body of the two Resolve* funcs. Centralised so
// the error-wrapping rule (ErrNotInstalled wraps exec.ErrNotFound) is
// impossible to forget in a new Resolve path.
func resolve(bin string) error {
	if strings.TrimSpace(bin) == "" {
		return fmt.Errorf("%w: empty bin name", ErrNotInstalled)
	}
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrNotInstalled, bin, err)
	}
	return nil
}
