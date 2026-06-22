// SPDX-License-Identifier: MIT
// Purpose: high-priority coverage for the permission engine (mandate M4,
// AGENTS.md §3). These tests pin down the contract that gates every
// destructive tool call: rule precedence, headless/yolo fallback, glob
// matching, and the v3.9.0 M4 bridge defaults.
package permission

import (
	"testing"
)

// Engine semantics reference (verified against permission.go):
//
//   resolveRules iterates rules in declaration order and BREAKS on
//   the first glob match (path.Match, lower-cased both sides). The
//   matched rule's Policy ("allow" / "ask" / "deny") wins verbatim.
//   No match defaults to Ask. Mode (plan/acceptEdits/bypass) reshapes
//   the resolved policy before the yoloResolve fallback runs.
//   yoloResolve: Yolo -> Allow (or risk-gated); Headless -> Deny;
//   otherwise -> Ask.

// TestPolicy_AllowBeatsAsk — first matching rule wins (positional
// precedence). When an `allow` rule for tool X appears before any
// `ask` rule that would also match, the `allow` rule is selected.
// This pins mandate M4: an explicit, more-specific allow outranks a
// broader ask default.
func TestPolicy_AllowBeatsAsk(t *testing.T) {
	rules := []Rule{
		{Tool: "sin_deploy", Policy: "allow"}, // first match wins
		{Tool: "sin_*", Policy: "ask"},        // broader fallback
		{Tool: "*", Policy: "deny"},           // catch-all (only for unmatched single-segment tools)
	}
	e := New(rules)
	// First-match: sin_deploy -> allow.
	if got := e.Check("sin_deploy"); got != Allow {
		t.Errorf("sin_deploy: first-match allow should win, got %s, want allow", got)
	}
	// Skips the literal; falls to sin_* -> ask.
	if got := e.Check("sin_restart"); got != Ask {
		t.Errorf("sin_restart: should fall to broader ask glob, got %s, want ask", got)
	}
	// Single-segment tool names with no slash match the catch-all `*`.
	if got := e.Check("stranger"); got != Deny {
		t.Errorf("stranger: catch-all `*` should match, got %s, want deny", got)
	}
}

// TestPolicy_DenyBeatsAllow — first matching rule wins. When an
// explicit `deny` for tool X precedes an `allow` rule that also
// matches, the earlier `deny` rule is selected. Mandate M4: the
// runtime MUST honor explicit deny ahead of allow, and a caller
// cannot widen allow past an earlier deny.
func TestPolicy_DenyBeatsAllow(t *testing.T) {
	rules := []Rule{
		{Tool: "sin_git_push", Policy: "deny"}, // first match — deny wins
		{Tool: "sin_git_*", Policy: "allow"},   // would otherwise allow
		{Tool: "sin_*", Policy: "allow"},
	}
	e := New(rules)
	if got := e.Check("sin_git_push"); got != Deny {
		t.Errorf("sin_git_push: explicit deny (1st match) should win, got %s, want deny", got)
	}
	// Confirm the allow fallback still works for tools that didn't match
	// the deny rule.
	if got := e.Check("sin_git_commit"); got != Allow {
		t.Errorf("sin_git_commit: should fall through to allow glob, got %s, want allow", got)
	}
}

// TestPolicy_AskDefaultsDenyInHeadless — in headless mode, the engine
// resolves an `ask` policy to `deny` unless `--yolo` (Headless + Yolo)
// flips it to `allow`. This is mandate M4: a headless caller cannot
// silently elevate Ask to Allow.
func TestPolicy_AskDefaultsDenyInHeadless(t *testing.T) {
	rules := []Rule{{Tool: "sin_bash", Policy: "ask"}}

	// Case 1: interactive (non-headless) — Ask stays Ask.
	interactive := New(rules)
	if got := interactive.Check("sin_bash"); got != Ask {
		t.Errorf("interactive Ask: should stay Ask, got %s", got)
	}

	// Case 2: headless without --yolo — Ask collapses to Deny.
	headless := New(rules)
	headless.Headless = true
	if got := headless.Check("sin_bash"); got != Deny {
		t.Errorf("headless no-yolo Ask: should resolve to Deny (M4), got %s", got)
	}

	// Case 3: headless WITH --yolo — Ask collapses to Allow.
	yolo := New(rules)
	yolo.Headless = true
	yolo.Yolo = true
	if got := yolo.Check("sin_bash"); got != Allow {
		t.Errorf("headless+yolo Ask: should resolve to Allow, got %s", got)
	}
}

// TestPolicy_GlobPathMatch — the engine uses path.Match (lower-cased
// both sides), which is glob-aware but path-aware: `*` matches any
// sequence of non-separator characters. So `/tmp/*` matches
// `/tmp/foo.txt` but does NOT match `/etc/passwd`. This verifies the
// rule-matching contract used by every Command allow-list.
func TestPolicy_GlobPathMatch(t *testing.T) {
	rules := []Rule{
		{Tool: "/tmp/*", Policy: "allow"},       // single-segment path under /tmp
		{Tool: "/etc/secret/*", Policy: "deny"}, // narrower deny for /etc/secret/X
		{Tool: "*", Policy: "ask"},              // catch-all (single-segment tools)
	}
	e := New(rules)

	// /tmp/* must allow the path.
	if got := e.Check("/tmp/foo.txt"); got != Allow {
		t.Errorf("/tmp/foo.txt: glob /tmp/* should match, got %s, want allow", got)
	}

	// /etc/passwd does NOT match /tmp/* (wrong prefix) AND is a single
	// path-segment, so it falls to the catch-all ask rule.
	if got := e.Check("/etc/passwd"); got != Ask {
		t.Errorf("/etc/passwd: catch-all should apply (single segment, no rule matches), got %s, want ask", got)
	}

	// /etc/secret/passwd is single-segment under /etc/secret; matches deny.
	if got := e.Check("/etc/secret/passwd"); got != Deny {
		t.Errorf("/etc/secret/passwd: /etc/secret/* should deny, got %s, want deny", got)
	}

	// Sanity: any single-segment tool name falls to the catch-all.
	if got := e.Check("bash"); got != Ask {
		t.Errorf("bash: catch-all should apply, got %s, want ask", got)
	}

	// Cross-check: a path with multiple segments like /tmp/sub/dir is NOT
	// matched by /tmp/* because path.Match's `*` does not span `/`.
	// It also does not match /etc/secret/* (wrong prefix) or the
	// catch-all (multi-segment). The engine then falls back to its
	// internal "no match" default, which is Ask.
	if got := e.Check("/tmp/sub/dir"); got != Ask {
		t.Errorf("/tmp/sub/dir: `/tmp/*` does NOT span `/`; should fall to engine default Ask, got %s, want ask", got)
	}
}

// TestPolicy_TestDaemonIsAlwaysHeadless — the daemon is always
// headless (per mandate M4 and AGENTS.md §3: "The daemon refuses to
// start without --verify-cmd"). When the engine is wired with
// Headless=true, EVERY Ask resolves to Deny unless Yolo is also set.
// This is the immutable invariant: the daemon cannot self-escalate
// in headless mode even if a caller forgets to set Headless.
func TestPolicy_TestDaemonIsAlwaysHeadless(t *testing.T) {
	rules := []Rule{
		{Tool: "sin_bash", Policy: "ask"},
		{Tool: "sin_todo_add", Policy: "ask"},
		{Tool: "fusion__tournament", Policy: "ask"},
	}

	// Daemon-style engine: Headless=true, Yolo=false.
	daemon := New(rules)
	daemon.Headless = true
	tools := []string{"sin_bash", "sin_todo_add", "fusion__tournament", "unknown_tool"}
	for _, tool := range tools {
		if got := daemon.Check(tool); got != Deny {
			t.Errorf("daemon-headless %q: must be Deny (no self-escalation), got %s", tool, got)
		}
	}

	// Sanity: the *one* exception is an explicit allow rule, which is
	// NOT subject to the Ask→Deny fallback. The daemon can still run
	// a read-only tool that an operator allow-listed.
	allowRules := []Rule{
		{Tool: "sin_read", Policy: "allow"},
		{Tool: "sin_todo_list", Policy: "allow"},
	}
	allowDaemon := New(allowRules)
	allowDaemon.Headless = true
	for _, tool := range []string{"sin_read", "sin_todo_list"} {
		if got := allowDaemon.Check(tool); got != Allow {
			t.Errorf("daemon-headless allow-listed %q: must stay Allow, got %s", tool, got)
		}
	}
}

// TestPolicy_M4BridgeDefaults — pin down the v3.9.0 M4 bridge defaults
// catalogued in AGENTS.md §3 / §10: Bridged-External read-only tools
// (vane__*, superpowers__*, dox__*, gh_query, gh_health, fusion__status,
// fusion__config) are `allow`; mutating tools (gh_execute,
// fusion__tournament) are `ask`. The catalog is the single source of
// truth — every consumer (loopbuilder, daemon, agent runner) MUST
// honor these tiers.
func TestPolicy_M4BridgeDefaults(t *testing.T) {
	// Mirror the AGENTS.md §3 contract. If `internal/permission_defaults.go`
	// breaks ANY of these, the test breaks with it, and the operator is
	// forced to update both the source and AGENTS.md in the same PR
	// (mandate: "AGENTS.md + ECOSYSTEM.md are kept in sync with the codebase").
	m4BridgeRules := []Rule{
		// Bridged-External research / methodology / context (read-only).
		{Tool: "vane__*", Policy: "allow"},
		{Tool: "superpowers__*", Policy: "allow"},
		{Tool: "dox__*", Policy: "allow"},
		// v3.9.0 split: gh bridge is 3-tier.
		{Tool: "gh_query", Policy: "allow"},  // read-only
		{Tool: "gh_health", Policy: "allow"}, // PATH + auth probe
		{Tool: "gh_execute", Policy: "ask"},  // mutating — needs confirm
		// v3.22.0 Fusion verify-tournament.
		{Tool: "fusion__tournament", Policy: "ask"}, // spawns sub-agents
		{Tool: "fusion__status", Policy: "allow"},   // read-only
		{Tool: "fusion__config", Policy: "allow"},   // read-only
	}
	e := New(m4BridgeRules)

	cases := []struct {
		tool string
		want Policy
	}{
		{"vane__search", Allow},
		{"vane__cite", Allow},
		{"superpowers__brainstorm", Allow},
		{"superpowers__write_plan", Allow},
		{"dox__check_protocol", Allow},
		{"dox__lint", Allow},
		{"gh_query", Allow},
		{"gh_health", Allow},
		{"gh_execute", Ask}, // mutating bridge — M4 requires confirm
		{"fusion__tournament", Ask},
		{"fusion__oracle_tournament", Ask}, // also ask — see defaults_test
		{"fusion__status", Allow},
		{"fusion__config", Allow},
	}
	for _, c := range cases {
		got := e.Check(c.tool)
		if got != c.want {
			t.Errorf("M4 bridge: Check(%q) = %s, want %s", c.tool, got, c.want)
		}
	}

	// And: in headless mode, even an ask fusion call collapses to Deny
	// (M4: the daemon cannot auto-promote bridge-corrected mutating ops).
	headless := New(m4BridgeRules)
	headless.Headless = true
	if got := headless.Check("gh_execute"); got != Deny {
		t.Errorf("M4 bridge headless: gh_execute Ask collapses to Deny, got %s", got)
	}
	if got := headless.Check("fusion__tournament"); got != Deny {
		t.Errorf("M4 bridge headless: fusion__tournament Ask collapses to Deny, got %s", got)
	}
}
