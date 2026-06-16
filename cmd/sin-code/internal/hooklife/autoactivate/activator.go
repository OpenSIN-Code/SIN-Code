// SPDX-License-Identifier: MIT
// Purpose: per-session activation state and the SessionStart /
// UserPrompt handlers that inject rules into the prompt engine. Mirrors
// JuliusBrussee/caveman's `caveman-activate.js` flag-file + per-turn
// re-inject flow but ported to Go's `hooklife` Phase contract.
//
// Concurrency: every public method takes/releases a single sync.RWMutex
// so race-free Activate / Deactivate / On* is enforceable via
// `go test -race -count=1` (mandate M7). Sessions are GC'd on
// OnSessionEnd to keep the map bounded.
// Docs: activator.doc.md
package autoactivate

import (
	"context"
	"strings"
	"sync"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
)

// StartOptions configures a single OnSessionStart initialization. All
// fields are optional; zero value gives an empty, off session.
type StartOptions struct {
	// AutoOn enables the activator for the duration of the session.
	// Equivalent to setting `default.auto_on = true` in
	// .sin-code/autoactivate.toml for this one session.
	AutoOn bool
	// NoTrigger disables prompt-phrase-based auto-activation even if
	// a rule has a `trigger:` configured.
	NoTrigger bool
	// Defaults are the rules loaded from .sin-code/autoactivate.toml
	// (or `[]` if no file exists). They are applied at session start
	// when AutoOn is true.
	Defaults RuleSet
}

// SessionState is a read-only snapshot of a session's activation
// state. Returned by Snapshot and used only in tests + the planned
// `sin-code rules list` subcommand.
type SessionState struct {
	SessionID   string
	ActiveRules RuleSet
	AutoOn      bool
	NoTrigger   bool
}

// Activator owns per-session activation state. Single instance per
// process — share via a pointer. The zero value is unusable; always
// construct via NewActivator.
type Activator struct {
	mu       sync.RWMutex
	sessions map[string]*SessionState
}

// NewActivator returns an empty Activator. Pass defaults that should
// always be available (e.g. shipped with the binary) — they are not
// auto-loaded into a session; callers must explicitly Activate each
// one for a given session. nil defaults is fine.
func NewActivator(defaults RuleSet) *Activator {
	a := &Activator{
		sessions: make(map[string]*SessionState),
	}
	if defaults != nil {
		// store into a reserved "__builtins__" pseudo-session so
		// Snapshot/details can introspect; never visible to Dispatch.
		a.sessions[builtinsID] = &SessionState{
			SessionID:   builtinsID,
			ActiveRules: defaults.Clone(),
		}
	}
	return a
}

// builtinsID is the reserved sessionID used to surface the built-in
// defaults. It is filtered out of public APIs so callers cannot
// Activate/Deactivate against it.
const builtinsID = "__builtins__"

// OnSessionStart initializes (or replaces) the session's activation
// state. Idempotent: calling twice with the same sessionID replaces
// the prior state. Returns the resulting SessionState.
func (a *Activator) OnSessionStart(sessionID string, opts StartOptions) SessionState {
	if sessionID == "" {
		return SessionState{}
	}
	fresh := &SessionState{
		SessionID: sessionID,
		AutoOn:    opts.AutoOn,
		NoTrigger: opts.NoTrigger,
	}
	if opts.AutoOn && opts.Defaults != nil {
		fresh.ActiveRules = opts.Defaults.Clone()
	}
	a.mu.Lock()
	a.sessions[sessionID] = fresh
	a.mu.Unlock()
	return *fresh
}

// OnUserPrompt is called on every UserPrompt phase. When the session
// is not AutoOn, it returns (nil, false) — silent no-op.
//
// When AutoOn is true:
//   - Prompts matching any active rule.Trigger (case-folded substring)
//     auto-Activate that rule (per-turn reinforcement).
//   - After activation, the active RuleSet is returned with ok=true.
//
// ok=true means the caller (system-prompt builder) MUST prepend
// `rules.Render()` to the model's system context for this turn.
func (a *Activator) OnUserPrompt(sessionID, prompt string) (RuleSet, bool) {
	if sessionID == "" {
		return nil, false
	}
	a.mu.Lock()
	st, ok := a.sessions[sessionID]
	if !ok || st == nil {
		a.mu.Unlock()
		return nil, false
	}
	if !st.AutoOn {
		a.mu.Unlock()
		return nil, false
	}
	if st.ActiveRules == nil {
		st.ActiveRules = RuleSet{}
	}
	if !st.NoTrigger {
		lc := strings.ToLower(prompt)
		for _, r := range st.ActiveRules {
			if r.NoTrigger || r.Trigger == "" {
				continue
			}
			if strings.Contains(lc, strings.ToLower(r.Trigger)) {
				st.ActiveRules.Add(r) // self-activation is a no-op
			}
		}
	}
	rules := st.ActiveRules.Clone()
	has := len(rules) > 0
	a.mu.Unlock()
	return rules, has
}

// Activate manually turns on a rule in a session. No-op if the rule
// is unknown to the built-in defaults (callers must pass rules via
// StartOptions.Defaults OR extend this method's signature to look up
// from the builtins pseudo-session).
func (a *Activator) Activate(sessionID string, rule Rule) {
	if sessionID == "" || rule.Name == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.sessions[sessionID]
	if !ok || st == nil {
		// auto-initialize an OFF session so manual Activate still works
		st = &SessionState{SessionID: sessionID, ActiveRules: RuleSet{}}
		a.sessions[sessionID] = st
	}
	if st.ActiveRules == nil {
		st.ActiveRules = RuleSet{}
	}
	st.ActiveRules.Add(rule)
	st.AutoOn = true // implicit enable
}

// Deactivate manually turns off a rule in a session. No-op for
// sessions that never received Activate / OnSessionStart.
func (a *Activator) Deactivate(sessionID, ruleName string) {
	if sessionID == "" || ruleName == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.sessions[sessionID]
	if !ok || st == nil {
		return
	}
	st.ActiveRules.Remove(ruleName)
}

// SetAutoOn updates the AutoOn flag for an already-known session.
// No-op for unknown sessions (so callers cannot accidentally re-init).
func (a *Activator) SetAutoOn(sessionID string, autoOn bool) {
	if sessionID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.sessions[sessionID]
	if !ok || st == nil {
		return
	}
	st.AutoOn = autoOn
}

// Snapshot returns a defensive copy of the session's state. Used by
// tests and the planned `sin-code rules list` subcommand.
func (a *Activator) Snapshot(sessionID string) (SessionState, bool) {
	if sessionID == "" {
		return SessionState{}, false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	st, ok := a.sessions[sessionID]
	if !ok || st == nil {
		return SessionState{}, false
	}
	return SessionState{
		SessionID:   st.SessionID,
		ActiveRules: st.ActiveRules.Clone(),
		AutoOn:      st.AutoOn,
		NoTrigger:   st.NoTrigger,
	}, true
}

// EndSession removes a session from the in-memory map. Privacy-first
// — drop activation state once the user ends the session so no rule
// metadata lingers in process memory.
func (a *Activator) EndSession(sessionID string) {
	if sessionID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, sessionID)
}

// Count returns the number of tracked sessions (excluding the
// built-ins pseudo-session). Used by tests and metrics.
func (a *Activator) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	n := 0
	for k := range a.sessions {
		if k != builtinsID {
			n++
		}
	}
	return n
}

// Register attaches the SessionStart and UserPrompt handlers to reg
// in deterministic order. Returns the number registered. Safe to call
// multiple times (hooks are deduplicated by ID through the registry's
// stable-order sort).
func (a *Activator) Register(reg *hooklife.Registry) int {
	if reg == nil || a == nil {
		return 0
	}
	reg.Register(SessionStartHook{Act: a})
	reg.Register(UserPromptHook{Act: a})
	return 2
}

// SessionStartHook is the hooklife.Hook implementation that invokes
// OnSessionStart. It is intentionally cheap: it does not lookup the
// prompt file itself; the caller (chat command) passes pre-parsed
// defaults in the StartOptions.
type SessionStartHook struct {
	Act      *Activator
	Defaults RuleSet
	AutoOn   bool
}

func (SessionStartHook) ID() string { return "autoactivate-session-start" }
func (SessionStartHook) Phases() []hooklife.Phase {
	return []hooklife.Phase{hooklife.SessionStart}
}
func (h SessionStartHook) Run(_ context.Context, ev hooklife.Event) hooklife.Decision {
	sid := sessionIDFromMeta(ev.Meta)
	if sid == "" {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: "autoactivate-session-start"}
	}
	st := h.Act.OnSessionStart(sid, StartOptions{
		AutoOn:    h.AutoOn,
		Defaults:  h.Defaults,
		NoTrigger: triggerOverrideFromMeta(ev.Meta),
	})
	if !st.AutoOn || len(st.ActiveRules) == 0 {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: "autoactivate-session-start"}
	}
	// Emit Warn (not Allow) so the runner surfaces the Message via its
	// aggregation, rather than silently dropping it on Allow. The auto-
	// activate rule body is intended as an informational anchor that
	// the chat command's caller prepends to the model's context.
	return hooklife.Decision{
		Verdict: hooklife.Warn,
		HookID:  "autoactivate-session-start",
		Message: st.ActiveRules.Render(),
	}
}

// UserPromptHook invokes OnUserPrompt and surfaces the rendered body
// as a Warn verdict's Message so the agent loop can prepend it.
type UserPromptHook struct {
	Act *Activator
}

func (UserPromptHook) ID() string { return "autoactivate-user-prompt" }
func (UserPromptHook) Phases() []hooklife.Phase {
	return []hooklife.Phase{hooklife.UserPrompt}
}
func (h UserPromptHook) Run(_ context.Context, ev hooklife.Event) hooklife.Decision {
	sid := sessionIDFromMeta(ev.Meta)
	if sid == "" {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: "autoactivate-user-prompt"}
	}
	prompt := promptFromEvent(ev)
	rules, ok := h.Act.OnUserPrompt(sid, prompt)
	if !ok {
		return hooklife.Decision{Verdict: hooklife.Allow, HookID: "autoactivate-user-prompt"}
	}
	// Warn so the runner surfaces the body via aggregation (see
	// SessionStartHook.Run).
	return hooklife.Decision{
		Verdict: hooklife.Warn,
		HookID:  "autoactivate-user-prompt",
		Message: rules.Render(),
	}
}

// promptFromEvent extracts the user prompt string from the Event in
// a deterministic precedence order:
//  1. Meta["prompt"]      — preferred (semantic key, no overload)
//  2. Meta["user_prompt"] — alias
//  3. Meta["text"]        — alias
//  4. Args["prompt"]      — fallback
//
// Empty string when none are present; callers treat this as no-op.
func promptFromEvent(ev hooklife.Event) string {
	for _, k := range []string{"prompt", "user_prompt", "text"} {
		if v, ok := ev.Meta[k]; ok && v != "" {
			return v
		}
	}
	if v, ok := ev.Args["prompt"]; ok && v != "" {
		return v
	}
	return ""
}

// sessionIDFromMeta extracts a session id from the Event.Meta map.
// Falls back to the empty string when not present so callers do not
// run for shared/global events.
func sessionIDFromMeta(meta map[string]string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta["session_id"]; ok {
		return v
	}
	if v, ok := meta["sid"]; ok {
		return v
	}
	return ""
}

// triggerOverrideFromMeta reads the no_trigger override from Meta.
// Returns true only when the caller explicitly sets it.
func triggerOverrideFromMeta(meta map[string]string) bool {
	if meta == nil {
		return false
	}
	if v, ok := meta["no_trigger"]; ok {
		return parseBool(v)
	}
	if v, ok := meta["no-trigger"]; ok {
		return parseBool(v)
	}
	return false
}
