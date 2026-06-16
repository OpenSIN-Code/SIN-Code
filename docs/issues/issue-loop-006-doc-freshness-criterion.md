# [loop-006] Stop-gate must verify README, AGENTS.md, and CHANGELOG — not just code

**Labels:** `loop-engineering` `autonomy` `p1`
**Branch:** `agent-loop-engineering`
**Affects:** `stopgate/stopgate.go`, `goalcontract/goalcontract.go`

---

## Problem

The stop-gate today evaluates `DeterministicChecks` (build/test/vet/lint) and
`SemanticCriteria` (LLM judge). But none of the auto-detected checks verify
that documentation was updated. An agent can ship code that passes all tests,
is lint-clean, and introduces no new TODOs — yet leaves README.md, AGENTS.md,
CHANGELOG.md, and affected `doc.md` files completely stale.

The LLM judge *could* catch this if the semantic criterion says "docs updated"
— but only if that criterion is always auto-injected. Currently it is not.

---

## Root cause

```go
// internal/goalcontract/goalcontract.go — autoDetectChecks
func autoDetectChecks(workspace string) []orchestrator.Check {
    // builds go build + test + vet + no-new-todos
    // MISSING: no check for doc staleness
    return checks
}
```

```go
// internal/goalcontract/goalcontract.go — Resolve
// SemanticCriteria only populated when explicitly passed via --criteria.
// Auto-detection adds zero semantic criteria.
```

---

## Proposed solution

### 1. Deterministic doc-freshness check: CHANGELOG must mention the session

The simplest deterministic gate: when CHANGELOG.md exists and the diff touches
`.go` files, confirm that the diff also touches `CHANGELOG.md`:

```go
// internal/goalcontract/goalcontract.go
const changelogUpdatedScript = `set -e
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi
if [ ! -f CHANGELOG.md ]; then exit 0; fi
# Check if any non-test .go files were modified
go_changed=$(git diff --name-only HEAD | grep '\.go$' | grep -v '_test\.go$' || true)
[ -z "$go_changed" ] && exit 0   # no production Go changes: skip
# Check if CHANGELOG.md was also modified
cl_changed=$(git diff --name-only HEAD | grep -i 'CHANGELOG' || true)
if [ -z "$cl_changed" ]; then
  echo "CHANGELOG.md was not updated. Production Go files were changed."
  echo "Add a bullet under [Unreleased] describing what changed."
  exit 1
fi
exit 0`
```

Add to `autoDetectChecks`:

```go
if fileExists(filepath.Join(workspace, "CHANGELOG.md")) {
    checks = append(checks, orchestrator.Check{
        Kind:    orchestrator.CheckPredicate,
        Name:    "changelog-updated",
        Cmd:     []string{"sh", "-c", changelogUpdatedScript},
        Timeout: 1 * time.Minute,
    })
}
```

### 2. Deterministic doc.md freshness — flag stale package docs

```go
const docMdFreshnessScript = `set -e
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi
# Find changed .go files and their directories
changed_dirs=$(git diff --name-only HEAD | grep '\.go$' | grep -v '_test\.go$' \
  | xargs -I{} dirname {} 2>/dev/null | sort -u || true)
[ -z "$changed_dirs" ] && exit 0
fail=0
for dir in $changed_dirs; do
  doc="$dir/$(basename $dir).doc.md"
  # Some packages use a flat doc.md name
  if [ -f "$doc" ]; then
    cl=$(git diff --name-only HEAD | grep -F "$doc" || true)
    if [ -z "$cl" ]; then
      echo "STALE DOC: $doc was not updated despite changes in $dir"
      fail=1
    fi
  fi
done
[ $fail -eq 1 ] && exit 1
exit 0`
```

### 3. Always-on semantic criterion for docs

```go
// internal/goalcontract/goalcontract.go — Resolve, after autoDetectChecks
if opts.AutoDetect {
    // Auto-inject semantic doc criterion regardless of language/repo type.
    c.SemanticCriteria = append(c.SemanticCriteria,
        "If the change affects user-visible behaviour, CLI flags, environment "+
            "variables, or public APIs: README.md, AGENTS.md (if it exists), and "+
            "all affected doc.md files were updated to reflect the change.",
        "CHANGELOG.md (if it exists) has a new entry under [Unreleased] "+
            "describing what was changed and why.",
    )
}
```

### 4. Stop-gate re-injection must name the specific stale files

When a doc-freshness check fails, the re-injection message must list the exact
files that need updating so the model can act immediately:

```go
// internal/stopgate/stopgate.go — failedCheckNames already returns the check name
// The deterministic check output (from the shell script) already names the file.
// Ensure the Diagnosis() propagation includes stdout from failed predicates:

// internal/orchestrator/verifier.go — Check.Run should capture stdout
type CheckResult struct {
    Check   Check
    Passed  bool
    Output  string  // stdout+stderr from predicate, empty for non-predicate kinds
    Elapsed time.Duration
}
// This output is already in Diagnosis() if the orchestrator passes it through.
// Verify that the chain Check -> CheckResult.Output -> Verdict.Diagnosis() works
// and that the stop-gate re-injection includes it word-for-word.
```

---

## Acceptance criteria

- [ ] `changelog-updated` predicate added to `autoDetectChecks` when CHANGELOG.md exists
- [ ] `doc-md-freshness` predicate added when any `*.doc.md` exists in a changed package
- [ ] two semantic criteria (doc + changelog) always auto-injected in `Resolve`
- [ ] stop-gate re-injection message includes the exact stale file names from predicate output
- [ ] `--no-doc-criterion` flag on `goal add` and daemon to opt out per goal
- [ ] unit tests for both predicate scripts (using temp git repos)
- [ ] `go test -race` green
