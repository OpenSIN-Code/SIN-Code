# [loop-002] Auto-inject "tests must exist and pass" criterion — no human reminder needed

**Labels:** `loop-engineering` `autonomy` `p0`
**Branch:** `sincode-loop-system`
**Status:** DONE
**Affects:** `goalcontract/goalcontract.go`, `stopgate/stopgate.go`

---

## Problem

Currently `autoDetectChecks` adds `go build`, `go test ./...`, `go vet`, and
`no-new-todos`. But it does NOT verify that **new tests were written** for the
code the agent changed. An agent can pass `go test ./...` with zero new test
cases — because the existing tests already pass. The stop-gate confirms "tests
pass" but never "tests were written". A human then has to say "please write
tests". This is exactly the babysitting the loop system must eliminate.

---

## Root cause

```go
// internal/goalcontract/goalcontract.go — current autoDetectChecks
func autoDetectChecks(workspace string) []orchestrator.Check {
    var checks []orchestrator.Check
    if fileExists(filepath.Join(workspace, "go.mod")) {
        checks = append(checks, orchestrator.DefaultGoChecks()...)
        checks = append(checks, orchestrator.Check{
            Kind:    orchestrator.CheckPredicate,
            Name:    "no-new-todos",
            Cmd:     []string{"sh", "-c", noNewTodosScript},
            Timeout: 1 * time.Minute,
        })
    }
    return checks
    // MISSING: no check that new test files exist for changed packages
}
```

The `SemanticCriteria` slice is only used when an explicit `--criteria` flag is
passed. Auto-detection never injects a "tests written" semantic criterion.

---

## Proposed solution

### 1. Add a deterministic "new test coverage" check

A shell predicate that fails when the diff introduces new `.go` files (or
modifies existing ones in non-test packages) but adds zero new `_test.go`
files. It names the offending packages so the stop-gate re-injection can
point the model straight at them:

```go
// internal/goalcontract/goalcontract.go
// newTestCoverageScript counts untracked files too (git ls-files --others)
// so brand-new packages that haven't been staged yet are still caught.
// Outside a git repo it exits 0 — cannot judge, so never blocks.
const newTestCoverageScript = `set -e
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi

changed=$( { git diff --name-only HEAD 2>/dev/null
             git ls-files --others --exclude-standard 2>/dev/null; } | sort -u )
[ -z "$changed" ] && exit 0

non_test=$(echo "$changed" | grep '\.go$' | grep -v '_test\.go$' || true)
[ -z "$non_test" ] && exit 0   # only test files changed: fine

pkgs=$(echo "$non_test" | xargs -I{} dirname {} 2>/dev/null | sort -u || true)
missing=""
for pkg in $pkgs; do
  if [ -z "$(find "$pkg" -maxdepth 1 -name '*_test.go' 2>/dev/null)" ]; then
    missing="$missing $pkg"
  fi
done
if [ -n "$missing" ]; then
  echo "MISSING TESTS: the following changed packages have no _test.go file:"
  for p in $missing; do echo "  - $p"; done
  exit 1
fi

test_changed=$(echo "$changed" | grep '_test\.go$' || true)
if [ -z "$test_changed" ]; then
  echo "Non-test Go files were changed but no test file was added or modified."
  echo "Changed packages must have new or updated tests."
  exit 1
fi
exit 0`

func autoDetectChecks(workspace string, opt ResolveOptions) []orchestrator.Check {
    var checks []orchestrator.Check
    if fileExists(filepath.Join(workspace, "go.mod")) {
        checks = append(checks, orchestrator.DefaultGoChecks()...)
        checks = append(checks, orchestrator.Check{
            Kind:    orchestrator.CheckPredicate,
            Name:    "no-new-todos",
            Cmd:     []string{"sh", "-c", noNewTodosScript},
            Timeout: 1 * time.Minute,
        })
        if !opt.NoTestCriterion {
            checks = append(checks, orchestrator.Check{
                Kind:    orchestrator.CheckPredicate,
                Name:    "new-test-coverage",
                Cmd:     []string{"sh", "-c", newTestCoverageScript},
                Timeout: 1 * time.Minute,
            })
        }
    }
    return checks
}
```

### 2. Auto-inject a semantic criterion for test quality

Beyond the existence check, the LLM judge confirms depth. Added automatically
in `Resolve` when auto-detecting a Go workspace:

```go
// internal/goalcontract/goalcontract.go — Resolve, after autoDetectChecks
if isGo && !opts.NoTestCriterion {
    c.SemanticCriteria = appendUnique(c.SemanticCriteria,
        "New or updated _test.go files exist for every changed package, "+
            "cover the happy path and at least one error case, "+
            "and pass under `go test -race ./...`.",
    )
}
```

### 3. Stop-gate re-injection message is specific

The predicate script names the exact packages missing tests in its stdout.
Because `orchestrator.CheckResult.Output` is already forwarded into
`Verdict.Diagnosis()`, the stop-gate re-injection message naturally includes
the package list from the script — no extra wiring needed.

---

## Implementation notes (added after build)

**Why check for `_test.go` file existence AND test_changed separately:**
The first pass (check each package dir for any `_test.go` file) catches
the case where a brand-new package ships with zero tests. The second pass
(check if any test file appeared in the diff) catches the case where an
existing package was modified but its already-present test file was not
touched. Both cases need separate treatment.

**Untracked files via `git ls-files --others --exclude-standard`:**
The original design only used `git diff --name-only HEAD`. This misses
brand-new files that have never been staged. A Go agent building a new package
from scratch will produce untracked files, and those packages have no tests by
definition. Merging the two streams and deduplicating with `sort -u` gives
complete coverage.

**Why exit 0 outside a git repo:**
The predicate is a "fail-closed when we can judge, fail-open when we cannot"
design. Running the check in a non-git directory (e.g., a one-shot temp
workspace) would produce confusing false negatives. The semantic criterion
(LLM judge) still runs and covers the intent.

**`appendUnique` prevents duplicate semantic criteria:**
If a user also passes `--criteria` with a similar phrase, `appendUnique` stops
the stop-gate from evaluating a near-identical criterion twice. Idempotent
construction of the criteria list is important because `Resolve` may be called
multiple times across daemon restarts for the same goal.

**`NoTestCriterion` is on `ResolveOptions`, not `GoalContract`:**
This is intentional. `ResolveOptions` controls what gets built into the
contract. Once the contract is built and stored, it is immutable. The
`NoTestCriterion` option lets operators disable the criterion at resolve-time
(either per-goal via `goal add` or globally via the daemon flag), but after
the contract is persisted the stop-gate enforces it regardless.

---

## Acceptance criteria

- [x] `autoDetectChecks` adds `new-test-coverage` predicate for Go repos
- [x] `Resolve` injects a semantic criterion for test quality when auto-detecting
- [x] stop-gate blocks completion when changed packages have no test file
- [x] re-injection message names the specific packages missing tests
- [x] `NoTestCriterion` on `ResolveOptions` to opt out per resolve call
- [x] unit tests for the predicate script logic (table-driven, temp git repos)
- [x] `go test -race` green
