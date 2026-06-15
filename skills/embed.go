// SPDX-License-Identifier: MIT
// Purpose: embed all project-local agent skills into the SIN-Code binary.
// Docs: skills.doc.md
package skills

import "embed"

// SkillsFS embeds every skill directory under the repository-root skills/ folder.
// The embedded filesystem is consumed by the `sin-code skills` subcommand via
// github.com/Songmu/skillsmith so users can install bundled skills into
// ~/.claude/skills/ or ~/.agents/skills/.
//
//go:embed *
var SkillsFS embed.FS
