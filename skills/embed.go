// SPDX-License-Identifier: MIT
// Purpose: embed all project-local agent skills into the SIN-Code binary.
// Docs: skills.doc.md
package skills

import (
	"embed"
	"io/fs"
	"sync"
)

// SkillsFS embeds every skill directory under the repository-root skills/ folder.
// The embedded filesystem is consumed by the `sin-code skills` subcommand via
// github.com/Songmu/skillsmith so users can install bundled skills into
// ~/.claude/skills/ or ~/.agents/skills/.
//
//go:embed *
var SkillsFS embed.FS

// listFSOnce holds the lazily-built flattened view of SkillsFS. Skillsmith
// expects all skill directories at the root of the FS, but SIN-Code organizes
// skills into category folders (code-skills/, shop-skills/, ...). listFSOnce
// maps each leaf skill directory back to the root by skill name.
var listFSOnce = sync.OnceValues(func() (fs.FS, error) {
	return newFlatSkillFS(SkillsFS)
})

// ListFS returns a flattened fs.FS suitable for skillsmith.Discover and
// skillsmith.CopySkills. The returned FS exposes every leaf skill directory
// at the root level (e.g. "code-skills/skill-code-add-endpoint" becomes "skill-code-add-endpoint").
func ListFS() (fs.FS, error) {
	return listFSOnce()
}
