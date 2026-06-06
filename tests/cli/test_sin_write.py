"""Purpose: Tests for the `sin-write` CLI shim.

Docs: test_sin_write.doc.md

Run as module:
    python -m sin_code_bundle.cli_shims.sin_write <path> --content "..."

Tests cover:
- New file (success, no backup)
- Existing file (success, backup created)
- Invalid Python (refused, file not written, backup restored)
- --content-from-file (read from disk)
- --no-verify (skip syntax check)
"""
from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path


def _run_cli(*args: str) -> subprocess.CompletedProcess:
    """Run `python -m sin_code_bundle.cli_shims.sin_write` with the given args."""
    return subprocess.run(
        [sys.executable, "-m", "sin_code_bundle.cli_shims.sin_write", *args],
        capture_output=True,
        text=True,
        timeout=30,
    )


def test_sin_write_new_file(tmp_path: Path):
    """Writing a new file returns success, no backup."""
    f = tmp_path / "out.txt"
    result = _run_cli(str(f), "--content", "hello")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["success"] is True
    assert data["chars"] == 5
    assert data["backup"] is None
    assert f.read_text() == "hello"


def test_sin_write_existing_file_creates_backup(tmp_path: Path):
    """Overwriting an existing file creates a .bak before replacement."""
    f = tmp_path / "out.txt"
    f.write_text("original")
    result = _run_cli(str(f), "--content", "new")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["success"] is True
    assert data["backup"] == str(f) + ".bak"
    assert f.read_text() == "new"
    # .bak contains the original content
    assert (tmp_path / "out.txt.bak").read_text() == "original"


def test_sin_write_invalid_python_refused(tmp_path: Path):
    """A .py file with broken syntax is refused and NOT written to disk."""
    f = tmp_path / "broken.py"
    f.write_text("original_valid")
    result = _run_cli(str(f), "--content", "def broken(:")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["success"] is False
    assert "syntax error" in data["error"]
    # The original file is preserved (atomic restore from .bak)
    assert f.read_text() == "original_valid"


def test_sin_write_no_verify_skips_check(tmp_path: Path):
    """`--no-verify` allows writing broken Python without SyntaxError check."""
    f = tmp_path / "broken.py"
    result = _run_cli(str(f), "--content", "def broken(:", "--no-verify")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["success"] is True
    assert data["verified"] is False
    assert f.read_text() == "def broken(:"


def test_sin_write_content_from_file(tmp_path: Path):
    """`--content-from-file` reads payload from disk (avoids argv limits)."""
    src = tmp_path / "src.txt"
    src.write_text("payload from file")
    dst = tmp_path / "dst.txt"
    result = _run_cli(str(dst), "--content-from-file", str(src))
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["success"] is True
    assert dst.read_text() == "payload from file"


def test_sin_write_content_from_stdin(tmp_path: Path):
    """`--content-from-file -` reads payload from stdin."""
    dst = tmp_path / "from_stdin.txt"
    result = subprocess.run(
        [
            sys.executable,
            "-m",
            "sin_code_bundle.cli_shims.sin_write",
            str(dst),
            "--content-from-file",
            "-",
        ],
        input="stdin payload",
        capture_output=True,
        text=True,
        timeout=30,
    )
    assert result.returncode == 0
    assert dst.read_text() == "stdin payload"


def test_sin_write_requires_content_flag(tmp_path: Path):
    """Missing --content / --content-from-file is a CLI usage error."""
    f = tmp_path / "out.txt"
    result = _run_cli(str(f))
    # argparse exits non-zero when a required --mutually-exclusive group is empty
    assert result.returncode != 0
