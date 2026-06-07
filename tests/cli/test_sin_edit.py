# SPDX-License-Identifier: MIT
"""Purpose: Tests for the `sin-edit` CLI shim.

Docs: test_sin_edit.doc.md

Run as module:
    python -m sin_code_bundle.cli_shims.sin_edit <file> --old "..." --new "..."

Tests cover:
- Successful patch (old→new)
- Nonexistent file (graceful JSON error)
- Anchor not found (graceful JSON error)
- Empty --new (acts as delete)
- --intent flag (recorded in patch metadata)
"""
from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path


def _run_cli(*args: str) -> subprocess.CompletedProcess:
    """Run `python -m sin_code_bundle.cli_shims.sin_edit` with the given args."""
    return subprocess.run(
        [sys.executable, "-m", "sin_code_bundle.cli_shims.sin_edit", *args],
        capture_output=True,
        text=True,
        timeout=30,
    )


def test_sin_edit_successful_patch(tmp_path: Path):
    """A unique anchor is replaced and the file is updated on disk."""
    f = tmp_path / "x.py"
    f.write_text("print('old')\n")
    result = _run_cli(str(f), "--old", "print('old')", "--new", "print('new')")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["success"] is True
    assert "Patch applied" in data["message"]
    assert f.read_text() == "print('new')\n"


def test_sin_edit_nonexistent_file(tmp_path: Path):
    """Editing a missing file returns a JSON error (not a crash)."""
    f = tmp_path / "does_not_exist.py"
    result = _run_cli(str(f), "--old", "x", "--new", "y")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert "error" in data
    assert "not found" in data["error"]


def test_sin_edit_anchor_not_found(tmp_path: Path):
    """An anchor that doesn't match anything returns a graceful error."""
    f = tmp_path / "x.py"
    f.write_text("print('hello')\n")
    result = _run_cli(str(f), "--old", "this string does not exist", "--new", "y")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["success"] is False
    assert "anchor not found" in data["error"]
    # File unchanged
    assert f.read_text() == "print('hello')\n"


def test_sin_edit_empty_new_acts_as_delete(tmp_path: Path):
    """`--new ""` (the default) deletes the anchor from the file."""
    f = tmp_path / "x.py"
    f.write_text("keep\ndelete_me\nkeep\n")
    result = _run_cli(str(f), "--old", "delete_me\n", "--new", "")
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["success"] is True
    # The anchor (and its trailing newline) is gone
    assert "delete_me" not in f.read_text()


def test_sin_edit_intent_recorded(tmp_path: Path):
    """`--intent "..."` is preserved in the patch metadata for auditing."""
    f = tmp_path / "x.py"
    f.write_text("foo\n")
    result = _run_cli(
        str(f), "--old", "foo", "--new", "bar", "--intent", "rename foo → bar"
    )
    assert result.returncode == 0
    data = json.loads(result.stdout)
    assert data["success"] is True
    assert data["intent"] == "rename foo → bar"
    assert data["patch"]["intent"] == "rename foo → bar"


def test_sin_edit_requires_old_flag(tmp_path: Path):
    """Missing --old is a CLI usage error (argparse rejects)."""
    f = tmp_path / "x.py"
    f.write_text("x")
    result = _run_cli(str(f), "--new", "y")
    assert result.returncode != 0
