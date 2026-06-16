# assets/loader.go — disk walk

`LoadDir(root, kind)` recursively loads every `*.md` file under
`root` and parses it as the given kind.

## Skill convention

Skills are organized as `<root>/<skill-name>/SKILL.md` (ECC and
Kiro convention). The loader accepts only the filename `SKILL.md`
when `kind == KindSkill`; other `.md` files in the same directory
are ignored so a skill's `references.md` or `examples.md` doesn't
get loaded as its own skill.

## Name fallback

If the frontmatter omits `name`, the loader defaults to the file
stem (or the parent dir name for skills). This means a hand-edited
file with no name still gets a stable identity.

## `LoadStandardLayout`

Tries the four common layout roots (`agents/`, `commands/`,
`.agents/skills/`, `skills/`) in order, tolerating missing dirs.
This is the entry point the CLI uses; the importer uses `LoadDir`
directly because it knows the exact source layout.

## Related files

- `asset.go` — the parsed shape
- `validate.go` — called by the CLI after loading
- `importer.go` — uses `LoadDir` with `KindSkill`
