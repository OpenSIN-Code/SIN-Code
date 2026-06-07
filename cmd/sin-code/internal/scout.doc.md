# `scout.doc.md` — Code Search Subcommand

Searches codebases with regex, semantic, symbol, and usage search modes, including context lines and result ranking.

## What it does

- **Regex search:** Compiles user-provided regex and matches line-by-line across all text files in a directory tree.
- **Semantic search:** Splits query into words, joins with `.*` wildcard, and case-insensitively matches any order.
- **Symbol search:** Looks for function/class/variable/type definitions matching the query (e.g., `func QueryName`, `class QueryName`).
- **Usage search:** Looks for bare-word references to the query symbol anywhere in the code.
- **Context lines:** Returns `radius=2` lines above and below each match with line numbers and a `>` marker on the match line.
- **Relevance scoring:** Boosts matches in source files (+15), definition lines (+20), and penalizes comments (-10) and test files (-5).
- **Basic dead-code detection:** Tracks which files have matches during symbol/usage searches (reported in future versions).

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `ScoutCmd` into the root cobra command
- `cmd/sin-code/internal/scout.go` — self-contained search engine
- `cmd/sin-code/internal/sckg.go` — shares the file-walking and skip-directory logic pattern

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--query` | *(required)* | Search query (regex, words, or symbol name) |
| `--path` | `.` | Directory to search |
| `--search_type` | `regex` | Mode: `regex`, `semantic`, `symbol`, `usage` |
| `--format` | `text` | Output: `text` or `json` |
| `--max_results` | `50` | Hard stop after N matches |

- **File size limit:** Files >5MB are skipped silently to avoid memory issues.
- **Skip directories:** `.git`, `node_modules`, `vendor`, `__pycache__`, `dist`, `build`, `target`, and any directory starting with `.`.
- **Relevance clamped:** 0-100 range.

## Usage examples

```bash
# Regex search for all main functions
sin-code scout --query "func.*main" --path . --search_type regex --format json

# Semantic search (matches "user auth" in any order, case-insensitive)
sin-code scout --query "user authentication" --search_type semantic

# Find where a symbol is defined
sin-code scout --query "handleRequest" --search_type symbol

# Find all usages of a variable
sin-code scout --query "config" --search_type usage --max_results 100
```

## Known caveats / footguns

- **Regex mode is line-by-line:** `^` and `$` match line boundaries, not file boundaries. Multi-line regex patterns will not work as expected.
- **Semantic search is primitive:** Splits on whitespace and joins with `.*` — it is NOT a real semantic/vector search. It works well for finding keywords near each other.
- **Symbol search is regex-based:** May miss definitions with unusual formatting (e.g., `func\nName` split across lines).
- **Max results is a hard stop:** Once reached, the walker aborts with a synthetic error. The remaining files are not scanned.
- **Binary files are not excluded:** Only directories are skipped; binary files are read as text and may produce garbage matches.