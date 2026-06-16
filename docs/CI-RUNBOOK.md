# CI Runbook — operating manual behind AGENTS.md §13

> **Audience:** any agent (human or AI) who needs to debug or extend
> the SIN-Code CI gate. AGENTS.md §13 is the *spec* (what the gate
> is, what blocks merge); this runbook is the *operating manual*
> (why each step is the way it is, what to do when something breaks,
> how to evolve the gate as the codebase grows).
>
> **Last verified:** 2026-06-16, against PR #144
> (`feat/learning-subsystem`).

## 1. Workflow inventory

The four workflows that gate merges to `main`, in the order GitHub
runs them:

| Workflow | File | Trigger | Blocking? |
|---|---|---|---|
| `lint-and-security` | `.github/workflows/lint.yml` | PR, push to main | yes (3 of its jobs) |
| `go-ci` | `.github/workflows/go-ci.yml` | PR, push to main | yes (1 job) |
| `ceo-audit` | `.github/workflows/ceo-audit.yml` | PR, push to main | yes (1 job, 47-gate audit) |
| `ci` (Python) | `.github/workflows/ci.yml` | PR, push to main | conditional (only if `**.py` etc. changed) |

The "blocking" column is the source of truth for which checks must
be green to merge. It is **not** the same as the `gh pr checks`
output, which also lists informational / auto-managed jobs (Vercel
previews, SARIF uploads that don't gate, etc.). See AGENTS.md §13.1
for the full map.

## 2. Workflow anatomy

### 2.1 `lint-and-security`

```yaml
name: lint-and-security
on:
  pull_request:          # every PR
  push:
    branches: [main]    # every push to main
jobs:
  golangci:    golangci-lint v2.5.0, --timeout=5m, default config
  govulncheck: official Go vuln scanner
  gosec-sarif: gosec -no-fail -fmt sarif, uploaded to Code Scanning
```

What each step is for, what to change when the toolchain moves:

- **golangci-lint v2.5.0** — this is the floor. Bumping to v2.6.0+ is
  a breaking-change PR in itself: pre-existing warnings may flip,
  the `--timeout` default changes, and some linters get new defaults.
  Always do a dry run on a throwaway branch first: `golangci-lint run
  --timeout=5m ./...` and count the new findings. If >10 new issues,
  file a follow-up cleanup PR before bumping.

- **govulncheck** — `go install golang.org/x/vuln/cmd/govulncheck@latest`
  means **always latest**, not pinned. When Go releases a security
  advisory, govulncheck picks it up within hours. This is intentional
  (we want the gate to fire on new vulns the moment they land in
  the database). If a new vuln fires on `main`, the workflow fails
  on the next push. The fix: bump the dependency that introduced
  the vuln, or add a `//nolint:govet` annotation if it's a
  false-positive — but **never** silence govulncheck with a workflow
  filter, that's how breaches happen.

- **gosec-sarif** — runs with `-no-fail` so SARIF always generates
  even on findings. The actual fail signal comes from
  `github/codeql-action/upload-sarif` which treats at least one
  warning-level finding as non-success. See §3.3 below for how to
  read the SARIF when this fails.

### 2.2 `go-ci`

```yaml
name: go-ci
on:
  pull_request:
  push:
    branches: [main]
jobs:
  test:
    steps:
      - run: go build ./cmd/sin-code
      - run: go vet ./cmd/sin-code/... ./cmd/sin-code/internal/...
      - run: pip install pyyaml
      - run: python3 scripts/validate_skill.py --all-bundled --strict
      - run: go test ./cmd/sin-code/ ./cmd/sin-code/internal/ -count=1 -v
      - run: go test -tags=lsp_live ...   # opt-in, only if env var set
      - run: go test -race ./cmd/sin-code/internal/...   # opt-in
```

The four `run:` steps map 1:1 to AGENTS.md §13.2 checklist items
2-5. Order matters: `go build` catches the obvious misses first;
`go vet` catches the structural ones; `validate_skill.py` catches
the skill-schema regressions; `go test` is the long-pole. Never
reorder — reordering wastes a long timeout on a build that would
have failed in 1s.

The two `opt-in` tests (`-tags=lsp_live`, `-race` on the
`internal/...` tree) are skipped by default because they multiply
the runtime 5-10x. They run only when an env var is set in the
workflow dispatch. Do not enable them in the default run.

### 2.3 `ceo-audit`

Runs the 47-gate audit (posture, security, dependencies, tests,
docs, compliance, performance, supply-chain). Posts a PR comment
with the grade (A/B/C) and top 3 risks. Fails if grade < B.

- **Local dry-run** (for iterating without waiting for CI):

  ```bash
  ceo-audit.sh --profile QUICK --grade B
  ```

- **Full audit** (post-release, before tagging):

  ```bash
  ceo-audit.sh --profile FULL --grade A
  ```

- **n8n delegation** is the default (see AGENTS.md §M1) — the
  workflow calls the n8n webhook, which runs the audit on the
  OCI free-tier VM. The local dry-run above is for fast iteration
  only; release-blocking audits always go through n8n.

### 2.4 `ci` (Python)

Path-filtered. Only triggers on changes to `**.py`, `pyproject.toml`,
`setup.py`, `setup.cfg`, `requirements*.txt`. For a Go-only PR, this
workflow will not run at all and the "lint & format (ruff)" /
"test (py${{ matrix.python-version }})" jobs in the PR view are
absent. **Do not add Python lint fixes to a Go PR** to make CI
pass — the workflow simply isn't there for that diff.

## 3. Locally-reproduced failure patterns

The four error classes that have actually blocked PRs in the
last 6 months, with the exact fix.

### 3.1 `golangci-lint: File is not properly formatted (gofmt)`

**Symptom:** `golangci-lint` reports a `gofmt` finding, e.g.:

```
cmd/sin-code/internal/assets/asset.go:30:1: File is not properly formatted (gofmt)
        Model        string   `yaml:"model,omitempty"`          // agents
        ^
```

**Cause:** new code or a rebase introduced unaligned struct tags,
unaligned const blocks, or unaligned import groups. `gofmt` would
have caught it on commit; `golangci-lint` catches it on PR.

**Fix:**

```bash
# 1. Confirm the failure
gofmt -l cmd/sin-code/ 2>/dev/null
# 2. Apply gofmt
gofmt -w cmd/sin-code/<path>
# 3. Verify
gofmt -l cmd/sin-code/ 2>/dev/null   # should be empty
go build ./cmd/sin-code
```

This is the **#1** CI failure mode. It is preventable by running
`gofmt -w .` before every commit, or by enabling a pre-commit hook
that does so. `scripts/dev_install.sh` installs such a hook.

### 3.2 `golangci-lint: undefined: fooBar`

**Symptom:** `golangci-lint` reports a `typecheck` or `unused`
finding, e.g.:

```
cmd/sin-code/internal/instinct/manager.go:42:13: undefined: NewInstinctWithConfig
```

**Cause:** the symbol was renamed or moved in a refactor commit,
but a sibling file (e.g. a test, a doc reference, a string
constant) still points at the old name.

**Fix:** `git grep NewInstinctWithConfig` to find all references,
then update them. The fix is almost always mechanical but requires
human judgment about whether the *old* name was the typo or the
*new* name is.

### 3.3 `gosec (SARIF upload): failure` — but `gosec` plain is "pass"

**Symptom:** The PR shows two `gosec` entries:

1. `gosec` plain — 3s, "pass"
2. `gosec (SARIF upload)` — 1-2min, "failure"

**Cause:** `gosec -no-fail` always exits 0 so the SARIF file is
always written, but the upload step treats the SARIF's
"warning"-level findings as non-success. The plain `gosec` line
in the PR view is the GitHub-Checks-API artifact that always
shows "pass" for this workflow. See AGENTS.md §13.5 for the full
explanation.

**Fix:**

1. Download the SARIF artifact: open the failed `gosec (SARIF upload)`
   job → "Artifacts" → download `gosec.sarif` (or `gosec`).
2. Open in a SARIF viewer (e.g. the VS Code `SARIF Viewer`
   extension, or upload to https://sarifweb.azurewebsites.net/).
3. Each finding has a `ruleId` (e.g. `G404`, `G105`) — look up the
   rule in gosec's rule catalog.
4. Common findings and their fixes:
   - `G101` (hardcoded credentials) — move to env var or Infisical
   - `G102` (bind to all interfaces) — bind to `127.0.0.1` if local
   - `G104` (unchecked error) — add `if err != nil` or `_ =`
   - `G107` (URL via http://) — use `https://` or accept as `nolint`
   - `G304` (file path via variable) — `gosec` is wrong here, add
     `// #nosec G304 -- path is from trusted config`
5. To suppress, add `// #nosec GXXX -- justification` immediately
   above the line. **Never** suppress globally in `.gosec.json` —
   that defeats the scanner.

### 3.4 `go test: timeout`

**Symptom:** `go test` job times out at 6 minutes (the default
`jobs.test.timeout-minutes`).

**Cause:** usually a single test that does a blocking syscall
(reading from a hung network conn, waiting on a goroutine that
never returns). Sometimes a legitimate slow test that grew
without anyone noticing.

**Fix:**

1. Get the verbose output: click the failed `go test` job →
   "Annotations" or "Raw log". The output starts with
   `=== RUN TestXxx`.
2. Identify the hung test. The last `=== RUN` before the timeout
   is the culprit.
3. Reproduce locally with `-run TestXxx` and a long timeout:
   `go test -run TestXxx -timeout 5m -v ./cmd/sin-code/...`
4. Add `context.Context` propagation, or a `t.Cleanup()` to abort
   the test on `t.Done()`. If the test legitimately needs >6min,
   split it into smaller sub-tests.

### 3.5 `ceo-audit: grade < B`

**Symptom:** audit posts a comment with the grade and the top 3
risks. Grade is below the configured threshold (default B).

**Cause:** one of the 47 gates is failing. The comment shows which
ones.

**Fix:** iterate locally with `ceo-audit.sh --profile QUICK --grade B`.
The audit's output is JSON (in `ceo-audit-output/score.json`); parse
it to see which gates failed. Common culprits: missing test coverage
on a new file, missing docstring on an exported symbol, a
dependency that the license-compliance gate flags.

## 4. Recovery

### 4.1 A check is stuck `pending` for >10 min

**Cause:** GitHub-Actions runner pool exhausted (rare but happens
during peak hours), or the workflow's own setup failed silently.

**Fix:**

```bash
gh run list --workflow=<workflow-name> --limit 5
gh run view <run-id> --jobs   # see if a specific job is the bottleneck
# Cancel the stuck run
gh run cancel <run-id>
# Re-run the workflow on the same commit
gh workflow run <workflow-name> --ref <branch>
```

The PR will re-trigger the workflow on the next push. If the runner
pool is exhausted, push any small commit (e.g. a fixup) to
re-trigger.

### 4.2 A test is flaky

**Cause:** timing-sensitive test, network call, shared global state.

**Fix:** in the test file, mark the test with `t.Skip("flaky; see
#123")` and file a follow-up issue. **Do not** add `time.Sleep` or
`for i := 0; i < 100; i++` retries — those are how flaky tests
become permanently broken tests. The pattern is: skip, file
issue, fix root cause, un-skip.

### 4.3 The runner OOM-killed a test

**Symptom:** `go test` job exits with no useful output, but the
runner log shows "Killed" or "Out of memory".

**Fix:** the test is loading too much into memory (probably a
large fixture or a global cache). Two options:
- Split the test into smaller sub-tests with `t.Run()`
- Move the heavy setup to a `TestMain` that pre-loads once

In either case, the fix is in the test, not the workflow.

## 5. Branch-protection evolution

The `required_status_checks` list in `.github/settings/branch-protection.json`
(or in the GitHub UI under Settings → Branches → Branch protection
rules → `main`) currently is:

```
lint-and-security/golangci-lint
lint-and-security/govulncheck
go-ci/test
ceo-audit/ceo-audit
```

When you add a new workflow (e.g. `performance-bench.yml` with a
`bench` job), the steps to make it blocking are:

1. Add the workflow under `.github/workflows/`.
2. Make sure it has a stable job name (`name: bench` in the
   workflow YAML — this is what GitHub shows in the checks list).
3. Push a test commit so GitHub registers the check.
4. In the branch-protection settings, add the new check to the
   "Require status checks to pass before merging" list.
5. Update AGENTS.md §13.1 to list the new check.
6. Update this runbook to add a new section.

Steps 1-3 can be a single PR. Steps 4-6 are a follow-up
admin PR. **Never** add a check to the required list without
having a passing run on `main` first — otherwise every open PR
is suddenly red.

## 6. Tooling

| Tool | Purpose | Where |
|---|---|---|
| `scripts/ci-precheck.sh` | Mirror of §13.2 as an executable | this repo |
| `scripts/dev_install.sh` | Install pre-commit hooks including `gofmt -w` | this repo |
| `scripts/adr-ci.sh` | ADR CI check (generates ADRs, fails on critical issues) | this repo |
| `ceo-audit.sh` | Local dry-run of the 47-gate audit | from the SIN-Code bundle |
| `gh` CLI | PR status, run cancel, workflow re-run | system PATH |

## 7. Related

- AGENTS.md §13 — the spec
- AGENTS.md §13.5 — the `gosec` mystery
- AGENTS.md §13.6 — `scripts/ci-precheck.sh` usage
- `scripts/ci-precheck.sh` — the executable
- `scripts/ci-precheck.sh.doc.md` — the script's own CoDoc
