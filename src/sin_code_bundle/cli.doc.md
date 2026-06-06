# cli.py

Unified `sin` CLI (Typer-based) for the entire SIN-Code stack. Each
subcommand lazy-imports the underlying subsystem so a missing
optional dependency (e.g. `sin_code_sckg`) does not break the rest
of the CLI.

## Dependencies

- `typer` (required) — CLI framework
- `shutil` (stdlib) — for `which()` lookups
- Optional subsystems imported on demand: `sin_code_sckg`, `sin_code_ibd`,
  `sin_code_adw`, `sin_code_oracle`, `sin_code_poc`, `sin_code_efsm`,
  `sin_code_orchestration`, `sin_brain`, `mcp`

## Touched by

- The `sin` console script entry point (defined in `pyproject.toml`)
- `install.sh` — invokes `sin gitnexus setup`, `sin markitdown setup`,
  `sin rtk setup` after install
- `~/.config/opencode/opencode.json` — the `mcp.serve` entry can spawn
  this CLI in stdio mode

## What it does

The CLI is split into several sub-`Typer` apps, each with its own
command tree:

| Sub-app | Purpose | Key commands |
|---------|---------|--------------|
| `sin status` / `bootstrap` / `review` / `debt` / `verify` | top-level orchestration | status, bootstrap, review, debt, verify, preflight, doctor |
| `sin code …` | **Unified coding workflow hub** (v1.1.0) | review, debt, verify, preflight, codocs, sckg, audit, oracle, adw, ibd, discover, scout, grasp, map, harvest, full |
| `sin gitnexus …` | GitNexus bridge | doctor, setup, index, status, context, impact, ai-context |
| `sin markitdown …` | MarkItDown bridge | doctor, setup, convert |
| `sin rtk …` | RTK bridge | doctor, setup, gain |
| `sin codocs …` | CoDocs validator | check, check-inline, list, install-skill |
| `sin sin-code …` | Go-tool runner | run, agents-md |
| `sin ceo-audit …` | CEO audit | run, install, status |
| `sin mcp-config` | emit / merge MCP client configs |
| `sin agents-md` | upsert AGENTS.md |
| `sin serve` | start the unified MCP server (stdio) |
| `sin bench` | run the A/B benchmark |
| `sin hooks-install` / `hooks-uninstall` / `hooks-list` | opencode hooks |
| `sin skills` | compile `skills/*.md` to a target agent |
| `sin policy` | view/edit `.sin/policy.yaml` |

## `sin code` Unified Hub (v1.1.0+)

A single shortcut entry point for the full SIN-Code coding workflow.
Routes to the underlying subcommand and translates positional args
where needed (e.g. `sin debt <path>` → `sin debt --root <path>`).

| Action | Routes to | Aliases |
|--------|-----------|---------|
| `review` | `sin review` | `ibd` |
| `debt` | `sin debt` (positional → `--root`) | `adw` |
| `verify` | `sin verify` | `oracle` |
| `preflight` | `sin preflight` | — |
| `preflight-write` | `sin preflight-write` | — |
| `codocs` | `sin codocs` | — |
| `sckg` | `sin sin-code run scout` | — |
| `audit` | `sin ceo-audit` | — |
| `discover` / `scout` / `grasp` / `map` / `harvest` | `sin sin-code run <tool>` | — |
| `full` | Pipeline: preflight + codocs + debt | — |

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

# Knowledge graph (SCKG)
sin code sckg stats
```

### `sin code full` pipeline

Runs in order:
1. **`preflight`** — GitNexus index freshness check
2. **`codocs check .`** — CoDocs validation (.doc.md companion files)
3. **`debt --root .`** — Architectural debt analysis (ADW)

Continues even if a step fails (exit code 0 at the end with `WARN` lines
for failed steps). Use individual `sin code <action>` calls for strict
CI behavior.

## Important constants

- `app` — root `Typer()`
- `gitnexus_app`, `markitdown_app`, `rtk_app`, `codocs_app`,
  `sin_code_app`, `ceo_audit_app` — sub-`Typer()`s
- `_SIN_CODE_TOOLS` — mapping of Go binary name → upstream repo name
- `_EXCLUDE = {"venv", ".venv", "node_modules", ".git", "__pycache__"}` —
  passed to the analysis subsystems to skip junk dirs

## Key helpers

- `_sin_code_tool_path(name)` — returns `~/.local/bin/<name>` if it
  exists, else `which(name)`, else `None`
- `_require(module, hint)` — import a subsystem or `typer.Exit(1)`
  with a clear install hint
- `_normalize_root_flag(args)` — converts positional path → `--root` flag
  for commands like `debt` that take `--root` not positional

## Usage

```bash
# Show which subsystems are installed
sin status

# Wire GitNexus into every supported agent
sin gitnexus setup

# Start the unified MCP server (stdio; used by opencode/codex)
sin serve

# Unified coding workflow (NEW in v1.1.0)
sin code full
sin code debt .
sin code review file_a.py file_b.py
```

## Known caveats

- The `serve` command registers tools defensively: missing subsystems
  produce a silent skip (no `mcp.tool` registration), so an agent
  calling `impact` against a host without `sin_code_sckg` will get
  a `Tool not found` error from the MCP client, not a Python traceback.
- `_require()` exits with `typer.Exit(1)` on `ImportError`. Run inside
  CI by checking `$?` after each call.
- `sin code <action>` is a thin wrapper that `subprocess.run`s the
  underlying `sin <full-cmd>`. It inherits the underlying exit code.
- `sin code full` continues on errors — use individual `sin code <action>`
  calls for strict CI behavior.
- Section separators and command groups inside this file are a
  convenience for human readers; the order of `@app.command()`
  decorators is the order `sin --help` lists them.
