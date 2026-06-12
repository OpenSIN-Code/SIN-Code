// SPDX-License-Identifier: MIT
// Purpose: mandatory verification gate (mandate M3, AGENTS.md §8).
// The agent loop MUST NOT report success until this gate passes (default poc).
package verify

import (
	"context"
	"fmt"
	"strings"
)

type Mode string

const (
	ModePoC    Mode = "poc"
	ModeOracle Mode = "oracle"
	ModeOff    Mode = "off"
)

type Result struct {
	Passed bool   `json:"passed"`
	Report string `json:"report"`
	Mode   Mode   `json:"mode"`
}

// Runner executes a verification backend against the workspace.
type Runner func(ctx context.Context, workspace string) (passed bool, report string, err error)

type Gate struct {
	mode    Mode
	runners map[Mode]Runner
}

func NewGate(mode string, poc, oracle Runner) *Gate {
	m := Mode(strings.ToLower(strings.TrimSpace(mode)))
	if m != ModePoC && m != ModeOracle && m != ModeOff {
		m = ModePoC
	}
	return &Gate{
		mode:    m,
		runners: map[Mode]Runner{ModePoC: poc, ModeOracle: oracle},
	}
}

func (g *Gate) Mode() Mode { return g.mode }

// SetMode allows a slash command override (mandate C8).
func (g *Gate) SetMode(m Mode) {
	if m == ModePoC || m == ModeOracle || m == ModeOff {
		g.mode = m
	}
}

// Run executes the configured verification. ModeOff always passes.
func (g *Gate) Run(ctx context.Context, workspace string) Result {
	if g.mode == ModeOff {
		return Result{Passed: true, Mode: ModeOff, Report: "verification disabled"}
	}
	runner := g.runners[g.mode]
	if runner == nil {
		return Result{Passed: false, Mode: g.mode,
			Report: fmt.Sprintf("no runner wired for verify_mode=%s", g.mode)}
	}
	passed, report, err := runner(ctx, workspace)
	if err != nil {
		return Result{Passed: false, Mode: g.mode, Report: "verifier error: " + err.Error()}
	}
	return Result{Passed: passed, Mode: g.mode, Report: report}
}
