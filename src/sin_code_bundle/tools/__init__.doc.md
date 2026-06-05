# Purpose: Companion doc for the tools/ subpackage — one-off maintainer CLIs.
# Docs: tools/__init__.py

# `sin_code_bundle.tools` — Maintainer CLIs

## What it is

This subpackage collects **one-off, maintainer-facing CLIs** that are shipped
with the `sin-code-bundle` wheel but are not part of the agent's MCP tool
surface. They are invoked from a terminal by a human or a CI script.

Each module is a single-file, dependency-light script that uses only
Python's standard library so it works in minimal environments (no
`sin-code-bundle[all]` install required, no third-party HTTP clients).

## Why a subpackage?

Previously these helpers lived in `tools/` at the repo root (a bash script
alongside a `Makefile`). Two issues:

1. They were not on the import path of the published wheel, so users who
   installed from PyPI could not run them without first cloning the repo.
2. They mixed bash and Python, which made them harder to test and port
   to Windows.

Moving them into `sin_code_bundle.tools.*` fixes both: `python -m
sin_code_bundle.tools.pypi_setup --help` works anywhere the wheel is
installed, and the codebase gets a single language (Python) and a single
test surface (`pytest tests/test_tools_*.py`).

## Modules

| Module | Purpose | One-liner |
|--------|---------|-----------|
| `pypi_setup` | Register a PyPI Trusted Publisher | `python -m sin_code_bundle.tools.pypi_setup --api-token pypi-...` |

## Tests

- `tests/test_pypi_setup.py` — payload builder, arg parsing, HTTP
  response handling (mocked, no real PyPI call).

## Notes

- These are NOT registered in `pyproject.toml [project.scripts]` — they
  are invoked via `python -m sin_code_bundle.tools.<name>`, not as
  top-level commands. This keeps the wheel's `bin/` namespace clean and
  reserved for the agent-facing `sin`, `sin-serve`, `sin-serve-mcp`
  entry points.
- Each module is a *standalone* script: the only dependency is the
  Python standard library. No `requests`, no `httpx`, no `sin-brain`.
  This means the maintainer can run it in any Python 3.11+ env, even
  a stripped-down `python:3.11-slim` Docker image.
