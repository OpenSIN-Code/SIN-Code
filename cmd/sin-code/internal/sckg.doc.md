# `sckg.doc.md` — Semantic Codebase Knowledge Graph Subcommand

Builds and queries a knowledge graph of a codebase: files, functions, types, imports, and their relationships.

## What it does

- **Builds a graph** from source code with `nodes` (files, functions, classes, types, modules) and `edges` (imports, contains, defines relationships).
- **Go AST parsing:** Extracts functions and type specs with line numbers using `go/parser`.
- **Python/JS/TS parsing:** Regex-based extraction of classes and functions.
- **Query engine:** Searches nodes by name, path, or type, then finds related nodes via edge traversal (one hop).
- **Statistics:** Reports node/edge counts, type distributions, top imports, and orphan nodes.
- **Export:** Dumps the full graph as JSON for external visualization or analysis.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `SckgCmd` into the root cobra command
- `cmd/sin-code/internal/sckg.go` — self-contained graph builder and query engine
- `cmd/sin-code/internal/discover.go` — reuses `extractDependencies` for import edges
- `cmd/sin-code/internal/grasp.go` — reuses `detectLanguage` and structure extraction concepts
- `cmd/sin-code/internal/map.go` — shares the file-walking and module-detection logic

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--action` | `build` | Action: `build`, `query`, `stats`, `export` |
| `--query` | `""` | Query string (required for `action=query`) |
| `--format` | `text` | Output: `text` or `json` |

- **Graph scope:** Only files with known languages (Go, Python, JS, TS, Rust, Java) are included. Markdown, JSON, text files are excluded.
- **File size limit:** Files >2MB are skipped during graph construction.
- **Query depth:** One hop only. Related nodes are direct neighbors via edges.
- **Top imports limit:** 10 most-imported modules shown in stats.

## Usage examples

```bash
# Build the knowledge graph
sin-code sckg . --action build --format json

# Query for auth-related symbols
sin-code sckg . --action query --query "auth" --format json

# Show graph statistics
sin-code sckg . --action stats

# Export full graph for visualization
sin-code sckg . --action export > graph.json
```

## Known caveats / footguns

- **Graph is rebuilt on every invocation:** There is no persistent graph store. Large codebases (10K+ files) will take noticeable time to rebuild.
- **Query is substring-based, not indexed:** `query` is lowercased and matched with `strings.Contains`. No fuzzy matching or tokenization.
- **Go-only deep parsing:** Functions and types are extracted via AST only for Go. Python/JS/Rust use regex heuristics with lower fidelity.
- **No call-graph edges:** `imports` and `contains` edges exist, but `calls` (function-to-function) is not implemented.
- **Orphan nodes in stats:** Nodes with zero connections. Often legitimate (e.g., unexported utility functions), but may indicate dead code.
- **Memory usage:** The entire graph is held in memory. Very large codebases may hit RAM limits.