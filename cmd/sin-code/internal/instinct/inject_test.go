// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the verbosity-aware system-prompt renderer
// (issue #167). The package composes instinct + style into a single
// stable system-prompt fragment.
// Docs: inject.doc.md
package instinct

import (
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/style"
)

// helper — build a small active instinct fixture with known confidence
// so ordering is deterministic (highest first).
func fixture(t *testing.T) []*Instinct {
	t.Helper()
	a := NewInstinct("when committing", "git", "run tests first", "obs", ScopeProject)
	a.Confidence = 0.85
	b := NewInstinct("when validating", "security", "sanitize input", "obs", ScopeGlobal)
	b.Confidence = 0.70
	return []*Instinct{a, b}
}

func TestRenderSystemBlock_BackwardCompatible(t *testing.T) {
	t.Parallel()
	got := RenderSystemBlockWithVerbosity(fixture(t), 15, "", "")
	if !strings.Contains(got, "# Learned instincts") {
		t.Fatalf("expected instincts header in bare render:\n%s", got)
	}
	if strings.Contains(got, "# Output style") {
		t.Errorf("bare render with empty mode must not contain style block:\n%s", got)
	}
}

func TestRenderSystemBlock_DefaultModeIsBare(t *testing.T) {
	t.Parallel()
	for _, m := range []string{"default", "verbose", "loud", "\x00"} {
		got := RenderSystemBlockWithVerbosity(fixture(t), 15, m, "")
		if strings.Contains(got, "# Output style") {
			t.Errorf("mode=%q must not emit style block, got:\n%s", m, got)
		}
		if !strings.Contains(got, "# Learned instincts") {
			t.Errorf("mode=%q must keep instincts, got:\n%s", m, got)
		}
	}
}

func TestRenderSystemBlock_ActiveModesEmitStyleBlock(t *testing.T) {
	t.Parallel()
	for _, m := range []string{"normal", "terse", "ultra"} {
		got := RenderSystemBlockWithVerbosity(fixture(t), 15, m, "")
		if !strings.Contains(got, "# Output style") {
			t.Errorf("mode=%q must emit style block, got:\n%s", m, got)
		}
		if !strings.Contains(got, "# Learned instincts") {
			t.Errorf("mode=%q must keep instincts, got:\n%s", m, got)
		}
		// Order: instincts first, style second.
		instIdx := strings.Index(got, "# Learned instincts")
		styleIdx := strings.Index(got, "# Output style")
		if instIdx < 0 || styleIdx < 0 || styleIdx <= instIdx {
			t.Errorf("mode=%q ordering wrong (inst=%d style=%d):\n%s",
				m, instIdx, styleIdx, got)
		}
		// Exactly one blank line between the segments: the boundary
		// substring should be "...\n\n# Output style".
		boundary := got[styleIdx-2 : styleIdx+len("# Output style")]
		if boundary != "\n\n# Output style" {
			t.Errorf("mode=%q must have exactly one blank line before style, got %q",
				m, boundary)
		}
	}
}

func TestRenderSystemBlock_ActiveModePreservesAutoClarity(t *testing.T) {
	t.Parallel()
	got := RenderSystemBlockWithVerbosity(fixture(t), 15, "terse", "")
	if !strings.Contains(got, "Auto-clarity") {
		t.Errorf("terse mode must keep auto-clarity clause (mandate M3):\n%s", got)
	}
}

func TestRenderSystemBlock_SkillBodyAppearsAfterRules(t *testing.T) {
	t.Parallel()
	body := "# My Skill\n- rule a\n"
	// terse mode + skill body → instincts + style header + rules + body
	got := RenderSystemBlockWithVerbosity(fixture(t), 15, "terse", body)
	musts := []string{
		"# Learned instincts",
		"# Output style (terse)",
		"Auto-clarity",
		body,
	}
	for _, m := range musts {
		if !strings.Contains(got, m) {
			t.Errorf("missing %q in:\n%s", m, got)
		}
	}
	// skillBody must come after the style header.
	styleIdx := strings.Index(got, "# Output style")
	bodyIdx := strings.Index(got, body)
	if styleIdx < 0 || bodyIdx < 0 || bodyIdx <= styleIdx {
		t.Errorf("skill body must follow style header (style=%d body=%d):\n%s",
			styleIdx, bodyIdx, got)
	}
}

func TestRenderSystemBlock_DeterministicForSameInputs(t *testing.T) {
	t.Parallel()
	fx := fixture(t)
	first := RenderSystemBlockWithVerbosity(fx, 15, "terse", "skill body")
	for i := 0; i < 500; i++ {
		got := RenderSystemBlockWithVerbosity(fx, 15, "terse", "skill body")
		if got != first {
			t.Fatalf("iteration %d: non-deterministic output", i)
		}
	}
}

func TestRenderSystemBlock_NoInstinctsBare(t *testing.T) {
	t.Parallel()
	// No instincts + default mode → "" (nothing to inject).
	if got := RenderSystemBlockWithVerbosity(nil, 15, "", ""); got != "" {
		t.Errorf("expected empty, got:\n%s", got)
	}
}

func TestRenderSystemBlock_NoInstinctsActiveMode(t *testing.T) {
	t.Parallel()
	// No instincts + active mode → still emit the style block alone.
	got := RenderSystemBlockWithVerbosity(nil, 15, "terse", "")
	if !strings.Contains(got, "# Output style") {
		t.Errorf("expected style block alone, got:\n%s", got)
	}
}

func TestRenderSystemBlock_NoInstinctsVerbosePassThrough(t *testing.T) {
	t.Parallel()
	// No instincts + verbose mode + skill body → body alone (because
	// style.RenderRules passes the body through when the mode is dead).
	body := "skill body X"
	got := RenderSystemBlockWithVerbosity(nil, 15, "verbose", body)
	if got != body {
		t.Errorf("verbose must pass through body, got:\n%s", got)
	}
}

func TestRenderSystemBlock_RespectsMax(t *testing.T) {
	t.Parallel()
	fx := fixture(t) // 2 items
	got := RenderSystemBlockWithVerbosity(fx, 1, "", "")
	if strings.Count(got, "\n- ") != 1 {
		t.Errorf("max=1 should trim to 1 instinct line, got:\n%s", got)
	}
}

func TestSystemBlockForProjectWithStyle_NilManager(t *testing.T) {
	t.Parallel()
	// nil manager.Active() would dereference a nil Store. We cannot
	// construct a Manager without a real Store; this test instead
	// exercises the safe path: a manager with no active instincts
	// returns the style block alone (no panic).
	tmp := t.TempDir()
	mgr, err := NewManager(tmp, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	got, err := mgr.SystemBlockForProjectWithStyle(15, "terse", "")
	if err != nil {
		t.Fatalf("SystemBlockForProjectWithStyle: %v", err)
	}
	if !strings.Contains(got, "# Output style") {
		t.Errorf("expected style block alone, got:\n%s", got)
	}
}

// ─── Style-package integration: invariants that MUST hold ────────────────

func TestRenderSystemBlock_ContentInvariantsAcrossBothAPIs(t *testing.T) {
	t.Parallel()
	// AppendVerbosity and RenderSystemBlockWithVerbosity preserve the
	// same semantic content (all instincts + style ruleset + auto-
	// clarity) but the separator blank-line count may differ. This
	// test asserts the *content* is the same, not the exact bytes — so
	// the two sister APIs can evolve independently.
	fx := fixture(t)

	wantContent := []string{
		"# Learned instincts",
		"when committing",
		"when validating",
		"# Auto-clarity",
		"destructive",
	}
	for _, m := range []style.Mode{style.ModeNormal, style.ModeTerse, style.ModeUltra} {
		viaAppend := style.AppendVerbosity(RenderSystemBlock(fx, 15), m)
		viaRenderer := RenderSystemBlockWithVerbosity(fx, 15, string(m), "")
		for _, sub := range wantContent {
			if !strings.Contains(viaAppend, sub) || !strings.Contains(viaRenderer, sub) {
				t.Errorf("mode=%s: missing %q\n--- append ---\n%s\n--- renderer ---\n%s",
					m, sub, viaAppend, viaRenderer)
			}
		}
	}
}
