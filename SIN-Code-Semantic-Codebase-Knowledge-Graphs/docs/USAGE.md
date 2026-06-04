# Usage — SCKG

## CLI

The package installs the `sckg` command.

### `sckg build`

Parse a repository and persist the knowledge graph.

```bash
sckg build                 # uses config.yaml (root + excludes)
sckg build --root ./src    # override the root
sckg build --verbose
```

### `sckg find <name>`

Find every symbol whose name matches.

```bash
sckg find KnowledgeGraph
```

Output is a JSON list of `{id, name, kind, file, line_start, ...}`.

### `sckg impact <fqid>`

Blast-radius / impact analysis for a fully-qualified symbol id
(`<file>:<kind>:<name>`).

```bash
sckg impact "src/sin_code_sckg/graph.py:class:KnowledgeGraph"
```

Returns upstream/downstream counts, affected files, and a `risk_score` (0..1).

### `sckg arch`

High-level architecture overview: total nodes/edges and the top hubs by
out-degree.

```bash
sckg arch
```

### `sckg serve`

Run the MCP server (requires the `mcp` optional dependency).

```bash
sckg serve
```

## Python API

```python
from sin_code_sckg import KnowledgeGraph

kg = KnowledgeGraph(storage_path=".sin/knowledge.graph")
kg.build_from_repo(".", exclude=["venv", "node_modules", ".git"])

print(kg.find_symbol("parse"))
print(kg.impact_analysis("src/x.py:function:parse"))
print(kg.explain_architecture(top_k=10))
```

## MCP tools

When run via `sckg serve`, the following tools are exposed to the agent:

| Tool | Description |
|------|-------------|
| `find_symbol` | Find a symbol by name. |
| `impact_analysis` | Blast radius for a fully-qualified symbol id. |
| `architecture_overview` | Hubs and global graph stats. |
| `downstream_deps` | What uses this symbol (downstream dependencies). |
