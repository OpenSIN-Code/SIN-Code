# Complexity Audit

`sin-code audit complexity` is a repo-wide, ponytail-audit-style complexity scan. It lists candidates for reduction, ranks them, and respects `// sin-debt:` markers.

## Tags

| Tag | Meaning | Example |
|-----|---------|---------|
| `delete` | Dead code or unused flexibility | Unread flag/config variable |
| `stdlib` | Hand-rolled thing the stdlib ships | Custom `contains` instead of `strings.Contains` |
| `native` | Dependency or code the platform already ships | Custom slice reverse instead of `slices.Reverse` |
| `yagni` | Abstraction with one implementation | Interface with a single implementation |
| `shrink` | Same logic, fewer lines | Wrapper that only delegates, single-export file |

## Output Format

```text
<tag> <what to cut>. <replacement>. [path]
```

Findings are followed by:

```text
net: -<N> lines, -<M> deps possible.
```

or, when no findings remain:

```text
Lean already. Ship.
```

## CLI

```bash
sin-code audit complexity
sin-code audit complexity ./cmd/sin-code
sin-code audit complexity --format json
sin-code audit complexity --tags yagni,delete
sin-code audit complexity --rank deps
sin-code audit complexity --strict --max-net-lines 1000
```

## sin-debt markers

A `// sin-debt:` comment above a finding marks it as approved and removes it from the net total:

```go
// sin-debt: legacy shim
type Reader interface { Read() }
```

The output appends `(approved: sin-debt marker <reason>)` and the line is excluded from `net:`.

## CEO-audit integration

`sin-code ceo-audit` runs the complexity scan as gate 48. Score contribution: `+1` per 100 removable lines (rounded down).

```bash
sin-code ceo-audit .
sin-code ceo-audit . --strict --max-net-lines 500
```

## Static pass

The deterministic static pass (no LLM) detects:

- Single-implementation interfaces (`yagni`)
- `New*` factory functions with a single caller assumption (`yagni`)
- Functions that only return the result of another call (`shrink`)
- Files exporting exactly one top-level symbol (`shrink`)
- Unread variables whose names contain `Flag` or `Config` (`delete`)
- Common hand-rolled helpers (`stdlib`)

The optional LLM second pass (`--no-llm` to disable) only receives the top-N static findings.

## Markdown output

`--format markdown` emits a table with tag, problem, replacement, path, and estimated lines.

## JSON output

`--format json` returns `audit.Result`:

```json
{
  "findings": [
    {
      "tag": "yagni",
      "problem": "interface Reader has only one likely implementation",
      "replacement": "inline Reader as concrete type",
      "path": "...",
      "line": 5,
      "line_count": 1,
      "approved": false
    }
  ],
  "net_lines": 1,
  "deps_removable": 0,
  "status": "net: -1 lines, -0 deps possible."
}
```
