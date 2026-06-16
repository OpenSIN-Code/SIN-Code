// SPDX-License-Identifier: MIT
// Purpose: tests for issue #157 — spec.drift policy key.
package spec

import "testing"

func TestParsePolicy(t *testing.T) {
	cases := map[string]Policy{
		"off":     PolicyOff,
		"OFF":     PolicyOff,
		"  Off ":  PolicyOff,
		"warn":    PolicyWarn,
		"warning": PolicyWarn,
		"error":   PolicyError,
		"strict":  PolicyError,
		"":        PolicyError, // empty defaults to error (fail-closed)
		"bogus":   PolicyError, // unknown defaults to error
	}
	for in, want := range cases {
		if got := ParsePolicy(in); got != want {
			t.Errorf("ParsePolicy(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckReport_ShouldBlock(t *testing.T) {
	// Empty report: no block.
	empty := &CheckReport{}
	if empty.ShouldBlock(PolicyError) {
		t.Error("empty report should not block even in error mode")
	}
	if empty.ShouldBlock(PolicyOff) {
		t.Error("empty report should not block in off mode")
	}

	// Report with a must-failure.
	failing := &CheckReport{
		Results: []CheckResult{{ID: "R1", Passed: false, Priority: Must}},
	}
	if !failing.ShouldBlock(PolicyError) {
		t.Error("must-failure should block in error mode")
	}
	if failing.ShouldBlock(PolicyWarn) {
		t.Error("must-failure should NOT block in warn mode")
	}
	if failing.ShouldBlock(PolicyOff) {
		t.Error("must-failure should NOT block in off mode")
	}

	// Report with only should-priority failures: never blocks.
	shouldFail := &CheckReport{
		Results: []CheckResult{{ID: "R1", Passed: false, Priority: Should}},
	}
	if shouldFail.ShouldBlock(PolicyError) {
		t.Error("should-failure should never block")
	}

	// Skipped failures: never block.
	skipped := &CheckReport{
		Results: []CheckResult{{ID: "R1", Passed: false, Skipped: true, Priority: Must}},
	}
	if skipped.ShouldBlock(PolicyError) {
		t.Error("skipped failures should never block")
	}
}
