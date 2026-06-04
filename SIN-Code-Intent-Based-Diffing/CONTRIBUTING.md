# Contributing to `sin-code-ibd`

Thanks for your interest in improving this part of the SIN-Code stack.

## Development setup

```bash
git clone https://github.com/OpenSIN-Code/SIN-Code-Intent-Based-Diffing.git
cd SIN-Code-Intent-Based-Diffing
python -m venv .venv && source .venv/bin/activate
pip install -e .
pip install pytest
```

## Before opening a pull request

1. Keep changes focused; one logical change per PR.
2. Add or update tests for any behavioral change.
3. Make sure the suite is green:
   ```bash
   pytest -q
   ```
4. Keep public APIs stable, or document breaking changes in `CHANGELOG.md`.
5. Follow the existing code style (type hints, `from __future__ import annotations`).

## Design principles

- **Graceful degradation:** never hard-crash when an optional external tool is
  missing — report it as unavailable and continue.
- **No silent success:** prefer an explicit "unknown/unverified" result over a
  misleading "pass".
- **Small, composable units:** each module should be usable on its own and via
  the `ibd` CLI.

## Reporting issues

Open an issue at
https://github.com/OpenSIN-Code/SIN-Code-Intent-Based-Diffing/issues with a minimal reproduction.
