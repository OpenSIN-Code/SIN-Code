# install-skill.sh

## What it does

Idempotent installer for the `ceo-audit` skill. Links the source directory
into the opencode runtime path (`~/.config/opencode/skills/ceo-audit`),
normalizes script permissions, verifies SIN-Code + Python dependencies,
and runs a smoke test.

## When to use

- After cloning the skill source into a new location and wanting opencode to use it
- After upgrading the skill to refresh the runtime link
- As a smoke test before relying on the audit in CI

## Flags

| Flag | Purpose |
|------|---------|
| `--force` | Re-link even if destination is a regular file/dir/symlink to elsewhere |
| `--dry-run` | Print the plan, change nothing |
| `--source=DIR` | Use `DIR` as the skill source (default: this skill's root) |
| `--skip-smoke` | Skip the final `audit.sh --help` smoke test |

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Install (or already-installed) succeeded |
| 1 | Core SIN-Code tool missing on PATH |
| 2 | Filesystem error (link failed, permission denied) |
| 3 | Smoke test failed (`audit.sh --help` did not produce output) |

## Why the source-equals-destination case is "OK"

When the skill is **already** located at `~/.config/opencode/skills/ceo-audit`
(the normal case on a dev machine), source and destination resolve to the
same canonical path. The script detects this and treats it as a no-op
success — no symlink dance, no removal. This is intentional: it means
`bash scripts/install-skill.sh` is safe to run on every CI checkout.

## See also

- `validate-install.sh` — read-only verification (no changes)
- `audit.sh` — the main entry point
- `SKILL.md` — top-level documentation
