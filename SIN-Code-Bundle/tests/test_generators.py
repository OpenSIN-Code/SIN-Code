"""Tests for sin_code_bundle.generators (sin init / sin agents-md)."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from sin_code_bundle.generators import (
    SUPPORTED_AGENTS,
    render_agent_config,
    render_agents_md,
    write_agent_config,
    write_agents_md,
)


# --------------------------------------------------------------------------- #
# AGENTS.md
# --------------------------------------------------------------------------- #
def test_agents_md_has_core_loop():
    md = render_agents_md()
    assert "non-negotiable loop" in md
    assert "impact(" in md
    assert "verify_tests" in md
    assert "architectural_debt" in md


def test_agents_md_hard_rules():
    md = render_agents_md()
    assert "Never claim" in md
    assert "ADW" in md


# --------------------------------------------------------------------------- #
# Config shapes
# --------------------------------------------------------------------------- #
def test_opencode_config_shape():
    _, content = render_agent_config("opencode")
    data = json.loads(content)
    assert data["mcp"]["sin"]["command"] == ["sin", "serve"]
    assert data["mcp"]["sin"]["enabled"] is True
    assert data["$schema"] == "https://opencode.ai/config.json"


def test_codex_config_shape():
    _, content = render_agent_config("codex")
    assert "[mcp_servers.sin]" in content
    assert 'command = "sin"' in content
    assert 'args = ["serve"]' in content


def test_hermes_config_shape():
    _, content = render_agent_config("hermes")
    assert "mcp_servers:" in content
    assert "sin:" in content
    assert "command:" in content


def test_unknown_agent_raises():
    with pytest.raises(ValueError, match="unknown agent"):
        render_agent_config("nonexistent")  # type: ignore[arg-type]


# --------------------------------------------------------------------------- #
# Idempotent merge
# --------------------------------------------------------------------------- #
def test_idempotent_merge_preserves_existing_opencode(tmp_path: Path, monkeypatch):
    cfg = tmp_path / "opencode.json"
    cfg.write_text(json.dumps({"theme": "dark", "mcp": {"other": {}}}))
    monkeypatch.chdir(tmp_path)
    write_agent_config("opencode", "local")
    data = json.loads(cfg.read_text())
    assert data["theme"] == "dark"  # untouched
    assert "other" in data["mcp"]  # untouched
    assert data["mcp"]["sin"]["command"] == ["sin", "serve"]  # added


def test_idempotent_merge_preserves_existing_codex(tmp_path: Path, monkeypatch):
    existing_toml = '[mcp_servers.other]\ncommand = "other-tool"\n'
    codex_dir = tmp_path / ".codex"
    codex_dir.mkdir()
    cfg = codex_dir / "config.toml"
    cfg.write_text(existing_toml)
    monkeypatch.chdir(tmp_path)
    write_agent_config("codex", "local")
    result = cfg.read_text()
    assert "[mcp_servers.other]" in result  # untouched
    assert "[mcp_servers.sin]" in result  # added


# --------------------------------------------------------------------------- #
# Dry-run: nothing written
# --------------------------------------------------------------------------- #
@pytest.mark.parametrize("agent", SUPPORTED_AGENTS)
def test_dry_run_writes_nothing(tmp_path: Path, monkeypatch, agent: str):
    monkeypatch.chdir(tmp_path)
    path, _ = write_agent_config(agent, "local", dry_run=True)
    assert not path.exists()


# --------------------------------------------------------------------------- #
# AGENTS.md file operations
# --------------------------------------------------------------------------- #
def test_agents_md_written(tmp_path: Path):
    path, written = write_agents_md(tmp_path)
    assert written is True
    assert path.exists()
    assert "non-negotiable loop" in path.read_text()


def test_agents_md_no_overwrite_without_force(tmp_path: Path):
    target = tmp_path / "AGENTS.md"
    target.write_text("keep me")
    _, written = write_agents_md(tmp_path, force=False)
    assert written is False
    assert target.read_text() == "keep me"


def test_agents_md_force_overwrites(tmp_path: Path):
    target = tmp_path / "AGENTS.md"
    target.write_text("old content")
    _, written = write_agents_md(tmp_path, force=True)
    assert written is True
    assert target.read_text() != "old content"


def test_agents_md_dry_run_writes_nothing(tmp_path: Path):
    path, written = write_agents_md(tmp_path, dry_run=True)
    assert not path.exists()
    assert written is False


# --------------------------------------------------------------------------- #
# CLI smoke via typer.testing
# --------------------------------------------------------------------------- #
def test_cli_init_dry_run(tmp_path: Path, monkeypatch):
    from typer.testing import CliRunner

    from sin_code_bundle.cli import app

    monkeypatch.chdir(tmp_path)
    result = CliRunner().invoke(app, ["init", "opencode", "--dry-run", "--no-agents-md"])
    assert result.exit_code == 0
    assert "Would write" in result.output


def test_cli_init_all_dry_run(tmp_path: Path, monkeypatch):
    from typer.testing import CliRunner

    from sin_code_bundle.cli import app

    monkeypatch.chdir(tmp_path)
    result = CliRunner().invoke(app, ["init", "all", "--dry-run"])
    assert result.exit_code == 0
    for ag in SUPPORTED_AGENTS:
        assert ag in result.output


def test_cli_init_unknown_agent(tmp_path: Path, monkeypatch):
    from typer.testing import CliRunner

    from sin_code_bundle.cli import app

    monkeypatch.chdir(tmp_path)
    result = CliRunner().invoke(app, ["init", "unknown_agent", "--dry-run"])
    assert result.exit_code != 0


def test_cli_agents_md_dry_run(tmp_path: Path, monkeypatch):
    from typer.testing import CliRunner

    from sin_code_bundle.cli import app

    monkeypatch.chdir(tmp_path)
    result = CliRunner().invoke(app, ["agents-md", "--dry-run"])
    assert result.exit_code == 0
    assert "non-negotiable loop" in result.output
