# ci-precheck.sh — mirror the §13.2 checklist as an executable

The single entry point for everything AGENTS.md §13 says to run before
opening a PR. Don't copy-paste the checklist by hand.

## What it does

Runs each step of §13.2 in order, prints a labeled timing line, and
exits non-zero on the first failure so it composes with `&&` chains.
The actual step list:

1. `gofmt -l .` — fastest check, catches the most common lint failure
2. `go build ./cmd/sin-code` — exact go-ci step
3. `go vet ./cmd/sin-code/... ./cmd/sin-code/internal/...` — exact go-ci step
4. `python3 scripts/validate_skill.py --all-bundled --strict` — exact go-ci step
5. `go test ./cmd/sin-code/ ./cmd/sin-code/internal/ -count=1` — exact go-ci step
6. `golangci-lint run --timeout=5m ./...` — exact lint-and-security step
7. `govulncheck ./...` — exact lint-and-security step
8. `gosec -no-fail -fmt sarif -out /tmp/gosec.sarif ./...` — exact lint-and-security SARIF step
9. `ceo-audit.sh --profile QUICK --grade B` — local dry-run of the ceo-audit gate

With `--pr <N>`, additionally polls `gh pr checks <N>` and translates
the result into an exit code:

- exit 0: local steps green AND remote required checks green
- exit 1: a local step failed
- exit 2: a remote required check is red

## What it does NOT do

- **No `go test -race`.** Race coverage is a pre-commit-hook concern,
  not a CI concern. Including `-race` here would multiply runtime
  ~10x without changing what CI catches.
- **No `gosec` exit-code gating.** The bare `gosec` line in the PR
  view is the GitHub-Checks-API artifact (see AGENTS.md §13.5) and
  always shows "fail" when the SARIF upload has at least one
  warning. The script runs gosec with `-no-fail` so it doesn't
  pollute the local exit code with the same artifact.
- **No branch-protection mutation.** This script never touches
  GitHub settings. For that, see `docs/CI-RUNBOOK.md` §5.

## Exit code semantics

| Code | Meaning |
|---|---|
| 0 | all local steps green (and remote required checks green, if `--pr` was passed) |
| 1 | a local step failed; re-run with bash -x to find the failing line |
| 2 | a remote required check is red on `--pr <N>`; see `gh pr checks <N>` and CI-RUNBOOK §4 |

## Dependencies

The script **gracefully degrades** when a tool is missing — the
unavailable steps are reported as `skipped` rather than failing.
This is intentional: it means the script is useful even on a
machine that only has `go` installed.

Tools the script uses (all optional except `go` and `python3`):

| Tool | Where | Install |
|---|---|---|
| `go` | `$PATH` | system package manager |
| `python3` | `$PATH` | system package manager |
| `gofmt` | ships with Go | — |
| `golangci-lint` v2.5.0 | `$PATH` | `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0` |
| `govulncheck` | `$PATH` | `go install golang.org/x/vuln/cmd/govulncheck@latest` |
| `gosec` | `$PATH` | `go install github.com/securego/gosec/v2/cmd/gosec@latest` |
| `ceo-audit.sh` | `$PATH` | part of the SIN-Code bundle (delegated via n8n per AGENTS.md §M1) |
| `gh` | `$PATH` | only needed for `--pr <N>` |
| `pyyaml` | Python | `pip install pyyaml` (auto-installed by the script) |

## Why not `go test -race`

The CI workflow `go-ci/test` runs:

```
go test ./cmd/sin-code/ ./cmd/sin-code/internal/ -count=1 -v
```

Note: **no `-race`**. The reason is wall-clock budget: a `go test
./cmd/sin-code/...` with `-race` takes 5-10x longer (60-180s vs 6-30s
for the same suite without race) and the GitHub-Actions-hosted runner
is shared with other jobs. Race coverage is enforced by a
**local pre-commit hook** (see `scripts/dev_install.sh`) that runs
the race tests before `git commit` is allowed. This is the only
place where `-race` is correct.

## Related

- AGENTS.md §13.1 — the list of blocking CI checks
- AGENTS.md §13.2 — the pre-PR checklist (this script mirrors it)
- AGENTS.md §13.5 — why the bare `gosec` line is not in the required list
- docs/CI-RUNBOOK.md — the operating manual behind this script
