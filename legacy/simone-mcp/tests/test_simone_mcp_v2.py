"""Additional workspace-boundary tests for Simone file tools."""

from __future__ import annotations

from pathlib import Path

import pytest

from simone_mcp.core import read_file, write_file


def test_explicit_root_allows_a_different_workspace(tmp_path: Path) -> None:
    workspace = tmp_path / "project"
    workspace.mkdir()
    target = workspace / "result.txt"

    result = write_file(
        {
            "root": str(workspace),
            "path": str(target),
            "content": "verified\n",
            "overwrite": True,
        }
    )

    assert result["ok"] is True
    assert target.read_text(encoding="utf-8") == "verified\n"


def test_explicit_root_blocks_parent_escape(tmp_path: Path) -> None:
    workspace = tmp_path / "project"
    workspace.mkdir()
    outside = tmp_path / "outside.txt"
    outside.write_text("outside\n", encoding="utf-8")

    result = read_file(
        {
            "root": str(workspace),
            "path": str(workspace / ".." / "outside.txt"),
        }
    )

    assert result["ok"] is False
    assert "outside workspace" in result["error"]


def test_unapproved_workspace_root_is_rejected() -> None:
    result = read_file(
        {
            "root": "/",
            "path": "/etc/hosts",
        }
    )

    assert result["ok"] is False
    assert "outside allowed roots" in result["error"]


def test_symlink_escape_is_blocked(tmp_path: Path) -> None:
    workspace = tmp_path / "project"
    workspace.mkdir()
    outside = tmp_path / "outside.txt"
    outside.write_text("outside\n", encoding="utf-8")
    link = workspace / "linked.txt"

    try:
        link.symlink_to(outside)
    except (OSError, NotImplementedError):
        pytest.skip("symlinks are unavailable on this platform")

    result = read_file(
        {
            "root": str(workspace),
            "path": str(link),
        }
    )

    assert result["ok"] is False
    assert "outside workspace" in result["error"]
