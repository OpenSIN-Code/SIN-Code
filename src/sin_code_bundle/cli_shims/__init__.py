"""Purpose: CLI shim package — exposes MCP-only file-ops as real binaries.

Docs: __init__.doc.md

Each sub-module here is a thin argparse wrapper around the functions in
`sin_code_bundle.file_ops`. The MCP server (`mcp_server.py`) uses the
SAME underlying functions, so behavior is identical between the two
surfaces.

Exposed as console scripts (see pyproject.toml [project.scripts]):
    sin-read    → cli.sin_read:main
    sin-write   → cli.sin_write:main
    sin-edit    → cli.sin_edit:main
    sin-bash    → cli.sin_bash:main
    sin-search  → cli.sin_search:main
"""
from __future__ import annotations
