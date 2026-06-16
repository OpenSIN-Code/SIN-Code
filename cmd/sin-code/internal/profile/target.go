// SPDX-License-Identifier: MIT
// Package profile renders the single-source-of-truth project profile
// (docs/agent-profiles/sin-profile.md, issue #175) into every supported
// per-host-agent format. It mirrors the install-path / Format model of
// internal/skilldist (issue #169) but operates at the **repository** level
// (writes under the repo root, not $HOME) and emits one artifact per
// target.
//
// # Marker-fence covenant
//
// Every write through `Render` for FormatRule / FormatMarker goes through
// the same marker-fence primitive as internal/skilldist:
//
//	<!-- SIN-CODE-SKILL-START: <skill> -->
//	# Skill: <skill>
//	… rendered body …
//	<!-- SIN-CODE-SKILL-END:   <skill> -->
//
// We deliberately reuse the "SIN-CODE-SKILL" marker prefix even though
// we are rendering a profile, not a skill: the two systems share
// per-agent rule files (`.codex/rules/sin-code.md`,
// `.cursor/rules/sin-code.mdc`, etc.) so a single fence convention keeps
// downstream installers (skilldist, future skill-aware rmtree passes)
// able to find both kinds of content with one regex. A subsequent
// `profile render` call with unchanged bytes produces byte-identical
// output, exactly as skilldist demands.
//
// # Targets
//
// Targets is the single source of truth for the per-agent install path
// templates. The map is intentionally a structural copy of the table in
// AGENTS.md §10: adding a target is non-breaking; renaming or removing
// one is a major bump. Skill name is fixed at "sin-code" because the
// profile is one fixed artifact.
package profile

import (
	"fmt"
	"sort"
)

// Format kinds — match internal/skilldist so the public surface stays
// parallel across the two packages.
const (
	FormatDir    = "dir"
	FormatRule   = "rule"
	FormatMarker = "marker"
)

// ProfileSkill is the canonical skill name substituted into every
// per-agent install path. Single source of truth; changing this is a
// major bump per AGENTS.md §10.
const ProfileSkill = "sin-code"

// DefaultSourcePath is the in-repo location of the source markdown.
const DefaultSourcePath = "docs/agent-profiles/sin-profile.md"

// Target is one supported host-agent family.
//
//	Name         — short id used on the CLI: `claude-code`, `cursor`,
//	               `copilot`, …
//	DisplayName  — human label used in `profile list` / `profile render`
//	               table output and `--json` payloads.
//	InstallPath  — path template relative to the **repository** root
//	               (cwd). Contains a `<skill>` placeholder that is
//	               replaced at write time with ProfileSkill. Mutli-skill
//	               files (copilot) omit the placeholder.
//	Format       — one of FormatDir / FormatRule / FormatMarker.
//
// # Stability
//
// (Name, DisplayName) is the public API surface exposed via
// `sin-code profile render <name>`. Adding a target is non-breaking;
// renaming or removing one is a major bump per AGENTS.md §10.
type Target struct {
	Name        string
	DisplayName string
	InstallPath string
	Format      string
}

// Targets is the single source of truth for supported host agents.
// Any addition here MUST also be reflected in:
//
//	cmd/sin-code/profile_cmd.go (cobra shell completion for
//	                             `profile <subcmd>`),
//	AGENTS.md §10 (naming-and-stability matrix),
//	CHANGELOG.md [Unreleased] (additions bullet).
//
// The set is intentionally twice the size of the original AGENTS.md
// §10 skilldist table because Claude Code / Codex / Gemini / opencode
// also preserve their own project-level profile conventions:
// claude-code reads CLAUDE.md / skills/, codex reads AGENTS.md, etc.
// We write into the convention those agents expect so an opening
// maintainer session sees the rules without forcing them to re-read
// `$HOME/.claude/skills/sin-code/SKILL.md`.
var Targets = map[string]Target{
	"claude-code": {
		Name:        "claude-code",
		DisplayName: "Claude Code",
		InstallPath: ".claude/skills/sin-code/SKILL.md",
		Format:      FormatDir,
	},
	"opencode": {
		Name:        "opencode",
		DisplayName: "opencode",
		InstallPath: ".config/opencode/skills/sin-code/SKILL.md",
		Format:      FormatDir,
	},
	"gemini": {
		Name:        "gemini",
		DisplayName: "Gemini CLI",
		InstallPath: ".gemini/skills/sin-code/SKILL.md",
		Format:      FormatDir,
	},
	"codex": {
		Name:        "codex",
		DisplayName: "Codex CLI",
		InstallPath: ".codex/rules/sin-code.md",
		Format:      FormatRule,
	},
	"cursor": {
		Name:        "cursor",
		DisplayName: "Cursor",
		InstallPath: ".cursor/rules/sin-code.mdc",
		Format:      FormatRule,
	},
	"windsurf": {
		Name:        "windsurf",
		DisplayName: "Windsurf",
		InstallPath: ".windsurf/rules/sin-code.md",
		Format:      FormatRule,
	},
	"cline": {
		Name:        "cline",
		DisplayName: "Cline",
		InstallPath: ".clinerules/sin-code.md",
		Format:      FormatRule,
	},
	"copilot": {
		Name:        "copilot",
		DisplayName: "GitHub Copilot",
		InstallPath: ".github/copilot-instructions.md",
		Format:      FormatMarker,
	},
}

// TargetNames returns every registered id in deterministic (alphabetical)
// order. A stable order is required so `profile render all` and `profile
// verify` produce identical log output across machines.
//
// We use the standard library sort.Strings rather than a hand-rolled
// insertion sort: Go's map iteration order is non-deterministic, and a
// hand-rolled sort that depends on the input order produces different
// output across runs — which would break the byte-stable contract.
func TargetNames() []string {
	out := make([]string, 0, len(Targets))
	for k := range Targets {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MustTarget returns the Target with the given Name; it panics if the
// target is unknown. Used by tests + CLI shell-completion paths where
// an unknown id is a programmer error.
func MustTarget(name string) Target {
	t, ok := Targets[name]
	if !ok {
		panic(fmt.Sprintf("profile: unknown target %q (registered: %v)", name, TargetNames()))
	}
	return t
}
