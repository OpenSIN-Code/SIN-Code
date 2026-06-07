# `poc.doc.md` — Proof-of-Correctness Subcommand

Verifies that code satisfies its specification by comparing required functions/classes/imports from a spec document against actual source code.

## What it does

- **Parses spec documents** (markdown, text, JSON) to extract requirements using regex patterns:
  - `must implement X`, `requires X`, `should have X`, `function X`, `method X`, `class X`, `struct X`, `type X`, `interface X`
  - Also recursively extracts requirements from markdown code blocks (` ```...``` `).
- **Walks code directories** or reads a single code file and extracts all symbols (functions, classes, types) using language-specific parsers.
- **Checks requirements:** For each required symbol from the spec, searches the code for a matching name (case-insensitive, with `_` and `-` normalization).
- **Forbidden pattern checks:** Detects `os.Exit(` in non-main Go files (library code anti-pattern) and `TODO`/`FIXME` in non-test code.
- **Produces coverage score:** Percentage of required symbols found in code.
- **Report format:** Per-check status (`pass`/`fail`/`warn`) with file:line references.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `PocCmd` into the root cobra command
- `cmd/sin-code/internal/poc.go` — self-contained spec parser and verifier
- `cmd/sin-code/internal/oracle.go` — shares `extractSymbols` pattern and `symbolInfo` struct
- `cmd/sin-code/internal/grasp.go` — reuses `detectLanguage` and structure extraction patterns
- `cmd/sin-code/internal/sckg.go` — shares symbol extraction concepts

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--spec` | `""` | Specification file (markdown, text, json) |
| `--code` | `""` | Code file or directory to verify |
| `--format` | `text` | Output: `text` or `json` |

- **Requirement extraction limit:** Regex-based; may miss requirements phrased unusually or buried in prose without keywords.
- **Code walk limit:** Files >2MB are skipped. Unknown language files are skipped for symbol extraction.
- **Coverage formula:** `passed_requirements / total_requirements * 100`. Does not measure line coverage or branch coverage.

## Usage examples

```bash
# Verify a spec against a single file
sin-code poc --spec spec.md --code src/main.py

# Verify against a directory
sin-code poc --spec requirements.json --code ./src/ --format json

# Check code without a spec (forbidden patterns only)
sin-code poc --code cmd/sin-code/main.go
```

## Known caveats / footguns

- **Spec parsing is keyword-based:** If the spec says "we need a user handler" without using `function`/`class`/`must implement`, it will NOT be extracted.
- **Name normalization is lossy:** `user-handler` and `user_handler` both match `userhandler` after normalization. May produce false positives for similarly named symbols.
- **Forbidden pattern checks are minimal:** Only `os.Exit` in Go library code and `TODO`/`FIXME` are checked. Not a comprehensive linter.
- **No signature verification:** The check only verifies that a symbol *exists*, not that its parameters, return types, or behavior match the spec.
- **False negatives in dynamic languages:** Python/JS symbol extraction is regex-based. Decorators, monkey-patching, and dynamically generated functions are invisible.
- **Coverage != test coverage:** This is spec-to-code coverage, not line/branch coverage. A 100% score means all required symbols exist, not that they are tested.