// SPDX-License-Identifier: MIT
// Purpose: built-in hooks — block-no-verify, config-protection,
// post-edit-format, post-edit-typecheck, quality-gate, cost-tracker,
// suggest-compact. Each is a thin wrapper around an existing
// SIN-Code subsystem via a small interface; concrete implementations
// live in internal/adapters.
// Docs: builtin.doc.md
package hooklife

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

// execCommandContext is overridable in tests so the error branch of
// PostEditFormat is deterministic without touching real formatters.
var execCommandContext = exec.CommandContext

// --- block-no-verify: refuse commits that skip verification ---

type BlockNoVerify struct{}

func (BlockNoVerify) ID() string      { return "block-no-verify" }
func (BlockNoVerify) Phases() []Phase { return []Phase{PreToolUse} }
func (BlockNoVerify) Run(_ context.Context, ev Event) Decision {
	if ev.Tool != "Bash" {
		return Decision{Verdict: Allow}
	}
	cmd := ev.Args["command"]
	if strings.Contains(cmd, "--no-verify") || strings.Contains(cmd, "-n") && strings.Contains(cmd, "git commit") {
		return Decision{Verdict: Block, Message: "git commit --no-verify is not allowed; fix the failing checks instead."}
	}
	return Decision{Verdict: Allow}
}

// --- config-protection: block edits to protected paths ---

type ConfigProtection struct {
	Protected []string // e.g. [".git/", "go.sum", ".env"]
}

func (ConfigProtection) ID() string      { return "config-protection" }
func (ConfigProtection) Phases() []Phase { return []Phase{PreToolUse} }
func (c ConfigProtection) Run(_ context.Context, ev Event) Decision {
	if ev.Tool != "Edit" && ev.Tool != "Write" {
		return Decision{Verdict: Allow}
	}
	p := ev.Args["path"]
	for _, prot := range c.Protected {
		if strings.Contains(p, prot) {
			return Decision{Verdict: Block, Message: "refusing to modify protected path: " + p}
		}
	}
	return Decision{Verdict: Allow}
}

// --- post-edit-format: format files after edits ---

type PostEditFormat struct {
	// Formatter maps a file extension to a command template; %s = file path.
	Formatter map[string]string
}

func DefaultFormatters() map[string]string {
	return map[string]string{
		".go":  "gofmt -w %s",
		".rs":  "rustfmt %s",
		".py":  "ruff format %s",
		".ts":  "prettier --write %s",
		".tsx": "prettier --write %s",
		".js":  "prettier --write %s",
	}
}

func (PostEditFormat) ID() string      { return "post-edit-format" }
func (PostEditFormat) Phases() []Phase { return []Phase{PostToolUse} }
func (f PostEditFormat) Run(ctx context.Context, ev Event) Decision {
	if ev.Tool != "Edit" && ev.Tool != "Write" {
		return Decision{Verdict: Allow}
	}
	path := ev.Args["path"]
	tmpl, ok := f.Formatter[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return Decision{Verdict: Allow}
	}
	parts := strings.Fields(strings.Replace(tmpl, "%s", path, 1))
	if len(parts) == 0 {
		return Decision{Verdict: Allow}
	}
	if err := execCommandContext(ctx, parts[0], parts[1:]...).Run(); err != nil {
		return Decision{Verdict: Warn, Message: "format failed for " + path + ": " + err.Error()}
	}
	return Decision{Verdict: Allow}
}

// --- post-edit-typecheck: hook into your LSP/type checker ---

// TypeChecker is satisfied by SIN-Code's existing internal/lsp client.
type TypeChecker interface {
	Diagnostics(ctx context.Context, path string) (errs []string, err error)
}

type PostEditTypecheck struct{ Checker TypeChecker }

func (PostEditTypecheck) ID() string      { return "post-edit-typecheck" }
func (PostEditTypecheck) Phases() []Phase { return []Phase{PostToolUse} }
func (t PostEditTypecheck) Run(ctx context.Context, ev Event) Decision {
	if t.Checker == nil || (ev.Tool != "Edit" && ev.Tool != "Write") {
		return Decision{Verdict: Allow}
	}
	errs, err := t.Checker.Diagnostics(ctx, ev.Args["path"])
	if err != nil {
		return Decision{Verdict: Allow}
	}
	if len(errs) > 0 {
		return Decision{Verdict: Warn, Message: "type errors:\n" + strings.Join(errs, "\n")}
	}
	return Decision{Verdict: Allow}
}

// --- quality-gate: run verification before commit ---

// Verifier is satisfied by SIN-Code's existing internal/verify package.
type Verifier interface {
	QualityGate(ctx context.Context, workdir string) (passed bool, report string, err error)
}

type QualityGate struct{ Verifier Verifier }

func (QualityGate) ID() string      { return "quality-gate" }
func (QualityGate) Phases() []Phase { return []Phase{PreToolUse} }
func (q QualityGate) Run(ctx context.Context, ev Event) Decision {
	if q.Verifier == nil || ev.Tool != "Bash" {
		return Decision{Verdict: Allow}
	}
	if !strings.Contains(ev.Args["command"], "git commit") {
		return Decision{Verdict: Allow}
	}
	passed, report, err := q.Verifier.QualityGate(ctx, ev.Workdir)
	if err != nil {
		return Decision{Verdict: Warn, Message: "quality gate error: " + err.Error()}
	}
	if !passed {
		return Decision{Verdict: Block, Message: "quality gate failed:\n" + report}
	}
	return Decision{Verdict: Allow}
}

// --- cost-tracker: record spend per tool/session ---

// Ledger is satisfied by SIN-Code's existing internal/ledger package.
type Ledger interface {
	Track(tool string, meta map[string]string)
}

type CostTracker struct{ Ledger Ledger }

func (CostTracker) ID() string      { return "cost-tracker" }
func (CostTracker) Phases() []Phase { return []Phase{PostToolUse} }
func (c CostTracker) Run(_ context.Context, ev Event) Decision {
	if c.Ledger != nil {
		c.Ledger.Track(ev.Tool, ev.Meta)
	}
	return Decision{Verdict: Allow}
}

// --- suggest-compact: warn when context is getting large ---

type SuggestCompact struct {
	// TokensUsed returns current context size; wire to internal/headroom.
	TokensUsed func() int
	Threshold  int // e.g. 150000
}

func (SuggestCompact) ID() string      { return "suggest-compact" }
func (SuggestCompact) Phases() []Phase { return []Phase{Stop, PreCompact} }
func (s SuggestCompact) Run(_ context.Context, _ Event) Decision {
	if s.TokensUsed == nil || s.Threshold <= 0 {
		return Decision{Verdict: Allow}
	}
	if s.TokensUsed() >= s.Threshold {
		return Decision{Verdict: Warn, Message: "context is large; consider /compact"}
	}
	return Decision{Verdict: Allow}
}
