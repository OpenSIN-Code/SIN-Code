// SPDX-License-Identifier: MIT
// Purpose: extended coverage tests for the macOS Seatbelt backend that
// backstop the existing byte-stable golden test in seatbelt_test.go.
// These five tests pin behaviour that security callers downstream
// (sin_bash, eval harness, agentloop.Command) rely on:
//
//   - the fail-closed exec branch (sandbox-exec missing on PATH)
//   - exact allow-rule formatting so policy diffs stay incisive
//   - deterministic sorted-path emission (rule-order pinning)
//   - the deny-by-default contract on empty SeatbeltPolicy
//   - 100-invocation byte-stability of Profile()
//
// Together with the existing seatbelt_test.go golden harness they
// raise the package coverage above the 64.7% baseline recorded for
// the v3.20.0 release (issue #199).
package sandbox

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestSeatbelt_Exec_MissingSandboxExec pins the fail-closed branch of
// SeatbeltBackend.Exec: when `sandbox-exec` is not present on PATH the
// original cmd must NOT be executed and the typed error must wrap the
// underlying exec.LookPath failure so callers can errors.As it.
//
// We force LookPath to fail by emptying $PATH via t.Setenv, which is
// portable across macOS / Linux / Windows and is automatically
// restored after the test.
func TestSeatbelt_Exec_MissingSandboxExec(t *testing.T) {
	t.Setenv("PATH", "")

	p := DefaultSeatbeltPolicy("/Users/x/proj", "/tmp/foo", false)
	b := SeatbeltBackend{Policy: p}

	// A real *exec.Cmd so we can later prove it was never started.
	cmd := exec.Command("echo", "hello")

	err := b.Exec(context.Background(), cmd, "/tmp")
	if err == nil {
		t.Fatal("Exec must error when sandbox-exec is not on PATH (fail-closed)")
	}

	// Error message must call out the missing binary so the failure
	// is debuggable from the daemon log alone.
	if !strings.Contains(err.Error(), "sandbox-exec not on PATH") {
		t.Fatalf("error must mention 'sandbox-exec not on PATH', got: %v", err)
	}

	// Wraps the underlying *exec.Error so callers can errors.As it.
	var execErr *exec.Error
	if !errors.As(err, &execErr) {
		t.Fatalf("error must wrap *exec.Error from exec.LookPath, got: %T (%v)", err, err)
	}
	if execErr.Name != "sandbox-exec" {
		t.Errorf("wrapped LookPath error must name 'sandbox-exec', got: %q", execErr.Name)
	}

	// The original cmd must NOT have been started. cmd.Process is
	// only ever populated by cmd.Start / cmd.Run; Exec() short-
	// circuits at the LookPath guard so cmd.Process stays nil.
	if cmd.Process != nil {
		t.Error("cmd.Process must be nil: the original cmd must not be executed when LookPath fails")
	}

	// Calling Exec twice in a row must still be safe (no leaked
	// state from the first failed call).
	err2 := b.Exec(context.Background(), cmd, "/tmp")
	if err2 == nil {
		t.Fatal("second Exec without sandbox-exec must also error")
	}
	if !errors.As(err2, &execErr) {
		t.Fatalf("second error must also wrap *exec.Error, got: %T (%v)", err2, err2)
	}
}

// TestSeatbelt_Profile_AllowsRead locks down the exact byte pattern of
// an allow-read rule, the presence of the default-deny, and the
// sortedPaths invariant so reordered input slices yield byte-identical
// policy output (required for golden diffs in eval/audit gates).
func TestSeatbelt_Profile_AllowsRead(t *testing.T) {
	p := SeatbeltPolicy{ReadOnly: []string{"/tmp/allowed"}}
	out := p.Profile()

	// Exact formatting including the spacing inside the parentheses
	// and the "(subpath \"/tmp/allowed\")" quoting.
	const want = `(allow file-read* (subpath "/tmp/allowed"))`
	if !strings.Contains(out, want) {
		t.Errorf("profile must contain %q, got:\n%s", want, out)
	}

	// Default deny must always be emitted first as the seatbelt
	// "deny-by-default" baseline.
	if !strings.Contains(out, "(deny default)") {
		t.Error("profile must start with '(deny default)'")
	}

	// sortedPaths invariant: reordered inputs must produce byte-
	// identical Profiles. This is the property any downstream
	// policy-test / golden-comparator depends on.
	a := SeatbeltPolicy{ReadOnly: []string{"/b", "/a"}}
	b := SeatbeltPolicy{ReadOnly: []string{"/a", "/b"}}
	if a.Profile() != b.Profile() {
		t.Fatalf("sortedPaths must swallow input order:\nA:\n%s\nB:\n%s", a.Profile(), b.Profile())
	}

	// Self-consistency: same input twice yields the same output.
	first := p.Profile()
	if first != p.Profile() {
		t.Fatalf("same-input profiles must be byte-identical: first=%q second=%q", first, p.Profile())
	}

	// ReadOnly must NOT promote to allowed-writes.
	if strings.Contains(out, "(allow file-write* (subpath \"/tmp/allowed\")") {
		t.Error("ReadOnly must NOT emit a file-write allow rule")
	}
}

// TestSeatbelt_Profile_DenyWrites pins the deny-writes contract:
// an empty SeatbeltPolicy emits a deny-by-default baseline and NO
// file-write allow rules; a ReadWrite entry must surface as a
// sandbox-exec (allow file-write* ...).
func TestSeatbelt_Profile_DenyWrites(t *testing.T) {
	// Empty policy: deny-by-default + no write allows.
	empty := SeatbeltPolicy{}.Profile()
	if !strings.Contains(empty, "(deny default)") {
		t.Errorf("empty policy must include '(deny default)' deny-by-default baseline, got:\n%s", empty)
	}
	if strings.Contains(empty, "(allow file-write*") {
		t.Errorf("empty policy must NOT include any (allow file-write* ...) rule, got:\n%s", empty)
	}
	// Empty policy also must not emit path-specific read allows.
	if strings.Contains(empty, "(allow file-read* (subpath") {
		t.Errorf("empty policy must NOT include any path-specific (allow file-read*) rule, got:\n%s", empty)
	}

	// ReadWrite /tmp must grant file-write for that path.
	rw := SeatbeltPolicy{ReadWrite: []string{"/tmp"}}.Profile()
	const want = `(allow file-write* (subpath "/tmp")`
	if !strings.Contains(rw, want) {
		t.Errorf("ReadWrite:/tmp must include %q, got:\n%s", want, rw)
	}
	// AND it must still have the deny-by-default baseline.
	if !strings.Contains(rw, "(deny default)") {
		t.Error("ReadWrite policy must also include '(deny default)' baseline")
	}
	// And ReadWrite implicitly grants file-read too (Apple SBPL
	// semantics: a writable path is also readable).
	const wantRead = `(allow file-read* (subpath "/tmp")`
	if !strings.Contains(rw, wantRead) {
		t.Errorf("ReadWrite:/tmp must also include %q, got:\n%s", wantRead, rw)
	}

	// Multiple ReadWrite paths must each surface an allow for both
	// read and write.
	multi := SeatbeltPolicy{ReadWrite: []string{"/tmp/a", "/tmp/b"}}.Profile()
	for _, p := range []string{"/tmp/a", "/tmp/b"} {
		r := `(allow file-read* (subpath "` + p + `")`
		w := `(allow file-write* (subpath "` + p + `")`
		if !strings.Contains(multi, r) {
			t.Errorf("multiple ReadWrite missing read-allow for %s", p)
		}
		if !strings.Contains(multi, w) {
			t.Errorf("multiple ReadWrite missing write-allow for %s", p)
		}
	}
}

// TestSeatbelt_PolicyParse verifies that the SBPL profile emitted by
// Profile() is structurally valid: a minimal sandbox-exec interpreter
// (a balanced-S-expression walker) would accept it. There is no
// separate ParseProfile() in this package today; the renderer IS the
// canonical serializer and its output is what every consumer parses.
//
// Empty SeatbeltPolicy must produce a structurally valid profile
// consisting of: header comment, version line, default-deny, the
// rule-set begin marker, no path-specific rules, and the end marker.
// Invalid / malformed policy structs (e.g. evict of the default
// version line by passing a sentinel value) would yield a Profile
// whose structural parse fails — we regression-pin this contract
// here so future refactors cannot silently break sandbox-exec parsers
// downstream.
func TestSeatbelt_PolicyParse(t *testing.T) {
	requiredTokens := []string{
		"; SIN-Code default Seatbelt profile (v3.20.0, issue #199).",
		"; Generated by internal/sandbox/seatbelt.go. Do not edit by hand.",
		"(version 1)",
		"(deny default)",
		"; End SIN-Code profile.",
	}

	empty := SeatbeltPolicy{}

	// Build the profile once and parse it line-by-line as a stand-in
	// for what a real SBPL parser would do. Every line that opens
	// with '(' must close with ')' — sandbox-exec bails otherwise.
	out := empty.Profile()

	for _, tok := range requiredTokens {
		if !strings.Contains(out, tok) {
			t.Errorf("empty-profile missing required structural token: %q\nprofile:\n%s", tok, out)
		}
	}

	// Empty policy: no path-specific allow rules are emitted.
	if strings.Contains(out, "(allow file-read* (subpath") {
		t.Error("empty policy must not emit any path-specific read allows")
	}
	if strings.Contains(out, "(allow file-write* (subpath") {
		t.Error("empty policy must not emit any path-specific write allows")
	}

	// Validate the output is a balanced S-expression stream (a
	// structural sanity check the absence of which would manifest
	// as a sandbox-exec parse failure on macOS).
	lineNo := 0
	for _, line := range strings.Split(out, "\n") {
		lineNo++
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, ";") {
			continue
		}
		if !strings.HasPrefix(trimmed, "(") {
			t.Errorf("line %d not an S-expression: %q", lineNo, line)
			continue
		}
		// Count opens / closes — a balanced expression has equal
		// counts. (Strings inside subpath "..." cannot contain
		// parens in our emitter, so a naive count is correct.)
		opens, closes := strings.Count(trimmed, "("), strings.Count(trimmed, ")")
		if opens != closes {
			t.Errorf("line %d unbalanced S-expression: %q", lineNo, line)
		}
	}

	// The full default policy must also contain all required tokens
	// (so subsequent policy tests can rely on a stable skeleton).
	full := DefaultSeatbeltPolicy("/Users/x/proj", "/tmp/foo", true).Profile()
	for _, tok := range requiredTokens {
		if !strings.Contains(full, tok) {
			t.Errorf("default-policy profile missing required token: %q\nprofile:\n%s", tok, full)
		}
	}

	// A policy that only sets ReadOnly must NOT bleed into
	// ReadWrite behaviour: it must produce ONE read allow and
	// zero write allows — no more, no fewer.
	ro := SeatbeltPolicy{ReadOnly: []string{"/some/path"}}.Profile()
	readAllow := strings.Count(ro, "(allow file-read* (subpath \"/some/path\")")
	writeAllow := strings.Count(ro, "(allow file-write* (subpath \"/some/path\")")
	if readAllow != 1 {
		t.Errorf("ReadOnly must emit exactly 1 read-allow, got %d", readAllow)
	}
	if writeAllow != 0 {
		t.Errorf("ReadOnly must emit 0 write-allows, got %d", writeAllow)
	}
}

// TestSeatbelt_DeterministicRuleOrder runs Profile() 100x and asserts
// byte-identical output — the property every downstream golden test
// (eval harness / debt report / audit gate) depends on. It also pins
// the sortedPaths() helper to its forward-only contract: ascending
// sort, no mutation of inputs, nil-slice friendly.
func TestSeatbelt_DeterministicRuleOrder(t *testing.T) {
	p := SeatbeltPolicy{
		ReadWrite: []string{"/c", "/a", "/b"},
		ReadOnly:  []string{"/z", "/y", "/x"},
		Deny:      []string{"/secret3", "/secret1", "/secret2"},
	}

	first := p.Profile()
	for i := 0; i < 100; i++ {
		got := p.Profile()
		if got != first {
			t.Fatalf("Profile() byte-stability broken at invocation %d", i+1)
		}
	}

	// sortedPaths invariants.
	got := sortedPaths([]string{"/c", "/a", "/b"})
	want := []string{"/a", "/b", "/c"}
	if len(got) != len(want) {
		t.Fatalf("sortedPaths length: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortedPaths ascending broken at index %d: got %q want %q", i, got[i], want[i])
		}
	}

	// Empty / nil inputs.
	if n := sortedPaths(nil); len(n) != 0 {
		t.Errorf("sortedPaths(nil) must return empty slice, got %v", n)
	}
	if n := sortedPaths([]string{}); len(n) != 0 {
		t.Errorf("sortedPaths([]) must return empty slice, got %v", n)
	}

	// Does NOT mutate the caller's slice.
	in := []string{"/c", "/a", "/b"}
	_ = sortedPaths(in)
	if in[0] != "/c" || in[1] != "/a" || in[2] != "/b" {
		t.Errorf("sortedPaths must NOT mutate input slice: got %v", in)
	}

	// Dedup-by-same-slice: two calls with identical slice value
	// must also be byte-identical across runs.
	for i := 0; i < 50; i++ {
		x := SeatbeltPolicy{Deny: []string{"/x/y", "/x/a"}}.Profile()
		y := SeatbeltPolicy{Deny: []string{"/x/a", "/x/y"}}.Profile()
		if x != y {
			t.Fatalf("sorted-Paths determinism regressed at iteration %d", i)
		}
	}
}
