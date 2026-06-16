# assets/importer.go — ECC skill harvest

`ImportSkills(opts)` reads a cloned source repo (typically
`./vendor/ecc`), loads every `SKILL.md` from the canonical locations
(`.agents/skills/`, `.kiro/skills/`, `skills/`), validates them,
applies the domain/exclude filters, stamps `origin`/`license`
attribution, and writes the survivors to a destination dir.

## What it does NOT do

- It does **not** vendor copyrighted prompt bodies silently. The
  caller is expected to set `--license` from the source `LICENSE`
  file. `Origin` defaults to `"ECC"`.
- It does **not** de-duplicate by content — only by name. Two skills
  with the same name and different bodies will be merged by name
  (first wins).
- It does **not** run a hook or update agent config. Wire the
  destination dir into `LoadStandardLayout` (in `loader.go`) or
  call the loader directly.

## Default exclusions

A short list of ECC content/business skills that are not useful for
a coding agent (`article-writing`, `brand-voice`, `investor-materials`
…). Override with `--exclude` if you want everything.

## Related files

- `loader.go` — `LoadDir(dir, KindSkill)` is the underlying loader
- `validate.go` — `Validate` is the schema gate
- `cli.go` — wires `ImportSkills` into `assets import`
