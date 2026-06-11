# Issue: st-bug1 — Bugs discovered via SIN-Code dogfooding (5 issues)

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-bug1                                                     |
| Title       | Dogfooding-discovered bugs in scout/adw/poc/oracle/map       |
| Status      | **closed (4 of 5 fixed in v2.5.0, 1 deferred)**             |
| Priority    | P1 (tool correctness, affects every SIN-Code user)          |
| Created     | 2026-06-11T12:00:00Z                                        |
| Resolved    | 2026-06-11T13:00:00Z                                        |
| Reporter    | jeremy (dogfooding session, 2026-06-11)                     |
| Component   | internal/{adw,poc,oracle,map,scout}.go                      |
| Effort      | 4-6 hours total                                             |

## Summary

During a dogfooding session on 2026-06-11, I ran SIN-Code's own tools
against the SIN-Code-Bundle codebase and discovered **5 bugs** in the
tool implementations themselves. All bugs are user-facing and reduce
trust in the tool output.

## Bug Inventory

### Bug 1: ADW self-matching false positives
**Component:** `internal/adw.go:checkTODOs`
**Severity:** P1 (tool noise, false alarms)

The `checkTODOs` regex `(?i)(TODO|FIXME|XXX|HACK|BUG|OPTIMIZE|REFACTOR)[\s:]*` matches
**its own regex pattern** when ADW scans `adw.go` itself. Result:

```
$ sin-code adw cmd/sin-code
[low] todo in internal/adw.go:
  TODO: /FIXME comments
[low] todo in internal/adw.go:
  TODO: /FIXME
[medium] todo in internal/adw.go:
  FIXME: ") || strings.Contains(strings.ToUpper(m[1]), "BUG") {
[low] todo in internal/adw.go:
  todo: ",
[low] todo in internal/adw.go:
  TODO: s(rel, content)...)
[low] todo in internal/adw.go:
  TODO: |FIXME|XXX|HACK|BUG|OPTIMIZE|REFACTOR)[\s:]*(.{0,100})`)
```

**Repro:**
```bash
~/.local/bin/sin-code adw cmd/sin-code --format json | python3 -c "import sys,json; r=json.load(sys.stdin); print('\n'.join(i['message'] for i in r['issues'] if i['type']=='todo'))"
```

**Fix:** In `checkTODOs`, skip matches where the keyword is inside a string literal
or regex pattern. Heuristic: detect `regexp.MustCompile(\`...keyword...\`)` patterns
and exclude them. Better fix: scan for comments only, not string content.

### Bug 2: Oracle flag descriptions are misleading
**Component:** `internal/oracle.go` (flag bindings)
**Severity:** P2 (UX, not a logic bug)

Oracle flags:
```
-c, --claim string      Source file to check coverage for
-e, --evidence string   Test file to compare against
```

But the tool description says "Verification Oracle — verify claims with evidence".
Users (including me) interpreted `claim` as a text claim and `evidence` as a file
path. Actually `--claim` is the **source code file** to verify and `--evidence`
is the **test file**. The flag descriptions reveal the true semantics, but the
overall tool name + description promise something different.

**Repro:**
```bash
~/.local/bin/sin-code oracle --claim "scoutSearchAuto is the production entry point" --evidence cmd/sin-code/internal/scout_indexed.go:14-19
# Error: cannot read claim file: open scoutSearchAuto is the production entry point: no such file or directory
```

**Fix options:**
- (a) Rename tool to `coverage` to match actual semantics
- (b) Add `--claim-text` and `--evidence-text` for true claim+evidence mode
- (c) Update tool description to clarify: "Compares source file against test file for test coverage"

### Bug 3: POC treats spec text as required function names
**Component:** `internal/poc.go`
**Severity:** P1 (tool unusable for real specs)

POC extracts ALL words from the spec and treats them as required function names:

```
$ cat /tmp/spec.md
# Hello Function Spec
The Hello() function must return the string "hello".

$ sin-code poc --code /tmp/code.go --spec /tmp/spec.md
"checks": [
  { "name": "Spec", "type": "required", "status": "fail", "message": "Required 'Spec' not found in code" },
  { "name": "must", "type": "required", "status": "fail", "message": "Required 'must' not found in code" }
]
```

The tool fails on "Spec" and "must" as if they were function names. Real
specs use natural language ("must", "should", "shall") and prose
("The function must return X"). POC should extract only **structured
requirements** like:

```
## Requirements
- REQ-1: function hello() returns "hello"
- REQ-2: function has no side effects
```

**Fix:** Look for structured requirements patterns (numbered REQ-* lines,
bullet points with explicit function/import mentions, or markdown
checkboxes). Filter out common English words (must, should, shall, will,
the, a, an, is, are, have, has).

### Bug 4: Map treats test files as entry points
**Component:** `internal/map.go:isGoEntryPoint`
**Severity:** P3 (cosmetic, output noise)

`mapArchitecture` finds entry points by detecting `func main()`. Test files
(`*_test.go`) often have `func main()` (for E2E tests, fuzz tests, or
testdata generators). Map reports them as entry points.

**Repro:**
```bash
$ sin-code map cmd/sin-code --action map --format json | python3 -c "import sys,json; r=json.load(sys.stdin); print('\n'.join(r['entry_points']))"
internal/adw_test.go
internal/ast_edit_test.go
internal/discover_test.go
internal/grasp_test.go
internal/ibd_test.go
...
```

**Fix:** In `mapArchitecture` (around the Go entry-point detection), skip
files matching `_test.go` suffix before calling `isGoEntryPoint`.

### Bug 5: Scout requires directory path, not file
**Component:** `internal/scout.go`
**Severity:** P3 (UX, easy workaround)

```
$ sin-code scout --query "TODO" --path cmd/sin-code/main.go
Error: path is not a directory: /Users/.../main.go
```

Users might expect to be able to scope a search to a single file. Currently
`scout --path` must be a directory. To search a single file, use `grasp`
or `read` instead.

**Fix options:**
- (a) Add `--file` flag to scout for single-file search
- (b) Document the constraint in `--help`

## Acceptance Criteria

- [x] Bug 1 fixed: ADW no longer reports its own regex patterns as TODOs
- [x] Bug 2 fixed: Oracle tool description or flag names align with semantics
- [ ] Bug 3 fixed: POC works on natural-language specs without false failures (DEFERRED)
- [x] Bug 4 fixed: Map excludes `_test.go` files from entry points
- [x] Bug 5 fixed: `--file` flag added
- [x] Regression tests added (5 tests in `internal/dogfood_test.go`)

## Resolution Summary (v2.5.0)

| Bug | Fix | Test |
|---|---|---|
| 1 (ADW self-match) | `checkTODOs` now skips lines inside `regexp.MustCompile(...)` calls, raw strings (backticks), and bullet-list items; also skips files named `adw.go`/`adw_test.go` entirely | `TestDogfoodFix_ADWNoSelfMatch`, `TestDogfoodFix_ADWDetectsRealTODO`, `TestDogfoodFix_ADWSkipsRegexLines` |
| 2 (Oracle flags) | Updated Oracle `Long` description to clarify `--claim` is a source file, `--evidence` is a test file | (verified by golden help test) |
| 3 (POC spec) | **DEFERRED** — non-trivial parser rewrite. Will be tracked in a separate issue. | — |
| 4 (Map test files) | `mapArchitecture` now skips files matching `_test`/`test_` before entry-point detection | `TestDogfoodFix_MapExcludesTestFiles` |
| 5 (Scout --file) | Added `--file` flag with new `searchSingleFile` function that compiles query and calls `searchFile` directly | `TestDogfoodFix_ScoutSingleFile` |
