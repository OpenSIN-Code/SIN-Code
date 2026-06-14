# sin-verify.yml

Pull-request verification workflow for the Python `sin-code-bundle` package still shipped in this repo.

## What this workflow does

1. Installs the local Python package (`pip install -e ".[dev]"`) so the tests run against the code in the PR, not the released PyPI version.
2. Runs the Python test suite with `pytest -q`.
3. Verifies the audit-chain integrity via `sin_code_bundle.policy.AuditLog`.

## Related files

- `pyproject.toml` — Python package definition for `sin-code-bundle`.
- `tests/` — Python test suite.
- `requirements-ecosystem.txt` — optional ecosystem packages installed separately.

## Caveats

- This package is the legacy Python bundle; the active runtime is the Go `sin-code` binary. The workflow is kept for regression coverage of the Python helpers.
- Installing from PyPI (`pip install sin-code-bundle[dev]`) would test the released package, not the PR. The workflow intentionally uses `-e ".[dev]"` to test the checked-out source.
