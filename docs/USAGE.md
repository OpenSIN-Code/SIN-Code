# Usage — SIN-Code Bundle

The package installs the `sin` command. Every command degrades gracefully: if a
required subsystem is not installed, `sin` reports it instead of crashing.

## `sin status`

Show which subsystems are installed and importable.

```bash
sin status
```

## `sin bootstrap [repo]`

Initialize available subsystems for a repository: build the knowledge graph,
record a complexity baseline, and initialize the cost ledger under `.sin/`.

```bash
sin bootstrap .
sin bootstrap ./my-project
```

## `sin review <file_a> <file_b>`

Semantic review of a change using IBD (intents + risk).

```bash
sin review before.py after.py
```

## `sin verify <module> <function>`

Proof-of-correctness for a function using POC.

```bash
sin verify mymodule.py my_function
```

## `sin debt [root]`

Architectural debt overview using ADW.

```bash
sin debt .
```

## `sin serve`

Run the unified MCP server. Tools are registered only for subsystems that are
installed.

```bash
sin serve
```

### Unified MCP tools

| Tool | Backing subsystem |
|------|-------------------|
| `impact` | SCKG |
| `semantic_diff` | IBD |
| `architectural_debt` | ADW |

## Recommended agent workflow

1. `sin bootstrap .` once per repo.
2. Agent queries `impact` / `semantic_diff` while planning and editing.
3. Before reporting done, the agent calls the Oracle's `verify_change`.
4. `sin debt` and the ADW circuit breaker keep cost/complexity in check.
