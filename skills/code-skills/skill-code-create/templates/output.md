# Template: Output Format

Docs: ../SKILL.md

## Skill Creation Report

```markdown
# Created Skill: {name}

## Category & Location
- Category: {category}
- Directory: `skills/{category}-skills/{name}/`

## Files
- SKILL.md
- LICENSE
- context/triggers.md
- frameworks/standards.md
- tasks/workflow.md
- templates/output.md
- templates/prompt.md

## Naming
- Bundled name: `skill-{category}-{descriptive-name}`
- Frontmatter `name:` matches directory name.

## Validation
- `python3 scripts/validate_skill.py --all-bundled --strict`: pass
- `go build ./...`: pass
- `go test ./... -race -count=1`: pass

## Documentation Updates
- [ ] README.md: add skill to bundled skills list.
- [ ] AGENTS.md: update category table and verification date.
- [ ] CHANGELOG.md: add Unreleased entry.
- [ ] ECOSYSTEM.md: add skill to ecosystem map if applicable.

## Next Steps
- [ ] Add optional scripts/tests/lib.
- [ ] Create `.claude/skills/{name}` symlink if local discovery is needed.
- [ ] Rebuild `sin-code` binary to embed the new skill.
```
