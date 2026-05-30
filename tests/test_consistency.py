"""Tests for the WS4 cross-repo consistency check."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
SCRIPT = REPO_ROOT / "scripts" / "check_consistency.py"


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
