from pathlib import Path

from typer.testing import CliRunner

from sin_code_bundle import codocs
from sin_code_bundle.cli import app

runner = CliRunner()


def _write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def test_scan_finds_valid_reference(tmp_path):
    _write(tmp_path / "router.py", "# Docs: router.doc.md\n\nx = 1\n")
    _write(tmp_path / "router.doc.md", "# router\n")
    refs = codocs.scan(tmp_path)
    assert len(refs) == 1
    assert refs[0].exists is True
    assert refs[0].doc == "router.doc.md"


def test_find_broken_detects_missing(tmp_path):
    _write(tmp_path / "service.py", "# Docs: service.doc.md\n")
    broken = codocs.find_broken(tmp_path)
    assert len(broken) == 1
    assert broken[0].doc == "service.doc.md"
    assert broken[0].exists is False


def test_ts_slash_comment_reference(tmp_path):
    _write(tmp_path / "types.ts", "// Docs: types.doc.md\nexport const x = 1\n")
    _write(tmp_path / "types.doc.md", "# types\n")
    broken = codocs.find_broken(tmp_path)
    assert broken == []


def test_reference_after_shebang(tmp_path):
    _write(tmp_path / "run.sh", "#!/bin/bash\n# Docs: run.doc.md\necho hi\n")
    _write(tmp_path / "run.doc.md", "# run\n")
    broken = codocs.find_broken(tmp_path)
    assert broken == []


def test_excludes_default_dirs(tmp_path):
    _write(tmp_path / "node_modules" / "dep.py", "# Docs: missing.doc.md\n")
    assert codocs.find_broken(tmp_path) == []


def test_no_reference_is_ignored(tmp_path):
    _write(tmp_path / "plain.py", "x = 1\n")
    # A file without any Docs: reference produces no entries.
    assert codocs.scan(tmp_path) == []


def test_reference_below_head_window_is_ignored(tmp_path):
    # A `Docs:` mention deep in the file (e.g. inside a string) is not a header
    # reference and must not be treated as one.
    body = "\n".join(["x = 1"] * 10) + "\n# Docs: deep.doc.md\n"
    _write(tmp_path / "deep.py", body)
    assert codocs.scan(tmp_path) == []


def test_cli_check_ok(tmp_path):
    _write(tmp_path / "router.py", "# Docs: router.doc.md\n")
    _write(tmp_path / "router.doc.md", "# router\n")
    result = runner.invoke(app, ["codocs", "check", str(tmp_path)])
    assert result.exit_code == 0
    assert "OK" in result.stdout


def test_cli_check_broken_exits_nonzero(tmp_path):
    _write(tmp_path / "router.py", "# Docs: router.doc.md\n")
    result = runner.invoke(app, ["codocs", "check", str(tmp_path)])
    assert result.exit_code == 1
    assert "MISSING" in result.stdout


def test_cli_check_json(tmp_path):
    _write(tmp_path / "router.py", "# Docs: router.doc.md\n")
    result = runner.invoke(app, ["codocs", "check", str(tmp_path), "--json"])
    assert result.exit_code == 1
    assert "router.doc.md" in result.stdout


def test_status_includes_codocs():
    result = runner.invoke(app, ["status"])
    assert result.exit_code == 0
    assert "CoDocs" in result.stdout
