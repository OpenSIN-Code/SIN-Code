# SPDX-License-Identifier: MIT
"""Shared pytest fixtures and import-path bootstrap for sin_code_bundle tests.

Run from this directory:

    python3 -m pytest test_*.py -v --tb=short

or from the repo root:

    python3 -m pytest src/sin_code_bundle/test_*.py -v --tb=short
"""

from __future__ import annotations

import os
import sys
from pathlib import Path

import pytest

# ── Import-path bootstrap ────────────────────────────────────────────────
# Ensure the parent ``src`` directory is on sys.path so that
# ``import sin_code_bundle`` works even when the package is not
# pip-installed in the current environment (e.g. running from a
# source checkout on CI).
_SRC_DIR = Path(__file__).resolve().parent.parent
if str(_SRC_DIR) not in sys.path:
    sys.path.insert(0, str(_SRC_DIR))


# ── Fixtures ─────────────────────────────────────────────────────────────


@pytest.fixture()
def tmp_workspace(tmp_path: Path) -> Path:
    """A clean temporary directory that serves as a simulated workspace root."""
    return tmp_path


@pytest.fixture()
def tmp_toml(tmp_path: Path) -> Path:
    """Path to a non-existent TOML file inside a temp dir (safe to create)."""
    return tmp_path / "config.toml"


@pytest.fixture()
def env_clean(monkeypatch: pytest.MonkeyPatch) -> pytest.MonkeyPatch:
    """Remove all SIN_* environment variables for the duration of the test.

    Ensures config tests are deterministic regardless of the host's
    ambient environment.
    """
    for key in list(os.environ):
        if key.startswith("SIN_"):
            monkeypatch.delenv(key, raising=False)
    return monkeypatch


@pytest.fixture()
def sample_toml(tmp_path: Path) -> Path:
    """A TOML file with known values for config-merge tests."""
    p = tmp_path / "sample.toml"
    p.write_text(
        '[tui]\ntheme = "dark"\nhistory_size = 1000\n\n'
        '[opencode]\nmodel = "gpt-4o"\napi_key = "sk-secret-123"\n',
        encoding="utf-8",
    )
    return p


@pytest.fixture()
def sample_opencode_json(tmp_path: Path) -> Path:
    """An opencode.json file with a ``sin`` sub-object."""
    p = tmp_path / "opencode.json"
    p.write_text(
        '{"sin": {"model": "claude-3.5-sonnet"}, "other": {"foo": "bar"}}',
        encoding="utf-8",
    )
    return p


@pytest.fixture()
def py_file(tmp_path: Path) -> Path:
    """A minimal valid Python file for file-ops round-trip tests."""
    p = tmp_path / "sample.py"
    p.write_text("print('hello')\n", encoding="utf-8")
    return p
