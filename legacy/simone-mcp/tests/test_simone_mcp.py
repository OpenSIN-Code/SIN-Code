"""Security regression tests for Simone file tools."""

from __future__ import annotations

from pathlib import Path

from simone_mcp.core import edit_file, patch_file, read_file, write_file


def test_file_tools_allow_paths_inside_workspace(
    tmp_path: Path,
    monkeypatch,
) -> None:
    workspace = tmp_path / "workspace"
    workspace.mkdir()
    monkeypatch.chdir(workspace)

    target = workspace / "notes.txt"
    written = write_file(
        {
            "path": str(target),
            "content": "hello world\n",
            "overwrite": True,
        }
    )
    assert written["ok"] is True

    edited = edit_file(
        {
            "path": str(target),
            "old_string": "world",
            "new_string": "Simone",
        }
    )
    assert edited["ok"] is True

    read = read_file({"path": str(target)})
    assert read["ok"] is True
    assert "hello Simone" in read["content"]


def test_file_tools_reject_paths_outside_workspace(
    tmp_path: Path,
    monkeypatch,
) -> None:
    workspace = tmp_path / "workspace"
    workspace.mkdir()
    monkeypatch.chdir(workspace)
    outside = tmp_path / "outside.txt"
    outside.write_text("secret\n", encoding="utf-8")

    attempts = [
        write_file(
            {
                "path": str(outside),
                "content": "overwritten",
                "overwrite": True,
            }
        ),
        read_file({"path": str(outside)}),
        edit_file(
            {
                "path": str(outside),
                "old_string": "secret",
                "new_string": "changed",
            }
        ),
        patch_file(
            {
                "path": str(outside),
                "diff": "@@ -1,1 +1,1 @@\n-secret\n+changed\n",
            }
        ),
    ]

    assert all(result["ok"] is False for result in attempts)
    assert outside.read_text(encoding="utf-8") == "secret\n"
