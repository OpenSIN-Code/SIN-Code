// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when loop is refactored
// Purpose: core type definitions for the agent loop package. Extracted
// from loop.go for readability — pure file split, same package.
package agentloop

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

type Completion struct {
	Text      string
	ToolCalls []ToolCall
	Raw       session.Message
	// Usage carries token accounting returned by the model provider. All
	// fields optional; zero values mean "unknown" and never trigger the
	// budget guard (issue #151).
	Usage Usage
}

// Usage carries token accounting returned by the model provider. All fields
// optional; zero values mean "unknown" and never trigger the budget guard.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// ThinkingTokens is the count of tokens the model spent on its
	// internal reasoning phase (Claude / Anthropic-style providers,
	// OpenRouter gateways). Zero means "unknown / not surfaced" and
	// is never treated as a budget signal.
	ThinkingTokens int
}

type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type LocalToolFunc func(ctx context.Context, name string, args map[string]any) (string, error)

type AskFunc func(tc ToolCall) bool

// StopSnapshot is the read-only view of a run handed to the stop-gate when
// the worker proposes completion (no more tool calls AND the verify-gate
// passed). It carries just enough signal for an independent evaluator to
// decide whether the goal is truly done.
type StopSnapshot struct {
	Prompt       string
	FinalOutput  string
	Turns        int
	ToolsUsed    []string
	VerifyPassed bool
	SessionID    string
}

// StopDecision is the verdict returned by a StopGate. Complete=false forces
// the loop to keep working, re-injecting OpenCriteria as the next instruction.
type StopDecision struct {
	Complete     bool
	OpenCriteria []string
	Report       string
}

// StopGate decouples completion authority from the worker. It is consulted
// only AFTER the verify-gate passes, and may reject the proposed completion
// (Complete=false) to force continued work — the core anti-babysitting hook.
// A nil StopGate preserves the legacy behavior exactly.
type StopGate func(ctx context.Context, snap StopSnapshot) StopDecision

// Reflection is the worker's self-critique of a proposed completion. Issues
// non-empty means the agent found problems in its own work and should fix
// them before the stop-gate is consulted.
type Reflection struct {
	Issues []string
	Notes  string
}

// Reflector performs a self-critique pass on a proposed completion. Returning
// a Reflection with non-empty Issues forces one more work turn. A nil
// Reflector disables the reflection step (legacy behavior).
// Issue #152.
type Reflector func(ctx context.Context, snap StopSnapshot) Reflection

// TournamentRunner is the interface for fusion verify-tournaments (issue
// #290). The loop calls ShouldRun to check if a verify-fail warrants a
// tournament fan-out, and Run to execute it. The prompt is passed so the
// tournament can fan it out to each provider — without it, forked sessions
// would run with an empty task. On success, Run returns the winner's output
// and token count. On failure, the loop falls back to the legacy same-model
// retry. Defined here (not in internal/fusion) to avoid a circular import
// (fusion imports agentloop for Result).
type TournamentRunner interface {
	ShouldRun(vr verify.Result) bool
	Run(ctx context.Context, prompt string) (output string, tokens int, err error)
}

type Result struct {
	SessionID string `json:"session_id"`
	Summary   string `json:"summary"`
	Verified  bool   `json:"verified"`
	Turns     int    `json:"turns"`
	Tokens    int    `json:"tokens,omitempty"`
	// Continuation is true when the run hit maxTurns with AllowContinuation
	// enabled: the work is checkpointed (not failed) and should be resumed.
	Continuation bool `json:"continuation,omitempty"`
	// OpenCriteria carries the unmet acceptance criteria when the run ends
	// without verified completion (stop-gate reject or continuation).
	OpenCriteria []string `json:"open_criteria,omitempty"`
}
