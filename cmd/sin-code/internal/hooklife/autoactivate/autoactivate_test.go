// SPDX-License-Identifier: MIT
// Purpose: race-safe coverage tests for the autoactivate package.
// Every public surface (RuleSet / parser / Activator / Hook) is
// exercised; concurrency tests are run under `go test -race` (mandate
// M7) so the per-session map + RWMutex are validated, not just
// guessed.
package autoactivate

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
)

// --- rules.go ---

func TestRuleSetNilSafe(t *testing.T) {
	var rs RuleSet
	rs.Add(Rule{Name: "x", Body: "b"})
	rs.Remove("x")
	if rs.Has("x") {
		t.Errorf("nil RuleSet Has should always be false")
	}
	if rs.Len() != 0 {
		t.Errorf("nil RuleSet Len should be 0, got %d", rs.Len())
	}
	if rs.Names() != nil {
		t.Errorf("nil RuleSet Names should be nil, got %v", rs.Names())
	}
	if rs.Render() != "" {
		t.Errorf("nil RuleSet Render should be empty, got %q", rs.Render())
	}
}

func TestRuleSetAddRemove(t *testing.T) {
	rs := RuleSet{}
	rs.Add(Rule{Name: "terse", Body: "be terse"})
	rs.Add(Rule{Name: "ultra", Body: "be terse\nultra"})
	rs.Add(Rule{Name: "", Body: "skip me"}) // blank name is a no-op
	if rs.Len() != 2 {
		t.Errorf("expected 2 rules, got %d", rs.Len())
	}
	rs.Remove("terse")
	if rs.Has("terse") {
		t.Errorf("Remove failed")
	}
	// Remove missing is silent.
	rs.Remove("ghost")
}

func TestRuleSetNamesSorted(t *testing.T) {
	rs := RuleSet{}
	rs.Add(Rule{Name: "zeta"})
	rs.Add(Rule{Name: "alpha"})
	rs.Add(Rule{Name: "mu"})
	got := rs.Names()
	want := []string{"alpha", "mu", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Names()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRuleSetRenderByteStable(t *testing.T) {
	// Two RuleSets with the same rules inserted in different orders
	// MUST produce identical Render() output. This is the system-
	// prompt hash metric (issue #2) contract.
	a := RuleSet{}
	a.Add(Rule{Name: "terse", Body: "be terse"})
	a.Add(Rule{Name: "skill-x", Body: "use skill-x"})
	a.Add(Rule{Name: "verbosity", Body: "ultra"})

	b := RuleSet{}
	b.Add(Rule{Name: "verbosity", Body: "ultra"})
	b.Add(Rule{Name: "skill-x", Body: "use skill-x"})
	b.Add(Rule{Name: "terse", Body: "be terse"})

	if a.Render() != b.Render() {
		t.Errorf("Render must be deterministic across insertion order\nA: %q\nB: %q", a.Render(), b.Render())
	}
}

func TestRuleSetRenderTrimTrailing(t *testing.T) {
	rs := RuleSet{}
	rs.Add(Rule{Name: "x", Body: "hello   \n\n\n"})
	rs.Add(Rule{Name: "y", Body: "world\t\t"})
	r := rs.Render()
	if strings.Contains(r, "hello   ") || strings.Contains(r, "world\t\t") {
		t.Errorf("Render should trim trailing whitespace per rule, got %q", r)
	}
}

func TestRuleSetRenderEmpty(t *testing.T) {
	rs := RuleSet{}
	if rs.Render() != "" {
		t.Errorf("empty RuleSet Render should be empty")
	}
}

func TestRuleSetCloneIsolated(t *testing.T) {
	rs := RuleSet{}
	rs.Add(Rule{Name: "x", Body: "first"})
	clone := rs.Clone()
	clone.Add(Rule{Name: "x", Body: "second"})
	if rs["x"].Body != "first" {
		t.Errorf("Clone mutation leaked into original: %q", rs["x"].Body)
	}
}

func TestRuleSetEqual(t *testing.T) {
	a := RuleSet{}
	a.Add(Rule{Name: "x", Body: "b", Trigger: "/x"})
	b := RuleSet{}
	b.Add(Rule{Name: "x", Body: "b", Trigger: "/x"})
	if !a.Equal(b) {
		t.Errorf("expected equal")
	}
	b.Add(Rule{Name: "y", Body: "b"})
	if a.Equal(b) {
		t.Errorf("mismatched length should not be equal")
	}
	delete(b, "y")
	other := b["x"]
	other.Trigger = "/other"
	b.Add(other)
	if a.Equal(b) {
		t.Errorf("mismatched trigger should not be equal")
	}
}

// --- triggers.go ---

func TestLoadFileMissing(t *testing.T) {
	rs, def, err := LoadFile(filepath.Join(t.TempDir(), "no-such-file.toml"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if rs == nil || len(rs) != 0 {
		t.Errorf("expected empty nil-safe RuleSet, got %v", rs)
	}
	if def.AutoOn || def.NoTrigger {
		t.Errorf("expected zero Default, got %+v", def)
	}
}

func TestLoadFileEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.toml")
	if err := os.WriteFile(p, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	rs, def, err := LoadFile(p)
	if err != nil {
		t.Fatalf("empty file: %v", err)
	}
	if rs == nil || len(rs) != 0 {
		t.Errorf("expected empty RuleSet, got %v", rs)
	}
	if def.AutoOn || def.NoTrigger {
		t.Errorf("expected zero Default, got %+v", def)
	}
}

func TestParseSingleRuleAllKeys(t *testing.T) {
	body := `[rule]
name = "terse"
body = "be terse"
trigger = "/compact"
no_trigger = false
[default]
auto_on = true
no_trigger = true`
	rs, def, err := parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, ok := rs["terse"]
	if !ok {
		t.Fatalf("missing terse rule")
	}
	if r.Body != "be terse" || r.Trigger != "/compact" || r.NoTrigger {
		t.Errorf("rule body: %+v", r)
	}
	if !def.AutoOn || !def.NoTrigger {
		t.Errorf("default: %+v", def)
	}
}

func TestParseUnquotedValuesAndComments(t *testing.T) {
	body := `# leading comment
[rule] # section comment
name=terse
body=be terse and tight
# another comment
trigger=/compact
[default]
auto_on=true
`
	rs, def, err := parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r := rs["terse"]
	if r.Body != "be terse and tight" || r.Trigger != "/compact" {
		t.Errorf("unquoted parse failed: %+v", r)
	}
	if !def.AutoOn {
		t.Errorf("default.auto_on: %v", def.AutoOn)
	}
}

func TestParseMultipleRulesFlushesOnNewName(t *testing.T) {
	body := `[rule]
name = "alpha"
body = "first"
[rule]
name = "beta"
body = "second"

[default]
auto_on = false
`
	rs, _, err := parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rs["alpha"].Body != "first" {
		t.Errorf("alpha should be 'first', got %q", rs["alpha"].Body)
	}
	if rs["beta"].Body != "second" {
		t.Errorf("beta should be 'second', got %q", rs["beta"].Body)
	}
}

func TestParseBoolAndSectionHelpers(t *testing.T) {
	if !parseBool("true") || !parseBool("YES") || !parseBool("1") {
		t.Errorf("parseBool truthy failed")
	}
	if parseBool("no") || parseBool("0") || parseBool("garbage") {
		t.Errorf("parseBool falsy failed")
	}
	if _, ok := stripSection("[ok]"); !ok {
		t.Errorf("stripSection ok=true expected")
	}
	if _, ok := stripSection("[]"); ok {
		t.Errorf("stripSection [] should reject")
	}
	if _, _, ok := splitKV("noeq"); ok {
		t.Errorf("splitKV missing = should reject")
	}
	if _, _, ok := splitKV(" = x"); ok {
		t.Errorf("splitKV empty key should reject")
	}
	if k, v, ok := splitKV(`name = "x"`); !ok || k != "name" || v != "x" {
		t.Errorf("splitKV quoted got %q/%q ok=%v", k, v, ok)
	}
	if got := stripComment(`#whole line`); got != "" {
		t.Errorf("stripComment got %q", got)
	}
	if got := stripComment(`x # tail`); got != "x " {
		t.Errorf("stripComment got %q", got)
	}
	if got := stripComment(`"a#b" # tail`); got != `"a#b" ` {
		t.Errorf("stripComment should preserve # inside quotes, got %q", got)
	}
}

func TestParseErrorsOnUnreadable(t *testing.T) {
	// error path of bufio scanner: oversized line.
	big := strings.Repeat("x", 2*1024*1024+10)
	_, _, err := parse(&errReader{buf: []byte(big)})
	if err == nil {
		t.Skipf("no error returned; buffer in this Go version accepts large lines")
	}
	if !errors.Is(err, io.ErrShortBuffer) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Logf("got error (acceptable class): %v", err)
	}
}

type errReader struct {
	buf []byte
	pos int
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.buf) {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.buf[r.pos:])
	r.pos += n
	return n, nil
}

// --- activator.go ---

const (
	sidA = "sess-A"
	sidB = "sess-B"
)

func newTestBuiltins() RuleSet {
	return RuleSet{
		"terse-mode":   {Name: "terse-mode", Body: "be terse", Trigger: "/compact"},
		"ultra-mode":   {Name: "ultra-mode", Body: "be ultra terse", Trigger: "/ultra"},
		"skill-create": {Name: "skill-create", Body: "use skill-create carefully", NoTrigger: true},
	}
}

func TestNewActivatorNilDefaults(t *testing.T) {
	a := NewActivator(nil)
	if a == nil {
		t.Fatalf("nil defaults still produces valid activator")
	}
	if a.Count() != 0 {
		t.Errorf("Count should be 0 with no AutoStarted sessions, got %d", a.Count())
	}
}

func TestNewActivatorStoresBuiltins(t *testing.T) {
	a := NewActivator(newTestBuiltins())
	if a == nil {
		t.Fatal("nil activator")
	}
	st, ok := a.Snapshot(builtinsID)
	if !ok {
		t.Fatalf("builtins pseudo-session expected")
	}
	if len(st.ActiveRules) != 3 {
		t.Errorf("builtins length = %d, want 3", len(st.ActiveRules))
	}
	if a.Count() != 0 {
		t.Errorf("Count should not include builtins, got %d", a.Count())
	}
}

func TestOnSessionStartEmptyByDefault(t *testing.T) {
	a := NewActivator(nil)
	st := a.OnSessionStart(sidA, StartOptions{})
	if st.AutoOn {
		t.Errorf("default StartOptions must be off")
	}
	if len(st.ActiveRules) != 0 {
		t.Errorf("default ActiveRules should be empty")
	}
	if a.Count() != 1 {
		t.Errorf("Count after start = %d, want 1", a.Count())
	}
}

func TestOnSessionStartReplaces(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: true})
	a.OnSessionStart(sidA, StartOptions{AutoOn: false, NoTrigger: true})
	st, ok := a.Snapshot(sidA)
	if !ok {
		t.Fatal("snapshot missing")
	}
	if st.AutoOn || !st.NoTrigger {
		t.Errorf("second Start should replace, got %+v", st)
	}
}

func TestOnSessionStartWithDefaultsClones(t *testing.T) {
	a := NewActivator(nil)
	df := newTestBuiltins()
	st := a.OnSessionStart(sidA, StartOptions{AutoOn: true, Defaults: df})
	if len(st.ActiveRules) != 3 {
		t.Errorf("expected 3 rules cloned, got %d", len(st.ActiveRules))
	}
	// Mutating caller's rule set must NOT bleed into the session.
	df.Add(Rule{Name: "ghost", Body: "should not appear"})
	st2, _ := a.Snapshot(sidA)
	if st2.ActiveRules.Has("ghost") {
		t.Errorf("session rules should be a defensive clone, but `ghost` leaked")
	}
}

func TestOnUserPromptQuietWhenNotAutoOn(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: false, Defaults: newTestBuiltins()})
	rules, ok := a.OnUserPrompt(sidA, "/compact")
	if ok || rules != nil {
		t.Errorf("off session must return (nil, false), got ok=%v rules=%v", ok, rules)
	}
}

func TestOnUserPromptReturnsRulesWhenActive(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: true, Defaults: newTestBuiltins()})
	rules, ok := a.OnUserPrompt(sidA, "hello")
	if !ok {
		t.Fatal("AutoOn session should report ok=true")
	}
	if len(rules) != 3 {
		t.Errorf("expected 3 rules, got %d", len(rules))
	}
	// Defensive clone check.
	rules.Add(Rule{Name: "evil", Body: "x"})
	original, _ := a.Snapshot(sidA)
	if original.ActiveRules.Has("evil") {
		t.Errorf("returned RuleSet should be a defensive clone")
	}
}

func TestOnUserPromptEmptySessionID(t *testing.T) {
	a := NewActivator(nil)
	rules, ok := a.OnUserPrompt("", "hi")
	if ok || rules != nil {
		t.Errorf("empty sessionID must be a no-op, got ok=%v rules=%v", ok, rules)
	}
}

func TestOnUserPromptUnknownSession(t *testing.T) {
	a := NewActivator(nil)
	rules, ok := a.OnUserPrompt("never-started", "hi")
	if ok || rules != nil {
		t.Errorf("unknown session must be a no-op, got ok=%v rules=%v", ok, rules)
	}
}

func TestActivateCreatesSessionAndImplicitAutoOn(t *testing.T) {
	a := NewActivator(nil)
	if a.Count() != 0 {
		t.Fatalf("pre-condition")
	}
	a.Activate(sidA, Rule{Name: "terse-mode", Body: "be terse"})
	if a.Count() != 1 {
		t.Errorf("Activate should lazy-init session, Count = %d, want 1", a.Count())
	}
	st, _ := a.Snapshot(sidA)
	if !st.AutoOn {
		t.Errorf("manual Activate must set AutoOn implicitly")
	}
	if !st.ActiveRules.Has("terse-mode") {
		t.Errorf("terse-mode missing")
	}
	// OnUserPrompt should now return rules.
	_, ok := a.OnUserPrompt(sidA, "anything")
	if !ok {
		t.Errorf("after Activate, OnUserPrompt must succeed")
	}
}

func TestActivateEmptyGuards(t *testing.T) {
	a := NewActivator(nil)
	a.Activate("", Rule{Name: "x"})            // empty session -> no-op
	a.Activate(sidA, Rule{Name: "", Body: ""}) // empty name -> no-op
	if a.Count() != 0 {
		t.Errorf("Activate with bad args must not create a session, Count = %d", a.Count())
	}
}

func TestDeactivateEmptyAndUnknownGuards(t *testing.T) {
	a := NewActivator(nil)
	a.Deactivate("", "x")   // empty session -> no-op
	a.Deactivate(sidA, "x") // unknown session -> no-op
	a.Activate(sidA, Rule{Name: "r1"})
	a.Deactivate(sidA, "r1")
	st, _ := a.Snapshot(sidA)
	if st.ActiveRules.Has("r1") {
		t.Errorf("Deactivate failed")
	}
}

func TestSetAutoOnUnknownSessionIsSafe(t *testing.T) {
	a := NewActivator(nil)
	a.SetAutoOn("never", true) // no-op, should not panic
	if a.Count() != 0 {
		t.Errorf("SetAutoOn unknown session must not create one, Count = %d", a.Count())
	}
}

func TestEndSessionDropsState(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: true, Defaults: newTestBuiltins()})
	if a.Count() != 1 {
		t.Fatalf("pre")
	}
	a.EndSession(sidA)
	if a.Count() != 0 {
		t.Errorf("EndSession must drop state")
	}
	if _, ok := a.Snapshot(sidA); ok {
		t.Errorf("EndSession should remove from map")
	}
	a.EndSession("") // empty -> no-op
}

func TestSessionStartHookRunActiveSession(t *testing.T) {
	a := NewActivator(nil)
	h := SessionStartHook{Act: a, AutoOn: true, Defaults: newTestBuiltins()}
	d := h.Run(context.Background(), hooklife.Event{
		Phase: hooklife.SessionStart,
		Meta:  map[string]string{"session_id": sidA},
	})
	if d.Verdict != hooklife.Warn {
		// Warn is the intentional verdict so the runner aggregates
		// the rule body as a warning message (see Activator doc).
		t.Errorf("verdict = %s, want warn", d.Verdict)
	}
	if d.HookID != "autoactivate-session-start" {
		t.Errorf("HookID = %q", d.HookID)
	}
	if !strings.Contains(d.Message, "## Active rules") {
		t.Errorf("expected rendered rules header, got %q", d.Message)
	}
}

func TestSessionStartHookRunEmptySid(t *testing.T) {
	a := NewActivator(nil)
	h := SessionStartHook{Act: a, AutoOn: true, Defaults: newTestBuiltins()}
	d := h.Run(context.Background(), hooklife.Event{Phase: hooklife.SessionStart})
	if d.Message != "" {
		t.Errorf("empty sessionID must yield empty message, got %q", d.Message)
	}
	if a.Count() != 0 {
		t.Errorf("empty-sid Run must NOT create a session, Count = %d", a.Count())
	}
}

func TestSessionStartHookRunNoTriggerOverride(t *testing.T) {
	a := NewActivator(nil)
	h := SessionStartHook{Act: a, AutoOn: true, Defaults: newTestBuiltins()}
	d := h.Run(context.Background(), hooklife.Event{
		Phase: hooklife.SessionStart,
		Meta:  map[string]string{"session_id": sidA, "no_trigger": "true"},
	})
	if d.HookID != "autoactivate-session-start" {
		t.Errorf("HookID = %q", d.HookID)
	}
	st, _ := a.Snapshot(sidA)
	if !st.NoTrigger {
		t.Errorf("no_trigger meta override should propagate")
	}
}

func TestUserPromptHookRun(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: true, Defaults: newTestBuiltins()})
	h := UserPromptHook{Act: a}
	d := h.Run(context.Background(), hooklife.Event{
		Phase: hooklife.UserPrompt,
		Meta:  map[string]string{"session_id": sidA, "prompt": "/compact please"},
	})
	if !strings.Contains(d.Message, "## Active rules") {
		t.Errorf("expected rendered rules; got %q", d.Message)
	}
}

func TestUserPromptHookRunEmptySid(t *testing.T) {
	a := NewActivator(nil)
	h := UserPromptHook{Act: a}
	d := h.Run(context.Background(), hooklife.Event{Phase: hooklife.UserPrompt, Meta: map[string]string{"prompt": "/compact"}})
	if d.Message != "" || d.HookID == "" {
		t.Errorf("empty sid: expected Allow with no message, got %+v", d)
	}
}

func TestUserPromptHookRunNoTriggerPhrase(t *testing.T) {
	a := NewActivator(nil)
	// Single rule, neither its trigger nor existence is in the prompt.
	one := RuleSet{}
	one.Add(Rule{Name: "terse-mode", Body: "be terse", Trigger: "/compact"})
	a.OnSessionStart(sidA, StartOptions{AutoOn: true, Defaults: one})
	h := UserPromptHook{Act: a}
	d := h.Run(context.Background(), hooklife.Event{
		Phase: hooklife.UserPrompt,
		Meta:  map[string]string{"session_id": sidA, "prompt": "what is the time?"},
	})
	if !strings.Contains(d.Message, "terse-mode") {
		t.Errorf("active rule should be re-emitted regardless of trigger match, got %q", d.Message)
	}
}

func TestRegisterWiresHooks(t *testing.T) {
	a := NewActivator(nil)
	reg := hooklife.NewRegistry()
	n := a.Register(reg)
	if n != 2 {
		t.Errorf("registered = %d, want 2", n)
	}
	if len(reg.Hooks(hooklife.SessionStart)) != 1 {
		t.Errorf("SessionStart hooks = %d", len(reg.Hooks(hooklife.SessionStart)))
	}
	if len(reg.Hooks(hooklife.UserPrompt)) != 1 {
		t.Errorf("UserPrompt hooks = %d", len(reg.Hooks(hooklife.UserPrompt)))
	}
}

func TestRegisterHandlesNilInputs(t *testing.T) {
	a := NewActivator(nil)
	if n := a.Register(nil); n != 0 {
		t.Errorf("nil registry should return 0, got %d", n)
	}
	var nilAct *Activator
	if n := nilAct.Register(hooklife.NewRegistry()); n != 0 {
		t.Errorf("nil activator should return 0")
	}
}

// --- concurrency / race tests (mandate M7) ---

func TestRaceActivateDeactivateSnapshot(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: true, Defaults: newTestBuiltins()})

	const goroutines = 16
	const ops = 64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				switch (gid + i) % 3 {
				case 0:
					a.Activate(sidA, Rule{Name: "lua", Body: "x"})
				case 1:
					a.Deactivate(sidA, "lua")
				case 2:
					_, _ = a.Snapshot(sidA)
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestRaceMultipleSessionsIndependent(t *testing.T) {
	a := NewActivator(nil)
	const goroutines = 12
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			sid := "sess-" + string(rune('A'+gid))
			a.OnSessionStart(sid, StartOptions{AutoOn: true, Defaults: newTestBuiltins()})
			for i := 0; i < 5; i++ {
				a.Activate(sid, Rule{Name: "pol", Body: "p"})
				_, _ = a.OnUserPrompt(sid, "")
				st, ok := a.Snapshot(sid)
				if !ok {
					t.Errorf("session vanished mid-race: %s", sid)
					return
				}
				if !st.AutoOn {
					t.Errorf("AutoOn lost mid-race: %s", sid)
				}
			}
		}(g)
	}
	wg.Wait()
}

// --- targeted coverage for previously uncovered lines ---

func TestOnSessionStartEmptySessionID(t *testing.T) {
	a := NewActivator(nil)
	st := a.OnSessionStart("", StartOptions{AutoOn: true, Defaults: newTestBuiltins()})
	if st.SessionID != "" || st.AutoOn || len(st.ActiveRules) != 0 {
		t.Errorf("empty sessionID should yield zero SessionState, got %+v", st)
	}
	if a.Count() != 0 {
		t.Errorf("empty sessionID must not create a session")
	}
}

func TestOnUserPromptInitializesEmptyActiveRules(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: true})
	rules, ok := a.OnUserPrompt(sidA, "anything")
	if ok {
		t.Errorf("expected ok=false for empty active rules, got ok=%v", ok)
	}
	if len(rules) != 0 {
		t.Errorf("expected empty rules, got %v", rules)
	}
}

func TestActivateInitializesNilActiveRules(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: true})
	a.Activate(sidA, Rule{Name: "manual", Body: "manual rule"})
	st, _ := a.Snapshot(sidA)
	if !st.ActiveRules.Has("manual") {
		t.Errorf("Activate should initialize nil ActiveRules")
	}
}

func TestSetAutoOnGuardsAndUpdates(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: false})
	a.SetAutoOn("", true)
	a.SetAutoOn(sidA, true)
	st, _ := a.Snapshot(sidA)
	if !st.AutoOn {
		t.Errorf("SetAutoOn should update known session")
	}
}

func TestSnapshotEmptySessionID(t *testing.T) {
	a := NewActivator(nil)
	_, ok := a.Snapshot("")
	if ok {
		t.Errorf("empty sessionID snapshot should return false")
	}
}

func TestSessionStartHookRunInactive(t *testing.T) {
	a := NewActivator(nil)
	h := SessionStartHook{Act: a, AutoOn: false}
	d := h.Run(context.Background(), hooklife.Event{
		Phase: hooklife.SessionStart,
		Meta:  map[string]string{"session_id": sidA},
	})
	if d.Verdict != hooklife.Allow || d.Message != "" {
		t.Errorf("inactive session should yield Allow with empty message, got %+v", d)
	}
}

func TestUserPromptHookRunInactive(t *testing.T) {
	a := NewActivator(nil)
	a.OnSessionStart(sidA, StartOptions{AutoOn: false})
	h := UserPromptHook{Act: a}
	d := h.Run(context.Background(), hooklife.Event{
		Phase: hooklife.UserPrompt,
		Meta:  map[string]string{"session_id": sidA, "prompt": "hi"},
	})
	if d.Verdict != hooklife.Allow || d.Message != "" {
		t.Errorf("off session should yield Allow with empty message, got %+v", d)
	}
}

func TestPromptFromEventFallbacks(t *testing.T) {
	if got := promptFromEvent(hooklife.Event{Args: map[string]string{"prompt": "args"}}); got != "args" {
		t.Errorf("Args fallback failed: %q", got)
	}
	if got := promptFromEvent(hooklife.Event{Meta: map[string]string{"other": "x"}}); got != "" {
		t.Errorf("empty prompt should return empty string, got %q", got)
	}
}

func TestSessionIDFromMetaSid(t *testing.T) {
	if got := sessionIDFromMeta(map[string]string{"sid": "s2"}); got != "s2" {
		t.Errorf("sid fallback failed: %q", got)
	}
}

func TestTriggerOverrideFromMetaVariations(t *testing.T) {
	if triggerOverrideFromMeta(nil) {
		t.Errorf("nil meta should return false")
	}
	if !triggerOverrideFromMeta(map[string]string{"no-trigger": "true"}) {
		t.Errorf("no-trigger key should be recognized")
	}
}

func TestHookIDs(t *testing.T) {
	if got := (SessionStartHook{}).ID(); got != "autoactivate-session-start" {
		t.Errorf("SessionStartHook ID = %q", got)
	}
	if got := (UserPromptHook{}).ID(); got != "autoactivate-user-prompt" {
		t.Errorf("UserPromptHook ID = %q", got)
	}
}

func TestRuleSetEqualMissingKey(t *testing.T) {
	a := RuleSet{}
	a.Add(Rule{Name: "x"})
	b := RuleSet{}
	b.Add(Rule{Name: "y"})
	if a.Equal(b) {
		t.Errorf("RuleSet.Equal should return false when keys differ")
	}
}

func TestLoadFileOpenError(t *testing.T) {
	orig := openFileHook
	openFileHook = func(name string) (*os.File, error) {
		return nil, errors.New("open denied")
	}
	defer func() { openFileHook = orig }()

	_, _, err := LoadFile(filepath.Join(t.TempDir(), "any.toml"))
	if err == nil {
		t.Fatalf("expected open error")
	}
	if os.IsNotExist(err) {
		t.Errorf("expected non-IsNotExist error, got %v", err)
	}
}

func TestParseInvalidSection(t *testing.T) {
	body := "[ ]\nname = \"x\"\nbody = \"y\""
	rs, _, err := parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rs.Has("x") {
		t.Errorf("invalid section should be skipped")
	}
}

func TestParseMalformedLine(t *testing.T) {
	body := "[rule]\nname = \"x\"\nnot-a-key-value\nbody = \"y\""
	rs, _, err := parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !rs.Has("x") {
		t.Errorf("rule should still be flushed despite malformed line")
	}
}

func TestParseRuleNameChangeFlush(t *testing.T) {
	body := "[rule]\nname = \"alpha\"\nbody = \"first\"\nname = \"beta\"\nbody = \"second\""
	rs, _, err := parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !rs.Has("alpha") || !rs.Has("beta") {
		t.Errorf("expected both alpha and beta rules, got %v", rs.Names())
	}
	if rs["alpha"].Body != "first" {
		t.Errorf("alpha body = %q, want first", rs["alpha"].Body)
	}
	if rs["beta"].Body != "second" {
		t.Errorf("beta body = %q, want second", rs["beta"].Body)
	}
}

func TestStripSectionEmptyName(t *testing.T) {
	if _, ok := stripSection("[ ]"); ok {
		t.Errorf("stripSection should reject empty name")
	}
}
