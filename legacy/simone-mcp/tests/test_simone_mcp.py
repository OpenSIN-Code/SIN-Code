import asyncio
import json
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "src"
sys.path.insert(0, str(SRC))

from mcp_server import (  # noqa: E402
    _build_realtime_url,
    build_agent_card,
    dashboard,
    execute_simone_action,
    find_references,
    find_symbol,
    get_project_overview,
    insert_after_symbol,
    process_lsp_task,
    replace_symbol_body,
)


def test_cli_print_card():
    result = subprocess.run(
        [sys.executable, "src/cli.py", "print-card"],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    )
    card = json.loads(result.stdout)
    assert card["name"] == "simone-mcp"
    assert card["endpoints"]["dashboard"] == "/dashboard"


def test_agent_card_shape():
    card = build_agent_card("http://localhost:8234")
    assert card["name"] == "simone-mcp"
    assert "code.find_symbol" in card["capabilities"]


def test_health_action_and_async_task():
    health = asyncio.run(execute_simone_action({"action": "simone.mcp.health"}))
    assert health["status"] == "ok"
    task = asyncio.run(process_lsp_task({"symbol": "demo_symbol"}))
    assert task["ok"] is True
    assert task["symbol"] == "demo_symbol"


def test_symbol_tools_on_python_file(tmp_path: Path):
    source = tmp_path / "sample.py"
    source.write_text(
        """def hello_world():\n    return 1\n\n\nclass Greeter:\n    pass\n""",
        encoding="utf-8",
    )

    symbol = find_symbol({"symbol": "hello_world", "root": str(tmp_path)})
    assert symbol["ok"] is True
    assert symbol["count"] == 1

    refs = find_references({"symbol": "hello_world", "root": str(tmp_path)})
    assert refs["ok"] is True
    assert refs["count"] >= 1

    replaced = replace_symbol_body(
        {"symbol": "hello_world", "file": str(source), "body": "return 2"}
    )
    assert replaced["ok"] is True
    assert "return 2" in source.read_text(encoding="utf-8")

    inserted = insert_after_symbol(
        {"symbol": "Greeter", "file": str(source), "text": "# inserted after class"}
    )
    assert inserted["ok"] is True
    assert "# inserted after class" in source.read_text(encoding="utf-8")


def test_project_overview_and_dashboard():
    overview = get_project_overview({"root": str(ROOT)})
    assert overview["ok"] is True
    assert overview["fileCount"] > 0

    html = asyncio.run(dashboard())
    assert "Quick Actions" in html
    assert "activate_simone" in html
    assert "listener_state" not in html


def test_realtime_url_builder():
    assert (
        _build_realtime_url("https://example.supabase.co")
        == "wss://example.supabase.co/realtime/v1"
    )
