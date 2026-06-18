# Standards

## Skill Standard

Every SIN-Code / OpenCode skill must contain:

- `SKILL.md` with YAML frontmatter.
- `context/` with at least one `.md` file.
- `frameworks/` with at least one `.md` file.
- `tasks/` with at least one `.md` file.
- `templates/` with at least one `.md` file.
- `LICENSE` file.

## Frontmatter

Required keys: `name`, `description`.
Strict keys: `lifecycle` (one of `native`, `external`, `deprecated`, `internal`).
Optional keys: `license`, `compatibility`, `metadata`, `required_tools`.

## Bundled Skill Naming

- Directory: `skills/<category>-skills/skill-<category>-<name>/`.
- Frontmatter `name:` must match the directory name.
- Categories: `browser`, `code`, `debug`, `design`, `ecosystem`, `github`, `infrastructure`, `memory`, `multimodal`, `planning`, `process`, `shop`.

## Documentation Updates

Bundled skills require updates to:

- `README.md` — list of bundled skills.
- `AGENTS.md` — architecture and naming rules.
- `CHANGELOG.md` — release note.
- `ECOSYSTEM.md` — ecosystem inventory if applicable.

## Validation

Always run `python3 scripts/validate_skill.py --all-bundled --strict` before committing.
