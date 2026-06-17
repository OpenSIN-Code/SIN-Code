// SPDX-License-Identifier: MIT
// Purpose: race-clean tests for the deterministic prompt → mode
// classifier. Checks that the decision matrix is byte-stable per
// input and that the "no signal" fallback is reachable.
package autolevel

import (
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
)

func TestClassifyExplicitPlan(t *testing.T) {
	cases := []struct {
		prompt string
		mode   permission.Mode
		reason string
	}{
		{"what does this code do?", permission.ModePlan, "explicit plan / read-only verb"},
		{"explain the auth flow", permission.ModePlan, "explicit plan / read-only verb"},
		{"plan this refactor first", permission.ModePlan, "explicit plan / read-only verb"},
		{"dry-run the migration", permission.ModePlan, "explicit plan / read-only verb"},
	}
	for _, c := range cases {
		got := Classify(c.prompt)
		if got.Mode != c.mode {
			t.Errorf("Classify(%q) mode = %q, want %q (reason=%q)", c.prompt, got.Mode, c.mode, got.Reason)
		}
		if got.Reason != c.reason {
			t.Errorf("Classify(%q) reason = %q, want %q", c.prompt, got.Reason, c.reason)
		}
	}
}

func TestClassifyExplicitAccept(t *testing.T) {
	cases := []string{
		"edit the greeting function",
		"add tests for store.go",
		"implement the new endpoint",
		"refactor dispatcher to use sync.Map",
		"make it print uppercase",
		"rename Type to Mode",
	}
	for _, p := range cases {
		got := Classify(p)
		if got.Mode != permission.ModeAcceptEdits {
			t.Errorf("Classify(%q) mode = %q, want acceptEdits", p, got.Mode)
		}
	}
}

func TestClassifyDestructive(t *testing.T) {
	cases := []string{
		"rm -rf node_modules",
		"force push the rewrite branch",
		"drop table users",
		"wipe the cache",
	}
	for _, p := range cases {
		got := Classify(p)
		if got.Mode != permission.ModeBypass {
			t.Errorf("Classify(%q) mode = %q, want bypass", p, got.Mode)
		}
	}
}

func TestClassifyNoSignal(t *testing.T) {
	cases := []struct {
		prompt string
		expect permission.Mode
		reason string
	}{
		{"hello", permission.ModeDefault, "no classifier signal"},
		{"", permission.ModeDefault, "no classifier signal"},
		// "lunch?" ends with `?` so explicit_question_only fires
		// (weak signal, weight=3) — Plan is the conservative pick.
		{"lunch?", permission.ModePlan, "ending with ?"},
	}
	for _, c := range cases {
		got := Classify(c.prompt)
		if got.Mode != c.expect {
			t.Errorf("Classify(%q) mode = %q, want %q", c.prompt, got.Mode, c.expect)
		}
		if got.Reason != c.reason {
			t.Errorf("Classify(%q) reason = %q, want %q", c.prompt, got.Reason, c.reason)
		}
	}
}

func TestClassifyByteStable(t *testing.T) {
	a := Classify("edit the foo function")
	b := Classify("edit the foo function")
	c := Classify("edit the foo function")
	if a != b || b != c {
		t.Fatalf("Classify must be byte-stable:\n%v\n!=\n%v\n!=\n%v", a, b, c)
	}
}

func TestClassifyPlanWinsOverAccept(t *testing.T) {
	// a prompt that says "refactor X" (accept) AND ends with "?"
	// should pick plan (higher weight) because plan is a stronger
	// signal for read-only intent.
	got := Classify("explain how to refactor X?")
	if got.Mode != permission.ModePlan {
		t.Errorf("plan must beat accept when both fire: got %q (%s)",
			got.Mode, got.Reason)
	}
}

func TestClassifyQuestionMarkWeak(t *testing.T) {
	// "??" alone should NOT emit a plan reason because the
	// plan rule's weight (3) is below `add tests` (6).
	got := Classify("add tests??")
	if got.Mode != permission.ModeAcceptEdits {
		t.Errorf("explicit_test_accept beats question mark: got %q (%s)",
			got.Mode, got.Reason)
	}
}

func TestClassifyTiebreakByHitIndex(t *testing.T) {
	// The production rules all have distinct weights, so the
	// equal-weight tie-break branch is only reachable via the
	// package-level rules hook variable.
	orig := rules
	defer func() { rules = orig }()

	rules = []rule{
		{
			Name:    "late_hit",
			Mode:    permission.ModePlan,
			Reason:  "late hit",
			Weight:  5,
			Phrases: []string{"world"},
		},
		{
			Name:    "early_hit",
			Mode:    permission.ModeAcceptEdits,
			Reason:  "early hit",
			Weight:  5,
			Phrases: []string{"hello"},
		},
	}

	got := Classify("hello world")
	if got.Mode != permission.ModeAcceptEdits || got.Reason != "early hit" {
		t.Errorf("tiebreak by earliest hit: got mode=%q reason=%q, want acceptEdits/early hit", got.Mode, got.Reason)
	}
}
