// SPDX-License-Identifier: MIT
// Purpose: extract the `required_tools` field from a skill's SKILL.md YAML
// frontmatter and merge it into the agent loop's tool-coverage enforcer
// (issue #248). When a skill is activated (via --activate or
// .sin-code/autoactivate.toml), its required_tools list is additive —
// appended to any config-level required_tools with deduplication.
//
// The parser uses gopkg.in/yaml.v3 (already a direct dependency, M2-safe)
// and mirrors the frontmatter delimiting convention used by skilldist and
// instinct/frontmatter.go: the file starts with `---` and the frontmatter
// block terminates at the next `---` line.
package skillmgr

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillFrontmatter is the minimal subset of a SKILL.md frontmatter we
// unmarshal. yaml.v3 ignores unknown keys, so adding new fields to the
// frontmatter does not break this struct.
type skillFrontmatter struct {
	Name          string   `yaml:"name"`
	RequiredTools []string `yaml:"required_tools"`
}

// ExtractRequiredTools reads <skillName>/SKILL.md from skillFS, parses its
// YAML frontmatter, and returns the `required_tools` list. Returns nil
// (no error) when the skill exists but has no required_tools field — the
// caller treats nil as "no coverage constraints from this skill".
//
// The skillFS is expected to be the flattened view produced by
// skills.ListFS() (each skill directory at the root), but any fs.FS that
// exposes "<skillName>/SKILL.md" works.
func ExtractRequiredTools(skillFS fs.FS, skillName string) ([]string, error) {
	if skillName == "" {
		return nil, nil
	}
	raw, err := fs.ReadFile(skillFS, fmt.Sprintf("%s/SKILL.md", skillName))
	if err != nil {
		return nil, fmt.Errorf("skillmgr: read SKILL.md for %q: %w", skillName, err)
	}
	return ParseRequiredTools(string(raw))
}

// ParseRequiredTools extracts the `required_tools` list from the YAML
// frontmatter of a SKILL.md body. Returns nil (no error) when the
// frontmatter has no required_tools field or the file has no frontmatter.
//
// The frontmatter delimiting rule mirrors skilldist.StripFrontmatter and
// instinct/frontmatter.Unmarshal: the file starts with `---` on its own
// line and the block terminates at the next `---` line. Both LF and CRLF
// are tolerated.
func ParseRequiredTools(raw string) ([]string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(raw, "---") {
		return nil, nil
	}
	rest := strings.TrimPrefix(raw, "---")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, nil
	}
	fmBlock := rest[:idx]

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(fmBlock), &fm); err != nil {
		return nil, fmt.Errorf("skillmgr: parse frontmatter: %w", err)
	}

	out := make([]string, 0, len(fm.RequiredTools))
	for _, t := range fm.RequiredTools {
		t = strings.TrimSpace(t)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// MergeRequiredTools combines existing required tools with the required_tools
// extracted from the named skills in skillFS. The result is deduplicated and
// sorted lexicographically so the output is byte-stable for a given
// (existing, skillNames, skillFS) tuple.
//
// Skills that are not found in skillFS or that have no required_tools are
// silently skipped — this is best-effort: an activated rule name like
// "terse" does not correspond to a skill and should not cause an error.
//
// The existing slice is not mutated; a new slice is returned.
func MergeRequiredTools(existing []string, skillNames []string, skillFS fs.FS) []string {
	seen := make(map[string]bool)
	merged := make([]string, 0, len(existing))

	for _, t := range existing {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			seen[t] = true
			merged = append(merged, t)
		}
	}

	for _, name := range skillNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tools, err := ExtractRequiredTools(skillFS, name)
		if err != nil {
			continue
		}
		for _, t := range tools {
			t = strings.TrimSpace(t)
			if t != "" && !seen[t] {
				seen[t] = true
				merged = append(merged, t)
			}
		}
	}

	sort.Strings(merged)
	return merged
}
