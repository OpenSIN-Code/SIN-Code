# validate-install.sh

## What it does

Read-only verification that the ceo-audit skill is **correctly installed**
and all dependencies are present. Prints a green/red status line per
check and exits 0 only when every check passes.

## When to use

- As a CI pre-flight step before invoking `audit.sh` (catch missing
  tools early with a clearer error than the audit's own)
- After upgrading the skill or moving it to a new machine
- As the first thing to run when the audit mysteriously fails
  ("maybe my venv is broken?")

## Five checks

| # | Check | What it verifies |
|---|-------|------------------|
| 1 | Script permissions | All 12 `.sh` files under `scripts/` are executable |
| 2 | Python dependencies | `jinja2`, `pyyaml` importable; `pytest`/`cryptography`/`requests` optional |
| 3 | SIN-Code toolchain | All 7 core tools on PATH: discover, map, grasp, scout, execute, harvest, orchestrate |
| 4 | `audit.sh --help` | Main entry point runs and produces a help banner |
| 5 | Required files | `SKILL.md`, `README.md`, `CHANGELOG.md`, all templates, all `lib/`, all `scripts/*.py`, `tests/` |

Plus a bonus: **pytest discovery** — confirms the test files are
importable. Skipped if pytest is not installed.

## Flags

| Flag | Effect |
|------|--------|
| `--quiet` / `-q` | Silent on success, only print failures. Useful in CI |

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | All checks passed |
| 1 | One or more checks failed (printed with `[FAIL]` prefix) |

## See also

- `install-skill.sh` — fixes the most common failures (creates symlink, chmods scripts, runs smoke test)
- `audit.sh` — main entry
- `benchmark.sh` — performance regression detection
- `SKILL.md` — top-level documentation
