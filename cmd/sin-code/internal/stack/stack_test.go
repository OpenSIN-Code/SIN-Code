// SPDX-License-Identifier: MIT
// Purpose: tests for the stack orchestrator. All tests are hermetic: no
// network (no superpowers.Install — it shells out to git), no real vane
// instance, no real $HOME. Every test uses setupTestHome to redirect
// $SIN_CODE_HOME to a t.TempDir().
// Docs: stack.doc.md
package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dox"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/superpowers"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vane"
)

// ── Test helpers ───────────────────────────────────────────────────────

// setupTestHome redirects $SIN_CODE_HOME to a fresh t.TempDir() and
// returns its absolute path. t.Setenv automatically restores the
// previous value when the test exits. Returns the home dir AND the
// per-test working directory (also under t.TempDir so go-cmp-style
// assertions are stable).
func setupTestHome(t *testing.T) (home, cwd string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("SIN_CODE_HOME", home)
	cwd = t.TempDir()
	t.Chdir(cwd)
	return home, cwd
}

// seedSuperpowersLayers writes two minimal fake SKILL.md files into
// $SIN_CODE_HOME/skills/superpowers/<name>/ and a PinState so that
// superpowers.List() and superpowers.CurrentPin() succeed without
// ever calling Install() (which would require network + git).
// Returns the seeded SHA for assertions.
func seedSuperpowersLayers(t *testing.T, home string) (sha string) {
	t.Helper()
	const fakeSHA = "0123456789abcdef0123456789abcdef01234567"
	skillsRoot := superpowers.SkillsDir()
	for _, name := range []string{"alpha", "bravo"} {
		dir := filepath.Join(skillsRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		body := "---\nname: " + name + "\ndescription: fake " + name + " skill\n---\n\n# " + name + "\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	// Write the pin file in the exact shape superpowers expects.
	pinPath := filepath.Join(skillsRoot, ".sin-code-pin")
	if err := os.WriteFile(pinPath, []byte(`{
  "sha": "`+fakeSHA+`",
  "branch": "main",
  "updated_at": "2026-06-13T00:00:00Z"
}`), 0o644); err != nil {
		t.Fatalf("write pin: %v", err)
	}
	return fakeSHA
}

// ── Tests: Install ─────────────────────────────────────────────────────

// TestInstallDoxAndVaneOnly verifies that Install(opts{SkipSuperpowers: true})
// writes a dox block into AGENTS.md and that the block contains the
// SIN-Code dox:begin marker exactly once after two consecutive calls
// (idempotency).
func TestInstallDoxAndVaneOnly(t *testing.T) {
	home, cwd := setupTestHome(t)
	_ = home // dox.InjectRoot does not read home; vane does — and we skip vane below.
	agents := filepath.Join(cwd, dox.AgentsFileName)
	if err := os.WriteFile(agents, []byte("# Project\n\nHello.\n"), 0o644); err != nil {
		t.Fatalf("seed agents: %v", err)
	}
	opts := InstallOptions{
		SkipSuperpowers: true,
		SkipVane:        true,
		AgentsMDPath:    agents,
	}
	rep1 := Install(opts)
	if !rep1.AllOK {
		t.Fatalf("expected rep1.AllOK=true (superpowers+vane skipped, dox ok), got: %#v", rep1.Components)
	}
	// The dox layer must be OK (it is the only one we exercise here
	// that does not require external state).
	var doxComp *Component
	for i := range rep1.Components {
		if rep1.Components[i].Layer == string(LayerDox) {
			doxComp = &rep1.Components[i]
			break
		}
	}
	if doxComp == nil || !doxComp.OK {
		t.Fatalf("dox component missing or not OK: %#v", rep1.Components)
	}

	// Idempotency: re-run, count marker occurrences.
	_ = Install(opts)
	body, err := os.ReadFile(agents)
	if err != nil {
		t.Fatalf("read agents: %v", err)
	}
	count := strings.Count(string(body), dox.BeginMarker)
	if count != 1 {
		t.Fatalf("expected exactly 1 dox:begin marker after 2 installs, got %d\nbody:\n%s", count, string(body))
	}
}

// ── Tests: Doctor ──────────────────────────────────────────────────────

// TestDoctorReportsMissingLayers: an empty root with no SIN_CODE_HOME
// install has no skills, no pin, no vane config — Doctor must report
// AllOK=false. Note: we DO seed $SIN_CODE_HOME so the function can
// resolve home without touching the real user filesystem.
func TestDoctorReportsMissingLayers(t *testing.T) {
	home, _ := setupTestHome(t)
	_ = home
	rep := Doctor(t.TempDir())
	if rep.AllOK {
		t.Fatalf("expected AllOK=false on empty install, got: %#v", rep.Components)
	}
	// superpowers: must report not-installed.
	var sup *Component
	for i := range rep.Components {
		if rep.Components[i].Name == "superpowers" {
			sup = &rep.Components[i]
			break
		}
	}
	if sup == nil {
		t.Fatalf("superpowers component missing from Doctor: %#v", rep.Components)
	}
	if sup.OK {
		t.Fatalf("expected superpowers OK=false on empty install, got true: %s", sup.Detail)
	}
}

// TestDoctorVaneDownIsNotFatal: a configured vane URL with an
// unreachable instance is reported as vane.health OK=true with a
// "DOWN" detail (graceful degradation: configuration is valid, the
// instance is just unreachable). The overall report may be false
// because superpowers is not seeded, but vane.health MUST be OK.
func TestDoctorVaneDownIsNotFatal(t *testing.T) {
	setupTestHome(t)
	// Force the vane config to an unreachable URL so the test is
	// hermetic regardless of whether a real Vane instance is running.
	if err := vane.SaveConfig(vane.Config{BaseURL: "http://127.0.0.1:1", TimeoutSeconds: 1}); err != nil {
		t.Fatalf("save vane config: %v", err)
	}
	rep := Doctor(t.TempDir())
	var health *Component
	for i := range rep.Components {
		if rep.Components[i].Name == "vane.health" {
			health = &rep.Components[i]
			break
		}
	}
	if health == nil {
		t.Fatalf("vane.health component missing: %#v", rep.Components)
	}
	if !health.OK {
		t.Fatalf("vane.health must be OK (down is informational), got false: %s", health.Detail)
	}
	if !strings.Contains(strings.ToUpper(health.Detail), "DOWN") &&
		!strings.Contains(strings.ToUpper(health.Detail), "INFORMATIONAL") {
		t.Fatalf("vane.health detail should mention DOWN/informational, got: %s", health.Detail)
	}
}

// ── Tests: helpers ─────────────────────────────────────────────────────

// TestFormatOutput exercises the three marker paths: ✓ (ok), ✗ (fail),
// - (skipped). A single Report with one of each must produce a
// stable, greppable string.
func TestFormatOutput(t *testing.T) {
	r := Report{Components: []Component{
		{Name: "ok.layer", Layer: "x", OK: true, Detail: "fine"},
		{Name: "fail.layer", Layer: "y", OK: false, Detail: "broken"},
		{Name: "skip.layer", Layer: "z", OK: true, Skipped: true, Detail: "skipped by request"},
	}}
	out := Format(r)
	for _, want := range []string{"ok.layer", "fail.layer", "skip.layer"} {
		if !strings.Contains(out, want) {
			t.Errorf("Format output missing %q\noutput:\n%s", want, out)
		}
	}
	// Marker sanity: each component line should contain its marker.
	if !strings.Contains(out, "✓") {
		t.Errorf("expected ✓ marker in output:\n%s", out)
	}
	if !strings.Contains(out, "✗") {
		t.Errorf("expected ✗ marker in output:\n%s", out)
	}
	if !strings.Contains(out, "-") {
		t.Errorf("expected - marker in output:\n%s", out)
	}
	if !strings.Contains(out, "DEGRADED") {
		t.Errorf("expected DEGRADED overall in output:\n%s", out)
	}
}

// TestShortSHA: boundary cases — empty, <12, ==12, >12.
func TestShortSHA(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "abc"},
		{"0123456789ab", "0123456789ab"},        // exactly 12 → unchanged
		{"0123456789abcdef", "0123456789ab"},    // >12 → first 12
		{"   0123456789abc   ", "0123456789ab"}, // 13 chars trimmed → 13 → first 12
		{"\tabc\n", "abc"},
	}
	for _, tc := range cases {
		got := shortSHA(tc.in)
		if got != tc.want {
			t.Errorf("shortSHA(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
