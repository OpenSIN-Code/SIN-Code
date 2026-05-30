# Plan: Operational Hardening

Status: implemented (Bundle)
Owner: unassigned
Scope: all 7 SIN-Code repositories (SCKG, IBD, POC, EFSM, ADW, Verification-Oracle, Bundle)

## Motivation

The stack is functionally complete and every repo has a green local `pytest`
run, but it is not yet operationally production-ready. There is no automated
verification on push, no release process, and no guard against the repos
drifting out of sync. This plan covers the concrete, near-term engineering work
needed to make the stack trustworthy to install and contribute to.

This plan deliberately contains **no new features** — only CI, release,
packaging, and consistency work.

## Workstreams

### WS1 — Continuous Integration (per repo)
Add a GitHub Actions workflow (`.github/workflows/ci.yml`) to every repo that:
- runs on `push` and `pull_request`
- sets up a matrix of Python 3.11 / 3.12 / 3.13
- installs the package with `pip install -e .`
- runs `pytest -q`
- (where the repo declares optional extras) installs them and re-runs

Acceptance:
- A green check is required on every PR before merge.
- A failing test blocks the merge.

### WS2 — Lint & format gate
Adopt `ruff` (lint + format) across all repos with a single shared config.
- Add `ruff` to a `[project.optional-dependencies] dev` group.
- Add a `lint` job to CI: `ruff check .` and `ruff format --check .`.
- Fix existing violations in one mechanical commit per repo.

Acceptance:
- `ruff check .` is clean on every repo.

### WS3 — Release & packaging
- Add a `release.yml` workflow triggered on tag `v*` that builds an sdist +
  wheel (`python -m build`) and attaches them to a GitHub Release.
- Verify each `pyproject.toml` has correct metadata: `license`, `authors`,
  `readme`, `classifiers`, `urls` (Homepage/Repository/Issues).
- Decide and document a versioning policy (SemVer, already noted in CHANGELOGs).

Acceptance:
- Pushing a tag produces a downloadable wheel + sdist per repo.
- `pip install <wheel>` works in a clean environment.

### WS4 — Cross-repo consistency check
The Bundle depends on the 6 subsystems via local path installs. Add a small
script (`scripts/check_consistency.py` in the Bundle) that asserts:
- every subsystem package version matches the Bundle's expectation
- every subsystem exposes the CLI entry point the Bundle's `status` command
  probes for
- the MCP tool names advertised by `sin mcp-config` match the tools actually
  registered in each `mcp_server.py`

Wire it into the Bundle's CI as a non-blocking (warning) job first, then
promote to blocking once green.

Acceptance:
- `python scripts/check_consistency.py` exits 0 against the current repos.

### WS5 — Editable-install developer bootstrap
Add a documented one-command dev setup (the multi-repo `pip install -e` loop
from the Bundle README) as `scripts/dev_install.sh`, plus a matching
`scripts/run_all_tests.sh` that iterates the repos and aggregates results.

Acceptance:
- A fresh clone of all repos can be set up and fully tested with two commands.

## Sequencing

WS1 and WS2 are independent and can land first (per repo, in parallel).
WS3 depends on WS1 being green. WS4 and WS5 live only in the Bundle and depend
on WS1.

## Out of scope

- Any new runtime feature or tool.
- The larger architectural items tracked in `docs/plans/sota-roadmap.md`.
