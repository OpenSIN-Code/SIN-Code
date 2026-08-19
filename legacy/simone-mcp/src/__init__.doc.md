# `__init__.py` (src/) — Simone MCP Top-level Package

What this file does: empty package marker for the `src/` namespace. The actual package lives at `src/simone_mcp/`. This file is intentionally empty so both `import src` and `import simone_mcp` work.

## Dependency map

- Imports: nothing.
- Imported by: `setuptools` (build system), `simone_mcp` (nested package).

## Caveats / footguns

- Adding code here is almost always a mistake — the real exports live in `src/simone_mcp/__init__.py`.
