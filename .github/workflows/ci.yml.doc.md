# ci.yml

Python CI for the `sin-code-bundle` package still shipped in this repo.

## What this workflow does

- **lint**: Runs `ruff check .` and `ruff format --check .`.
- **test**: Installs the package with dev extras and runs `pytest -q` on Python 3.11, 3.12, and 3.13. Then installs optional extras (`lsp`, `bench`, `mcp`) and re-runs tests.
- **consistency**: Runs `scripts/check_consistency.py` (non-blocking).

## Related files

- `pyproject.toml` — Python package definition and `[dev]` extras.
- `tests/` — Python test suite.
- `scripts/check_consistency.py` — cross-repo consistency checker.

## Caveats

- Several tests create temporary git commits, so the workflow configures a global git identity and sets `init.defaultBranch` to `main` before running pytest.
- Some Python tests exercise the standalone `execute` and `scout` Go binaries. The workflow builds the local `sin-code` binary and installs thin wrapper scripts (`execute`, `scout`) so the legacy Python shims can find the tools.
- The optional extras install uses `|| echo ...` so the workflow continues even if optional dependencies fail.
