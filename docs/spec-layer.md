# SIN-Code Spec-Layer

A **Spec** is a single human-edited `*.spec.md` file that captures
the contract a change must satisfy. It is the bridge between
**intent** (what a human wants) and **verification** (what the
agent and CI can check).

> **Code:** `cmd/sin-code/internal/spec/` (~370 LOC, ~145 LOC tests).
> Native Go, zero new dependencies.
>
> **CLI:** `sin-code spec validate | show | check | author`.
>
> **Issue:** #122 (parent), #157 (this hardening pass).
> **Spec:** this file is the canonical reference.

## What the Spec-Layer is for

Three things, in priority order:

1. **The contract is reviewable.** A `*.spec.md` is a Markdown
   file in the repo. It appears in `git log`, in PRs, in code
   review. A human can read it in 30 seconds and decide whether
   the agent's interpretation matches their intent.
2. **The contract is checkable.** Each Acceptance Criterion has
   an optional `verify: <shell-command>` annotation. The drift
   checker runs those commands and reports pass/fail. A spec
   that doesn't pass its own criteria is a spec that's out of
   date.
3. **The contract is enforceable.** The CI gate fails the PR if
   `spec.drift` is set to `error` in `.sin-code.yml` and any
   criterion fails. Drift is no longer "we noticed in code
   review" — it's a build break.

## File format

```markdown
# Add rate limiting to API

> One-line objective that an LLM can understand in one screen.

## Objective

The current API has no rate limit; abuse risk is high. Add a
per-user rate limit of 100 requests per minute, returning HTTP
429 when exceeded. The limit must apply to all authenticated
endpoints, not just the public ones.

## Requirements

- [must] R1: Each user is limited to 100 requests per minute
- [must] R2: HTTP 429 is returned with `Retry-After` header when the limit is exceeded
- [should] R3: The limit is configurable via `RATE_LIMIT_PER_MIN` env var
- [may] R4: A metric `rate_limit_exceeded_total` is exposed per-user

## Acceptance Criteria

- A1: `go test ./internal/ratelimit/...` exits 0   `verify: go test ./internal/ratelimit/... -count=1`
- A2: A burst of 200 requests in 1s returns 429 for at least 100 of them   `verify: ./scripts/load-test-rate-limit.sh 200 1`
- A3: The `RATE_LIMIT_PER_MIN` env var is read at startup and logged   `verify: grep RATE_LIMIT_PER_MIN cmd/sin-code/main.go`

## Invariants

- DO-NOT-MODIFY: the existing `auth.Session` struct (used by 4 other packages)
- DO-NOT-MODIFY: the public `RateLimit(r *Request) error` API surface
```

### Sections

| Section | Required? | Used by |
|---|---|---|
| `# Title` | yes | `sin spec show` (heading) |
| `## Objective` | yes | Self-authoring Planner prompt |
| `## Requirements` | yes | Self-authoring Implementer scope |
| `## Acceptance Criteria` | yes | Drift checker (runs the `verify:` commands) |
| `## Invariants` | no | Pre-commit hook warns when the diff touches an invariant |

### Priority

Requirements support a `[must]` / `[should]` / `[may]` prefix (in any
case, with or without brackets, with `:` or space separator).
Default is `[must]`. The drift checker only **fails** on `must`
criteria; `should` and `may` are reported as warnings.

### `verify:` annotation

Each Acceptance Criterion can end with `` `verify: <command>` ``.
The command is the **authoritative** check; the human text is
documentation. Run:

```bash
$ sin spec check feature.spec.md
A1: pass  (go test ./internal/ratelimit/... -count=1)
A2: fail  (./scripts/load-test-rate-limit.sh 200 1 — exit 1, see output)
A3: skip  (no verify: command)
```

The check is **deterministic** — no LLM, no flaky heuristics.
The author of the spec writes the check; the spec is only as
good as its `verify:` commands.

## Drift detection (the hardening)

Issue #157 adds three new behaviors on top of the existing parser:

### 1. Spec ↔ code signature check

A spec requirement like:

```markdown
- [must] R1: `RateLimit(r *Request) error` is in the public API
```

is checked by:
1. Finding the function in the code (Go: AST, Python: `ast`)
2. Comparing the signature
3. Reporting drift if the signature changed

The check is opt-in per requirement: a requirement is checked
only if it starts with a backtick-wrapped signature, like
`` `RateLimit(r *Request) error` ``. Plain-text requirements
are not signature-checked.

### 2. Invariant-watcher

A `## Invariants` entry like:

```markdown
- DO-NOT-MODIFY: the existing `auth.Session` struct
```

is checked by:
1. The pre-commit hook runs `git diff` on the staged files
2. If the diff touches a file matching the invariant (e.g.
   `internal/auth/session.go`), the commit is blocked with a
   clear message

The matcher is fuzzy: any file path that contains the invariant's
identifier triggers. This is a warning system, not a security
boundary — the operator can override with `--no-verify`.

### 3. CI gate

The new `spec-ci` workflow runs on every PR and on every push to
`main`. The check is **deterministic** (no LLM in CI):

```yaml
# .github/workflows/spec-ci.yml (n8n-delegated per mandate M1)
- run: sin spec check --all
  if: always()
```

Failure mode: any `must` criterion that fails → red check →
blocks the PR. `should` and `may` failures → warning → PR can
merge, but the issue is recorded.

The strictness is controlled by `.sin-code.yml`:

```yaml
spec:
  drift: error       # error|warn|off  (default: warn)
  invariants: error  # error|warn|off
  coverage: 0.8     # minimum fraction of must-criteria with verify: command
```

## Self-authoring (the high-value part)

`sin spec author "Add rate limiting to API"` runs:

1. **Planner LLM call** → produces a `*.spec.md` (Objective +
   Requirements + Acceptance Criteria)
2. **Implementer LLM call** → writes the code
3. **Drift check** → `sin spec check` on the result
4. **Loop** → if drift, retry up to 3 times (the LLM is asked
   to fix the specific failing criterion)
5. **Branch + PR** → `gh pr create` with the spec + code

The output is a working PR with both the spec and the
implementation, both passing the spec's own criteria.

The mode requires:
- A model client (the same one used by the chat command)
- `gh` CLI on PATH (for the PR)
- A `.sin-code.yml` with `model.default` set

If the mode fails (e.g. the LLM produces specs that never pass),
the CLI returns the best-attempt spec to the operator for
manual editing.

## CLI

```bash
# Parse and validate a spec
sin spec validate feature.spec.md

# Show the parsed spec (human or JSON)
sin spec show feature.spec.md
sin spec show --json feature.spec.md

# Run the verify: commands on every criterion
sin spec check feature.spec.md          # one spec
sin spec check --all                    # every .spec.md in the repo
sin spec check --all --json             # machine-readable

# Self-author a spec + implementation
sin spec author "Add rate limiting to API" --out specs/rate-limit.spec.md
sin spec author "Add rate limiting to API" --apply  # also opens a PR
```

## Storage

```
.spec.md                  (in the repo, anywhere the operator wants)
.sin-code.yml            (per-project config, see issue #155)
$RUNNER_TMP/             (the `sin spec check` output, ephemeral)
```

`sin spec check --all` walks the repo (default: `git ls-files
*.spec.md`) and runs every criterion's `verify:` command in
sequence. Failures are aggregated into a single report.

## Failure modes

- **`verify:` command hangs** → bounded by `spec.check.timeout`
  in `.sin-code.yml` (default: 60s). Timeout = fail.
- **`verify:` command needs network** → the CI runner has network.
  Local pre-commit runs may not; the operator can skip with
  `git commit --no-verify` (M3: this is the same as the git
  `--no-verify` ban, but applied to the spec check).
- **Spec is malformed** → `sin spec validate` returns the parse
  error and a non-zero exit. The pre-commit hook refuses the
  commit.

## Mandates

- **M1 (n8n CI):** the `spec-ci` workflow is n8n-delegated
- **M2 (single binary):** zero new dependencies — `ast` and
  `go/parser` are stdlib
- **M3 (verify gate):** this is the verify-gate upgrade
- **M5 (module path):** new code in `cmd/sin-code/internal/spec/`
  and `cmd/sin-code/spec_cmd.go`

## Related

- Issue #122 (parent, the spec skeleton)
- Issue #157 (this hardening pass)
- Issue #155 (`.sin-code.yml` policy key — `spec.drift`)
- `docs/PRP-WORKFLOW.md` (PRPs use specs as their objective source)
- `cmd/sin-code/internal/spec/spec.go` (the existing parser)
- `cmd/sin-code/spec_cmd.go` (the existing CLI)
- `internal/verify/` (the engine that consumes the spec)
