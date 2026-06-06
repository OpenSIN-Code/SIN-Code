# `{{ pkg }}/__init__.py`

## What it does
Package entry point for the `{{ name }}` MCP server. Exposes the package
version; the FastMCP server lives in `mcp_server.py`.

## Dependencies
- `mcp_server.py` (the actual FastMCP server)

## Usage
```python
from {{ pkg }} import __version__
print(__version__)
```
