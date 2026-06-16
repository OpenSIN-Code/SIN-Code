// SPDX-License-Identifier: MIT
// Purpose: token-usage recorder interface (issue #168). The llm package
// owns the abstraction so any recorder can be plugged in without the LLM
// client caring about persistence. The default implementation lives in
// internal/usage (SQLite-backed). Used by agentloop, eval/judge, daemon,
// and the spec author.
//
// Threading model: SessionID is propagated via context.Context using the
// exported SessionIDKey{} value. Callers that do not have a session
// (ad-hoc CLI, spec author dry-run) leave SessionID empty and the recorder
// stores the row with session_id=” (still aggregates correctly).
package llm

import "context"

// Source categorises which subsystem emitted the call. Mirrors
// internal/usage.Source so the persistence-side enum stays canonical. Kept
// here only so callers can avoid importing internal/usage.
type Source string

const (
	SourceChat    Source = "chat"
	SourceVerify  Source = "verify"
	SourceJudge   Source = "judge"
	SourceSummary Source = "summary"
	SourcePlan    Source = "plan"
	SourceAdHoc   Source = "adhoc"
)

// SessionIDKey is the context.Context key under which agentloop / daemon /
// chat pass the current session ID down to LLM calls. Reads with
// SessionIDFromContext(ctx).
type SessionIDKey struct{}

// SessionIDFromContext extracts the current session ID from ctx. Returns ""
// when no session is associated (e.g. spec author dry-run, ad-hoc CLI).
func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(SessionIDKey{}).(string); ok {
		return v
	}
	return ""
}

// WithSessionID returns a derived context that carries sid. The LLM client
// reads the session ID via SessionIDFromContext when persisting token usage.
func WithSessionID(parent context.Context, sid string) context.Context {
	if sid == "" {
		return parent
	}
	return context.WithValue(parent, SessionIDKey{}, sid)
}

// Recorder persists the parsed ChatResponse.Usage block (currently dropped
// in provider.go:42-46 — see issue #168). Implementations MUST be cheap
// (best-effort) and MUST NOT panic on nil / closed stores: a failed write
// is logged to stderr and swallowed.
type Recorder interface {
	RecordUsage(ctx context.Context, sessionID, model string, source Source, input, output, total int) error
}

// NopRecorder is a Recorder that drops every record. It is the default
// Client.Recorder — built and returned by NewClient without a usage store,
// so the LLM client works fine before persistence is wired in.
type NopRecorder struct{}

// RecordUsage is a no-op.
func (NopRecorder) RecordUsage(_ context.Context, _, _ string, _ Source, _, _, _ int) error {
	return nil
}
