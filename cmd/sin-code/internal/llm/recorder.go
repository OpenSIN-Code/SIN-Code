// SPDX-License-Identifier: MIT
// Purpose: LLM-Usage Recorder interface for the observability system
// (issue #168). Decouples the LLM client from the storage layer
// so any persistence backend (SQLite, file, no-op) can be plugged
// in without changing cmd/sin-code/internal/llm/provider.go.
//
// The Recorder is called once per LLM request, with the
// ChatResponse.Usage already parsed. The default NopRecorder
// drops every event (used when no store is wired, so the LLM
// client always has a valid Recorder to call).
//
// Docs: docs/TOKEN-TRACKING.md
package llm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// randomSessionID returns a hex-encoded 64-bit random session id.
// Generated once per process; cached for the lifetime of the
// binary so every LLM call within the same process shares the
// default id.
func randomSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should never fail on a sane OS, but if it
		// does we fall back to a fixed sentinel. The Recorder
		// contract is best-effort; a missing session id is
		// logged but never breaks the LLM call.
		return "session-fallback"
	}
	return hex.EncodeToString(b[:])
}

// Source is a coarse-grained taxonomy for usage events. The exact
// value set is deliberately small — anything finer-grained belongs
// in Tags on a future Event type, not in the Source enum.
type Source string

const (
	// SourceAdHoc marks one-off LLM calls from CLI commands
	// (sin-code tokens, sin-code spec author, etc.). Distinct
	// from SourceChat so dashboards can surface "spike in ad-hoc
	// calls" without confusing it with the long-running chat loop.
	SourceAdHoc Source = "ad-hoc"

	// SourceChat marks interactive chat sessions (sin-code chat,
	// TUI, WebUI). Long-running, may have many turns.
	SourceChat Source = "chat"

	// SourceAgent marks autonomous runs (sin-code daemon, eval-set
	// runner, PRP engine). Burst-y, often offline.
	SourceAgent Source = "agent"
)

// Recorder persists a single usage event. Implementations must be
// safe for concurrent use across many LLM calls (the LLM client
// itself runs goroutines for stream + retry).
//
// The default NopRecorder is the no-op implementation. Wire a real
// recorder via ClientOptions.Recorder when you want persistence.
type Recorder interface {
	// RecordUsage persists a single usage event. Returns an error
	// if the underlying store fails; the LLM client logs but
	// does not propagate the error (a failed usage write must not
	// break the user's request).
	RecordUsage(ctx context.Context, sessionID, model string, source Source, promptTokens, completionTokens, totalTokens int) error
}

// NopRecorder is the default Recorder. It implements the interface
// but does nothing — used when no recorder is wired. Cheap (a
// single function call with no allocation, no I/O), so it's safe
// to use in the hot path of the LLM client without measurable cost.
type NopRecorder struct{}

// RecordUsage for NopRecorder is a no-op. Always returns nil.
func (NopRecorder) RecordUsage(_ context.Context, _, _ string, _ Source, _, _, _ int) error {
	return nil
}

// SessionIDFromContext extracts a stable per-session identifier
// from the context. Returns "" if no session ID has been set.
//
// The convention is: callers (chat command, daemon, etc.) attach
// the session ID via WithSessionID(ctx, id) on every LLM call.
// The Recorder uses this to group usage by session in dashboards.
func SessionIDFromContext(ctx context.Context) string {
	if v := ctx.Value(sessionIDKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// WithSessionID returns a child context that carries the given
// session ID. Use it for every LLM call that belongs to a
// conversational session (chat, agent, daemon tick, etc.).
//
// Stateless commands that don't belong to a session (sin-code
// tokens, sin-code spec author) should pass a synthetic session
// ID derived from the command + invocation timestamp, so the
// Recorder can still group them.
func WithSessionID(parent context.Context, id string) context.Context {
	return context.WithValue(parent, sessionIDKey{}, id)
}

// sessionIDKey is the unexported context key for session IDs. Using
// an unexported empty struct ensures no other package can collide
// with our key (the standard Go idiom).
type sessionIDKey struct{}

// muSessionID protects a process-global fallback session ID for
// callers that don't set one explicitly. This is purely defensive:
// every well-behaved caller should set its own via WithSessionID.
var (
	muSessionID      sync.Mutex
	fallbackSession  string
)

// DefaultSessionID returns a stable, process-unique session
// identifier. Use it as the default in the LLM client when no
// session ID is present in the context.
//
// The id is a hex-encoded random 64-bit value, generated once
// per process. This means usage from a CLI invocation is grouped
// into a single session (the "process"), while a long-running
// chat loop can group by turn via explicit WithSessionID calls.
var (
	defaultSessionOnce sync.Once
	defaultSession     string
)

func init() {
	defaultSessionOnce.Do(func() {
		defaultSession = randomSessionID()
	})
}

// DefaultSessionID is the session ID used when no explicit
// WithSessionID was applied. Exposed so the LLM client can
// pass it to the Recorder.
func DefaultSessionID() string { return defaultSession }
