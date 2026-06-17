// SPDX-License-Identifier: MIT
// Purpose: tests for required_tools.go — frontmatter parsing, embedded FS
// extraction, and MergeRequiredTools deduplication. Uses the real embedded
// skills.ListFS() to verify that known skills like skill-code-build expose
// their required_tools at runtime.
package skillmgr

import (
	"testing"
	"testing/fstest"

	"github.com/OpenSIN-Code/SIN-Code/skills"
)

func TestParseRequiredTools_BasicList(t *testing.T) {
	raw := `---
name: skill-code-build
description: test
required_tools:
  - sin_edit
  - sin_test
  - sin_quality_gate
lifecycle: native
---

# skill-code-build
body
`
	tools, err := ParseRequiredTools(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"sin_edit", "sin_test", "sin_quality_gate"}
	if len(tools) != len(want) {
		t.Fatalf("got %v tools, want %v", tools, want)
	}
	for i, w := range want {
		if tools[i] != w {
			t.Errorf("tools[%d] = %q, want %q", i, tools[i], w)
		}
	}
}

func TestParseRequiredTools_NoField(t *testing.T) {
	raw := `---
name: some-skill
description: test
lifecycle: native
---

body
`
	tools, err := ParseRequiredTools(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tools != nil {
		t.Fatalf("expected nil for no required_tools, got %v", tools)
	}
}

func TestParseRequiredTools_NoFrontmatter(t *testing.T) {
	raw := "# Just a markdown file\n\nNo frontmatter here."
	tools, err := ParseRequiredTools(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tools != nil {
		t.Fatalf("expected nil for no frontmatter, got %v", tools)
	}
}

func TestParseRequiredTools_UnterminatedFrontmatter(t *testing.T) {
	raw := "---\nname: broken\nrequired_tools:\n  - sin_edit\nbody without closing"
	tools, err := ParseRequiredTools(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tools != nil {
		t.Fatalf("expected nil for unterminated frontmatter, got %v", tools)
	}
}

func TestParseRequiredTools_EmptyList(t *testing.T) {
	raw := `---
name: empty-tools
required_tools: []
---

body
`
	tools, err := ParseRequiredTools(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tools != nil {
		t.Fatalf("expected nil for empty required_tools, got %v", tools)
	}
}

func TestParseRequiredTools_CRLF(t *testing.T) {
	raw := "---\r\nname: crlf-skill\r\nrequired_tools:\r\n  - sin_edit\r\n  - sin_test\r\n---\r\n\r\nbody\r\n"
	tools, err := ParseRequiredTools(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %v", tools)
	}
	if tools[0] != "sin_edit" || tools[1] != "sin_test" {
		t.Fatalf("unexpected tools: %v", tools)
	}
}

func TestParseRequiredTools_TrimsWhitespace(t *testing.T) {
	raw := "---\nname: ws-skill\nrequired_tools:\n  -  sin_edit  \n  - \n---\n\nbody\n"
	tools, err := ParseRequiredTools(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 1 || tools[0] != "sin_edit" {
		t.Fatalf("expected [sin_edit], got %v", tools)
	}
}

func TestParseRequiredTools_MalformedYAML(t *testing.T) {
	raw := "---\nname: [bad yaml: {{{\n---\nbody\n"
	_, err := ParseRequiredTools(raw)
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestExtractRequiredTools_FromMapFS(t *testing.T) {
	fakeFS := fstest.MapFS{
		"my-skill/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: my-skill\nrequired_tools:\n  - sin_edit\n  - sin_test\n---\n\nbody\n"),
		},
	}
	tools, err := ExtractRequiredTools(fakeFS, "my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %v", tools)
	}
}

func TestExtractRequiredTools_EmptyName(t *testing.T) {
	tools, err := ExtractRequiredTools(fstest.MapFS{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tools != nil {
		t.Fatalf("expected nil for empty name, got %v", tools)
	}
}

func TestExtractRequiredTools_NotFound(t *testing.T) {
	_, err := ExtractRequiredTools(fstest.MapFS{}, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent skill")
	}
}

func TestExtractRequiredTools_NoRequiredTools(t *testing.T) {
	fakeFS := fstest.MapFS{
		"no-tools/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: no-tools\ndescription: test\n---\n\nbody\n"),
		},
	}
	tools, err := ExtractRequiredTools(fakeFS, "no-tools")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tools != nil {
		t.Fatalf("expected nil for skill with no required_tools, got %v", tools)
	}
}

func TestExtractRequiredTools_EmbeddedSkillCodeBuild(t *testing.T) {
	skillFS, err := skills.ListFS()
	if err != nil {
		t.Fatalf("skills.ListFS(): %v", err)
	}
	tools, err := ExtractRequiredTools(skillFS, "skill-code-build")
	if err != nil {
		t.Fatalf("ExtractRequiredTools(skill-code-build): %v", err)
	}
	want := []string{"sin_edit", "sin_test", "sin_quality_gate"}
	if len(tools) != len(want) {
		t.Fatalf("expected %d tools for skill-code-build, got %v (tools=%v)", len(want), len(tools), tools)
	}
	for _, w := range want {
		found := false
		for _, got := range tools {
			if got == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in required_tools for skill-code-build, got %v", w, tools)
		}
	}
}

func TestExtractRequiredTools_EmbeddedSkillDebugDeep(t *testing.T) {
	skillFS, err := skills.ListFS()
	if err != nil {
		t.Fatalf("skills.ListFS(): %v", err)
	}
	tools, err := ExtractRequiredTools(skillFS, "skill-debug-deep")
	if err != nil {
		t.Fatalf("ExtractRequiredTools(skill-debug-deep): %v", err)
	}
	want := []string{"sin_scout", "sin_grasp", "sin_poc"}
	if len(tools) != len(want) {
		t.Fatalf("expected %d tools for skill-debug-deep, got %v", len(want), tools)
	}
	for _, w := range want {
		found := false
		for _, got := range tools {
			if got == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in required_tools for skill-debug-deep, got %v", w, tools)
		}
	}
}

func TestMergeRequiredTools_DeduplicatesAndSorts(t *testing.T) {
	fakeFS := fstest.MapFS{
		"skill-a/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: skill-a\nrequired_tools:\n  - sin_edit\n  - sin_test\n---\n\nbody\n"),
		},
		"skill-b/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: skill-b\nrequired_tools:\n  - sin_edit\n  - sin_poc\n---\n\nbody\n"),
		},
	}
	existing := []string{"sin_edit", "sin_oracle"}
	merged := MergeRequiredTools(existing, []string{"skill-a", "skill-b"}, fakeFS)

	want := []string{"sin_edit", "sin_oracle", "sin_poc", "sin_test"}
	if len(merged) != len(want) {
		t.Fatalf("got %v, want %v", merged, want)
	}
	for i, w := range want {
		if merged[i] != w {
			t.Errorf("merged[%d] = %q, want %q (full: %v)", i, merged[i], w, merged)
		}
	}
}

func TestMergeRequiredTools_EmptyExisting(t *testing.T) {
	fakeFS := fstest.MapFS{
		"skill-a/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: skill-a\nrequired_tools:\n  - sin_edit\n---\n\nbody\n"),
		},
	}
	merged := MergeRequiredTools(nil, []string{"skill-a"}, fakeFS)
	if len(merged) != 1 || merged[0] != "sin_edit" {
		t.Fatalf("expected [sin_edit], got %v", merged)
	}
}

func TestMergeRequiredTools_NoSkills(t *testing.T) {
	existing := []string{"sin_edit", "sin_test"}
	merged := MergeRequiredTools(existing, nil, fstest.MapFS{})
	if len(merged) != 2 {
		t.Fatalf("expected 2 tools (existing only), got %v", merged)
	}
}

func TestMergeRequiredTools_NonExistentSkillSkipped(t *testing.T) {
	fakeFS := fstest.MapFS{}
	merged := MergeRequiredTools([]string{"sin_edit"}, []string{"nonexistent-skill"}, fakeFS)
	if len(merged) != 1 || merged[0] != "sin_edit" {
		t.Fatalf("expected [sin_edit], got %v", merged)
	}
}

func TestMergeRequiredTools_SkillWithNoRequiredToolsSkipped(t *testing.T) {
	fakeFS := fstest.MapFS{
		"no-tools/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: no-tools\ndescription: test\n---\n\nbody\n"),
		},
	}
	merged := MergeRequiredTools(nil, []string{"no-tools"}, fakeFS)
	if len(merged) != 0 {
		t.Fatalf("expected empty, got %v", merged)
	}
}

func TestMergeRequiredTools_EmptyAndWhitespaceSkillNamesSkipped(t *testing.T) {
	fakeFS := fstest.MapFS{
		"skill-a/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: skill-a\nrequired_tools:\n  - sin_edit\n---\n\nbody\n"),
		},
	}
	// Empty and whitespace-only skill names must be skipped (line 112-113);
	// only the real skill contributes its tools.
	merged := MergeRequiredTools(nil, []string{"", "  ", "skill-a"}, fakeFS)
	if len(merged) != 1 || merged[0] != "sin_edit" {
		t.Fatalf("expected [sin_edit], got %v", merged)
	}
}

func TestMergeRequiredTools_EmbeddedRealSkills(t *testing.T) {
	skillFS, err := skills.ListFS()
	if err != nil {
		t.Fatalf("skills.ListFS(): %v", err)
	}
	existing := []string{"sin_oracle"}
	merged := MergeRequiredTools(existing, []string{"skill-code-build", "skill-debug-deep"}, skillFS)

	want := []string{"sin_edit", "sin_grasp", "sin_oracle", "sin_poc", "sin_quality_gate", "sin_scout", "sin_test"}
	if len(merged) != len(want) {
		t.Fatalf("expected %d tools, got %d: %v", len(want), len(merged), merged)
	}
	for i, w := range want {
		if merged[i] != w {
			t.Errorf("merged[%d] = %q, want %q (full: %v)", i, merged[i], w, merged)
		}
	}
}

func TestMergeRequiredTools_IsByteStable(t *testing.T) {
	fakeFS := fstest.MapFS{
		"skill-a/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: skill-a\nrequired_tools:\n  - sin_edit\n  - sin_test\n---\n\nbody\n"),
		},
		"skill-b/SKILL.md": &fstest.MapFile{
			Data: []byte("---\nname: skill-b\nrequired_tools:\n  - sin_poc\n---\n\nbody\n"),
		},
	}
	existing := []string{"sin_oracle"}
	a := MergeRequiredTools(existing, []string{"skill-a", "skill-b"}, fakeFS)
	b := MergeRequiredTools(existing, []string{"skill-b", "skill-a"}, fakeFS)

	if len(a) != len(b) {
		t.Fatalf("byte-stability violated: different lengths %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("byte-stability violated at index %d: %q vs %q", i, a[i], b[i])
		}
	}
}
