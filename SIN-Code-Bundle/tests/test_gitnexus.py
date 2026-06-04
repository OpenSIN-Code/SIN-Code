"""Tests for the GitNexus bridge.

These never invoke real GitNexus/Node: subprocess and discovery are stubbed so
the suite runs in CI without a Node toolchain.
"""
from __future__ import annotations

import json
import time
from pathlib import Path

import pytest

from sin_code_bundle import gitnexus


# --------------------------------------------------------------------------- #
# Environment discovery
# --------------------------------------------------------------------------- #
def test_env_unavailable_without_npx(monkeypatch):
    monkeypatch.setattr(gitnexus.shutil, "which", lambda _: None)
    env = gitnexus.detect_env()
    assert env.available is False
    with pytest.raises(gitnexus.GitNexusError):
        env.base_cmd()


def test_base_cmd_uses_npx_and_pinned_package(monkeypatch):
    monkeypatch.setattr(gitnexus.shutil, "which", lambda name: f"/usr/bin/{name}")
    env = gitnexus.detect_env()
    assert env.available is True
    cmd = env.base_cmd()
    assert cmd[:3] == ["/usr/bin/npx", "-y", gitnexus.GITNEXUS_PACKAGE]


# --------------------------------------------------------------------------- #
# Index state
# --------------------------------------------------------------------------- #
def test_index_state_missing(tmp_path):
    state = gitnexus.index_state(str(tmp_path))
    assert state.exists is False
    assert state.stale is False
    assert state.path.name == gitnexus.INDEX_DIRNAME


def test_index_state_fresh(tmp_path):
    idx = tmp_path / gitnexus.INDEX_DIRNAME
    idx.mkdir()
    (idx / "graph.db").write_text("data")
    state = gitnexus.index_state(str(tmp_path))
    assert state.exists is True
    assert state.stale is False
    assert state.age_seconds is not None


def test_index_state_stale(tmp_path):
    idx = tmp_path / gitnexus.INDEX_DIRNAME
    idx.mkdir()
    f = idx / "graph.db"
    f.write_text("data")
    old = time.time() - (gitnexus.DEFAULT_STALE_SECONDS + 60)
    import os

    os.utime(f, (old, old))
    state = gitnexus.index_state(str(tmp_path))
    assert state.exists is True
    assert state.stale is True


# --------------------------------------------------------------------------- #
# ensure_index
# --------------------------------------------------------------------------- #
def test_ensure_index_raises_without_runtime(tmp_path, monkeypatch):
    monkeypatch.setattr(gitnexus.shutil, "which", lambda _: None)
    with pytest.raises(gitnexus.GitNexusError):
        gitnexus.ensure_index(str(tmp_path))


def test_ensure_index_auto_calls_analyze(tmp_path, monkeypatch):
    monkeypatch.setattr(gitnexus.shutil, "which", lambda name: f"/usr/bin/{name}")
    calls = {"analyze": 0}

    def fake_analyze(root, env=None, timeout=1800):
        calls["analyze"] += 1
        idx = Path(root) / gitnexus.INDEX_DIRNAME
        idx.mkdir(exist_ok=True)
        (idx / "graph.db").write_text("built")

    monkeypatch.setattr(gitnexus, "analyze", fake_analyze)
    state = gitnexus.ensure_index(str(tmp_path), auto=True)
    assert calls["analyze"] == 1
    assert state.exists is True


def test_ensure_index_no_auto_does_not_build(tmp_path, monkeypatch):
    monkeypatch.setattr(gitnexus.shutil, "which", lambda name: f"/usr/bin/{name}")
    monkeypatch.setattr(
        gitnexus, "analyze", lambda *a, **k: pytest.fail("should not analyze")
    )
    state = gitnexus.ensure_index(str(tmp_path), auto=False)
    assert state.exists is False


def test_ensure_index_skips_when_fresh(tmp_path, monkeypatch):
    monkeypatch.setattr(gitnexus.shutil, "which", lambda name: f"/usr/bin/{name}")
    idx = tmp_path / gitnexus.INDEX_DIRNAME
    idx.mkdir()
    (idx / "graph.db").write_text("fresh")
    monkeypatch.setattr(
        gitnexus, "analyze", lambda *a, **k: pytest.fail("should not rebuild")
    )
    state = gitnexus.ensure_index(str(tmp_path), auto=True)
    assert state.exists is True


# --------------------------------------------------------------------------- #
# Query wrappers
# --------------------------------------------------------------------------- #
class _Proc:
    def __init__(self, rc=0, out="ok", err=""):
        self.returncode = rc
        self.stdout = out
        self.stderr = err


def test_query_wrappers_invoke_correct_subcommands(monkeypatch):
    monkeypatch.setattr(gitnexus.shutil, "which", lambda name: f"/usr/bin/{name}")
    seen = {}

    def fake_run(cmd, cwd=None, timeout=300, capture=True):
        seen["cmd"] = cmd
        return _Proc(out="result")

    monkeypatch.setattr(gitnexus, "_run", fake_run)

    assert gitnexus.context("Foo.bar") == "result"
    assert "context" in seen["cmd"]
    assert gitnexus.impact("Foo.bar") == "result"
    assert "impact" in seen["cmd"]
    assert gitnexus.ai_context("add auth") == "result"
    assert "ai-context" in seen["cmd"]


def test_query_raises_on_failure(monkeypatch):
    monkeypatch.setattr(gitnexus.shutil, "which", lambda name: f"/usr/bin/{name}")
    monkeypatch.setattr(
        gitnexus, "_run", lambda *a, **k: _Proc(rc=2, out="", err="boom")
    )
    with pytest.raises(gitnexus.GitNexusError):
        gitnexus.context("X")


# --------------------------------------------------------------------------- #
# Agent wiring
# --------------------------------------------------------------------------- #
def test_setup_opencode_writes_json(tmp_path, monkeypatch):
    monkeypatch.setattr(Path, "home", classmethod(lambda cls: tmp_path))
    written = gitnexus.setup_agents(["opencode"])
    cfg = Path(written["opencode"])
    assert cfg.is_file()
    data = json.loads(cfg.read_text())
    assert data["mcp"]["gitnexus"]["command"][:3] == ["npx", "-y", gitnexus.GITNEXUS_PACKAGE]


def test_setup_codex_writes_toml_block(tmp_path, monkeypatch):
    monkeypatch.setattr(Path, "home", classmethod(lambda cls: tmp_path))
    written = gitnexus.setup_agents(["codex"])
    cfg = Path(written["codex"])
    text = cfg.read_text()
    assert "[mcp_servers.gitnexus]" in text
    # Idempotent: second run does not duplicate the block.
    gitnexus.setup_agents(["codex"])
    assert cfg.read_text().count("[mcp_servers.gitnexus]") == 1


def test_setup_hermes_writes_json(tmp_path, monkeypatch):
    monkeypatch.setattr(Path, "home", classmethod(lambda cls: tmp_path))
    written = gitnexus.setup_agents(["hermes"])
    data = json.loads(Path(written["hermes"]).read_text())
    assert "gitnexus" in data["mcpServers"]


def test_setup_unknown_agent_raises(tmp_path, monkeypatch):
    monkeypatch.setattr(Path, "home", classmethod(lambda cls: tmp_path))
    with pytest.raises(gitnexus.GitNexusError):
        gitnexus.setup_agents(["notanagent"])


def test_setup_preserves_existing_opencode_config(tmp_path, monkeypatch):
    monkeypatch.setattr(Path, "home", classmethod(lambda cls: tmp_path))
    cfg = tmp_path / ".config" / "opencode" / "opencode.json"
    cfg.parent.mkdir(parents=True)
    cfg.write_text(json.dumps({"theme": "dark", "mcp": {"other": {"x": 1}}}))
    gitnexus.setup_agents(["opencode"])
    data = json.loads(cfg.read_text())
    assert data["theme"] == "dark"
    assert "other" in data["mcp"]
    assert "gitnexus" in data["mcp"]
