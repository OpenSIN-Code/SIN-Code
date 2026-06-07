# SPDX-License-Identifier: MIT
"""Tests for the MarkItDown bridge.

No real MarkItDown / uvx invocation: discovery (shutil.which) and config HOME
are stubbed so the suite runs in CI without the tool installed.
"""

from __future__ import annotations

import json

import pytest

from sin_code_bundle import markitdown


# --------------------------------------------------------------------------- #
# Environment discovery
# --------------------------------------------------------------------------- #
def test_env_unavailable_without_runners(monkeypatch):
    monkeypatch.setattr(markitdown.shutil, "which", lambda _: None)
    env = markitdown.detect_env()
    assert env.mcp_available is False
    assert env.cli_available is False
    with pytest.raises(markitdown.MarkItDownError):
        env.mcp_command()
    with pytest.raises(markitdown.MarkItDownError):
        env.cli_cmd()


def test_mcp_command_prefers_uvx(monkeypatch):
    monkeypatch.setattr(markitdown.shutil, "which", lambda name: f"/usr/bin/{name}")
    cmd = markitdown.mcp_server_command()
    assert cmd == {"command": "uvx", "args": [markitdown.MARKITDOWN_MCP_PACKAGE]}


def test_mcp_command_falls_back_to_direct_exe(monkeypatch):
    def which(name):
        return f"/usr/bin/{name}" if name == markitdown.MARKITDOWN_MCP_PACKAGE else None

    monkeypatch.setattr(markitdown.shutil, "which", which)
    cmd = markitdown.mcp_server_command()
    assert cmd == {"command": markitdown.MARKITDOWN_MCP_PACKAGE, "args": []}


# --------------------------------------------------------------------------- #
# MCP wiring into agent configs
# --------------------------------------------------------------------------- #
@pytest.fixture
def fake_home(tmp_path, monkeypatch):
    monkeypatch.setattr(markitdown.Path, "home", classmethod(lambda cls: tmp_path))
    # Make the runner resolvable so wiring uses the uvx command.
    monkeypatch.setattr(markitdown.shutil, "which", lambda name: f"/usr/bin/{name}")
    return tmp_path


def test_setup_wires_all_agents(fake_home):
    written = markitdown.setup_agents()
    assert set(written) == set(markitdown.AGENTS)

    oc = json.loads((fake_home / ".config/opencode/opencode.json").read_text())
    assert oc["mcp"]["markitdown"]["command"] == ["uvx", "markitdown-mcp"]
    assert oc["mcp"]["markitdown"]["type"] == "local"

    codex = (fake_home / ".codex/config.toml").read_text()
    assert "[mcp_servers.markitdown]" in codex
    assert 'command = "uvx"' in codex
    assert '"markitdown-mcp"' in codex

    hermes = json.loads((fake_home / ".hermes/mcp.json").read_text())
    assert hermes["mcpServers"]["markitdown"]["command"] == "uvx"


def test_setup_preserves_existing_config(fake_home):
    oc_path = fake_home / ".config/opencode/opencode.json"
    oc_path.parent.mkdir(parents=True)
    oc_path.write_text(json.dumps({"mcp": {"other": {"enabled": True}}}))

    markitdown.setup_agents(["opencode"])
    data = json.loads(oc_path.read_text())
    assert "other" in data["mcp"]  # untouched
    assert "markitdown" in data["mcp"]  # added


def test_codex_wiring_is_idempotent(fake_home):
    markitdown.setup_agents(["codex"])
    markitdown.setup_agents(["codex"])
    codex = (fake_home / ".codex/config.toml").read_text()
    assert codex.count("[mcp_servers.markitdown]") == 1


def test_unknown_agent_raises(fake_home):
    with pytest.raises(markitdown.MarkItDownError):
        markitdown.setup_agents(["nope"])


# --------------------------------------------------------------------------- #
# CLI convert wrapper
# --------------------------------------------------------------------------- #
def test_convert_missing_file_raises(monkeypatch, tmp_path):
    monkeypatch.setattr(markitdown.shutil, "which", lambda name: f"/usr/bin/{name}")
    with pytest.raises(markitdown.MarkItDownError):
        markitdown.convert(str(tmp_path / "nope.pdf"))


def test_convert_runs_cli(monkeypatch, tmp_path):
    monkeypatch.setattr(markitdown.shutil, "which", lambda name: f"/usr/bin/{name}")
    src = tmp_path / "doc.txt"
    src.write_text("hello")

    class FakeProc:
        returncode = 0
        stdout = "# converted\n"
        stderr = ""

    def fake_run(cmd, **kwargs):
        assert cmd[0] == "/usr/bin/markitdown"
        assert cmd[1] == str(src)
        return FakeProc()

    monkeypatch.setattr(markitdown.subprocess, "run", fake_run)
    assert markitdown.convert(str(src)) == "# converted\n"
