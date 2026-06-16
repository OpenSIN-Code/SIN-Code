# evalharness/arms.go — built-in Arm constructors

Four pinned arm IDs (`__baseline__`, `__terse__`, `__lazy_skill__`,
`__user_skill__`) plus the user-supplied skill path. Every
non-baseline arm gets the canonical `Answer concisely.` prefix
(exported as `TersePrefix`) before its specific body, so the
delta `<arm> - terse` isolates the skill's own contribution.

## Why a verbosity arm

Issue #167 plans a verbosity mode (`default` / `verbose` /
`normal` / `terse` / `ultra`). For now, `VerbosityArm("", nil)`
returns `Arm{ID: "default"}` with no extra rendering; a future PR
will wire `internal/style.RenderSystemBlock(level)` into the
reader. The constructor signature is stable so the comparator
gets dimming for free.

## Skill discovery

`ReadBundledSkillBody(name)` walks every reasonable on-disk
location:

- `$SIN_SKILLS_DIR` (env, takes precedence)
- `<cwd>/skills`
- `~/.local/share/sin-code/skills`

If nothing matches, it returns `("", nil)` so `SkillArm` renders
the deterministic `[skill unavailable]` placeholder. Snapshots
stay byte-stable even when the skill file is removed.

## Round-trip

`SkillArm` and `LazySkillArm` use the same `renderSkillPrompt`
helper. Every prefix is identical across runs, so a snapshot
re-emitted tomorrow is byte-equal to today's.

## Related files

- `comparator.go` — the runner that consumes these arms.
- `snapshot.go` — produces per-arm rows once `Compare` returns.
- `types.go` — `Arm` struct.
