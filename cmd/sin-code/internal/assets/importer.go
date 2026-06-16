// SPDX-License-Identifier: MIT
// Purpose: harvest skills from a vendored source repo (e.g. ECC), validate,
// filter by domain, exclude business/content skills, stamp attribution,
// write to a destination dir. Mirrors the ECC skills-import flow in a
// clean-room Go reimplementation.
// Docs: importer.doc.md
package assets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// package-level hooks to make import error branches testable.
var (
	osMkdirAllHook      = os.MkdirAll
	osWriteFileHook     = os.WriteFile
	loadSourceSkillsHook = loadSourceSkills
)

// ImportOptions controls a skill harvest from a vendored source repo.
type ImportOptions struct {
	SourceBase     string   // root of the cloned source repo (e.g. ./vendor/ecc)
	DestDir        string   // where to write imported skills (e.g. ./skills/imported)
	Origin         string   // attribution stamped into frontmatter, e.g. "ECC"
	License        string   // license stamped into frontmatter
	IncludeDomains []string // only import skills whose name/domain matches (empty = all)
	ExcludeNames   []string // skip these skill names (business/content skills)
	DryRun         bool
}

// ImportReport summarizes an import.
type ImportReport struct {
	Considered int
	Imported   int
	Skipped    int
	Invalid    int
	Names      []string
	Issues     []Issue
}

// DefaultExclusions are ECC content/business skills irrelevant to a
// coding agent.
func DefaultExclusions() []string {
	return []string{
		"article-writing", "brand-voice", "investor-materials", "market-research",
		"video-editing", "x-api", "crosspost", "newsletter", "social-media",
		"content-strategy", "seo-optimization",
	}
}

// ImportSkills loads skills from SourceBase, validates them, applies
// filters, stamps attribution, and writes survivors to DestDir.
func ImportSkills(opts ImportOptions) (ImportReport, error) {
	var rep ImportReport
	skills, err := loadSourceSkillsHook(opts.SourceBase)
	if err != nil {
		return rep, err
	}
	exclude := toSet(opts.ExcludeNames)
	includeDom := opts.IncludeDomains

	if !opts.DryRun && opts.DestDir != "" {
		if err := osMkdirAllHook(opts.DestDir, 0o755); err != nil {
			return rep, err
		}
	}

	for _, a := range skills {
		rep.Considered++
		if exclude[strings.ToLower(a.Name)] {
			rep.Skipped++
			continue
		}
		if len(includeDom) > 0 && !matchesAnyDomain(a, includeDom) {
			rep.Skipped++
			continue
		}
		if issues := Validate(a); hasErrors(issues) {
			rep.Invalid++
			rep.Issues = append(rep.Issues, issues...)
			continue
		}
		// Stamp attribution.
		if a.Origin == "" {
			a.Origin = opts.Origin
		}
		if a.License == "" {
			a.License = opts.License
		}
		rep.Names = append(rep.Names, a.Name)
		rep.Imported++

		if opts.DryRun || opts.DestDir == "" {
			continue
		}
		data, err := a.Render()
		if err != nil {
			return rep, err
		}
		dst := filepath.Join(opts.DestDir, a.Name, "SKILL.md")
		if err := osMkdirAllHook(filepath.Dir(dst), 0o755); err != nil {
			return rep, err
		}
		if err := osWriteFileHook(dst, data, 0o644); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func loadSourceSkills(base string) ([]*Asset, error) {
	var all []*Asset
	// ECC keeps canonical skills under .agents/skills and language skills
	// under .kiro/skills.
	for _, sub := range []string{".agents/skills", ".kiro/skills", "skills"} {
		dir := filepath.Join(base, sub)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		loaded, err := LoadDir(dir, KindSkill)
		if err != nil {
			return nil, err
		}
		all = append(all, loaded...)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("no skills found under %s", base)
	}
	return dedupeByName(all), nil
}

func matchesAnyDomain(a *Asset, domains []string) bool {
	hay := strings.ToLower(a.Name + " " + a.Domain)
	for _, d := range domains {
		if strings.Contains(hay, strings.ToLower(d)) {
			return true
		}
	}
	return false
}

func dedupeByName(list []*Asset) []*Asset {
	seen := map[string]bool{}
	var out []*Asset
	for _, a := range list {
		key := strings.ToLower(a.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, a)
	}
	return out
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[strings.ToLower(s)] = true
	}
	return m
}

func hasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Level == "error" {
			return true
		}
	}
	return false
}
