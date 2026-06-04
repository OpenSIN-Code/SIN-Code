# Configuration — SCKG

SCKG reads `config.yaml` from the current directory (or `.sin/config.yaml`). If
no file is found, sensible defaults are used.

## Full reference

```yaml
repository:
  root: "."                 # directory to parse
  exclude:                  # directories to skip
    - node_modules
    - .git
    - __pycache__
    - venv
    - .venv

languages:                  # which Tree-sitter grammars to load
  - python
  - javascript
  - typescript

graph:
  storage: "./.sin/knowledge.graph"   # where the JSON graph is persisted
  temporal_depth: 50                  # number of commits to analyze for intent
  include_intent: true                # parse commit messages into intent nodes

mcp:
  host: "127.0.0.1"
  port: 8765
```

## Fields

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `repository.root` | string | `"."` | Root directory to parse. |
| `repository.exclude` | list | `[]` | Directory names/prefixes to ignore. |
| `languages` | list | python, js, ts | Grammars to load. A missing grammar is skipped with a warning. |
| `graph.storage` | string | `./.sin/knowledge.graph` | Persisted graph location. |
| `graph.temporal_depth` | int | `50` | How many recent commits to mine for intent. |
| `graph.include_intent` | bool | `true` | Whether to add Git-intent nodes/edges. |
| `mcp.host` / `mcp.port` | string/int | 127.0.0.1 / 8765 | MCP server bind address. |

## Notes

- The graph is stored as `networkx` node-link JSON. Delete `.sin/knowledge.graph`
  to force a full rebuild.
- Git-intent extraction is best-effort: if the directory is not a Git
  repository, intent nodes are simply skipped.
