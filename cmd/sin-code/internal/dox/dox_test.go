// SPDX-License-Identifier: MIT
// Purpose: tests for the dox (agent0ai/dox MIT) integration package.
// Covers marker-based idempotent injection (with co-existent
// Superpowers block), child-scaffold + parent-INDEX registration,
// broken-link / orphan / TODO detection, healthy-tree check, and the
// tree renderer. All tests are hermetic — no network, no real cwd.
// Docs: dox.doc.md
package dox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Frontmatter ────────────────────────────────────────────────────────

func TestParseFrontmatterFenced(t *testing.T) {
	body := "---\n" +
		"title: Hello World\n" +
		"owner: jane\n" +
		"---\n" +
		"\n# Body\n"
	fm := ParseFrontmatter(body)
	if fm["title"] != "Hello World" {
		t.Errorf("title: got %q, want %q", fm["title"], "Hello World")
	}
	if fm["owner"] != "jane" {
		t.Errorf("owner: got %q, want %q", fm["owner"], "jane")
	}
}

func TestParseFrontmatterUnfenced(t *testing.T) {
	body := "title: Quick\nowner: bob\n\n# Body\n"
	fm := ParseFrontmatter(body)
	if fm["title"] != "Quick" {
		t.Errorf("title: got %q", fm["title"])
	}
	if fm["owner"] != "bob" {
		t.Errorf("owner: got %q", fm["owner"])
	}
}

func TestParseFrontmatterNone(t *testing.T) {
	body := "# Just a heading\n\nNo frontmatter here.\n"
	fm := ParseFrontmatter(body)
	if len(fm) != 0 {
		t.Errorf("expected empty frontmatter, got %v", fm)
	}
}

// ── InjectRoot: marker-based idempotent injection ─────────────────────

func TestInjectRootCreatesNew(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "AGENTS.md")
	if err := InjectRoot(p, "hello body"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, BeginMarker) {
		t.Error("missing begin marker")
	}
	if !strings.Contains(s, EndMarker) {
		t.Error("missing end marker")
	}
	if !strings.Contains(s, "hello body") {
		t.Error("missing body")
	}
}

func TestInjectRootReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "AGENTS.md")
	if err := InjectRoot(p, "first body"); err != nil {
		t.Fatal(err)
	}
	if err := InjectRoot(p, "second body"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	s := string(got)
	// Exactly one begin / end pair.
	if c := strings.Count(s, BeginMarker); c != 1 {
		t.Errorf("begin marker count: got %d, want 1", c)
	}
	if c := strings.Count(s, EndMarker); c != 1 {
		t.Errorf("end marker count: got %d, want 1", c)
	}
	if strings.Contains(s, "first body") {
		t.Error("stale body still present")
	}
	if !strings.Contains(s, "second body") {
		t.Error("new body missing")
	}
}

// TestInjectRootIdempotentAndCoexistent is the HARD mandate: running
// InjectRoot twice must produce a byte-identical file, AND it must not
// touch the co-existent `<!-- SIN-Code superpowers:begin/end -->` block
// owned by the superpowers tool. This is the regression guard for the
// v3.7.0 release.
func TestInjectRootIdempotentAndCoexistent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "AGENTS.md")
	// Seed an AGENTS.md that ALREADY contains the superpowers block —
	// the order matches what `sin-code superpowers install --agents`
	// would produce on a real install.
	seed := "# Root\n\n" +
		"Some prose before the managed blocks.\n\n" +
		"<!-- SIN-Code superpowers:begin -->\n" +
		"superpowers-snippet-line-1\n" +
		"superpowers-snippet-line-2\n" +
		"<!-- SIN-Code superpowers:end -->\n"
	if err := os.WriteFile(p, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	// First injection of the dox block.
	if err := InjectRoot(p, "dox-body-v1"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	firstStr := string(first)

	// Superpowers block must still be intact, unchanged.
	if !strings.Contains(firstStr, "<!-- SIN-Code superpowers:begin -->") {
		t.Error("superpowers begin marker lost after dox injection")
	}
	if !strings.Contains(firstStr, "<!-- SIN-Code superpowers:end -->") {
		t.Error("superpowers end marker lost after dox injection")
	}
	for _, line := range []string{
		"superpowers-snippet-line-1",
		"superpowers-snippet-line-2",
	} {
		if !strings.Contains(firstStr, line) {
			t.Errorf("superpowers payload lost: missing %q", line)
		}
	}
	// Dox block must be present.
	if !strings.Contains(firstStr, BeginMarker) {
		t.Error("dox begin marker missing")
	}
	if !strings.Contains(firstStr, EndMarker) {
		t.Error("dox end marker missing")
	}
	if !strings.Contains(firstStr, "dox-body-v1") {
		t.Error("dox body missing")
	}

	// Second injection with the same body — must be byte-identical.
	if err := InjectRoot(p, "dox-body-v1"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(p)
	if string(first) != string(second) {
		t.Error("second injection produced different bytes — not idempotent")
	}

	// Third injection with a new body — must replace in place, must
	// not duplicate the begin / end markers, and must still leave the
	// superpowers block intact.
	if err := InjectRoot(p, "dox-body-v2"); err != nil {
		t.Fatal(err)
	}
	third, _ := os.ReadFile(p)
	thirdStr := string(third)
	if c := strings.Count(thirdStr, BeginMarker); c != 1 {
		t.Errorf("begin marker count after replace: got %d, want 1", c)
	}
	if c := strings.Count(thirdStr, EndMarker); c != 1 {
		t.Errorf("end marker count after replace: got %d, want 1", c)
	}
	if strings.Contains(thirdStr, "dox-body-v1") {
		t.Error("stale dox body still present after replace")
	}
	if !strings.Contains(thirdStr, "dox-body-v2") {
		t.Error("new dox body missing")
	}
	for _, line := range []string{
		"<!-- SIN-Code superpowers:begin -->",
		"superpowers-snippet-line-1",
		"<!-- SIN-Code superpowers:end -->",
	} {
		if !strings.Contains(thirdStr, line) {
			t.Errorf("superpowers block corrupted: missing %q", line)
		}
	}
}

func TestInjectRootEmptyPath(t *testing.T) {
	if err := InjectRoot("", "body"); err == nil {
		t.Error("expected error for empty path")
	}
}

// ── RemoveBlock ────────────────────────────────────────────────────────

func TestRemoveBlock(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "AGENTS.md")
	if err := InjectRoot(p, "removable"); err != nil {
		t.Fatal(err)
	}
	n, err := RemoveBlock(p)
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Errorf("expected positive byte count, got %d", n)
	}
	got, _ := os.ReadFile(p)
	if strings.Contains(string(got), BeginMarker) {
		t.Error("begin marker still present after RemoveBlock")
	}
	if strings.Contains(string(got), "removable") {
		t.Error("body still present after RemoveBlock")
	}
}

func TestRemoveBlockNoMarker(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(p, []byte("no block here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := RemoveBlock(p)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected 0 bytes removed, got %d", n)
	}
}

// ── Build + Scaffold ──────────────────────────────────────────────────

func TestBuildEmptyDir(t *testing.T) {
	dir := t.TempDir()
	tree, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tree == nil {
		t.Fatal("nil tree")
	}
	if tree.Name == "" {
		t.Error("root name empty")
	}
	if len(tree.Children) != 0 {
		t.Errorf("expected no children, got %d", len(tree.Children))
	}
}

func TestBuildWithChildren(t *testing.T) {
	dir := t.TempDir()
	// Create two sub-dirs, one with INDEX.md.
	if err := os.MkdirAll(filepath.Join(dir, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "alpha", IndexFileName),
		[]byte("---\ntitle: Alpha\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "beta", IndexFileName),
		[]byte("---\ntitle: Beta\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(tree.Children))
	}
	// Children must be sorted alphabetically: alpha, beta.
	if tree.Children[0].Name != "alpha" || tree.Children[1].Name != "beta" {
		t.Errorf("children not sorted: %s, %s",
			tree.Children[0].Name, tree.Children[1].Name)
	}
	if tree.Children[0].Title != "Alpha" {
		t.Errorf("alpha title: got %q", tree.Children[0].Title)
	}
}

func TestBuildLeaf(t *testing.T) {
	dir := t.TempDir()
	leaf := filepath.Join(dir, "leafy")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leaf, "LEAF.md"), []byte("leaf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tree, err := Build(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree.Children))
	}
	if !tree.Children[0].IsLeaf {
		t.Error("expected child to be a leaf")
	}
}

func TestBuildNotADirectory(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(f); err == nil {
		t.Error("expected error for non-directory")
	}
}

func TestBuildEmptyPath(t *testing.T) {
	if _, err := Build(""); err == nil {
		t.Error("expected error for empty path")
	}
}

// ── Scaffold ──────────────────────────────────────────────────────────

func TestScaffoldCreatesChildAndRegistersInParent(t *testing.T) {
	dir := t.TempDir()
	// Seed a root AGENTS.md so Scaffold treats the parent as a root
	// (writes AGENTS.md in the new child, not INDEX.md).
	if err := os.WriteFile(filepath.Join(dir, AgentsFileName),
		[]byte("---\ntitle: Root\n---\n# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	child, err := Scaffold(dir, "auth", "Auth subsystem")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(child, string(os.PathSeparator)+"auth") {
		t.Errorf("unexpected child path: %s", child)
	}
	// Child directory must exist and contain an AGENTS.md.
	childAgents := filepath.Join(child, AgentsFileName)
	if _, err := os.Stat(childAgents); err != nil {
		t.Fatalf("child AGENTS.md missing: %v", err)
	}
	body, err := os.ReadFile(childAgents)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Auth subsystem") {
		t.Error("child AGENTS.md missing title")
	}
	// Parent root must have been updated with a registration link.
	parentBody, err := os.ReadFile(filepath.Join(dir, AgentsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(parentBody), "[auth](auth/AGENTS.md)") {
		t.Errorf("parent missing child link; body:\n%s", string(parentBody))
	}
}

func TestScaffoldIdempotentRegistration(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, AgentsFileName),
		[]byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(dir, "ci", "CI subsystem"); err != nil {
		t.Fatal(err)
	}
	// Second call must NOT duplicate the registration line, but
	// WILL fail with ErrAlreadyExists because the dir is there.
	if _, err := Scaffold(dir, "ci", "CI subsystem"); err == nil {
		t.Error("expected ErrAlreadyExists on re-scaffold")
	}
	parentBody, _ := os.ReadFile(filepath.Join(dir, AgentsFileName))
	if c := strings.Count(string(parentBody), "[ci](ci/AGENTS.md)"); c != 1 {
		t.Errorf("expected 1 link, got %d", c)
	}
}

func TestScaffoldNonRootParentUsesIndex(t *testing.T) {
	dir := t.TempDir()
	// Parent is a non-root (no AGENTS.md) — child should get INDEX.md,
	// parent should also get INDEX.md (auto-seeded with the registration).
	child, err := Scaffold(dir, "core", "Core lib")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(child, IndexFileName)); err != nil {
		t.Errorf("child INDEX.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, IndexFileName)); err != nil {
		t.Errorf("parent INDEX.md missing: %v", err)
	}
}

// ── Check ─────────────────────────────────────────────────────────────

func TestCheckHealthyTree(t *testing.T) {
	dir := t.TempDir()
	// Root AGENTS.md.
	if err := os.WriteFile(filepath.Join(dir, AgentsFileName),
		[]byte("---\ntitle: Root\n---\n# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Healthy child with INDEX.md and a working link.
	child := filepath.Join(dir, "alpha")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, IndexFileName),
		[]byte("---\ntitle: Alpha\n---\n# Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Link from root → child must resolve.
	if err := InjectRoot(filepath.Join(dir, AgentsFileName),
		"- see [alpha](alpha/INDEX.md)\n"); err != nil {
		t.Fatal(err)
	}
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("expected healthy, got findings: %+v", findings)
	}
}

func TestCheckOrphanNode(t *testing.T) {
	dir := t.TempDir()
	// Root with no AGENTS.md ⇒ missing-agents error.
	orphan := filepath.Join(dir, "ghost")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "LEAF.md"),
		[]byte("leaf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The root has an AGENTS.md so the missing-agents check passes for
	// the root; the orphan is a leaf directory with no parent
	// registration. Since the orphan has LEAF.md, the Build step will
	// mark it as a leaf. The orphan check fires only for non-leaf
	// directories, so let's test the non-leaf variant too.
	nonLeaf := filepath.Join(dir, "lonely")
	if err := os.MkdirAll(nonLeaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonLeaf, "README.md"),
		[]byte("# Lonely\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Need an AGENTS.md at root for the build to proceed.
	if err := os.WriteFile(filepath.Join(dir, AgentsFileName),
		[]byte("# Root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Expect: orphan finding for "lonely".
	hasOrphan := false
	for _, f := range findings {
		if f.Kind == "orphan" && strings.HasSuffix(f.Path, "lonely") {
			hasOrphan = true
		}
	}
	if !hasOrphan {
		t.Errorf("expected orphan finding for lonely; got: %+v", findings)
	}
}

func TestCheckBrokenLink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, AgentsFileName),
		[]byte("# Root\n\n- [missing](nope.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	hasBroken := false
	for _, f := range findings {
		if f.Kind == "broken-link" {
			hasBroken = true
		}
	}
	if !hasBroken {
		t.Errorf("expected broken-link finding; got: %+v", findings)
	}
}

func TestCheckTODO(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, AgentsFileName),
		[]byte("# Root\n\nTODO: write me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	hasTODO := false
	for _, f := range findings {
		if f.Kind == "todo" {
			hasTODO = true
		}
	}
	if !hasTODO {
		t.Errorf("expected todo finding; got: %+v", findings)
	}
}

func TestCheckIgnoresFencedCodeLinks(t *testing.T) {
	dir := t.TempDir()
	// Link in a fenced code block must NOT be flagged as broken.
	body := "# Root\n\n" +
		"```\n" +
		"example: [fake](does-not-exist.md)\n" +
		"```\n"
	if err := os.WriteFile(filepath.Join(dir, AgentsFileName),
		[]byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.Kind == "broken-link" {
			t.Errorf("fenced code link flagged as broken: %+v", f)
		}
	}
}

func TestCheckMissingAgents(t *testing.T) {
	dir := t.TempDir()
	// No AGENTS.md at the root — should report missing-agents.
	if _, err := Check(dir); err == nil {
		// Check returns nil error and a finding; verify finding.
	}
	findings, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	hasMissing := false
	for _, f := range findings {
		if f.Kind == "missing-agents" {
			hasMissing = true
		}
	}
	if !hasMissing {
		t.Errorf("expected missing-agents finding; got: %+v", findings)
	}
}

// ── RenderTree ────────────────────────────────────────────────────────

func TestRenderTreeBasic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, AgentsFileName),
		[]byte("---\ntitle: MyRoot\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "alpha")
	b := filepath.Join(dir, "beta")
	for _, p := range []string{a, b} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(a, IndexFileName),
		[]byte("---\ntitle: Alpha\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, IndexFileName),
		[]byte("---\ntitle: Beta\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := RenderTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"MyRoot", "Alpha", "Beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q in:\n%s", want, out)
		}
	}
}

func TestRenderTreeLeafLabel(t *testing.T) {
	dir := t.TempDir()
	leaf := filepath.Join(dir, "ending")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leaf, "LEAF.md"),
		[]byte("leaf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := RenderTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "(leaf)") {
		t.Errorf("expected (leaf) tag in:\n%s", out)
	}
}

func TestRenderTreeEmptyDir(t *testing.T) {
	dir := t.TempDir()
	out, err := RenderTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Error("empty render for empty dir")
	}
}

// ── Helpers ───────────────────────────────────────────────────────────

func TestExtractMarkdownLinks(t *testing.T) {
	body := "see [a](a.md) and [b](b/c.md#anchor) and [http](https://example.com/x)."
	got := extractMarkdownLinks(body)
	want := []string{"a.md", "b/c.md#anchor", "https://example.com/x"}
	if len(got) != len(want) {
		t.Fatalf("got %d links, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("link %d: got %q, want %q", i, got[i], w)
		}
	}
}

func TestHasStandaloneTODO(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"TODO: write me", true},
		{"- TODO update later", true},
		{"no todo here", false},
		{"TODOS are fine", false}, // trailing S ⇒ not standalone
		{"fooTODObar", false},     // embedded ⇒ not standalone
		{"see TODO!", true},
		{"end of line TODO", true},
	}
	for _, c := range cases {
		if got := hasStandaloneTODO(c.line); got != c.want {
			t.Errorf("hasStandaloneTODO(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}
