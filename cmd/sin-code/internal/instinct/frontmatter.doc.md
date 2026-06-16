# instinct/frontmatter.go — Markdown I/O

On-disk format. Two sections: YAML frontmatter (machine) and a Markdown
body (human / agent).

## File layout

```markdown
---
id: slug-or-hash
trigger: "when committing"
confidence: 0.7
domain: git
scope: project
project_id: abc123
...
---

# When committing

## Action

Run the test suite first.

## Evidence

- observed in 4 sessions
- reinforced by post-commit hook
```

## Why a Markdown file (and not a SQLite row)

- inspectable in any editor
- diffable in git
- migratable by `cp -r` (and by `sin instinct export`)
- same shape the agent can `Read()` and reason about at runtime

## Why YAML frontmatter (and not JSON or TOML)

YAML is what the `affaan-m/ecc` ecosystem already speaks, and what
operators expect to hand-edit. JSON would force quoting hell on natural
language; TOML has weaker comment support.

## Related files

- `types.go` — `Instinct` struct field-by-field definition
- `store.go` — directory layout and atomic writes
- `cli.go` — `export` / `import` re-use Marshal + Unmarshal
