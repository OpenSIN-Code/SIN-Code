// SPDX-License-Identifier: MIT
// Purpose: Specialized sub-agent modes (issue #485). Each mode changes the
// system prompt emphasis and restricts the tool set to match the task focus.
// The `default` mode preserves exact current behavior — all other modes are
// additive restrictions on top of the permission engine (M4).
package agentmode

import (
	"fmt"
	"sort"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
)

// Mode is the string enum for specialized agent modes.
type Mode string

const (
	ModeDefault   Mode = "default"
	ModeArchitect Mode = "architect"
	ModeDebug     Mode = "debug"
	ModeCode      Mode = "code"
	ModeReview    Mode = "review"
)

// modeDef describes a mode's system-prompt prefix and tool blacklist.
// The blacklist is subtractive: tools in it are removed from the spec list.
// M4: restricted modes still go through the permission engine — mode
// filtering is additive, never a bypass.
type modeDef struct {
	name        Mode
	promptPrefix string
	// blacklistedTools are tool names that are NOT allowed in this mode.
	// Empty = all tools allowed (default, code).
	blacklistedTools map[string]bool
}

var modeDefs = map[Mode]modeDef{
	ModeDefault: {
		name:        ModeDefault,
		promptPrefix: "",
		// No restrictions — full tool surface, exact current behavior.
	},
	ModeArchitect: {
		name: ModeArchitect,
		promptPrefix: `You are operating in ARCHITECT mode. Your focus is planning, design, and architectural analysis.
Do NOT write or modify files. Analyze structure, patterns, tradeoffs, and design decisions.
Produce clear architectural recommendations, diagrams (as text), and rationale.
Use read-only tools (sin_read, sin_scout, sin_map, sin_grasp, sin_discover, sin_sckg) to understand the codebase.
Explain tradeoffs, risks, and alternatives. Think about scalability, maintainability, and extensibility.`,
		blacklistedTools: map[string]bool{
			"sin_write":      true,
			"sin_edit":       true,
			"sin_bash":       true,
			"sin_git_commit": true,
		},
	},
	ModeDebug: {
		name: ModeDebug,
		promptPrefix: `You are operating in DEBUG mode. Your focus is root-cause analysis, tracing, and diagnostics.
Do NOT write or modify files. Use read-only tools to trace execution, analyze logs, and identify root causes.
Examine code paths, follow data flow, and pinpoint the exact source of bugs.
Use sin_read, sin_scout, sin_sckg, sin_grasp to navigate. Use sin_bash only for read-only diagnostic commands
(grep, cat, git log, git diff — never write/modify). Propose fixes as descriptions, not patches.`,
		blacklistedTools: map[string]bool{
			"sin_write":      true,
			"sin_edit":       true,
			"sin_git_commit": true,
		},
	},
	ModeCode: {
		name: ModeCode,
		promptPrefix: `You are operating in CODE mode. Your focus is clean implementation, tests, and verification.
Write production-quality code with proper error handling and clear naming.
Always write tests for new functionality. Run verification after changes.
Follow existing code conventions. Prefer SIN tools over naive built-ins (M6).`,
		// Full tools — same as default but with a different prompt emphasis.
	},
	ModeReview: {
		name: ModeReview,
		promptPrefix: `You are operating in REVIEW mode. Your focus is code review: security, performance, and best practices.
Do NOT write or modify files. Do NOT execute bash commands. Use only read-only tools.
Analyze code for security vulnerabilities (OWASP top 10), performance bottlenecks, and best-practice violations.
Use sin_read, sin_scout, sin_sckg, sin_grasp to examine code. Report findings with severity levels.
Structure your review as: Security, Performance, Correctness, Maintainability, Style.`,
		blacklistedTools: map[string]bool{
			"sin_write":      true,
			"sin_edit":       true,
			"sin_bash":       true,
			"sin_git_commit": true,
		},
	},
}

// GetMode looks up a mode by name. Returns an error if the name is not
// a valid mode. Empty string defaults to ModeDefault (non-breaking).
func GetMode(name string) (Mode, error) {
	if name == "" {
		return ModeDefault, nil
	}
	m := Mode(name)
	if _, ok := modeDefs[m]; !ok {
		return "", fmt.Errorf("unknown agent mode %q: valid modes are %s", name, ValidModes())
	}
	return m, nil
}

// ValidModes returns a sorted, comma-separated list of all valid mode names.
func ValidModes() string {
	names := make([]string, 0, len(modeDefs))
	for m := range modeDefs {
		names = append(names, string(m))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// SystemPrompt returns the system-prompt prefix for this mode.
// For ModeDefault this is the empty string (non-breaking).
func (m Mode) SystemPrompt() string {
	def, ok := modeDefs[m]
	if !ok {
		return ""
	}
	return def.promptPrefix
}

// IsToolAllowed reports whether the named tool is allowed in this mode.
// M4: this is additive — the permission engine still gates every tool call.
// A tool allowed here may still be denied by the permission engine.
func (m Mode) IsToolAllowed(toolName string) bool {
	def, ok := modeDefs[m]
	if !ok {
		return true
	}
	if def.blacklistedTools == nil {
		return true
	}
	return !def.blacklistedTools[toolName]
}

// FilterTools returns the subset of specs that are allowed in this mode.
// For ModeDefault and ModeCode this returns the input unchanged (non-breaking).
func (m Mode) FilterTools(specs []agentloop.ToolSpec) []agentloop.ToolSpec {
	def, ok := modeDefs[m]
	if !ok || len(def.blacklistedTools) == 0 {
		return specs
	}
	filtered := make([]agentloop.ToolSpec, 0, len(specs))
	for _, s := range specs {
		if !def.blacklistedTools[s.Name] {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// IsRestricted reports whether this mode restricts the tool set
// (i.e. has a non-empty blacklist). Used by callers to decide whether
// to announce the restriction.
func (m Mode) IsRestricted() bool {
	def, ok := modeDefs[m]
	if !ok {
		return false
	}
	return len(def.blacklistedTools) > 0
}

// String returns the mode name as a string.
func (m Mode) String() string {
	return string(m)
}
