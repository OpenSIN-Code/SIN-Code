from __future__ import annotations

from pathlib import Path

from simone_mcp.protocol import _read_resource


def test_file_resource_stays_inside_workspace(tmp_path: Path) -> None:
    workspace = tmp_path / "workspace"
    workspace.mkdir()
    inside = workspace / "inside.txt"
    inside.write_text("inside", encoding="utf-8")
    outside = tmp_path / "outside.txt"
    outside.write_text("outside", encoding="utf-8")

    allowed = _read_resource("file:///inside.txt", str(workspace))
    escaped = _read_resource("file:///../outside.txt", str(workspace))

    assert allowed is not None
    assert allowed["text"] == "inside"
    assert escaped is None


def test_source_resource_ignores_untrusted_base(tmp_path: Path) -> None:
    workspace = tmp_path / "workspace"
    workspace.mkdir()
    inside = workspace / "module.py"
    inside.write_text("VALUE = 1\n", encoding="utf-8")
    outside = tmp_path / "secret.py"
    outside.write_text("SECRET = True\n", encoding="utf-8")

    allowed = _read_resource(
        "source://untrusted-base/module.py",
        str(workspace),
    )
    escaped = _read_resource(
        "source://untrusted-base/../secret.py",
        str(workspace),
    )

    assert allowed is not None
    assert "VALUE = 1" in allowed["text"]
    assert escaped is None
