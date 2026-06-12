// SPDX-License-Identifier: MIT
// Purpose: idempotent overlay block appended to each SKILL.md.
// The overlay tells the agent (and any human reader) that the file has
// been processed by SIN-Code, where the canonical copy lives, which
// commit SHA it was synced to, and which SIN-Code verifications apply.
// Docs: superpowers.doc.md
package superpowers

import (
	"os"
	"path/filepath"
	"strings"
)

// AppendOverlay returns true if it modified path, false if the overlay
// was already present (idempotent). It is safe to call repeatedly.
//
// The overlay is rendered between OverlayMarker / OverlayMarkerEnd
// sentinels (defined in superpowers.go) and is detected via a substring
// search. Any other text in the file is preserved verbatim.
func AppendOverlay(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	existing := string(body)
	if strings.Contains(existing, OverlayMarker) {
		return false
	}
	overlay := RenderOverlay(SkillOverlayContext{
		Path:        path,
		SkillsRoot:  filepath.Dir(filepath.Dir(path)), // <skills>/<name>/SKILL.md → <skills>
		CommitHint:  commitHint(path),
		OverlayKind: SkillOverlay,
	})
	var b strings.Builder
	b.WriteString(existing)
	if !strings.HasSuffix(existing, "\n") {
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(overlay)
	return os.WriteFile(path, []byte(b.String()), 0o644) == nil
}

// OverlayKind tags the rendered block with a stable identifier so the
// same RenderOverlay can produce skill- vs root-level overlays with
// different wording.
type OverlayKind string

const (
	SkillOverlay OverlayKind = "skill"
	RootOverlay  OverlayKind = "root"
)

// SkillOverlayContext is the data baked into the rendered overlay block.
type SkillOverlayContext struct {
	Path        string
	SkillsRoot  string
	CommitHint  string
	OverlayKind OverlayKind
}

// RenderOverlay is exported because the test suite asserts on the exact
// output and a future "scaffold new skill" command may want the same
// block in a freshly-created SKILL.md.
func RenderOverlay(ctx SkillOverlayContext) string {
	var b strings.Builder
	b.WriteString(OverlayMarker)
	b.WriteString("\n# SIN-Code overlay (auto-generated — do not edit by hand)\n")
	b.WriteString("# Path:        ")
	b.WriteString(ctx.Path)
	b.WriteByte('\n')
	if ctx.SkillsRoot != "" {
		b.WriteString("# Skills root: ")
		b.WriteString(ctx.SkillsRoot)
		b.WriteByte('\n')
	}
	if ctx.CommitHint != "" {
		b.WriteString("# Commit:      ")
		b.WriteString(ctx.CommitHint)
		b.WriteByte('\n')
	}
	b.WriteString("# Kind:        ")
	b.WriteString(string(ctx.OverlayKind))
	b.WriteByte('\n')
	b.WriteString("# Verifications: superpowers list / show / find run through `sin-code superpowers`.\n")
	b.WriteString("# Regenerate by re-running `sin-code superpowers install`.\n")
	b.WriteString(OverlayMarkerEnd)
	b.WriteByte('\n')
	return b.String()
}

// commitHint returns a short (8-char) commit SHA if the .sin-code-pin
// file is readable. Errors are swallowed — we only ever use the hint as
// a human-readable label, never for security decisions.
func commitHint(skillPath string) string {
	// skillPath = $Home/skills/superpowers/<name>/SKILL.md
	// skillsRoot = $Home/skills/superpowers
	skillsRoot := filepath.Dir(filepath.Dir(filepath.Dir(skillPath)))
	pinPath := filepath.Join(skillsRoot, ".sin-code-pin")
	b, err := os.ReadFile(pinPath)
	if err != nil {
		return ""
	}
	// Cheap parse: we don't care about full JSON validity here, only the
	// "sha" field. Falls back to first 8 chars of file content.
	s := string(b)
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "\"sha\":") {
			v := strings.TrimPrefix(line, "\"sha\":")
			v = strings.Trim(v, " \",")
			if len(v) >= 8 {
				return v[:8]
			}
			return v
		}
	}
	if len(s) >= 8 {
		return strings.TrimSpace(s[:8])
	}
	return ""
}
