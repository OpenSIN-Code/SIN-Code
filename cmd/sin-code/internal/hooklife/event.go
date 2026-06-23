// SPDX-License-Identifier: MIT
// Purpose: lifecycle event types and the Decision contract returned by
// every Hook. Three verdicts — Allow, Warn, Block — mirror ECC's
// exit-code semantics.
// Docs: event.doc.md
package hooklife

import "context"

// Phase identifies a lifecycle point, mirroring ECC's hook events.
type Phase string

const (
	PreToolUse   Phase = "PreToolUse"   // before a tool runs; may BLOCK
	PostToolUse  Phase = "PostToolUse"  // after a tool runs; may WARN
	Stop         Phase = "Stop"         // turn finished
	SessionStart Phase = "SessionStart" // session begins
	SessionEnd   Phase = "SessionEnd"   // session ends
	PreCompact   Phase = "PreCompact"   // before context compaction
	UserPrompt   Phase = "UserPrompt"   // a user prompt was submitted
)

// Verdict is the outcome a PreToolUse hook can return.
type Verdict int

const (
	Allow Verdict = iota // proceed
	Warn                 // proceed but surface a message
	Block                // stop the tool (ECC exit-code-2 equivalent)
)

func (v Verdict) String() string {
	switch v {
	case Block:
		return "block"
	case Warn:
		return "warn"
	default:
		return "allow"
	}
}

// Event is the payload passed to every hook.
type Event struct {
	Phase   Phase
	Tool    string            // tool name (Bash, Edit, Write, ...)
	Args    map[string]string // tool arguments (command, path, content, ...)
	Workdir string
	Meta    map[string]string // free-form context
}

// Decision is returned by a hook.
type Decision struct {
	Verdict Verdict
	Message string // shown to user/agent; required when Verdict != Allow
	HookID  string
}

// Hook is a single lifecycle handler.
type Hook interface {
	ID() string
	Phases() []Phase
	Run(ctx context.Context, ev Event) Decision
}
