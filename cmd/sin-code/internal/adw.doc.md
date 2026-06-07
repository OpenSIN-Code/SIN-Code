# `adw.doc.md` — Architectural Debt Watchdogs Subcommand

Detects architectural debt in codebases: god modules, circular dependencies, high coupling, long functions, large files, missing tests, and code smells.

## What it does

- **Scans all source files** and applies a configurable rule set to detect architectural issues.
- **God modules:** Files with >15 imports or >500 lines flagged as medium/high severity.
- **Circular dependencies:** Detects 2-file import cycles (A imports B, B imports A) flagged as critical.
- **High coupling:** Files imported by >10 other files flagged as medium.
- **Long functions:** Functions/classes >100 lines flagged as medium (Go AST, Python regex, JS brace-counting).
- **Large files:** >500 lines flagged as medium.
- **TODO/FIXME/XXX/HACK/BUG:** Comments flagged as low/medium depending on keyword severity.
- **Missing tests:** Source files without corresponding test files flagged as low.
- **Grading:** A-F score (100-point scale) with exit codes for CI integration.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `AdwCmd` into the root cobra command
- `cmd/sin-code/internal/adw.go` — self-contained debt scanner
- `cmd/sin-code/internal/discover.go` — reuses `extractDependencies` for import analysis
- `cmd/sin-code/internal/map.go` — shares `detectLanguage` and reverse-dependency concepts
- `cmd/sin-code/internal/grasp.go` — shares language detection and structure extraction patterns

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--format` | `text` | Output: `text` or `json` |
| `--strict` | `false` | Exit code 1 if any critical/high issues found |

- **Severity scoring:** Critical=-20, High=-10, Medium=-5, Low=-2 points from base 100.
- **Grades:** A (≥90), B (≥80), C (≥60), D (≥40), F (<40).
- **Exit codes:** 0 = ok, 1 = strict mode with issues, 2 = critical issues found (non-strict).
- **Function length threshold:** 100 lines (Go AST, Python indent-based, JS brace-counting).
- **File size threshold:** 500 lines.
- **Import thresholds:** God module >15 imports, high coupling >10 importers.

## Usage examples

```bash
# Scan current directory
sin-code adw .

# Strict mode for CI (fails on high/critical issues)
sin-code adw ./src --strict --format json

# Scan backend only
sin-code adw ./backend
```

## Known caveats / footguns

- **Circular dependency detection is limited to 2-file cycles:** A→B→C→A cycles are NOT detected. Only direct A↔B pairs are found.
- **Missing test detection is heuristic:** Assumes naming conventions (`_test.go`, `test_*.py`, `*.test.js`, `*Test.java`). Custom test layouts will produce false positives.
- **Function length for JS is brace-counting:** Counts `{` and `}` to find function boundaries. Nested objects inside functions may inflate the count.
- **Config/data files are excluded:** JSON, YAML, markdown, and text files are skipped. But large generated Go files (e.g., protobuf) may still trigger "large file" warnings.
- **Strict mode exit code 1:** In CI, use `--strict` to fail the pipeline on high/critical issues. Without it, only critical issues return exit code 2.
- **Python function length uses indentation:** Assumes consistent indentation. Mixed tabs/spaces may cause incorrect length measurement.