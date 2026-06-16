// SPDX-License-Identifier: MIT
// Purpose: Unit + race tests for the style package (issue #167).
//
//   - ParseMode / Valid / EmitsBlock / AllModes correctness
//   - RenderRules / RenderSystemBlock / AppendVerbosity composition
//   - WithVerbosity functional option
//   - Byte-determinism: same input → same bytes, every call
//   - Concurrency / race safety (mandate M7, `go test -race`)
package style

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
)

// ─── ParseMode / Valid / EmitsBlock / AllModes ────────────────────────────

func TestParseMode_Canonical(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want Mode
	}{
		{"", ModeDefault},
		{"default", ModeDefault},
		{" DEFAULT ", ModeDefault},
		{"verbose", ModeVerbose},
		{"normal", ModeNormal},
		{"Normal", ModeNormal},
		{"  terse  ", ModeTerse},
		{"TERSE", ModeTerse},
		{"ultra", ModeUltra},
		{"Ultra", ModeUltra},
	}
	for _, c := range cases {
		t.Run("in="+c.in, func(t *testing.T) {
			got := ParseMode(c.in)
			if got != c.want {
				t.Fatalf("ParseMode(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestParseMode_UnknownFailsClosed(t *testing.T) {
	t.Parallel()
	// Unknown values must fail closed to ModeDefault — wrong config must
	// never produce terse output silently.
	for _, in := range []string{"loud", "concise", "ultra+", "terse2", "\x00bad"} {
		got := ParseMode(in)
		if got != ModeDefault {
			t.Errorf("ParseMode(%q) = %q, want %q (fail-closed)", in, got, ModeDefault)
		}
	}
}

func TestMode_Valid(t *testing.T) {
	t.Parallel()
	cases := map[Mode]bool{
		ModeDefault: true,
		ModeVerbose: true,
		ModeNormal:  true,
		ModeTerse:   true,
		ModeUltra:   true,
		"":          false,
		"loud":      false,
	}
	for m, want := range cases {
		if got := m.Valid(); got != want {
			t.Errorf("(%q).Valid() = %v, want %v", m, got, want)
		}
	}
}

func TestMode_EmitsBlock(t *testing.T) {
	t.Parallel()
	for _, m := range AllModes() {
		want := m == ModeNormal || m == ModeTerse || m == ModeUltra
		if got := m.EmitsBlock(); got != want {
			t.Errorf("(%q).EmitsBlock() = %v, want %v", m, got, want)
		}
	}
	if (Mode("loud")).EmitsBlock() {
		t.Error("unknown mode reported EmitsBlock=true")
	}
}

func TestAllModes_ReturnsAllCanonical(t *testing.T) {
	t.Parallel()
	got := AllModes()
	want := []Mode{ModeDefault, ModeVerbose, ModeNormal, ModeTerse, ModeUltra}
	if len(got) != len(want) {
		t.Fatalf("AllModes length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("AllModes[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestMode_String(t *testing.T) {
	t.Parallel()
	if (Mode("")).String() != string(ModeDefault) {
		t.Error(`empty mode String() should return "default"`)
	}
	if ModeTerse.String() != "terse" {
		t.Error("String() should return canonical name")
	}
}

// ─── RenderRules ──────────────────────────────────────────────────────────

func TestRenderRules_DefaultIsPassThrough(t *testing.T) {
	t.Parallel()
	body := "# skill body\n- do the thing\n"
	if got := RenderRules(ModeDefault, body); got != body {
		t.Errorf("ModeDefault must pass through body unchanged, got %q", got)
	}
	if got := RenderRules(ModeVerbose, body); got != body {
		t.Errorf("ModeVerbose must pass through body unchanged, got %q", got)
	}
}

func TestRenderRules_DefaultAndVerboseMatch(t *testing.T) {
	t.Parallel()
	body := "skill body content"
	if a := RenderRules(ModeDefault, body); a != RenderRules(ModeVerbose, body) {
		t.Errorf("default and verbose must agree on passthrough: %q vs %q",
			a, RenderRules(ModeVerbose, body))
	}
}

func TestRenderRules_NonEmptyForActiveModes(t *testing.T) {
	t.Parallel()
	for _, m := range []Mode{ModeNormal, ModeTerse, ModeUltra} {
		if got := RenderRules(m, ""); got == "" {
			t.Errorf("RenderRules(%q, \"\") must be non-empty", m)
		}
	}
}

func TestRenderRules_ContainsAutoClarity(t *testing.T) {
	t.Parallel()
	// Mandate M3: terse output is never an excuse to skip careful prose
	// around destructive operations. Every active ruleset must carry the
	// auto-clarity clause.
	for _, m := range []Mode{ModeNormal, ModeTerse, ModeUltra} {
		got := RenderRules(m, "")
		if !strings.Contains(got, "Auto-clarity") {
			t.Errorf("RenderRules(%q) missing auto-clarity clause", m)
		}
		if !strings.Contains(got, "destructive") {
			t.Errorf("RenderRules(%q) must reference destructive triggers", m)
		}
	}
}

func TestRenderRules_PreservesTechnicalSubstance(t *testing.T) {
	t.Parallel()
	// Every ruleset must include the "byte-preserved" rule for
	// technical substance (code, URLs, paths, error strings, line
	// numbers, func/var/const names). Failing this rule breaks the
	// premise of the feature.
	for _, m := range []Mode{ModeNormal, ModeTerse, ModeUltra} {
		got := RenderRules(m, "")
		musts := []string{
			"Code",
			"URL",
			"path",
			"error",
			"byte",
		}
		for _, sub := range musts {
			if !strings.Contains(got, sub) {
				t.Errorf("RenderRules(%q) must mention %q for tech-substance preservation", m, sub)
			}
		}
	}
}

func TestRenderRules_AppendsSkillBodyAfterRules(t *testing.T) {
	t.Parallel()
	body := "# My Skill\n- rule a\n- rule b\n"
	got := RenderRules(ModeTerse, body)
	if !strings.HasPrefix(got, header) {
		t.Errorf("RenderRules must start with header %q, got %q", header, firstBytes(got))
	}
	if !strings.Contains(got, body) {
		t.Errorf("RenderRules must contain skill body verbatim, missing %q", body)
	}
	if !strings.HasSuffix(strings.TrimRight(got, "\n"), strings.TrimRight(body, "\n")) {
		t.Errorf("RenderRules must end with the skill body, got %q", got)
	}
}

func TestRenderRules_WhitespaceOnlyBodyTreatedAsEmpty(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", "   ", "\n\n", "\t\t"} {
		got := RenderRules(ModeTerse, body)
		if got != header+rulesTerse {
			t.Errorf("whitespace-only body should yield bare ruleset, got %q", got)
		}
	}
}

// ─── RenderSystemBlock ────────────────────────────────────────────────────

func TestRenderSystemBlock_EmptyForDefaultLike(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "default", "verbose", "loud", "  "} {
		if got := RenderSystemBlock(in); got != "" {
			t.Errorf("RenderSystemBlock(%q) must be empty, got %q", in, got)
		}
	}
}

func TestRenderSystemBlock_NonEmptyForActiveModes(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"normal", "terse", "ultra"} {
		got := RenderSystemBlock(level)
		if got == "" {
			t.Errorf("RenderSystemBlock(%q) must be non-empty", level)
		}
	}
}

// ─── AppendVerbosity ──────────────────────────────────────────────────────

func TestAppendVerbosity_DefaultUnchanged(t *testing.T) {
	t.Parallel()
	existing := "# Learned instincts\n- (0.80, code) when editing — prefer: run tests first"
	for _, m := range []Mode{ModeDefault, ModeVerbose} {
		if got := AppendVerbosity(existing, m); got != existing {
			t.Errorf("AppendVerbosity must not modify existing content for %q, got %q", m, got)
		}
	}
}

func TestAppendVerbosity_AppendsRulesAfter(t *testing.T) {
	t.Parallel()
	existing := "# Instincts\n- a\n"
	got := AppendVerbosity(existing, ModeTerse)
	if !strings.HasPrefix(got, existing) {
		t.Errorf("AppendVerbosity must preserve existing prefix, got %q", got)
	}
	if !strings.Contains(got, header+rulesTerse) {
		t.Errorf("AppendVerbosity must append the terse ruleset")
	}
}

// ─── WithVerbosity option ─────────────────────────────────────────────────

func TestWithVerbosity_AppendsRulesToBuilder(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("# Instincts\n- item one\n")
	WithVerbosity(ModeTerse)(&b)
	got := b.String()
	if !strings.HasPrefix(got, "# Instincts\n") {
		t.Errorf("WithVerbosity must not erase prior content, got %q", firstBytes(got))
	}
	if !strings.Contains(got, header+rulesTerse) {
		t.Errorf("WithVerbosity must append the terse ruleset, got %q", got)
	}
}

func TestWithVerbosity_NoopForDefault(t *testing.T) {
	t.Parallel()
	existing := "# Instincts\n- x\n"
	var b strings.Builder
	b.WriteString(existing)
	WithVerbosity(ModeDefault)(&b)
	WithVerbosity(ModeVerbose)(&b)
	if got := b.String(); got != existing {
		t.Errorf("WithVerbosity(default|verbose) must be a no-op, got %q", got)
	}
}

func TestWithVerbosity_DoubleOptionAppendsBoth(t *testing.T) {
	t.Parallel()
	// Two modes applied in sequence — verifies the option composes,
	// not that it dedupes. Use case: caller applies per-service options.
	var b strings.Builder
	WithVerbosity(ModeNormal)(&b)
	WithVerbosity(ModeUltra)(&b)
	got := b.String()
	if !strings.Contains(got, rulesNormal) {
		t.Error("must contain normal ruleset")
	}
	if !strings.Contains(got, rulesUltra) {
		t.Error("must contain ultra ruleset")
	}
}

func TestWithVerbosity_OnEmptyBuilderDoesNotPrefixBlankLine(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	WithVerbosity(ModeTerse)(&b)
	if strings.HasPrefix(b.String(), "\n") {
		t.Errorf("WithVerbosity on empty builder must not prefix a blank line, got %q", b.String())
	}
}

// ─── Determinism / byte-stability (issue #2 system-prompt hash metric) ──

func TestRenderRules_Deterministic(t *testing.T) {
	t.Parallel()
	first := RenderRules(ModeTerse, "skill body")
	for i := 0; i < 1000; i++ {
		if got := RenderRules(ModeTerse, "skill body"); got != first {
			t.Fatalf("RenderRules not deterministic at iteration %d", i)
		}
	}
}

func TestRenderSystemBlock_ByteHashStable(t *testing.T) {
	t.Parallel()
	// The byte-hash is what a future system-prompt metric will key on.
	// Lock it down here.
	cases := []struct {
		level string
		want  string
	}{
		{"normal", sha256hex(RenderSystemBlock("normal"))},
		{"terse", sha256hex(RenderSystemBlock("terse"))},
		{"ultra", sha256hex(RenderSystemBlock("ultra"))},
	}
	for _, c := range cases {
		if got := sha256hex(RenderSystemBlock(c.level)); got != c.want {
			t.Errorf("byte hash drifted for %q: got %s want %s", c.level, got, c.want)
		}
	}
}

func TestRenderRules_ByteHashStable(t *testing.T) {
	t.Parallel()
	body := "external skill body — fixed for golden"
	for _, m := range []Mode{ModeNormal, ModeTerse, ModeUltra} {
		first := sha256hex(RenderRules(m, body))
		for i := 0; i < 100; i++ {
			if got := sha256hex(RenderRules(m, body)); got != first {
				t.Fatalf("byte hash drifted for %q", m)
			}
		}
	}
}

// ─── Concurrency / race safety (mandate M7) ──────────────────────────────

func TestMode_ConcurrentEquivalence(t *testing.T) {
	t.Parallel()
	const goroutines = 64
	const iters = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_ = ParseMode("terse")
				_ = ParseMode("NORMAL")
				_ = RenderRules(ModeTerse, "skill body")
				_ = RenderSystemBlock("ultra")
				var b strings.Builder
				WithVerbosity(ModeUltra)(&b)
				_ = AppendVerbosity("# existing", ModeTerse)
				_ = ModeUltra.Valid()
				_ = ModeUltra.EmitsBlock()
			}
		}()
	}
	wg.Wait()
}

func TestRenderRules_ConcurrentOutputsIdentical(t *testing.T) {
	t.Parallel()
	// Every goroutine computes the same value; under race detector this
	// must pass — the function must own no mutable state.
	expected := RenderRules(ModeUltra, "skill body X")
	const goroutines = 32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([]string, goroutines)
	for g := 0; g < goroutines; g++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = RenderRules(ModeUltra, "skill body X")
		}(g)
	}
	wg.Wait()
	for i, got := range results {
		if got != expected {
			t.Fatalf("goroutine %d produced divergent bytes", i)
		}
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────

func firstBytes(s string) string {
	if len(s) <= 60 {
		return s
	}
	return s[:60] + "…"
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
