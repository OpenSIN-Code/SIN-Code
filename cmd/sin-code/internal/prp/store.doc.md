# prp/store.go — on-disk layout

PRPs live at `<workdir>/.sin/prp/<id>.md`. In-repo (not in
`~/.local/share`) so the plan is:

- visible in `git log`
- reviewable in a PR
- diffable across branches
- not silently lost when a developer switches machines

The `.sin/` prefix matches `sin instinct evolve --apply`'s output
dir convention — operators only have to remember one dotfile tree.

## Atomic writes

Every save uses `tmp` + `os.Rename`. A concurrent reader sees
either the old or the new file, never a half-written one.

## Missing dir

`Load` on a non-existent PRP returns an error (`os.IsNotExist`).
`List` on a missing dir returns `(nil, nil)` — an empty PRP set
is not an error condition.
