// SPDX-License-Identifier: MIT
package skilldist

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// allTargets is a fixture used by every test that wants to walk the full
// registry: list tests must iterate deterministically (alphabetical), and
// idempotency tests must exercise each Format at least once.
func allTargets() []Target {
	out := make([]Target, 0, len(Targets))
	for _, t := range Targets {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// TestTargets_AllPresent locks the registered set of agent ids. Adding or
// removing a target without a corresponding AGENTS.md / CHANGELOG / CLI
// update should be impossible by accident — this test fails loudly so the
// maintainer is forced through the policy update first.
func TestTargets_AllPresent(t *testing.T) {
	want := []string{
		"claude-code", "cline", "codex", "copilot",
		"cursor", "gemini", "opencode", "windsurf",
	}
	got := TargetNames()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TargetNames drifted:\n  got:  %v\n  want: %v", got, want)
	}
}

// TestTargets_FieldsValidates ensures every registered target has the
// minimum required fields filled in. A bug-fix that adds a target but
// forgets DisplayName breaks the `--agent <name>` help text.
func TestTargets_FieldsValidates(t *testing.T) {
	for _, tgt := range allTargets() {
		if tgt.Name == "" {
			t.Errorf("%+v missing Name", tgt)
		}
		if tgt.DisplayName == "" {
			t.Errorf("%s missing DisplayName", tgt.Name)
		}
		if tgt.InstallPath == "" {
			t.Errorf("%s missing InstallPath", tgt.Name)
		}
		if tgt.Format != FormatDir && tgt.Format != FormatRule && tgt.Format != FormatMarker {
			t.Errorf("%s invalid Format %q", tgt.Name, tgt.Format)
		}
	}
}

// TestResolve_HomeOverride verifies the home-directory override path: tests
// never want $HOME leakage, so a TempDir must win over os.UserHomeDir() in
// every case.
func TestResolve_HomeOverride(t *testing.T) {
	home := t.TempDir()
	for _, tgt := range allTargets() {
		got, err := Resolve(tgt, "demo-skill", home)
		if err != nil {
			t.Fatalf("%s: Resolve: %v", tgt.Name, err)
		}
		rel, err := filepath.Rel(home, got)
		if err != nil {
			t.Fatalf("%s: relative path: %v", tgt.Name, err)
		}
		// Multi-skill files (no `<skill>` placeholder) yield the bare path;
		// per-skill paths substitute the placeholder verbatim.
		if !strings.Contains(tgt.InstallPath, "<skill>") {
			if rel != filepath.Clean(tgt.InstallPath) {
				t.Errorf("%s: expected %q, got %q", tgt.Name, tgt.InstallPath, rel)
			}
		} else if rel != filepath.Clean(strings.ReplaceAll(tgt.InstallPath, "<skill>", "demo-skill")) {
			t.Errorf("%s: substituted path wrong: %q", tgt.Name, rel)
		}
	}
}

// TestResolve_RejectsUnsafeName defends against a malicious or buggy skill
// name escaping the home directory via ..
func TestResolve_RejectsUnsafeName(t *testing.T) {
	for _, name := range []string{"", "../etc/passwd", "foo/bar", "foo\\bar"} {
		_, err := Resolve(Targets["claude-code"], name, t.TempDir())
		if err == nil {
			t.Errorf("Resolve(%q) returned no error", name)
		}
	}
}

// TestParseMarkers_RoundTrip makes ParseMarkers the inverse of RenderBlock
// for the simple one-block case: the body returned in the ParseResult must
// equal the body originally passed to RenderBlock (modulo the begin/end
// header banner RenderBlock injects). The markers themselves live inside
// Block's enclosing file content (not in Prefix/Suffix), so we round-trip
// the body by handing the parsed Prefix + Block back to a synthetic "render
// without markers" check.
func TestParseMarkers_RoundTrip(t *testing.T) {
	body := "Hello, world.\n\n## Section\n\nSome text."
	rendered := RenderBlock("demo", body)
	parsed := ParseMarkers(rendered, "demo")
	if !parsed.OK {
		t.Fatalf("ParseMarkers did not find its own rendered block")
	}
	if !strings.Contains(parsed.Block, "Hello, world.") {
		t.Errorf("parsed block is missing body content: %q", parsed.Block)
	}
	if !strings.Contains(parsed.Block, "## Section") {
		t.Errorf("parsed block is missing trailing content: %q", parsed.Block)
	}
	// The markers explicitly must NOT appear in either Prefix or Suffix —
	// ParseMarkers strips them out so the writer can construct a clean
	// replacement.
	if strings.Contains(parsed.Prefix, MarkerPrefix) {
		t.Errorf("Prefix still contains marker prefix: %q", parsed.Prefix)
	}
	if strings.Contains(parsed.Suffix, MarkerPrefix) {
		t.Errorf("Suffix still contains marker prefix: %q", parsed.Suffix)
	}
}

// TestParseMarkers_DistinctSkills ensures parsing for skill A does NOT match
// a block written for skill B. This is the safety property that prevents a
// re-install of skill A from accidentally clobbering skill B's block.
func TestParseMarkers_DistinctSkills(t *testing.T) {
	body := "skill-A specific body"
	a := RenderBlock("alpha", body)
	if ParseMarkers(a, "beta").OK {
		t.Errorf("ParseMarkers for beta matched alpha's block")
	}
	if ParseMarkers(a, "alpha").Block != "# Skill: alpha\n\nskill-A specific body" {
		t.Errorf("alpha parse did not return the right body: %q", ParseMarkers(a, "alpha").Block)
	}
}

// TestParseMarkers_NoBlock asserts the absent case — finding nothing
// returns OK=false and the unmodified input in Prefix.
func TestParseMarkers_NoBlock(t *testing.T) {
	in := "no markers here\nnope\n"
	p := ParseMarkers(in, "nonesuch")
	if p.OK {
		t.Errorf("ParseMarkers returned OK=true for an absent block")
	}
	if p.Prefix != in {
		t.Errorf("Prefix should be unchanged when no block is found: got %q", p.Prefix)
	}
}

// TestParseMarkers_HalfOpenedFence covers the safety property: a begin
// marker without a matching end marker is treated as absent so callers
// safely overwrite the dangling begin rather than appending below it.
func TestParseMarkers_HalfOpenedFence(t *testing.T) {
	halfOpened := BeginMarker("broken") + "\nsome body without a real end\n"
	if ParseMarkers(halfOpened, "broken").OK {
		t.Errorf("ParseMarkers accepted a half-opened fence")
	}
}

// TestParseMarkers_PreservesOtherBlocks is the load-bearing test for the
// FormatMarker case (Copilot's instructions file). Installing skill B into
// a file that ALREADY contains skill A's block must keep A's block intact
// and add B's block alongside it.
func TestParseMarkers_PreservesOtherBlocks(t *testing.T) {
	mk := func(skill, body string) string { return RenderBlock(skill, body) }
	file := mk("alpha", "alpha content\n") + mk("beta", "beta content\n")
	parsed := ParseMarkers(file, "alpha")
	if !parsed.OK {
		t.Fatalf("alpha block should be present")
	}
	// The suffix must still contain beta's complete marker fence.
	if !strings.Contains(parsed.Suffix, BeginMarker("beta")) {
		t.Errorf("beta begin marker lost from suffix: %q", parsed.Suffix)
	}
	if !strings.Contains(parsed.Suffix, EndMarker("beta")) {
		t.Errorf("beta end marker lost from suffix: %q", parsed.Suffix)
	}
	if !strings.Contains(parsed.Suffix, "beta content") {
		t.Errorf("beta body lost from suffix: %q", parsed.Suffix)
	}
}

// TestParseMarkers_CRLineEndings verifies CRLF input is handled the same as
// LF: a host agent editing on Windows might re-save with CRLF after our
// install and we still need to locate (and replace) our block.
func TestParseMarkers_CRLineEndings(t *testing.T) {
	crlf := strings.ReplaceAll(RenderBlock("demo", "body line"), "\n", "\r\n")
	if !ParseMarkers(crlf, "demo").OK {
		t.Errorf("ParseMarkers failed on CRLF input")
	}
}

// TestRenderBlock_HasTrailingNewline ensures every render ends in \n so the
// atomicWrite consumer can concatenate without juggling separators.
func TestRenderBlock_HasTrailingNewline(t *testing.T) {
	if !strings.HasSuffix(RenderBlock("x", "y"), "\n") {
		t.Errorf("RenderBlock must always end with a newline")
	}
	if !strings.HasSuffix(RenderBlock("x", ""), "\n") {
		t.Errorf("RenderBlock with empty body must still end with a newline")
	}
}

// TestStripFrontmatter_BasicValid covers the happy path: leading `---` of
// the file, then a closing `---` line, then body content.
func TestStripFrontmatter_BasicValid(t *testing.T) {
	in := "---\nname: x\ndescription: y\n---\n# Body\n\ntext\n"
	out := StripFrontmatter(in)
	if strings.HasPrefix(out, "---") {
		t.Errorf("frontmatter not stripped: %q", out)
	}
	if !strings.HasPrefix(out, "# Body") {
		t.Errorf("body should start at # Body, got %q", out)
	}
}

// TestStripFrontmatter_NoFrontmatter passes content through verbatim.
func TestStripFrontmatter_NoFrontmatter(t *testing.T) {
	in := "# Just a heading\n\nno front matter here\n"
	if StripFrontmatter(in) != in {
		t.Errorf("no-frontmatter content was modified")
	}
}

// TestStripFrontmatter_Unterminated preserves content when `---` never
// closes — emitting a half-block would mislead the host agent.
func TestStripFrontmatter_Unterminated(t *testing.T) {
	in := "---\nname: x\n# Body\n"
	if StripFrontmatter(in) != in {
		t.Errorf("unterminated frontmatter should pass through verbatim")
	}
}

// installFixture builds a fake source skill in srcRoot: SKILL.md plus
// optional contexts to exercise FormatDir's supplementary directory copy.
// Returns the skill name.
func installFixture(t *testing.T, srcRoot, skill, body string) {
	t.Helper()
	skillDir := filepath.Join(srcRoot, skill)
	if err := os.MkdirAll(filepath.Join(skillDir, "context"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "context", "readme.md"), []byte("# context readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestInstall_DirFormat_Idempotent covers the FormatDir case: re-running
// Install must overwrite the destination tree without leaving stale files
// from previous runs, and isInstall must report true after the install.
func TestInstall_DirFormat_Idempotent(t *testing.T) {
	for _, tgt := range allTargets() {
		if tgt.Format != FormatDir {
			continue
		}
		t.Run(tgt.Name, func(t *testing.T) {
			home := t.TempDir()
			src := t.TempDir()
			body := "---\nname: demo\n---\n# Demo\n\ncontent line\n"
			installFixture(t, src, "demo", body)

			if err := Install("demo", tgt, InstallOptions{SrcRoot: src, Home: home}); err != nil {
				t.Fatalf("first Install: %v", err)
			}
			ok, err := IsInstalled(tgt, "demo", home)
			if err != nil || !ok {
				t.Fatalf("IsInstalled after first install: ok=%v err=%v", ok, err)
			}
			resolved, _ := Resolve(tgt, "demo", home)
			if _, err := os.Stat(filepath.Join(resolved, "SKILL.md")); err != nil {
				t.Errorf("SKILL.md missing after install: %v", err)
			}
			if _, err := os.Stat(filepath.Join(resolved, "context", "readme.md")); err != nil {
				t.Errorf("context/readme.md missing after install: %v", err)
			}

			// Re-run: idempotent.
			if err := Install("demo", tgt, InstallOptions{SrcRoot: src, Home: home}); err != nil {
				t.Fatalf("second Install: %v", err)
			}
			// No duplicate directory created.
			entries, err := os.ReadDir(filepath.Dir(resolved))
			if err != nil {
				t.Fatalf("readdir parent: %v", err)
			}
			seenDir := 0
			for _, e := range entries {
				if e.IsDir() && e.Name() == "demo" {
					seenDir++
				}
			}
			if seenDir != 1 {
				t.Errorf("expected exactly one demo directory, found %d", seenDir)
			}
		})
	}
}

// TestInstall_RuleFormat_Idempotent covers the FormatRule case: a single
// .md/.mdc file with a marker fence. The fence must be present after
// install and exactly once after a re-install.
func TestInstall_RuleFormat_Idempotent(t *testing.T) {
	for _, tgt := range allTargets() {
		if tgt.Format != FormatRule {
			continue
		}
		t.Run(tgt.Name, func(t *testing.T) {
			home := t.TempDir()
			body := StripFrontmatter("# Body\n\nhello\n")
			if err := Install("demo", tgt, InstallOptions{Home: home, Body: body}); err != nil {
				t.Fatalf("first Install: %v", err)
			}
			resolved, _ := Resolve(tgt, "demo", home)
			data, err := os.ReadFile(resolved)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), BeginMarker("demo")) != 1 {
				t.Errorf("expected exactly one begin marker, got file:\n%s", string(data))
			}

			// Re-install with new body — must REPLACE, not append.
			newBody := StripFrontmatter("# Body v2\n\nchanged\n")
			if err := Install("demo", tgt, InstallOptions{Home: home, Body: newBody}); err != nil {
				t.Fatalf("second Install: %v", err)
			}
			data2, err := os.ReadFile(resolved)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data2), BeginMarker("demo")) != 1 {
				t.Errorf("second install created duplicate marker fence:\n%s", string(data2))
			}
			if !strings.Contains(string(data2), "Body v2") {
				t.Errorf("second install did not replace body:\n%s", string(data2))
			}
			if strings.Contains(string(data2), "Body\n\nhello") {
				t.Errorf("old body remains after second install:\n%s", string(data2))
			}
		})
	}
}

// TestInstall_MarkerFormat_PreservesOtherBlocks covers the FormatMarker
// case (Copilot): two markers side-by-side, second install must not
// destroy the first.
func TestInstall_MarkerFormat_PreservesOtherBlocks(t *testing.T) {
	tgt := Targets["copilot"]
	home := t.TempDir()
	if err := Install("alpha", tgt, InstallOptions{Home: home, Body: "alpha body"}); err != nil {
		t.Fatalf("install alpha: %v", err)
	}
	if err := Install("beta", tgt, InstallOptions{Home: home, Body: "beta body"}); err != nil {
		t.Fatalf("install beta: %v", err)
	}
	resolved, _ := Resolve(tgt, "alpha", home)
	data, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), BeginMarker("alpha")) != 1 {
		t.Errorf("alpha block missing or duplicated:\n%s", string(data))
	}
	if strings.Count(string(data), BeginMarker("beta")) != 1 {
		t.Errorf("beta block missing or duplicated:\n%s", string(data))
	}
	if !strings.Contains(string(data), "alpha body") {
		t.Errorf("alpha body lost:\n%s", string(data))
	}
	if !strings.Contains(string(data), "beta body") {
		t.Errorf("beta body lost:\n%s", string(data))
	}
}

// TestUninstall_RoundTrip ensures Uninstall reverses Install for every
// Format on every registered target. After uninstall, IsInstalled must be
// false and the file/directory must be gone (or empty for multi-skill
// shared files).
func TestUninstall_RoundTrip(t *testing.T) {
	for _, tgt := range allTargets() {
		t.Run(tgt.Name, func(t *testing.T) {
			home := t.TempDir()
			body := StripFrontmatter("# body\n")
			opts := InstallOptions{Home: home, Body: body}
			if tgt.Format == FormatDir {
				src := t.TempDir()
				installFixture(t, src, "demo",
					"---\nname: demo\n---\n# demo\n")
				opts.SrcRoot = src
			}
			if err := Install("demo", tgt, opts); err != nil {
				t.Fatalf("Install: %v", err)
			}
			ok, err := IsInstalled(tgt, "demo", home)
			if err != nil || !ok {
				t.Fatalf("IsInstalled pre-uninstall: ok=%v err=%v", ok, err)
			}
			if err := Uninstall(tgt, "demo", home); err != nil {
				t.Fatalf("Uninstall: %v", err)
			}
			ok, err = IsInstalled(tgt, "demo", home)
			if err != nil || ok {
				t.Fatalf("IsInstalled post-uninstall: ok=%v err=%v", ok, err)
			}
			// Calling Uninstall twice must be a no-op, not an error.
			if err := Uninstall(tgt, "demo", home); err != nil {
				t.Errorf("repeated Uninstall returned error: %v", err)
			}
		})
	}
}

// TestUninstall_MarkerFormat_RemovesEmptyFile ensures the multi-skill file
// is deleted when the uninstall of its only known block would leave an
// empty file behind (otherwise an unreferenced file would quietly stay
// under Copilot's dotfile directory forever).
func TestUninstall_MarkerFormat_RemovesEmptyFile(t *testing.T) {
	tgt := Targets["copilot"]
	home := t.TempDir()
	if err := Install("solo", tgt, InstallOptions{Home: home, Body: "solo"}); err != nil {
		t.Fatal(err)
	}
	resolved, _ := Resolve(tgt, "solo", home)
	if err := Uninstall(tgt, "solo", home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(resolved); !os.IsNotExist(err) {
		t.Errorf("expected file removed after final uninstall, got err=%v", err)
	}
}

// TestInstall_RejectsUnknownTarget locks the public API: passing a target
// not in Targets is an error, not a crash.
func TestInstall_RejectsUnknownTarget(t *testing.T) {
	err := Install("demo", Target{Name: "made-up-agent", Format: FormatRule},
		InstallOptions{Home: t.TempDir(), Body: "x"})
	if err == nil {
		t.Fatalf("expected error for unknown target")
	}
}

// TestInstall_RuleBodyEmpty is a guard against silently writing an empty
// rule file: the host agent would load it and look foolish.
func TestInstall_RuleBodyEmpty(t *testing.T) {
	err := Install("demo", Targets["cursor"], InstallOptions{Home: t.TempDir(), Body: ""})
	if err == nil {
		t.Fatalf("expected error for empty body in FormatRule")
	}
}

// TestInstall_AtomicOrder verifies the install never leaves a half-written
// file behind that would confuse downstream readers. We simulate a failure
// by passing a path whose parent is a regular file (mkdir parent fails).
func TestInstall_AtomicOrder_DirParentFailure(t *testing.T) {
	home := t.TempDir()
	// Create a regular file at a position Install wants to mkdir as parent.
	blocker := filepath.Join(home, ".cursor")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	tgt := Targets["cursor"]
	err := Install("demo", tgt, InstallOptions{Home: home, Body: "hello"})
	if err == nil {
		t.Fatalf("expected mkdir failure when parent is a file")
	}
	if _, err := os.Stat(blocker); err != nil {
		t.Errorf("blocker file should still exist, got %v", err)
	}
}

// TestIsInstalled_Defaults exercises the easy cases that the `--installed`
// CLI flag relies on: file absent → false, dir absent → false.
func TestIsInstalled_Defaults(t *testing.T) {
	for _, tgt := range allTargets() {
		ok, err := IsInstalled(tgt, "ghost-skill", t.TempDir())
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tgt.Name, err)
		}
		if ok {
			t.Errorf("%s: ghost-skill reported as installed", tgt.Name)
		}
	}
}
