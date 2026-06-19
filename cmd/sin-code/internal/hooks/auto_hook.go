// SPDX-License-Identifier: MIT
// Purpose: auto-lint + auto-test hooks (issue #376). These hooks run after
// sin_write/sin_edit to format code and run tests. They are registered in
// chat_cmd.go when the feature is enabled. All errors degrade to warnings
// to avoid blocking the agent loop (mandate M3: verify gate is sacred).
package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
)

// AutoLintHook runs a language-appropriate formatter after sin_write/sin_edit.
type AutoLintHook struct {
	Enabled bool
}

// AutoTestHook runs tests after sin_write/sin_edit in the affected package/directory.
type AutoTestHook struct {
	Enabled     bool
	TimeoutSecs int
}

func (AutoLintHook) ID() string { return "auto-lint" }
func (AutoTestHook) ID() string { return "auto-test" }

func (AutoLintHook) Phases() []hooklife.Phase { return []hooklife.Phase{hooklife.PostToolUse} }
func (AutoTestHook) Phases() []hooklife.Phase { return []hooklife.Phase{hooklife.PostToolUse} }

func (h AutoLintHook) Run(ctx context.Context, ev hooklife.Event) hooklife.Decision {
	if !h.Enabled {
		return hooklife.Decision{Verdict: hooklife.Allow}
	}
	if ev.Tool != "sin_write" && ev.Tool != "sin_edit" {
		return hooklife.Decision{Verdict: hooklife.Allow}
	}
	pathStr := ev.Args["path"]
	if pathStr == "" {
		return hooklife.Decision{Verdict: hooklife.Allow}
	}
	return h.runFormatter(ctx, pathStr)
}

func (h AutoTestHook) Run(ctx context.Context, ev hooklife.Event) hooklife.Decision {
	if !h.Enabled {
		return hooklife.Decision{Verdict: hooklife.Allow}
	}
	if ev.Tool != "sin_write" && ev.Tool != "sin_edit" {
		return hooklife.Decision{Verdict: hooklife.Allow}
	}
	pathStr := ev.Args["path"]
	if pathStr == "" {
		return hooklife.Decision{Verdict: hooklife.Allow}
	}
	return h.runTests(ctx, pathStr)
}

func (AutoLintHook) runFormatter(ctx context.Context, filePath string) hooklife.Decision {
	ext := strings.ToLower(filepath.Ext(filePath))
	var cmd *exec.Cmd

	switch ext {
	case ".go":
		cmd = exec.CommandContext(ctx, "gofmt", "-w", filePath)
	case ".py":
		cmd = exec.CommandContext(ctx, "ruff", "format", filePath)
	case ".rs":
		cmd = exec.CommandContext(ctx, "rustfmt", filePath)
	case ".js", ".ts", ".jsx", ".tsx", ".json", ".css", ".html", ".md":
		cmd = exec.CommandContext(ctx, "prettier", "--write", filePath)
	default:
		return hooklife.Decision{Verdict: hooklife.Allow}
	}

	cmd.Dir = filepath.Dir(filePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return hooklife.Decision{
			Verdict: hooklife.Warn,
			Message: fmt.Sprintf("auto-lint failed for %s: %v", filePath, err),
		}
	}
	return hooklife.Decision{Verdict: hooklife.Allow}
}

func (h AutoTestHook) runTests(ctx context.Context, filePath string) hooklife.Decision {
	dir := filepath.Dir(filePath)
	ext := strings.ToLower(filepath.Ext(filePath))

	timeout := 30 * time.Second
	if h.TimeoutSecs > 0 {
		timeout = time.Duration(h.TimeoutSecs) * time.Second
	}

	var cmd *exec.Cmd
	switch ext {
	case ".go":
		cmd = exec.CommandContext(ctx, "go", "test", "-count=1", "-timeout=30s", "./...")
	case ".py":
		cmd = exec.CommandContext(ctx, "python", "-m", "pytest", "-x", dir)
	case ".rs":
		cmd = exec.CommandContext(ctx, "cargo", "test", "--quiet")
	case ".js", ".ts", ".jsx", ".tsx":
		cmd = exec.CommandContext(ctx, "npm", "test", "--", "--watch=false")
	default:
		return hooklife.Decision{Verdict: hooklife.Allow}
	}

	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd = exec.CommandContext(testCtx, cmd.Args[0], cmd.Args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return hooklife.Decision{
			Verdict: hooklife.Warn,
			Message: fmt.Sprintf("auto-test failed for %s: %v", dir, err),
		}
	}
	return hooklife.Decision{Verdict: hooklife.Allow}
}
