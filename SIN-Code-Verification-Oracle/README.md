# SIN-Code Verification Oracle (Repo 6)

The missing piece of the SIN-Code stack: an **independent, execution-based
verification layer** that answers the one question agents systematically get
wrong — *"Did it actually work?"* — without trusting the agent's self-report.

Where the other five repos are mostly **static analysis** (the signal agents
already have the most of), this repo provides **ground truth**: compiler
output, real test runs, real HTTP responses, and behavioral diffs of actual
execution. Plus an **eval harness** so you can *measure* whether your stack
improves results instead of believing it does.

## Why this matters

| Failure mode | Caught by | Caught here |
|---|---|---|
| Hallucinated "done", nothing runs | nobody | Execution Oracle |
| Type error introduced | maybe linter | Diagnostics Oracle |
| Tests pass but API response silently changed | nobody | Trace-Diff |
| "Is the stack even helping?" | nobody | Eval Harness |

## Components

- **`diagnostics.py`** — Adapts existing language servers/compilers/linters
  (pyright, ruff, tsc) as oracles. Degrades gracefully when a tool is absent.
  This is the cheapest, strongest correctness signal — and reuses battle-tested
  tools instead of re-implementing weaker AST checks.
- **`execution.py`** — Ground-truth runner: arbitrary commands, parsed pytest
  counts, and HTTP probes that boot a server, wait for the port, and assert on
  real responses.
- **`trace_diff.py`** — Captures observable behavior (stdout, exit code,
  artifacts, structured events) and diffs two runs, with noise normalization
  (timestamps/uuids/addresses) for determinism.
- **`oracle.py`** — Combines signals into a single `Verdict`. Refuses to return
  `PASS` when it had no ground truth (returns `UNVERIFIED` instead).
- **`eval_harness.py`** — SWE-bench-style runner. Tasks have *hidden*
  verification commands the agent never sees; resolution is judged purely by
  the Execution Oracle.

## Install

```bash
cd SIN-Code-Verification-Oracle
pip install -e .
# optional, for MCP server:
pip install -e '.[mcp]'
# install the language tools you want as oracles:
pip install pyright ruff   # npm i -g typescript  for tsc
```

## Usage

```bash
# Independent verdict — exits non-zero on FAIL so CI/agent loops can gate on it
oracle verify --test pytest --build "python -m compileall ."

# Just the diagnostics oracle
oracle diagnostics .

# Behavioral trace diff: capture before the edit, diff after
oracle trace-capture "python app.py --selfcheck" --out before.json
# ... agent edits code ...
oracle trace-diff "python app.py --selfcheck" --before before.json

# Measure your agent against a suite (baseline shown with no-op agent)
oracle eval examples/suite.example.json --label baseline
```

## MCP integration

```yaml
# ~/.config/opencode/config.yaml (or Codex/Hermes equivalent)
mcpServers:
  sin-code-oracle:
    command: oracle
    args: [serve]
```

The key tool is `verify_change`: agents should call it **before** reporting a
task complete. The returned `Verdict` includes `verified` and `confidence`, so
the agent knows how much to trust it.

## Wiring your real agent into the eval harness

`EvalHarness.run_suite(tasks, agent)` takes any callable
`agent(workspace_path, task)`. Replace the no-op in `cli.py:eval` with a call
into your agent (OpenCode/Codex/Hermes), point it at the copied `workspace`,
and the harness reports resolved-rate. Track that number across config changes.

## Design principles

1. **The agent's self-report is never an input.** Only ground truth counts.
2. **Refusing to confirm is a feature.** No signal → `UNVERIFIED`, not `PASS`.
3. **Reuse, don't re-implement.** Compilers/type-checkers are better oracles
   than anything we'd hand-roll.
4. **Measure everything.** If the eval number doesn't move, the feature didn't
   help — regardless of how clever it is.
```
