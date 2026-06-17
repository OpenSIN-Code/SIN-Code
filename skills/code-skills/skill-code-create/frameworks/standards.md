# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- SIN-Code skill standard (v3.20.0).
- `scripts/validate_skill.py` for validation (recurses one level deep under `skills/` for `--all-bundled`).
- Optional `scripts/create_skill.py` for scaffolding.
- `skills/flatfs.go` flattens category directories for `skillsmith` embedding.

## Skill Standard

- `SKILL.md` with YAML frontmatter (`name`, `description`, `license`, `compatibility`, `metadata`).
- Required directories: `context/`, `frameworks/`, `tasks/`, `templates/`.
- Recommended directories: `scripts/`, `tests/`, `lib/`.
- `compatibility` must be a YAML list.
- `metadata` should include `author`, `version`, and for external skills `lifecycle: external` plus `sources:`.

## Bundled Skill Layout

```
skills/<category>-skills/skill-<category>-<name>/
├── SKILL.md
├── LICENSE
├── context/
│   └── triggers.md
├── frameworks/
│   └── standards.md
├── tasks/
│   └── workflow.md
└── templates/
    ├── output.md
    └── prompt.md
```

## Constraints

- Bundled skill name and directory must match `skill-<category>-<name>`.
- Skill must live in the correct `skills/<category>-skills/` directory.
- No copyrighted material without license.
- Keep templates actionable.
- External/port skills must include `lifecycle: external` and `sources:` in `SKILL.md` metadata.
- Bundled skill changes require updating `README.md`, `AGENTS.md`, `CHANGELOG.md`, and `ECOSYSTEM.md` in the same PR.

## Quality Gates

- Strict validator passes.
- Frontmatter valid.
- All required directories populated.
- `LICENSE` file present.
- `go build ./...` and `go test ./... -race -count=1` pass.
- Docs updated if bundled.
