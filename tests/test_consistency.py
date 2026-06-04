"""Tests for the WS4 cross-repo consistency check."""

from __future__ import annotations

import importlib.util
import subprocess
import sys
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parent.parent
SCRIPT = REPO_ROOT / "scripts" / "check_consistency.py"

# Env-aware skip: the strict test below asserts that the script returns exit
# code 1 because at least one sin-code-* subsystem is missing. In a full
# [all]-extra install every subsystem is present and the script legitimately
# returns 0, which would make the strict-fails assertion meaningless.
_SUBSYSTEMS = (
    "sckg",
    "ibd",
    "poc",
    "efsm",
    "adw",
    "oracle",
    "orchestration",
    "review_interface",
)


def _all_subsystems_installed() -> bool:
    return all(importlib.util.find_spec(f"sin_code_{m}") is not None for m in _SUBSYSTEMS)


SKIP_IF_ALL_SUBSYSTEMS_PRESENT = pytest.mark.skipif(
    _all_subsystems_installed(),
    reason="all 8 sin-code-* subsystems installed in this env — strict-fails contract not exercisable",
)


def test_consistency_script_exists_and_is_executable():
    assert SCRIPT.is_file()


def test_consistency_passes_on_bundle_only_checkout():
    """On a clean bundle-only checkout the script must exit 0 (warnings only)."""
    result = subprocess.run(
        [sys.executable, str(SCRIPT)],
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, result.stdout + result.stderr
    assert "all consistency checks passed" in result.stdout


@SKIP_IF_ALL_SUBSYSTEMS_PRESENT
def test_consistency_strict_fails_without_subsystems():
    """--strict treats missing subsystems as failures (exit 1)."""
    result = subprocess.run(
        [sys.executable, str(SCRIPT), "--strict"],
        capture_output=True,
        text=True,
    )
    assert result.returncode == 1
    assert "not installed (strict)" in result.stdout


def test_version_alignment():
    """pyproject version must equal __init__.__version__."""
    import tomllib

    pyproject = tomllib.loads((REPO_ROOT / "pyproject.toml").read_text())
    declared = pyproject["project"]["version"]
    init_text = (REPO_ROOT / "src" / "sin_code_bundle" / "__init__.py").read_text()
    runtime = next(
        line.split("=", 1)[1].strip().strip('"').strip("'")
        for line in init_text.splitlines()
        if line.startswith("__version__")
    )
    assert declared == runtime
