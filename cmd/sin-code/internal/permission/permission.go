// SPDX-License-Identifier: MIT
// Purpose: allow/ask/deny engine that gates every tool call (mandate M4,
// AGENTS.md §8). Yolo bypasses Ask (never Deny). Headless: Ask -> Deny
// unless Yolo.
package permission

import (
	"path"
	"strings"
)

type Policy int

const (
	Deny Policy = iota
	Ask
	Allow
)

func (p Policy) String() string {
	switch p {
	case Allow:
		return "allow"
	case Ask:
		return "ask"
	default:
		return "deny"
	}
}

type Rule struct {
	Tool   string `json:"tool"`
	Policy string `json:"policy"`
}

type Mode string

const (
	// ModeDefault is the legacy behavior: every Ask is resolved by
	// Yolo / Headless. The operator sees an Ask prompt for any
	// non-allow rule.
	ModeDefault Mode = "default"
	// ModePlan is the conservative "review-before-acting" mode
	// (issue #193). It does NOT change rule matching — it only
	// forces mutating tools to Ask even if the rules say Allow.
	// Read-only tools stay Allow.
	ModePlan Mode = "plan"
	// ModeAcceptEdits is the "trust the agent for file edits" mode.
	// Edit/Write become Allow; everything else stays by-rule.
	ModeAcceptEdits Mode = "acceptEdits"
	// ModeBypass is the "trust the agent for everything" mode.
	// Every Allow-list tool is allowed; Deny is NEVER bypassed
	// (the same guarantee as Yolo, but exposed as a stable
	// mode rather than a CLI flag).
	ModeBypass Mode = "bypass"
)

type Engine struct {
	rules []Rule
	// Yolo bypasses every Ask (headless --yolo). Deny is NEVER bypassed.
	Yolo bool
	// Headless: Ask resolves to Deny unless Yolo is set.
	Headless bool
	// Mode is a session-wide permission mode (issue #193). Default
	// (empty) preserves legacy behavior. When set, the mode
	// reshapes the effective policy before the Yolo/Headless
	// fallback runs.
	Mode Mode
}

func New(rules []Rule) *Engine {
	return &Engine{rules: rules}
}

// SetMode changes the engine's mode at runtime. Returns an error
// for unknown modes.
func (e *Engine) SetMode(m Mode) error {
	switch m {
	case ModeDefault, ModePlan, ModeAcceptEdits, ModeBypass, "":
		e.Mode = m
		return nil
	default:
		return fmtErrorf("permission: unknown mode %q", string(m))
	}
}

// Check returns the effective policy for a tool name.
// First matching rule wins; no match defaults to Ask. The mode
// then reshapes the result:
//   - plan:     Edit/Write/Bash become Ask (read-only tools stay Allow)
//   - acceptEdits: Edit/Write become Allow; everything else stays
//   - bypass:   every Allow-list tool becomes Allow; Deny is NEVER
//               overridden
func (e *Engine) Check(tool string) Policy {
	p := Ask
	for _, r := range e.rules {
		if matched, _ := path.Match(strings.ToLower(r.Tool), strings.ToLower(tool)); matched {
			p = parse(r.Policy)
			break
		}
	}
	// Mode reshape (issue #193). Each mode is a no-op on the legacy
	// "default" mode.
	switch e.Mode {
	case ModePlan:
		// Force mutating tools to Ask regardless of the rule.
		if isMutatingTool(tool) {
			p = Ask
		}
	case ModeAcceptEdits:
		// Edit/Write are allowed; everything else stays.
		if isEditTool(tool) {
			p = Allow
		}
	case ModeBypass:
		// Every Allow-list tool becomes Allow; Deny stays Deny.
		if p != Deny {
			p = Allow
		}
	}
	if p == Ask {
		if e.Yolo {
			return Allow
		}
		if e.Headless {
			return Deny
		}
	}
	return p
}

// isMutatingTool returns true for tools that change files or
// execute commands. The list is conservative — anything we don't
// recognize is treated as mutating to be safe.
func isMutatingTool(tool string) bool {
	t := strings.ToLower(tool)
	// Read-only tools
	readOnly := map[string]bool{
		"read": true, "ls": true, "glob": true, "grep": true,
		"cat": true, "head": true, "tail": true, "find": true,
		"git_status": true, "git_log": true, "git_diff": true,
		"git_show": true, "git_branch": true,
	}
	if readOnly[t] {
		return false
	}
	// Everything else (Edit, Write, Bash, ...) is mutating.
	return true
}

// isEditTool returns true for file-editing tools.
func isEditTool(tool string) bool {
	t := strings.ToLower(tool)
	return strings.HasPrefix(t, "edit") || strings.HasPrefix(t, "write") || t == "multi_edit"
}

// fmtErrorf is a tiny local error helper to avoid importing fmt
// just for a single error string. (permission package keeps its
// import set minimal.)
func fmtErrorf(format string, args ...any) error {
	return &fmtError{format: format, args: args}
}

type fmtError struct {
	format string
	args   []any
}

func (e *fmtError) Error() string {
	// Simple %s/%v formatting without depending on fmt.
	out := e.format
	for _, a := range e.args {
		// Find the first %v or %s and replace. Single-pass; good
		// enough for the one use site.
		for i := 0; i+1 < len(out); i++ {
			if out[i] == '%' && (out[i+1] == 'v' || out[i+1] == 's' || out[i+1] == 'q') {
				out = out[:i] + stringFromAny(a) + out[i+2:]
				break
			}
		}
	}
	return out
}

// stringFromAny converts a Go value to a string for the error
// formatter. Only handles the few types we use (string, error).
func stringFromAny(a any) string {
	switch v := a.(type) {
	case string:
		return v
	case error:
		return v.Error()
	default:
		return "<unprintable>"
	}
}

func parse(s string) Policy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "allow":
		return Allow
	case "ask":
		return Ask
	default:
		return Deny
	}
}
