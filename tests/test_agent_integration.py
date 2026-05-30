"""Tests fuer WS2 (mcp-config) und WS4 (agents-md)."""

import json

from typer.testing import CliRunner

from sin_code_bundle import agents_md, mcp_config
from sin_code_bundle.cli import app

runner = CliRunner()


# --------------------------- WS2: mcp-config ---------------------------- #
def test_opencode_config_is_valid_json():
    out = mcp_config.generate_opencode()
    data = json.loads(out)
    assert data["mcp"]["sin"]["type"] == "local"
    assert data["mcp"]["sin"]["command"] == ["sin", "serve"]
    assert data["mcp"]["sin"]["enabled"] is True


def test_codex_config_has_table_header():
    out = mcp_config.generate_codex({"FOO": "bar"})
    assert "[mcp_servers.sin]" in out
    assert 'command = "sin"' in out
    assert 'args = ["serve"]' in out
    assert "[mcp_servers.sin.env]" in out
    assert 'FOO = "bar"' in out


def test_hermes_config_is_valid_yaml():
    import yaml

    out = mcp_config.generate_hermes()
    data = yaml.safe_load(out)
    assert data["mcp_servers"]["sin"]["command"] == "sin"
    assert data["mcp_servers"]["sin"]["args"] == ["serve"]


def test_generate_dispatch_unknown_raises():
    try:
        mcp_config.generate("unknown")
        assert False, "should have raised"
    except ValueError:
        pass


def test_cli_mcp_config_stdout():
    result = runner.invoke(app, ["mcp-config", "opencode"])
    assert result.exit_code == 0
    assert '"type": "local"' in result.stdout


def test_cli_mcp_config_unknown_client():
    result = runner.invoke(app, ["mcp-config", "bogus"])
    assert result.exit_code == 1


def test_mcp_config_write_merges_json(tmp_path):
    cfg = tmp_path / "opencode.json"
    cfg.write_text(json.dumps({"mcp": {"other": {"type": "local"}}, "keep": 1}))
    msg = mcp_config.merge_into_file("opencode", cfg)
    data = json.loads(cfg.read_text())
    assert "sin" in data["mcp"]
    assert "other" in data["mcp"]  # fremder Eintrag bleibt
    assert data["keep"] == 1
    assert "Merged" in msg


def test_mcp_config_write_codex_idempotent(tmp_path):
    cfg = tmp_path / "config.toml"
    mcp_config.merge_into_file("codex", cfg, {"K": "v"})
    first = cfg.read_text()
    mcp_config.merge_into_file("codex", cfg, {"K": "v"})
    second = cfg.read_text()
    # nur ein sin-Block, keine Duplikate
    assert second.count("[mcp_servers.sin]") == 1
    assert first.count("[mcp_servers.sin]") == 1


def test_mcp_config_codex_preserves_foreign_table(tmp_path):
    cfg = tmp_path / "config.toml"
    cfg.write_text('[mcp_servers.other]\ncommand = "x"\n')
    mcp_config.merge_into_file("codex", cfg)
    content = cfg.read_text()
    assert "[mcp_servers.other]" in content
    assert "[mcp_servers.sin]" in content


# --------------------------- WS4: agents-md ----------------------------- #
def test_agents_md_create(tmp_path):
    path = tmp_path / "AGENTS.md"
    msg = agents_md.upsert(path)
    content = path.read_text()
    assert agents_md.START_MARKER in content
    assert agents_md.END_MARKER in content
    assert "verify_tests" in content
    assert "Created" in msg


def test_agents_md_idempotent(tmp_path):
    path = tmp_path / "AGENTS.md"
    agents_md.upsert(path)
    first = path.read_text()
    agents_md.upsert(path)
    second = path.read_text()
    assert first == second
    assert second.count(agents_md.START_MARKER) == 1


def test_agents_md_preserves_user_content(tmp_path):
    path = tmp_path / "AGENTS.md"
    path.write_text("# My rules\n\nKeep this line.\n")
    agents_md.upsert(path)
    content = path.read_text()
    assert "Keep this line." in content
    assert agents_md.START_MARKER in content


def test_agents_md_only_block_changes(tmp_path):
    path = tmp_path / "AGENTS.md"
    path.write_text(
        f"# Custom\n\nIntro text.\n\n{agents_md.START_MARKER}\nOLD\n{agents_md.END_MARKER}\n\nTrailer.\n"
    )
    agents_md.upsert(path)
    content = path.read_text()
    assert "Intro text." in content
    assert "Trailer." in content
    assert "OLD" not in content
    assert "verify_tests" in content


def test_cli_agents_md(tmp_path):
    target = tmp_path / "AGENTS.md"
    result = runner.invoke(app, ["agents-md", "--path", str(target)])
    assert result.exit_code == 0
    assert target.exists()
