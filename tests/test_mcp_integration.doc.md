# tests/test_mcp_integration.py

## What
End-to-end integration tests for the SIN-Code MCP tools. Verifies the
public surface of `dap_bridge`, `interceptor`, and `orchestration_worktrees`
behave correctly when exercised as a real client would.

## Why
Unit tests cover internals; this module proves the MCP-exposed surface
keeps its contract so the `sin serve` MCP server stays stable.

## Coverage
- `SINInterceptor` — architectural rule enforcement
- `DAPSession` / `SINRuntimeTrace` — debug-adapter lifecycle
- `SINWorktreeOrchestrator` — git worktree orchestration primitives

## Running
```bash
pytest tests/test_mcp_integration.py -v
```

## Caveats
- Imports `sin_code_bundle` from `src/`, so `PYTHONPATH` is added at runtime
