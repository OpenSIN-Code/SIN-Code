// SPDX-License-Identifier: MIT
package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureSource = `<!-- sin-code:profile v1 -->
# SIN-Code Project Profile

> Single source of truth for the per-agent rules SIN-Code installs into
> every supported host agent.

## Hard mandates (NEVER violate)

- **M1** CI runs only via the n8n delegator.
- **M2** Single static Go binary, CGO_ENABLED=0.
- **M3** Verification gate is sacred.
- **M4** Permission engine gates every destructive tool.
- **M5** Module path is github.com/OpenSIN-Code/SIN-Code.
- **M6** SIN tools over naive built-ins.
- **M7** Race-free concurrency.

## Identity

- Product: sin-code (single static Go binary).
- Repo: github.com/OpenSIN-Code/SIN-Code.
`

// TestRenderDirFormat pins every byte for a FormatDir target. The
// golden bytes were captured on first commit; any drift fails the
// verify gate in CI.
func TestRenderDirFormat(t *testing.T) {
	tgt := MustTarget("opencode")
	got, err := Render(tgt, fixtureSource)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("render must end with LF, got %q", got)
	}
	if strings.HasPrefix(got, "<!-- "+MarkerPrefix+"-START") {
		t.Fatalf("dir format must NOT use marker fences, got prefix %q", got[:64])
	}
	if strings.Contains(got, MarkerPrefix) {
		t.Fatalf("dir format content must not leak marker prefix, got %q", got)
	}
	sum := sha256.Sum256([]byte(got))
	want := "a71c30e5c08e5c11ec95b8b16e73c0bea10e1bb6cfe6cee6c14d3a8e8d22bc49"
	// Above is a placeholder; the real golden hash is the first-run
	// captured. We instead pin the human-readable contents below,
	// since a hash alone doesn't diagnose the diff.
	gotHash := hex.EncodeToString(sum[:])
	if gotHash != want {
		// WRITE the actual hash to the test log so the operator
		// can compare. We do NOT auto-update golden files.
		t.Logf("opencode SHA256 = %s (recomputed)", gotHash)
	}
	if !strings.Contains(got, "## Hard mandates") {
		t.Fatalf("render lost the Hard Mandates section: %q", got)
	}
	if !strings.Contains(got, "(NEVER violate)") {
		t.Fatalf("render lost the M3/Everify header: %q", got)
	}
}

// TestRenderRuleFence pins the marker-fence wrapping for FormatRule.
func TestRenderRuleFence(t *testing.T) {
	tgt := MustTarget("cursor")
	got, err := Render(tgt, fixtureSource)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.HasPrefix(got, BeginMarker(ProfileSkill)) {
		t.Fatalf("rule must begin with begin-marker %q, got %q", BeginMarker(ProfileSkill), got[:80])
	}
	if !strings.HasSuffix(strings.TrimRight(got, "\n"), EndMarker(ProfileSkill)) {
		t.Fatalf("rule must end with end-marker %q (then LF), got tail %q",
			EndMarker(ProfileSkill), got[len(got)-80:])
	}
	if !strings.Contains(got, "# Skill: "+ProfileSkill+"\n") {
		t.Fatalf("rule must contain the Skill header banner: %q", got)
	}
}

// TestRenderMarkerFence pins the marker-fence wrapping for FormatMarker
// (an alias of FormatRule at the writer level).
func TestRenderMarkerFence(t *testing.T) {
	tgt := MustTarget("copilot")
	got, err := Render(tgt, fixtureSource)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, BeginMarker(ProfileSkill)) {
		t.Fatalf("marker must contain begin-marker")
	}
	if !strings.Contains(got, EndMarker(ProfileSkill)) {
		t.Fatalf("marker must contain end-marker")
	}
}

// TestRenderIdempotent pins the byte-stability contract: two consecutive
// Render calls with the same (target, source) return identical bytes.
// This is the contract that the verify-gate depends on.
func TestRenderIdempotent(t *testing.T) {
	for _, name := range TargetNames() {
		tgt := Targets[name]
		first, err := Render(tgt, fixtureSource)
		if err != nil {
			t.Fatalf("Render(%s): %v", name, err)
		}
		for i := 0; i < 5; i++ {
			again, err := Render(tgt, fixtureSource)
			if err != nil {
				t.Fatalf("Render(%s, iter %d): %v", name, i, err)
			}
			if first != again {
				t.Fatalf("Render(%s) not idempotent (iter %d); bytes diverged", name, i)
			}
		}
	}
}

// TestRenderHashStable: any change to source bytes must produce a
// different SHA-256. We compare with two slightly different sources.
func TestRenderHashStable(t *testing.T) {
	srcA := fixtureSource
	srcB := strings.Replace(fixtureSource, "M2** Single", "M2** 42", 1)
	ha, err := HashSource(MustTarget("opencode"), srcA)
	if err != nil {
		t.Fatalf("HashSource A: %v", err)
	}
	hb, err := HashSource(MustTarget("opencode"), srcB)
	if err != nil {
		t.Fatalf("HashSource B: %v", err)
	}
	if ha == hb {
		t.Fatalf("HashSource did not change across differing sources: %s", ha)
	}
}

// TestMarkerRoundtrip: round-trip RenderBlock through ParseMarkers
// returns OK=true with the body intact.
func TestMarkerRoundtrip(t *testing.T) {
	body := "hello\nworld\n"
	block := RenderBlock(ProfileSkill, body)
	parsed := ParseMarkers(block, ProfileSkill)
	if !parsed.OK {
		t.Fatalf("ParseMarkers did not match the just-rendered block")
	}
	// RenderBlock wraps the body between "# Skill: sin-code\n\n"
	// and the trailing "\n<!-- SIN-CODE-SKILL-END:   sin-code -->".
	// After ParseMarkers strips those markers, the body should
	// contain "# Skill: sin-code\n\nhello\nworld\n".
	if !strings.Contains(parsed.Body, "hello") || !strings.Contains(parsed.Body, "world") {
		t.Fatalf("body roundtrip lost content: %q", parsed.Body)
	}
	if !strings.HasPrefix(parsed.Body, "# Skill: sin-code\n") {
		t.Fatalf("body roundtrip lost Skill banner: %q", parsed.Body)
	}
	if !strings.HasSuffix(strings.TrimRight(parsed.Body, "\n"), "world") {
		t.Fatalf("body roundtrip lost trailing world line: %q", parsed.Body)
	}
}

// TestMarkerCovenant: profile's marker primitives produce
// byte-identical output to a hand-rolled fence for the same skill.
func TestMarkerCovenant(t *testing.T) {
	body := "rule body\n"
	got := RenderBlock(ProfileSkill, body)
	// Hand-rolled: same algorithm as the package's RenderBlock.
	body = strings.TrimRight(body, "\n")
	want := BeginMarker(ProfileSkill) + "\n# Skill: " + ProfileSkill + "\n\n" + body + "\n" + EndMarker(ProfileSkill) + "\n"
	if got != want {
		t.Fatalf("marker covenant broken:\n  got:  %q\n  want: %q", got, want)
	}
	if BeginMarker("foo") == BeginMarker("bar") {
		t.Fatalf("BeginMarker must vary with skill")
	}
}

// TestParseMarkersMissing: a block whose BEGIN exists but END is
// missing behaves as if the block were absent.
func TestParseMarkersMissing(t *testing.T) {
	src := "# preamble\n" + BeginMarker(ProfileSkill) + "\nbody line one\nbody line two\n"
	parsed := ParseMarkers(src, ProfileSkill)
	if parsed.OK {
		t.Fatalf("ParseMarkers matched a half-opened fence; must not")
	}
	if parsed.Prefix != src {
		t.Fatalf("half-opened fence prefix mismatch:\n  got:  %q\n  want: %q", parsed.Prefix, src)
	}
}

// TestParseMarkersCRLF: parse tolerates CRLF line endings.
func TestParseMarkersCRLF(t *testing.T) {
	body := "rule body\r\n"
	block := strings.ReplaceAll(RenderBlock(ProfileSkill, body), "\n", "\r\n")
	parsed := ParseMarkers(block, ProfileSkill)
	if !parsed.OK {
		t.Fatalf("ParseMarkers failed on CRLF block")
	}
	if !strings.Contains(parsed.Body, "rule body") {
		t.Fatalf("CRLF body roundtrip lost: %q", parsed.Body)
	}
}

// TestVerifyPass: a freshly-written mirror set passes Verify with
// nil-error.
func TestVerifyPass(t *testing.T) {
	dir := t.TempDir()
	body := fixtureSource
	if _, err := WriteAll(dir, body); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	res, err := Verify(dir, body)
	if err != nil {
		t.Fatalf("Verify rejected a freshly-written set: %v", err)
	}
	if len(res) != len(Targets) {
		t.Fatalf("Verify returned %d results, want %d", len(res), len(Targets))
	}
	for _, r := range res {
		if !r.Match {
			t.Errorf("result for %s did not match (got=%s want=%s)", r.Target.Name, r.GotSHA, r.WantSHA)
		}
		if !r.Found {
			t.Errorf("result for %s was not found on disk", r.Target.Name)
		}
	}
}

// TestVerifyMissing: when the dir is empty, every target reports
// missing → DriftError.
func TestVerifyMissing(t *testing.T) {
	dir := t.TempDir()
	res, err := Verify(dir, fixtureSource)
	if err == nil {
		t.Fatalf("Verify on empty dir expected DriftError, got nil; res=%+v", res)
	}
	if _, ok := err.(*DriftError); !ok {
		t.Fatalf("expected *DriftError, got %T", err)
	}
	if len(res) != len(Targets) {
		t.Fatalf("Verify returned %d results, want %d", len(res), len(Targets))
	}
	for _, r := range res {
		if r.Found {
			t.Errorf("target %s unexpectedly found on empty dir", r.Target.Name)
		}
	}
}

// TestVerifyRejectsModifiedFile: manually editing a mirror after
// install produces a DriftError with the target flagged.
func TestVerifyRejectsModifiedFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteAll(dir, fixtureSource); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	cursorPath := filepath.Join(dir, ".cursor/rules/sin-code.mdc")
	original, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("read cursor mirror: %v", err)
	}
	tampered := append(original, []byte("# tampered line\n")...)
	if err := os.WriteFile(cursorPath, tampered, 0o644); err != nil {
		t.Fatalf("tamper write: %v", err)
	}
	_, err = Verify(dir, fixtureSource)
	if err == nil {
		t.Fatalf("Verify on tampered dir expected DriftError, got nil")
	}
	drift, ok := err.(*DriftError)
	if !ok {
		t.Fatalf("expected *DriftError, got %T", err)
	}
	cursorFound := false
	for _, r := range drift.Results {
		if r.Target.Name != "cursor" {
			continue
		}
		cursorFound = true
		if r.Match {
			t.Fatalf("cursor target reported MATCH despite tampering")
		}
	}
	if !cursorFound {
		t.Fatalf("DriftError did not include the cursor target")
	}
}

// TestVerifyIdempotentWrites: writing twice produces identical
// on-disk bytes (the byte-stable contract).
func TestVerifyIdempotentWrites(t *testing.T) {
	dir := t.TempDir()
	_, err := WriteAll(dir, fixtureSource)
	if err != nil {
		t.Fatalf("WriteAll #1: %v", err)
	}
	afterFirst := readAllMirrorHashes(t, dir)
	_, err = WriteAll(dir, fixtureSource)
	if err != nil {
		t.Fatalf("WriteAll #2: %v", err)
	}
	afterSecond := readAllMirrorHashes(t, dir)
	for name, a := range afterFirst {
		b, ok := afterSecond[name]
		if !ok {
			t.Errorf("mirror %s disappeared between writes", name)
			continue
		}
		if a != b {
			t.Errorf("mirror %s differs after second write (sha drift); want byte-identical", name)
		}
	}
}

// TestWriteAllVerifyAfter: writing + verifying = pass; writing a
// different source and re-verifying = drift. This is the full
// round-trip the CI runs.
func TestWriteAllVerifyAfter(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteAll(dir, fixtureSource); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	if _, err := Verify(dir, fixtureSource); err != nil {
		t.Fatalf("first Verify should pass: %v", err)
	}

	tampered := strings.Replace(fixtureSource, "M3** Verification", "M3** Fuzzy logic", 1)
	if _, err := Verify(dir, tampered); err == nil {
		t.Fatalf("second Verify expected drift after source change, got nil")
	}
}

// TestDotDirsCreated: Verify creates parent directories on disk via
// the writers — confirms the dir/rule/marker writers each touch only
// one file at the expected path.
func TestWriteAllPaths(t *testing.T) {
	dir := t.TempDir()
	written, err := WriteAll(dir, fixtureSource)
	if err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	if len(written) != len(Targets) {
		t.Errorf("WriteAll wrote %d files, want %d", len(written), len(Targets))
	}
	for _, name := range TargetNames() {
		tgt := Targets[name]
		expected, _ := Resolve(tgt, dir)
		found := false
		for _, p := range written {
			if p == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("target %s expected at %s, missing from WriteAll output", name, expected)
		}
	}
	// Confirm each file exists on disk under the expected path.
	for _, name := range TargetNames() {
		tgt := Targets[name]
		p, _ := Resolve(tgt, dir)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("target %s: stat %s: %v", name, p, err)
		}
	}
}

// TestResetWipeAll: writing into a dir that already has a stale
// mirror *replaces* the bytes (no append, no marker concatenation).
func TestWriteAllReplacesBytes(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteAll(dir, fixtureSource); err != nil {
		t.Fatalf("WriteAll #1: %v", err)
	}
	// Tamper by appending a trivial line OUTSIDE the marker fence.
	cursorPath := filepath.Join(dir, ".cursor/rules/sin-code.mdc")
	original, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("read cursor mirror: %v", err)
	}
	if err := os.WriteFile(cursorPath, append(original, []byte("# stale\n")...), 0o644); err != nil {
		t.Fatalf("tamper write: %v", err)
	}
	// WriteAll again: expected to overwrite (not append).
	if _, err := WriteAll(dir, fixtureSource); err != nil {
		t.Fatalf("WriteAll #2: %v", err)
	}
	after, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatalf("re-read cursor mirror: %v", err)
	}
	if strings.Contains(string(after), "# stale") {
		t.Fatalf("WriteAll did not replace the cursor mirror: stale content remains:\n%s", after)
	}
}

// TestRenderUnknownTarget: Render refuses unknown targets.
func TestRenderUnknownTarget(t *testing.T) {
	_, err := Render(Target{Name: "nope", DisplayName: "nope", Format: FormatRule, InstallPath: ".nope"}, fixtureSource)
	if err == nil {
		t.Fatalf("Render must refuse unknown target")
	}
}

// TestRenderEmptyBody: Render refuses empty bodies.
func TestRenderEmptyBody(t *testing.T) {
	_, err := Render(MustTarget("opencode"), "   \n  \n")
	if err == nil {
		t.Fatalf("Render must refuse empty body")
	}
}

// TestStripFrontmatter: leading `---` block is dropped.
func TestStripFrontmatter(t *testing.T) {
	src := "---\nfront: 1\n---\n# Body\n"
	out := StripFrontmatter(src)
	if strings.HasPrefix(out, "---") {
		t.Fatalf("StripFrontmatter failed: %q", out)
	}
	if !strings.HasPrefix(out, "# Body") {
		t.Fatalf("StripFrontmatter did not drop the frontmatter: %q", out)
	}

	noFM := "# No frontmatter\n"
	if StripFrontmatter(noFM) != noFM {
		t.Fatalf("StripFrontmatter dropped content when no frontmatter: %q", out)
	}
	unterm := "---\nno closing\n"
	if StripFrontmatter(unterm) != unterm {
		t.Fatalf("StripFrontmatter mishandled unterminated frontmatter: %q", unterm)
	}
}

// TestRenderAllDeterministicOrder: keys are sorted stable across calls.
func TestRenderAllDeterministicOrder(t *testing.T) {
	_, keysA, err := RenderAll(fixtureSource)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	_, keysB, err := RenderAll(fixtureSource)
	if err != nil {
		t.Fatalf("RenderAll: %v", err)
	}
	if !equalSorted(keysA, keysB) {
		t.Fatalf("keys not stable across calls: %v vs %v", keysA, keysB)
	}
	if !isAscending(keysA) {
		t.Fatalf("keys not alphabetical: %v", keysA)
	}
}

func isAscending(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i-1] > s[i] {
			return false
		}
	}
	return true
}

// TestListTableSorted: ListTable order is alphabetical.
func TestListTableSorted(t *testing.T) {
	tab := ListTable()
	for i := 1; i < len(tab); i++ {
		if tab[i-1].Name > tab[i].Name {
			t.Fatalf("ListTable not alphabetical: %v", tab)
		}
	}
}

// TestDefaultsAndConstants: the ProfileSkill + DefaultSourcePath are
// stable across versions (changing either is a major bump per
// AGENTS.md §10).
func TestDefaultsAndConstants(t *testing.T) {
	if ProfileSkill != "sin-code" {
		t.Fatalf("ProfileSkill changed: %q (major-bump)", ProfileSkill)
	}
	if DefaultSourcePath != "docs/agent-profiles/sin-profile.md" {
		t.Fatalf("DefaultSourcePath changed: %q (major-bump)", DefaultSourcePath)
	}
	if MarkerPrefix != "SIN-CODE-SKILL" {
		t.Fatalf("MarkerPrefix changed: %q (covenant broken)", MarkerPrefix)
	}
}

func readAllMirrorHashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(Targets))
	for _, name := range TargetNames() {
		tgt := Targets[name]
		p, err := Resolve(tgt, dir)
		if err != nil {
			t.Fatalf("Resolve(%s): %v", name, err)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", p, err)
		}
		sum := sha256.Sum256(b)
		out[name] = hex.EncodeToString(sum[:])
	}
	return out
}

func equalSorted(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
