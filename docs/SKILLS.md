# Skill Lifecycle Markers (issue #139)

SIN-Code ecosystem skills have a **lifecycle** field in their
SKILL.md frontmatter. The lifecycle tells the operator and the
agent loop whether a skill is implemented natively in the
`sin-code` binary, externally in a separate repo, or deprecated.

## Values

| Value | Meaning |
|---|---|
| `native` | Implemented in the `sin-code` binary (a subcommand or a Go-native package). The bundled SKILL.md is documentation, not the implementation. |
| `external` | Implemented in a separate repo (Python MCP server or Go bundle). The bundled SKILL.md is a discovery copy. |
| `deprecated` | The upstream is archived. The bundled SKILL.md exists only so old configs don't break. |

## Where the source of truth lives

`scripts/lifecycle_map.yaml` is the single source of truth for the
lifecycle of every bundled skill. The `sync_lifecycle.py` script
reads this file and applies the lifecycle field to the SKILL.md
frontmatter.

## Workflow

```bash
# 1. Edit scripts/lifecycle_map.yaml to add a new skill or change
#    a lifecycle.
# 2. Apply the change to the SKILL.md frontmatter:
python3 scripts/sync_lifecycle.py --apply
# 3. Verify nothing is out of sync:
python3 scripts/sync_lifecycle.py --check
# 4. Validate the bundled skills in strict mode (the migration
#    is complete when --strict passes):
python3 scripts/validate_skill.py --all-bundled --strict
```

CI runs `sync_lifecycle.py --check` and `validate_skill.py --all-bundled --strict`
on every PR. A drift between the map and the SKILL.md is a hard
failure.

## CLI surface

`sin-code skill list` now prints a `[lifecycle]` column:

```
SKILL                             LIFECYCLE   claude-code   opencode
skill-process-goal                [native   ]  —            —
skill-process-grill                [external ]  —            —
skill-code-build                   [native   ]  —            —
...
```

`unknown` means the SKILL.md has no `lifecycle:` field — usually a
legacy skill that has not been migrated. Run
`scripts/sync_lifecycle.py --apply` to fix.

### Ecosystem skill install

```bash
sin-code skill install <name>      # clone/update one ecosystem MCP skill
sin-code skill install all         # install all non-deprecated ecosystem skills
sin-code skill status              # install + runnable state
sin-code skill doctor              # diagnose why skills are not runnable
```

`install all` skips deprecated skills (e.g. the shop skills) so a stale
upstream repo never breaks the batch. Deprecated skills can still be
installed explicitly by name. `doctor` checks every known ecosystem skill
and reports `not installed` (or the concrete verification failure) for any
skill that is not runnable.

## Acceptance criteria (from #139)

- [x] `lifecycle` field in every bundled SKILL.md (34/34 after migration)
- [x] `validate_skill.py --all-bundled --strict` passes
- [x] `sin-code skill list` shows lifecycle markers
- [x] `scripts/lifecycle_map.yaml` is the single source of truth
- [x] `scripts/sync_lifecycle.py` (--check / --apply / --diff)

## Why a separate script (not a go binary)

`scripts/sync_lifecycle.py` is stdlib-only and runs without a Go
toolchain. The skill migration is a one-time per-skill event, so
the cost of a Go binary is not justified. CI uses the Python script
in --check mode on every PR.
