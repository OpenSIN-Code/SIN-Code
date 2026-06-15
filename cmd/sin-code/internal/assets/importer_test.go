// SPDX-License-Identifier: MIT
// Purpose: tests for the skill importer — domain filter, exclusion list,
// attribution stamp, dry-run no-write guarantee.
// Docs: importer_test.doc.md
package assets

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkill(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: test skill\n---\n\n## Section\n" + body
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestImportSkillsFiltersAndStamps(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	skillsDir := filepath.Join(src, ".agents", "skills")

	writeSkill(t, skillsDir, "golang-patterns", "use table-driven tests and error wrapping")
	writeSkill(t, skillsDir, "article-writing", "how to write blog posts") // excluded
	writeSkill(t, skillsDir, "rust-testing", "use cargo test and proptest")

	rep, err := ImportSkills(ImportOptions{
		SourceBase:     src,
		DestDir:        dst,
		Origin:         "ECC",
		IncludeDomains: []string{"golang", "rust"},
		ExcludeNames:   DefaultExclusions(),
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if rep.Imported != 2 {
		t.Fatalf("expected 2 imported, got %d (%v)", rep.Imported, rep.Names)
	}
	// Verify attribution was stamped.
	data, err := os.ReadFile(filepath.Join(dst, "golang-patterns", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), "origin: ECC") {
		t.Fatalf("origin not stamped:\n%s", data)
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeSkill(t, filepath.Join(src, ".agents", "skills"), "go-patterns", "stuff here")

	rep, err := ImportSkills(ImportOptions{
		SourceBase: src, DestDir: dst, IncludeDomains: []string{"go"}, DryRun: true,
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if rep.Imported != 1 {
		t.Fatalf("expected 1 counted, got %d", rep.Imported)
	}
	if entries, _ := os.ReadDir(dst); len(entries) != 0 {
		t.Fatalf("dry run wrote files: %v", entries)
	}
}
