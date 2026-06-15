# skills/embed.go

Embeds every project-local agent skill directory into the `sin-code` binary.

## What this file does

`skills/embed.go` defines `SkillsFS`, an `embed.FS` that includes all files under
`skills/`. The `sin-code skills` subcommand (see `cmd/sin-code/skills_cmd.go`)
uses this filesystem to list and install bundled skills without cloning external
repositories.

## Dependencies

- `github.com/Songmu/skillsmith` — skill discovery, install, and status.
- `skills/` directory containing one subdirectory per skill.

## Important config values

- The `//go:embed *` directive embeds every file and directory under `skills/`.
- Each skill directory must contain a `SKILL.md` at its root to be discovered.

## Why this approach

Embedding keeps the skills version-locked to the binary release. Users can install
bundled skills offline and always get the version that shipped with their `sin-code`
build.

## Caveats

- Only files committed under `skills/` are embedded; `.claude/skills/` symlinks are
  not part of the binary.
- `skills/embed.go` itself must not import `cmd/sin-code` to avoid an import cycle.

## Usage

```go
import "github.com/OpenSIN-Code/SIN-Code/skills"

// list all bundled skills
entries, _ := skills.SkillsFS.ReadDir(".")
```
