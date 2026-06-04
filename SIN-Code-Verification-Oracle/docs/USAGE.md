# Usage — Verification Oracle

The package installs the `oracle` command.

## `oracle verify`

Compute an independent verdict from ground-truth signals. Exits non-zero on
`FAIL` so CI / agent loops can gate on it.

```bash
oracle verify --test "pytest -q"
oracle verify --test "pytest -q" --build "python -m compileall ."
oracle verify --test "pytest -q" --no-diagnostics
```

The verdict includes `status` (PASS / FAIL / UNVERIFIED), `verified`, and
`confidence`.

## `oracle diagnostics`

Run available linters / type-checkers as oracles.

```bash
oracle diagnostics --path .
```

Missing tools are reported as `unavailable` rather than failing.

## `oracle trace-capture` / `oracle trace-diff`

Capture observable behavior of a command, then diff a later run against it.

```bash
oracle trace-capture "python app.py --selfcheck" --out before.json
# ... agent edits code ...
oracle trace-diff "python app.py --selfcheck" --before before.json
```

## `oracle eval`

Run an eval suite (SWE-bench-style). Resolution is judged only by the Execution
Oracle, using hidden verification commands.

```bash
oracle eval examples/suite.example.json --label baseline
```

## `oracle serve`

Start the MCP server (requires `pip install -e ".[mcp]"`).

```bash
oracle serve
```

## MCP tools

| Tool | Description |
|------|-------------|
| `verify_change` | Independent verdict; call **before** reporting a task complete. |
| `run_diagnostics` | Run available diagnostics oracles on a path. |

## Wiring a real agent into the harness

`EvalHarness.run_suite(tasks, agent)` accepts any callable
`agent(workspace_path, task)`. Replace the no-op agent in `cli.py:eval` with a
call into your agent, point it at the copied workspace, and track the
resolved-rate across configuration changes.
