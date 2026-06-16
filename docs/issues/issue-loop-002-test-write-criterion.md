# [loop-002] Auto-inject "tests must exist and pass" criterion — no human reminder needed

**Labels:** `loop-engineering` `autonomy` `p0`
**Branch:** `agent-loop-engineering`
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
lines:

```go
// internal/goalcontract/goalcontract.go
const newTestCoverageScript = `set -e
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi

# Files changed in working tree vs HEAD
changed=$(git diff --name-only HEAD 2>/dev/null || true)
if [ -z "$changed" ]; then exit 0; fi

# Check if any non-test .go files were added/modified
non_test=$(echo "$changed" | grep '\.go$' | grep -v '_test\.go$' || true)
if [ -z "$non_test" ]; then exit 0; fi  # only test files changed: fine

# Check if any _test.go lines were added
test_lines=$(git diff HEAD | grep -E '^\+.*_test\.go|^\+\+\+ .*_test\.go' || true)
if [ -z "$test_lines" ]; then
  echo "Non-test Go files were changed but no test file was added or modified."
  echo "Changed packages must have new or updated tests."
  exit 1
fi
exit 0`

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
        // NEW: fail the stop-gate when non-test code changed but no tests
        // were written or updated. Forces the agent to write tests without
        // a human prompt.
        checks = append(checks, orchestrator.Check{
            Kind:    orchestrator.CheckPredicate,
            Name:    "new-test-coverage",
            Cmd:     []string{"sh", "-c", newTestCoverageScript},
            Timeout: 1 * time.Minute,
        })
    }
    return checks
}
```

### 2. Auto-inject a semantic criterion for test quality

Beyond the existence check, the LLM judge should confirm test quality. Add to
`Resolve` when auto-detecting:

```go
// internal/goalcontract/goalcontract.go — Resolve, after autoDetectChecks
if opts.AutoDetect && fileExists(filepath.Join(opts.Workspace, "go.mod")) {
    c.SemanticCriteria = append(c.SemanticCriteria,
        "New or updated _test.go files exist for every changed package, "+
            "cover the happy path and at least one error case, "+
            "and pass under `go test -race ./...`.",
    )
}
```

### 3. Stop-gate re-injection message must be specific

When the `new-test-coverage` predicate fails, the injected message must name
exactly which packages lack tests:

```go
// internal/goalcontract/goalcontract.go — extended noTestCoverageScript
const newTestCoverageScript = `set -e
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi
changed=$(git diff --name-only HEAD 2>/dev/null || true)
[ -z "$changed" ] && exit 0
non_test=$(echo "$changed" | grep '\.go$' | grep -v '_test\.go$' || true)
[ -z "$non_test" ] && exit 0
# Extract package directories
pkgs=$(echo "$non_test" | xargs -I{} dirname {} | sort -u)
for pkg in $pkgs; do
  test_files=$(find "$pkg" -maxdepth 1 -name '*_test.go' 2>/dev/null || true)
  if [ -z "$test_files" ]; then
    echo "MISSING TESTS: package $pkg has no _test.go file"
  fi
done
# Check if we emitted any missing lines
missing=$(echo "$pkgs" | while read pkg; do
  [ -z "$(find "$pkg" -maxdepth 1 -name '*_test.go' 2>/dev/null)" ] && echo "$pkg"
done)
[ -n "$missing" ] && exit 1
exit 0`
```

---

## Acceptance criteria

- [ ] `autoDetectChecks` adds `new-test-coverage` predicate for Go repos
- [ ] `Resolve` injects a semantic criterion for test quality when auto-detecting
- [ ] stop-gate blocks completion when changed packages have no test file
- [ ] re-injection message names the specific packages missing tests
- [ ] `--no-test-criterion` flag on `goal add` and daemon to opt out per goal
- [ ] unit tests for the predicate script logic (table-driven, temp git repos)
- [ ] `go test -race` green
