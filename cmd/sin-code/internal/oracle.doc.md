# `oracle.doc.md` — Verification Oracle Subcommand

Compares source files against test files to verify that every function/method has corresponding test coverage.

## What it does

- **Extracts symbols** from both source (claim) and test (evidence) files using language-specific parsers.
- **Normalizes test names** by stripping prefixes: `Test`, `test_`, `spec`, `it`, `should`, `can`, `will` (case-insensitive).
- **Matches source functions to tests:** A source function `handleRequest` matches test `TestHandleRequest` after normalization.
- **Reports coverage:** Percentage of source functions with at least one matching test.
- **Identifies uncovered functions:** Source symbols with no corresponding test.
- **Identifies orphaned tests:** Test functions that don't match any source function (may indicate outdated or renamed source).
- **Multi-language support:** Go (AST), Python (regex), JS/TS (regex), Rust (regex), Java (regex), generic fallback.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `OracleCmd` into the root cobra command
- `cmd/sin-code/internal/oracle.go` — self-contained coverage verifier
- `cmd/sin-code/internal/grasp.go` — reuses `detectLanguage` and `extractSymbols` patterns
- `cmd/sin-code/internal/poc.go` — shares `symbolInfo` struct and symbol extraction logic
- `cmd/sin-code/internal/sckg.go` — shares Go AST parsing and symbol extraction concepts

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--claim` | `""` | Source file to check coverage for (required) |
| `--evidence` | `""` | Test file to compare against (required) |
| `--format` | `text` | Output: `text` or `json` |

- **Name normalization:** Both claim and test names are lowercased; test prefixes are stripped. `handleRequest` matches `TestHandleRequest`, `test_handle_request`, `itHandleRequest`, etc.
- **Coverage formula:** `covered_symbols / total_claim_symbols * 100`. Not line coverage or branch coverage.
- **Method receiver normalization:** Go methods like `(Server).HandleRequest` are stored as `(Server).HandleRequest` — the test must match the full normalized name.

## Usage examples

```bash
# Verify Go source against its test file
sin-code oracle --claim cmd/sin-code/main.go --evidence cmd/sin-code/main_test.go

# Check Python coverage
sin-code oracle --claim src/main.py --evidence tests/test_main.py

# JSON output for CI integration
sin-code oracle --claim src/app.ts --evidence src/app.test.ts --format json
```

## Known caveats / footguns

- **Symbol-only coverage, not line coverage:** A function with a single empty test body counts as "covered". This does not measure actual test quality or assertion count.
- **Test name normalization is heuristic:** Complex naming conventions (e.g., BDD-style `Given_When_Then`) may not normalize correctly to match source function names.
- **Go method receivers are included:** The normalized name includes the receiver type: `(Server).handleRequest`. Tests must match this full string, not just `handleRequest`.
- **Does not support directory-level comparison:** Only single file pairs. For directory coverage, run `oracle` on each source/test pair individually or use a dedicated coverage tool.
- **False positives for orphaned tests:** A test named `test_setup` or `test_helpers` may not match any source function but is still a legitimate test. Review orphaned tests manually before deleting.
- **Generic symbol extraction is loose:** Falls back to regex matching any `function/def/fn/func/class/struct` keyword. May match non-function symbols in comments or strings.