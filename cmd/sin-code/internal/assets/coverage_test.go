// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests to reach 100% statement coverage.
package assets

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// captureStdout runs f and returns everything written to os.Stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	outCh := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		outCh <- buf.String()
	}()
	f()
	_ = w.Close()
	os.Stdout = old
	return <-outCh
}

// writeFile writes a file with 0o644 permissions.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ─── ParseAsset / Render ─────────────────────────────────────────────────

func TestParseAsset_MissingFrontmatter(t *testing.T) {
	_, err := ParseAsset(KindSkill, "x.md", []byte("no frontmatter"))
	if err == nil || !strings.Contains(err.Error(), "missing frontmatter") {
		t.Fatalf("expected missing frontmatter error, got %v", err)
	}
}

func TestParseAsset_UnterminatedFrontmatter(t *testing.T) {
	_, err := ParseAsset(KindSkill, "x.md", []byte("---\nname: x\n"))
	if err == nil || !strings.Contains(err.Error(), "unterminated frontmatter") {
		t.Fatalf("expected unterminated frontmatter error, got %v", err)
	}
}

func TestParseAsset_BadYAML(t *testing.T) {
	_, err := ParseAsset(KindSkill, "x.md", []byte("---\nname: [bad\n---\nbody"))
	if err == nil || !strings.Contains(err.Error(), "parse frontmatter") {
		t.Fatalf("expected parse frontmatter error, got %v", err)
	}
}

func TestRender_YAMLError(t *testing.T) {
	old := yamlMarshalHook
	yamlMarshalHook = func(v any) ([]byte, error) {
		return nil, fmt.Errorf("marshal failed")
	}
	defer func() { yamlMarshalHook = old }()

	a := &Asset{Kind: KindSkill, Name: "x", Body: "body"}
	_, err := a.Render()
	if err == nil || !strings.Contains(err.Error(), "marshal failed") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

// ─── Registry / Selector ─────────────────────────────────────────────────

func TestRegistry_GetAndForDomain(t *testing.T) {
	r := NewRegistry()
	r.Add(&Asset{Kind: KindAgent, Name: "go", Domain: "go"})
	r.Add(&Asset{Kind: KindSkill, Name: "rust", Domain: "rust"})

	if _, ok := r.Get(KindAgent, "go"); !ok {
		t.Fatal("Get should find go agent")
	}
	if _, ok := r.Get(KindAgent, "missing"); ok {
		t.Fatal("Get should not find missing")
	}
	if got := r.ForDomain("rust"); len(got) != 1 || got[0].Name != "rust" {
		t.Fatalf("ForDomain: %v", got)
	}
}

func TestSelector_SelectCommandsAndTieBreak(t *testing.T) {
	r := NewRegistry()
	r.AddAll([]*Asset{
		{Kind: KindCommand, Name: "z-cmd", Description: "z"},
		{Kind: KindCommand, Name: "a-cmd", Description: "a"},
	})
	sel := NewSelector(r)
	matches := sel.SelectCommands(Context{Keywords: []string{"cmd"}}, 1)
	if len(matches) != 1 || matches[0].Asset.Name != "a-cmd" {
		t.Fatalf("expected a-cmd first on tie-break, got %+v", matches)
	}
}

func TestSelector_NameKeywordAndEmpty(t *testing.T) {
	r := NewRegistry()
	r.Add(&Asset{Kind: KindAgent, Name: "go-reviewer", Description: "reviews"})
	sel := NewSelector(r)
	if got := sel.SelectAgents(Context{Keywords: []string{"go", ""}}, 3); len(got) != 1 || got[0].Score != 4 {
		t.Fatalf("expected name keyword match, got %+v", got)
	}
}

func TestSelector_SortByScore(t *testing.T) {
	r := NewRegistry()
	r.AddAll([]*Asset{
		{Kind: KindAgent, Name: "keyword-only", Domain: "", Description: "has reviews"},
		{Kind: KindAgent, Name: "domain-match", Domain: "go", Description: "go agent"},
	})
	sel := NewSelector(r)
	got := sel.SelectAgents(Context{Domain: "go", Keywords: []string{"reviews"}}, 3)
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %+v", got)
	}
	if got[0].Asset.Name != "domain-match" {
		t.Fatalf("expected domain-match first (higher score), got %+v", got)
	}
}

// ─── LoadDir / LoadStandardLayout ───────────────────────────────────────

func TestLoadDir_SkipsNonMD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "skip.txt"), "---\n---\n")
	writeFile(t, filepath.Join(dir, "ok.md"), "---\nname: ok\n---\n")
	loaded, err := LoadDir(dir, KindAgent)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Name != "ok" {
		t.Fatalf("expected one agent, got %+v", loaded)
	}
}

func TestLoadDir_SkipsNonSkillMD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x", "not-skill.md"), "---\nname: x\n---\n")
	loaded, err := LoadDir(dir, KindSkill)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected 0 skills, got %d", len(loaded))
	}
}

func TestLoadDir_DefaultNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agents", "no-name.md"), "---\n---\nbody here is longer than twenty")
	writeFile(t, filepath.Join(dir, "skills", "skill-name", "SKILL.md"), "---\n---\nbody here is longer than twenty")

	agents, err := LoadDir(filepath.Join(dir, "agents"), KindAgent)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Name != "no-name" {
		t.Fatalf("expected agent name 'no-name', got %+v", agents)
	}

	skills, err := LoadDir(filepath.Join(dir, "skills"), KindSkill)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Name != "skill-name" {
		t.Fatalf("expected skill name 'skill-name', got %+v", skills)
	}
}

func TestLoadDir_ParseError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.md"), "not frontmatter")
	_, err := LoadDir(dir, KindAgent)
	if err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLoadDir_ReadError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.md"), "---\n---\n")

	old := osReadFileHook
	osReadFileHook = func(path string) ([]byte, error) {
		return nil, fmt.Errorf("read failed")
	}
	defer func() { osReadFileHook = old }()

	_, err := LoadDir(dir, KindAgent)
	if err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestLoadDir_WalkError(t *testing.T) {
	dir := t.TempDir()
	old := walkDirHook
	walkDirHook = func(root string, fn fs.WalkDirFunc) error {
		return fmt.Errorf("walk failed")
	}
	defer func() { walkDirHook = old }()

	_, err := LoadDir(dir, KindAgent)
	if err == nil || !strings.Contains(err.Error(), "walk failed") {
		t.Fatalf("expected walk error, got %v", err)
	}
}

func TestLoadDir_WalkCallbackError(t *testing.T) {
	dir := t.TempDir()
	// Create an unreadable subdirectory so the WalkDir callback receives an error.
	badDir := filepath.Join(dir, "bad")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(badDir, 0o755)

	_, err := LoadDir(dir, KindAgent)
	if err == nil {
		t.Fatal("expected walk callback error")
	}
}

func TestLoadStandardLayout(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "agents", "a.md"), "---\nname: a\n---\n")
	writeFile(t, filepath.Join(base, "commands", "c.md"), "---\nname: c\n---\n")
	writeFile(t, filepath.Join(base, "skills", "s", "SKILL.md"), "---\nname: s\n---\n")

	loaded, err := LoadStandardLayout(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 assets, got %d", len(loaded))
	}
}

// ─── Validate / ValidateAll ─────────────────────────────────────────────

func TestValidate_AllIssueBranches(t *testing.T) {
	cases := []struct {
		name  string
		asset *Asset
		want  int // number of issues
	}{
		{
			name: "missing name",
			asset: &Asset{Kind: KindSkill, Name: "", Description: "d", Path: "x.md", Body: "## Section\n" +
				strings.Repeat("x", 20)},
			want: 1,
		},
		{
			name: "missing description",
			asset: &Asset{Kind: KindSkill, Name: "n", Description: "", Path: "x.md", Body: "## Section\n" +
				strings.Repeat("x", 20)},
			want: 1,
		},
		{
			name:  "short body",
			asset: &Asset{Kind: KindSkill, Name: "n", Description: "d", Path: "x.md", Body: "short"},
			want:  2,
		},
		{
			name:  "agent no model",
			asset: &Asset{Kind: KindAgent, Name: "n", Description: "d", Path: "x.md", Body: strings.Repeat("x", 20)},
			want:  2,
		},
		{
			name:  "command no dollar",
			asset: &Asset{Kind: KindCommand, Name: "n", Description: "d", Path: "x.md", Argument: "arg", Body: "no placeholder"},
			want:  2,
		},
		{
			name:  "skill no sections",
			asset: &Asset{Kind: KindSkill, Name: "n", Description: "d", Path: "x.md", Body: strings.Repeat("x", 20)},
			want:  1,
		},
		{
			name:  "unsafe unicode",
			asset: &Asset{Kind: KindSkill, Name: "n", Description: "d", Path: "x.md", Body: "## S\n" + "\u200B"},
			want:  2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := Validate(tc.asset)
			if len(issues) != tc.want {
				t.Fatalf("want %d issues, got %d: %v", tc.want, len(issues), issues)
			}
		})
	}
}

func TestValidateAll_Duplicate(t *testing.T) {
	a1 := &Asset{Kind: KindSkill, Name: "x", Description: "d", Path: "a.md", Body: "## S\n"}
	a2 := &Asset{Kind: KindSkill, Name: "x", Description: "d", Path: "b.md", Body: "## S\n"}
	issues := ValidateAll([]*Asset{a1, a2})
	found := false
	for _, is := range issues {
		if strings.Contains(is.Message, "duplicate") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected duplicate issue, got %v", issues)
	}
}

func TestContains_NoMatch(t *testing.T) {
	if contains("hello", "xyz") {
		t.Fatal("expected false")
	}
}

// ─── ImportSkills ────────────────────────────────────────────────────────

func TestImportSkills_LoadSourceError(t *testing.T) {
	old := loadSourceSkillsHook
	loadSourceSkillsHook = func(base string) ([]*Asset, error) {
		return nil, fmt.Errorf("load failed")
	}
	defer func() { loadSourceSkillsHook = old }()

	_, err := ImportSkills(ImportOptions{SourceBase: "src", DestDir: "dst"})
	if err == nil || !strings.Contains(err.Error(), "load failed") {
		t.Fatalf("expected load error, got %v", err)
	}
}

func TestImportSkills_MkdirAllError(t *testing.T) {
	old := osMkdirAllHook
	osMkdirAllHook = func(path string, perm fs.FileMode) error {
		return fmt.Errorf("mkdir failed")
	}
	defer func() { osMkdirAllHook = old }()

	src := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "go-patterns", "SKILL.md"),
		"---\nname: go-patterns\ndescription: d\n---\n\n## S\n"+strings.Repeat("x", 20))

	_, err := ImportSkills(ImportOptions{SourceBase: src, DestDir: "dst", IncludeDomains: []string{"go"}})
	if err == nil || !strings.Contains(err.Error(), "mkdir failed") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestImportSkills_DomainFilter(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "go-patterns", "SKILL.md"),
		"---\nname: go-patterns\ndescription: d\n---\n\n## S\n"+strings.Repeat("x", 20))
	writeFile(t, filepath.Join(src, ".agents", "skills", "rust-patterns", "SKILL.md"),
		"---\nname: rust-patterns\ndescription: d\n---\n\n## S\n"+strings.Repeat("x", 20))

	rep, err := ImportSkills(ImportOptions{SourceBase: src, DestDir: dst, IncludeDomains: []string{"go"}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Imported != 1 || rep.Skipped != 1 {
		t.Fatalf("expected 1 imported, 1 skipped, got %+v", rep)
	}
}

func TestImportSkills_InvalidAsset(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "bad", "SKILL.md"),
		"---\nname: bad\ndescription: \n---\n\n## S\n"+strings.Repeat("x", 20))

	rep, err := ImportSkills(ImportOptions{SourceBase: src, DestDir: dst, IncludeDomains: []string{"bad"}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Invalid != 1 || rep.Imported != 0 {
		t.Fatalf("expected 1 invalid, 0 imported, got %+v", rep)
	}
	if len(rep.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %+v", rep.Issues)
	}
}

func TestImportSkills_InvalidAsset_PrintsIssues(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "bad", "SKILL.md"),
		"---\nname: bad\ndescription: \n---\n\n## S\n"+strings.Repeat("x", 20))

	cmd := NewCommand("")
	cmd.SetArgs([]string{"import", "--source", src, "--dest", "", "--domains", "bad", "--dry-run"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			// import may return error because dest is empty; that's fine.
		}
	})
	if !strings.Contains(out, "!") {
		t.Fatalf("expected issue marker in output, got %s", out)
	}
}

func TestImportSkills_RenderError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "x", "SKILL.md"),
		"---\nname: x\ndescription: d\n---\n\n## S\n"+strings.Repeat("x", 20))

	old := yamlMarshalHook
	yamlMarshalHook = func(v any) ([]byte, error) { return nil, fmt.Errorf("render failed") }
	defer func() { yamlMarshalHook = old }()

	_, err := ImportSkills(ImportOptions{SourceBase: src, DestDir: dst, IncludeDomains: []string{"x"}})
	if err == nil || !strings.Contains(err.Error(), "render failed") {
		t.Fatalf("expected render error, got %v", err)
	}
}

func TestImportSkills_WriteFileError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "x", "SKILL.md"),
		"---\nname: x\ndescription: d\n---\n\n## S\n"+strings.Repeat("x", 20))

	old := osWriteFileHook
	osWriteFileHook = func(path string, data []byte, perm fs.FileMode) error {
		return fmt.Errorf("write failed")
	}
	defer func() { osWriteFileHook = old }()

	_, err := ImportSkills(ImportOptions{SourceBase: src, DestDir: dst, IncludeDomains: []string{"x"}})
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestImportSkills_DestDirMkdirError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "x", "SKILL.md"),
		"---\nname: x\ndescription: d\n---\n\n## S\n"+strings.Repeat("x", 20))

	oldMkdir := osMkdirAllHook
	oldWrite := osWriteFileHook
	defer func() { osMkdirAllHook = oldMkdir; osWriteFileHook = oldWrite }()

	calls := 0
	osMkdirAllHook = func(path string, perm fs.FileMode) error {
		calls++
		if calls == 2 { // second call is for the skill subdir
			return fmt.Errorf("subdir mkdir failed")
		}
		return os.MkdirAll(path, perm)
	}
	osWriteFileHook = func(path string, data []byte, perm fs.FileMode) error {
		return os.WriteFile(path, data, perm)
	}

	_, err := ImportSkills(ImportOptions{SourceBase: src, DestDir: dst, IncludeDomains: []string{"x"}})
	if err == nil || !strings.Contains(err.Error(), "subdir mkdir failed") {
		t.Fatalf("expected subdir mkdir error, got %v", err)
	}
}

func TestLoadSourceSkills_NoSkills(t *testing.T) {
	base := t.TempDir()
	_, err := loadSourceSkills(base)
	if err == nil || !strings.Contains(err.Error(), "no skills found") {
		t.Fatalf("expected no skills error, got %v", err)
	}
}

func TestLoadSourceSkills_LoadDirError(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	old := walkDirHook
	walkDirHook = func(root string, fn fs.WalkDirFunc) error {
		return fmt.Errorf("walk failed")
	}
	defer func() { walkDirHook = old }()

	_, err := loadSourceSkills(base)
	if err == nil || !strings.Contains(err.Error(), "walk failed") {
		t.Fatalf("expected walk error, got %v", err)
	}
}

func TestMatchesAnyDomain_False(t *testing.T) {
	a := &Asset{Name: "python", Domain: "python"}
	if matchesAnyDomain(a, []string{"go", "rust"}) {
		t.Fatal("expected no domain match")
	}
}

func TestDedupeByName_Duplicate(t *testing.T) {
	list := []*Asset{
		{Name: "Same"},
		{Name: "same"},
	}
	out := dedupeByName(list)
	if len(out) != 1 {
		t.Fatalf("expected 1, got %d", len(out))
	}
}

func TestHasErrors_True(t *testing.T) {
	if !hasErrors([]Issue{{Level: "error"}, {Level: "warn"}}) {
		t.Fatal("expected hasErrors true")
	}
}

// ─── CLI ─────────────────────────────────────────────────────────────────

func TestNewCommand_List(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "agents", "a.md"), "---\nname: a\ndescription: d\norigin: ECC\n---\n")
	writeFile(t, filepath.Join(base, "commands", "c.md"), "---\nname: c\ndescription: d\n---\n")

	cmd := NewCommand(base)
	cmd.SetArgs([]string{"list"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "agent") || !strings.Contains(out, "command") {
		t.Fatalf("expected list output, got %s", out)
	}
}

func TestNewCommand_ListWithKind(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "agents", "a.md"), "---\nname: a\ndescription: d\n---\n")
	writeFile(t, filepath.Join(base, "commands", "c.md"), "---\nname: c\ndescription: d\n---\n")

	cmd := NewCommand(base)
	cmd.SetArgs([]string{"list", "agent"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "agent") {
		t.Fatalf("expected agent in output, got %s", out)
	}
	if strings.Contains(out, "command") {
		t.Fatalf("did not expect command in output, got %s", out)
	}
}

func TestNewCommand_Validate(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "agents", "a.md"),
		"---\nname: a\ndescription: d\nmodel: m\ntools: [t]\n---\n"+strings.Repeat("x", 20))

	cmd := NewCommand(base)
	cmd.SetArgs([]string{"validate"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("validate returned error: %v", err)
		}
	})
	if !strings.Contains(out, "0 issues") {
		t.Fatalf("expected no issues, got %s", out)
	}
}

func TestNewCommand_ValidateErrors(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "agents", "a.md"), "---\nname: a\ndescription: \n---\n")

	cmd := NewCommand(base)
	cmd.SetArgs([]string{"validate"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err == nil {
			t.Fatal("expected validation error")
		}
	})
	if !strings.Contains(out, "error") {
		t.Fatalf("expected error output, got %s", out)
	}
}

func TestNewCommand_Show(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "agents", "a.md"), "---\nname: a\ndescription: d\n---\nbody text")

	cmd := NewCommand(base)
	cmd.SetArgs([]string{"show", "agent", "a"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "body text") {
		t.Fatalf("expected body text, got %s", out)
	}
}

func TestNewCommand_ShowNotFound(t *testing.T) {
	base := t.TempDir()
	cmd := NewCommand(base)
	cmd.SetArgs([]string{"show", "agent", "missing"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected not found error")
	}
}

func TestNewCommand_Import(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "go-patterns", "SKILL.md"),
		"---\nname: go-patterns\ndescription: d\n---\n\n## S\n"+strings.Repeat("x", 20))

	cmd := NewCommand("")
	cmd.SetArgs([]string{
		"import", "--source", src, "--dest", dst,
		"--domains", "go", "--dry-run",
	})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "go-patterns") {
		t.Fatalf("expected go-patterns in output, got %s", out)
	}
	if !strings.Contains(out, "dry run") {
		t.Fatalf("expected dry run message, got %s", out)
	}
}

func TestNewCommand_ImportError(t *testing.T) {
	cmd := NewCommand("")
	cmd.SetArgs([]string{"import", "--source", "", "--dest", "dst", "--domains", "go"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected import error")
	}
}

func TestNewCommand_ImportAll(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "go-patterns", "SKILL.md"),
		"---\nname: go-patterns\ndescription: d\n---\n\n## S\n"+strings.Repeat("x", 20))

	cmd := NewCommand("")
	cmd.SetArgs([]string{
		"import", "--source", src, "--dest", "", "--all", "--dry-run",
	})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "go-patterns") {
		t.Fatalf("expected go-patterns in output, got %s", out)
	}
}

func TestNewCommand_LoadError(t *testing.T) {
	base := t.TempDir()
	// Create a file so LoadStandardLayout calls LoadDir (not just skipping).
	writeFile(t, filepath.Join(base, "agents", "a.md"), "---\nname: a\ndescription: d\n---\n")
	cmd := NewCommand(base)
	cmd.SetArgs([]string{"list", "agent"})
	old := walkDirHook
	walkDirHook = func(root string, fn fs.WalkDirFunc) error {
		return fmt.Errorf("walk failed")
	}
	defer func() { walkDirHook = old }()

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected load error")
	}
}

func TestNewCommand_ValidateLoadError(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "agents", "a.md"), "---\nname: a\ndescription: d\n---\n")
	cmd := NewCommand(base)
	cmd.SetArgs([]string{"validate"})
	old := walkDirHook
	walkDirHook = func(root string, fn fs.WalkDirFunc) error {
		return fmt.Errorf("walk failed")
	}
	defer func() { walkDirHook = old }()

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected validate load error")
	}
}

func TestNewCommand_ShowLoadError(t *testing.T) {
	base := t.TempDir()
	writeFile(t, filepath.Join(base, "agents", "a.md"), "---\nname: a\ndescription: d\n---\n")
	cmd := NewCommand(base)
	cmd.SetArgs([]string{"show", "agent", "a"})
	old := walkDirHook
	walkDirHook = func(root string, fn fs.WalkDirFunc) error {
		return fmt.Errorf("walk failed")
	}
	defer func() { walkDirHook = old }()

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected show load error")
	}
}

func TestNewCommand_ImportWithDefaults(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, ".agents", "skills", "go-patterns", "SKILL.md"),
		"---\nname: go-patterns\ndescription: d\n---\n\n## S\n"+strings.Repeat("x", 20))

	cmd := NewCommand("")
	cmd.SetArgs([]string{"import", "--source", src, "--dest", "", "--dry-run"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "go-patterns") {
		t.Fatalf("expected go-patterns in output, got %s", out)
	}
}

func TestYAMLUnmarshal(t *testing.T) {
	// Exercise the yaml.Unmarshal error path in a controlled way.
	var a Asset
	if err := yaml.Unmarshal([]byte("name: [bad"), &a); err == nil {
		t.Fatal("expected yaml error")
	}
}
