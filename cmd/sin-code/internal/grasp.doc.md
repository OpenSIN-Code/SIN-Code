# `grasp.doc.md` — Deep Code Understanding Subcommand

Analyzes a single file to extract structure, dependencies, exports, and line metrics across Go, Python, JavaScript/TypeScript, Rust, Java, and other languages.

## What it does

- **Multi-language structure analysis** using Go AST for `.go` files, regex heuristics for Python/JS/TS/Rust/Java, and generic fallback patterns for other languages.
- **Line counting** with language-aware comment detection: block comments (`/* */`, `""""""`, `'''`), line comments (`//`, `#`, `//`), and blank line detection.
- **Dependency extraction** reuses `extractDependencies` from `discover.go` (Go AST, Python imports, JS imports).
- **Export detection** via Go AST exported identifiers, Python `__all__`, JS/TS `export` statements, and Rust `pub` items.
- **Produces a summary** with total lines, code lines, comment lines, blank lines, language, and modification time.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `GraspCmd` into the root cobra command
- `cmd/sin-code/internal/grasp.go` — self-contained; defines `analyzeFile`, `detectLanguage`, `extractStructure`, `extractExports`
- `cmd/sin-code/internal/discover.go` — reuses `extractDependencies` for import parsing
- `cmd/sin-code/internal/oracle.go` — reuses `detectLanguage` and `extractSymbols` patterns
- `cmd/sin-code/internal/poc.go` — reuses `extractSymbols` and `detectLanguage`
- `cmd/sin-code/internal/sckg.go` — reuses `detectLanguage` and structure extraction patterns

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--format` | `text` | Output: `text` or `json` |

- **Language detection:** 20+ extensions mapped + special cases for `Dockerfile` and `Makefile`. Unknown extensions return `"unknown"`.
- **Go structure extraction:** Uses `go/parser` with `parser.AllErrors` — best-effort parsing; partial files may still yield symbols.
- **File size limit:** No explicit limit, but files >500KB may slow down regex-based extractors.

## Usage examples

```bash
# Analyze a Go file
sin-code grasp cmd/sin-code/main.go --format json

# Analyze a Python module
sin-code grasp src/app.py

# Quick structure check for a TypeScript file
sin-code grasp frontend/components/Modal.tsx
```

## Known caveats / footguns

- **Go AST is the only "real" parser:** Python, JS/TS, Rust, and Java use regex heuristics. Multi-line definitions, decorators, and complex generics may be misreported.
- **Comment counting is heuristic:** Block comment start/end detection assumes well-formed comments. Nested block comments (e.g., `/* /* */ */`) are not handled correctly.
- **Export detection varies by language:** Go uses AST scope objects; Python requires `__all__`; JS/TS requires `export` keyword; Rust requires `pub`. Non-exported symbols are still listed in structure but not in exports.
- **Language detection is extension-only:** Files without standard extensions (e.g., shell scripts with no `.sh`) return `"unknown"`.
- **Binary files:** Not recommended. Text encoding is assumed; binary files will produce garbled output.