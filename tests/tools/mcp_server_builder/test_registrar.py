# SPDX-License-Identifier: MIT
"""
Purpose: Test the Registrar and McpServerEntry.
Docs: test_registrar.doc.md
"""

import json

from sin_code_bundle.tools.mcp_server_builder.registrar import (
    McpServerEntry,
    Registrar,
    build_local_entry,
)


class TestMcpServerEntry:
    def test_to_json_local(self):
        entry = McpServerEntry(
            name="x",
            type="local",
            command=["uvx", "x-mcp"],
            env={"K": "V"},
        )
        d = entry.to_json()
        assert d["type"] == "local"
        assert d["command"] == ["uvx", "x-mcp"]
        assert d["env"] == {"K": "V"}
        assert d["enabled"] is True

    def test_to_json_remote(self):
        entry = McpServerEntry(
            name="y",
            type="remote",
            url="https://example.com",
            headers={"Auth": "Bearer x"},
        )
        d = entry.to_json()
        assert d["url"] == "https://example.com"
        assert d["headers"]["Auth"] == "Bearer x"
        assert "command" not in d


class TestRegistrar:
    def test_register_creates_file(self, tmp_path):
        cfg = tmp_path / "opencode.json"
        reg = Registrar(config_path=cfg)
        entry = build_local_entry("my-tool", ["uvx", "my-tool-mcp"])
        path = reg.register(entry)
        assert path == cfg
        data = json.loads(cfg.read_text())
        assert "mcp" in data
        assert "my-tool" in data["mcp"]
        assert data["mcp"]["my-tool"]["type"] == "local"

    def test_register_replaces_existing(self, tmp_path):
        cfg = tmp_path / "opencode.json"
        reg = Registrar(config_path=cfg)
        reg.register(build_local_entry("a", ["a"]))
        reg.register(build_local_entry("a", ["a", "b"]))  # different command
        data = json.loads(cfg.read_text())
        assert data["mcp"]["a"]["command"] == ["a", "b"]

    def test_unregister_existing(self, tmp_path):
        cfg = tmp_path / "opencode.json"
        reg = Registrar(config_path=cfg)
        reg.register(build_local_entry("a", ["a"]))
        assert reg.unregister("a") is True
        data = json.loads(cfg.read_text())
        assert "a" not in data["mcp"]

    def test_unregister_missing(self, tmp_path):
        cfg = tmp_path / "opencode.json"
        reg = Registrar(config_path=cfg)
        assert reg.unregister("nope") is False

    def test_list_registered(self, tmp_path):
        cfg = tmp_path / "opencode.json"
        reg = Registrar(config_path=cfg)
        reg.register(build_local_entry("a", ["a"]))
        reg.register(build_local_entry("b", ["b"]))
        names = reg.list_registered()
        assert names == ["a", "b"]

    def test_show(self, tmp_path):
        cfg = tmp_path / "opencode.json"
        reg = Registrar(config_path=cfg)
        reg.register(build_local_entry("a", ["a"]))
        result = reg.show("a")
        assert result["type"] == "local"
        assert reg.show("missing") is None

    def test_jsonc_tolerated(self, tmp_path):
        """The registrar must parse opencode.jsonc (comments + trailing commas)."""
        cfg = tmp_path / "opencode.json"
        cfg.write_text(
            "{\n"
            "  // a comment\n"
            '  "mcp": {\n'
            '    "x": {"type": "local", "command": ["x"],},\n'
            "  },\n"
            "}\n",
            encoding="utf-8",
        )
        reg = Registrar(config_path=cfg)
        assert reg.list_registered() == ["x"]

    def test_preserves_existing_keys(self, tmp_path):
        cfg = tmp_path / "opencode.json"
        cfg.write_text(
            json.dumps({"$schema": "https://example.com", "username": "x"}),
            encoding="utf-8",
        )
        reg = Registrar(config_path=cfg)
        reg.register(build_local_entry("y", ["y"]))
        data = json.loads(cfg.read_text())
        assert data["$schema"] == "https://example.com"
        assert data["username"] == "x"
        assert "y" in data["mcp"]


class TestBuildLocalEntry:
    def test_basic(self):
        e = build_local_entry("x", ["a", "b"])
        assert e.type == "local"
        assert e.command == ["a", "b"]
        assert e.env == {}

    def test_with_env(self):
        e = build_local_entry("x", ["a"], env={"K": "V"})
        assert e.env == {"K": "V"}

    def test_disabled(self):
        e = McpServerEntry(name="x", type="local", command=["a"], enabled=False)
        assert e.to_json()["enabled"] is False

    def test_remote_entry_url_only(self):
        e = McpServerEntry(name="r", type="remote", url="https://x")
        d = e.to_json()
        assert d["url"] == "https://x"
        assert "command" not in d
        assert "env" not in d


class TestDiscover:
    def test_explicit_env(self, monkeypatch, tmp_path):
        cfg = tmp_path / "x.json"
        monkeypatch.setenv("OPENCODE_CONFIG", str(cfg))
        reg = Registrar()
        assert reg.config_path == cfg

    def test_explicit_config_path_wins(self, tmp_path):
        cfg = tmp_path / "y.json"
        reg = Registrar(config_path=cfg)
        assert reg.config_path == cfg
