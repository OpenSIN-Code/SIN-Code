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

## `sin code` — Unified Coding Workflow Hub (v1.1.0+)

A single shortcut entry point for the full SIN-Code coding workflow. Routes to
the underlying subcommand and translates positional args where needed.

```bash
sin code <action> [args...]
```

| Action | Routes to | Use case |
|--------|-----------|----------|
| `review` | `sin review` | Semantic review (IBD) |
| `debt` | `sin debt` (positional → `--root`) | Architectural debt |
| `verify` | `sin verify` | Proof-of-correctness (Oracle) |
| `preflight` | `sin preflight` | GitNexus index check |
| `preflight-write` | `sin preflight-write` | Pre-write safety gate |
| `codocs` | `sin codocs` | CoDocs validation |
| `sckg` | `sin sin-code run scout` | Knowledge graph |
| `audit` | `sin ceo-audit` | 47-gate repo audit |
| `discover` / `scout` / `grasp` / `map` / `harvest` | `sin sin-code run <tool>` | Go tool runners |
| `full` | preflight + codocs + debt | Full review pipeline |

Aliases: `oracle=verify`, `adw=debt`, `ibd=review`.

### Examples

```bash
# File discovery
sin code discover

# Architectural debt (positional path works as --root)
sin code debt .

# Verify with Oracle
sin code verify "pytest tests/"

# Full review pipeline (preflight + codocs + debt)
sin code full
```

### `sin code full` pipeline

Runs in order:
1. `preflight` — GitNexus index freshness check
2. `codocs check .` — CoDocs validation
3. `debt --root .` — Architectural debt analysis

Continues even if a step fails (exits 0 with `WARN` lines). Use individual
`sin code <action>` calls for strict CI behavior.

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
5. **Shortcut:** Use `sin code <action>` for any of the above without remembering
   which subcommand does what. `sin code full` runs the whole pre-commit checklist.
