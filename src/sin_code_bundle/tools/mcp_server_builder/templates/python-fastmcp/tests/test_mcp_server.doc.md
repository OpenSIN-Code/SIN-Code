# `tests/test_mcp_server.py`

## What it does
Smoke tests for every tool defined in `{{ pkg }}.mcp_server`. Each tool
gets a `test_*_returns_dict` and a `test_*_has_tool_field` test.

## Dependencies
- `pytest` (dev dependency)
- `{{ pkg }}.mcp_server` (the tool module)

## Usage
```bash
pytest -q
```
