# `map.doc.md` — Architecture Mapping Subcommand

Maps codebase architecture with dependency graphs, entry points, hot paths, orphaned files, and module-level analysis.

## What it does

- **Walks the full directory tree** and collects file metadata, language distribution, line counts, and imports for every source file.
- **Builds dependency graphs** with forward dependencies (what each file imports) and reverse dependencies (what imports each file).
- **Detects entry points** for Go (`main.go`, `func main()`), Python (`__main__.py`, `if __name__ == "__main__"`), JS/TS (`index.js`, `main.js`), Rust (`main.rs`), and Java (`public static void main`).
- **Identifies hot paths** — files imported by more than 2 others, sorted by importer count (top 20).
- **Finds orphans** — source files with no imports and no importers, excluding tests and configs.
- **Aggregates modules** — subdirectories with code, grouped by file count and languages.
- **Produces a summary** with total files, lines, test/config/doc counts, and language breakdown.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `MapCmd` into the root cobra command
- `cmd/sin-code/internal/map.go` — self-contained architecture analyzer
- `cmd/sin-code/internal/discover.go` — reuses `extractDependencies` and `detectLanguage`
- `cmd/sin-code/internal/sckg.go` — shares the graph-building and import-extraction concepts
- `cmd/sin-code/internal/adw.go` — reuses `detectLanguage` and reverse-dependency analysis

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--action` | `map` | Action: `map`, `summary`, `graph`, `hotpaths` (currently all alias to `map`) |
| `--format` | `text` | Output: `text` or `json` |

- **Hot path threshold:** Files imported by >2 others. Only top 20 shown.
- **Orphan detection:** Skips test files, config files, docs, and unknown language files.
- **File size limit:** Files >1MB are skipped for line counting and dependency extraction.
- **Import limit:** Max 20 dependencies per file (inherited from `discover.go`).

## Usage examples

```bash
# Full architecture map of current directory
sin-code map . --format json

# Map a specific project
sin-code map ./backend --action map

# Quick summary view (text)
sin-code map ./frontend
```

## Known caveats / footguns

- **Entry point detection is heuristic:** Looks for `func main()` in Go file content, not just `main.go`. May miss entry points in framework-specific files (e.g., Flask/Django entry points in Python).
- **Orphans may be legitimate:** A standalone utility script or a newly created file may correctly have no imports or importers. Review orphans before deleting.
- **Module aggregation is directory-based:** Every subdirectory becomes a module. Empty directories or directories with only non-code files are filtered out.
- **Reverse dependencies are import-based:** Only captures explicit imports (Go `import`, Python `import`, JS `import`/`require`). Runtime dynamic imports are invisible.
- **Circular dependencies:** Detected only in `adw.go` (not here). `map` shows the raw dependency graph without cycle analysis.