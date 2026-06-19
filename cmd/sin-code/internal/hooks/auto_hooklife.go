// SPDX-License-Identifier: MIT
// Purpose: hooklife.Hook adapters for the auto-lint and auto-test
// post-edit automation (issue #376). These wrap the same logic as the
// PostListener variants in auto_hook.go but conform to the hooklife.Hook
// interface so they can be registered on a hooklife.Registry alongside
// the autoactivate hooks. The hooklife runner dispatches PostToolUse
// events after sin_write/sin_edit complete; the hook inspects ev.Args
// for the edited path and runs gofmt + go vet (lint) or `go test` (test)
// in the file's package directory. Errors degrade to Warn verdicts so
// the agent loop is never blocked (mandate M3: verify gate is sacred).
package hooks

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
)

// autoLintHookID is the stable hook identifier surfaced by the runner.
const autoLintHookID = "auto-lint"

// AutoLintHook is a PostToolUse hooklife.Hook that runs gofmt + go vet
// on .go files touched by sin_write/sin_edit. It mirrors AutoLintListener
// but conforms to hooklife.Hook for the second-phase hooking system.
type AutoLintHook struct {
	Enabled bool
	// Timeout overrides AutoLintDefaultTimeout when non-zero.
	Timeout time.Duration
}

func (AutoLintHook) ID() string { return autoLintHookID }

func (AutoLintHook) Phases() []hooklife.Phase {
	return []hooklife.Phase{hooklife.PostToolUse}
}

func (h AutoLintHook) Run(ctx context.Context, ev hooklife.Event) hooklife.Decision {
	if !h.Enabled || !isEditTool(ev.Tool) {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: autoLintHookID}
	}
	path := ev.Args["path"]
	if path == "" || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: autoLintHookID}
	}
	workdir := ev.Workdir
	if workdir == "" {
		workdir = "."
	}
	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workdir, path)
	}
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = AutoLintDefaultTimeout
	}
	reports := runLintCommands(ctx, absPath, workdir, timeout)
	if len(reports) == 0 {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: autoLintHookID}
	}
	return hooklife.Decision{
		Verdict: hooklife.Warn,
		HookID:  autoLintHookID,
		Message: fmt.Sprintf("[auto-lint %s] %s", path, strings.Join(reports, "; ")),
	}
}

// autoTestHookID is the stable hook identifier surfaced by the runner.
const autoTestHookID = "auto-test"

// AutoTestHook is a PostToolUse hooklife.Hook that runs `go test` on the
// enclosing package whenever a *_test.go file is touched by sin_write/
// sin_edit. It mirrors AutoTestListener but conforms to hooklife.Hook.
type AutoTestHook struct {
	Enabled     bool
	TimeoutSecs int
}

func (AutoTestHook) ID() string { return autoTestHookID }

func (AutoTestHook) Phases() []hooklife.Phase {
	return []hooklife.Phase{hooklife.PostToolUse}
}

func (h AutoTestHook) Run(ctx context.Context, ev hooklife.Event) hooklife.Decision {
	if !h.Enabled || !isEditTool(ev.Tool) {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: autoTestHookID}
	}
	path := ev.Args["path"]
	if path == "" || !strings.HasSuffix(path, "_test.go") {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: autoTestHookID}
	}
	workdir := ev.Workdir
	if workdir == "" {
		workdir = "."
	}
	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workdir, path)
	}
	timeout := AutoTestDefaultTimeout
	if h.TimeoutSecs > 0 {
		timeout = time.Duration(h.TimeoutSecs) * time.Second
	}
	report := runTestCommand(ctx, absPath, workdir, timeout)
	if report == "" {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: autoTestHookID}
	}
	if len(report) > 4096 {
		report = report[:4096] + "\n[... truncated; rerun `sin_test` for the full log]"
	}
	return hooklife.Decision{
		Verdict: hooklife.Warn,
		HookID:  autoTestHookID,
		Message: fmt.Sprintf("[auto-test %s] FAIL: %s", path, report),
	}
}

// isEditTool returns true for the tool names that represent file edits.
// Both the legacy "sin_write"/"sin_edit" names and the hooklife-style
// "Write"/"Edit" names are accepted so the hook works regardless of
// which dispatch path fires it.
func isEditTool(name string) bool {
	switch name {
	case "sin_write", "sin_edit", "Write", "Edit":
		return true
	}
	return false
}
