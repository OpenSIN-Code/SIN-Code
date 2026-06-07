# `discover.doc.md` — File Discovery Subcommand

Discovers files with pattern matching, relevance scoring, and dependency parsing across Go, Python, JavaScript/TypeScript, and other languages.

## What it does

- **Walks directories** recursively, skipping noise folders (`.git`, `node_modules`, `vendor`, `__pycache__`, `.venv`, `.next`, `dist`, `build`, `target`, `.idea`, `.vscode`, `.pytest_cache`, coverage).
- **Matches glob patterns** (`**/*.go`, `cmd/**/*.go`) with a custom glob-to-regex engine supporting `**` (any depth), `*` (single segment), and `?` (single char).
- **Scores relevance** (0-100) based on: depth from root, file extension, filename keywords, and penalties for large/generated files.
- **Extracts dependencies** via Go AST (`go/parser`), Python import regex, and JS/TS `import`/`require` regex.
- **Sorts and limits** results by relevance, name, size, or mtime.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `DiscoverCmd` into the root cobra command
- `cmd/sin-code/internal/discover_test.go` — unit tests for glob matching, scoring, and dependency extraction
- `cmd/sin-code/internal/grasp.go` — reuses `extractDependencies` for dependency analysis
- `cmd/sin-code/internal/map.go` — reuses `extractDependencies` for architecture mapping
- `cmd/sin-code/internal/scout.go` — reuses directory walking logic for search
- `cmd/sin-code/internal/sckg.go` — reuses `extractDependencies` for graph building

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--pattern` | `**/*` | Glob pattern for file matching |
| `--sort_by` | `relevance` | Sort: `relevance`, `name`, `size`, `mtime` |
| `--format` | `text` | Output: `text` or `json` |
| `--limit` | `100` | Max results (early-stop heuristic at `limit*10` walked files) |

- **Relevance score caps:** 0-100 clamped; bonuses up to +20 for `main`, `index`, `config`, `go.mod`; penalty -20 for files >1MB, -30 for vendor/build paths.
- **Dependency limit:** Max 20 dependencies per file to avoid bloated output.
- **File size limit:** Files >500KB are skipped for dependency extraction.

## Usage examples

```bash
# Discover all Go files sorted by relevance
sin-code discover . --pattern "**/*.go" --sort_by relevance --format json

# Find config files in cmd/ subtree
sin-code discover ./cmd --pattern "**/*.yaml" --limit 50

# Discover Python files with dependency info
sin-code discover . --pattern "**/*.py" --format json
```

## Known caveats / footguns

- **Glob engine is custom, not standard:** `**` matches any number of directories, but complex bracket expressions (`[abc]`) are NOT supported. Use simple `*` and `?` only.
- **Go dependency extraction uses `go/parser`:** Requires valid Go syntax. Files with parser errors return empty dependencies silently.
- **Python/JS dependency extraction is regex-based:** May miss dynamic imports, conditional imports, or non-standard syntax.
- **Early-stop heuristic:** If `walked > limit*10` and `len(results) > limit`, the walker skips the rest of the current directory. Deep directories may be partially scanned.
- **Hidden directories:** Directories starting with `.` are skipped entirely, including `.github` and `.claude`.