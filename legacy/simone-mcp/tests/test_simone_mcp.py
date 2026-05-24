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


def test_replace_symbol_body_preserves_comments(tmp_path: Path):
    source = tmp_path / "commented.py"
    source.write_text(
        '# header comment\n\ndef calculate_sum(a: int, b: int) -> int:\n    """Calculate sum."""\n    return a + b\n\n\ndef calculate_product(a: int, b: int) -> int:\n    """Calculate product."""\n    return a * b\n',
        encoding="utf-8",
    )
    replaced = replace_symbol_body(
        {"symbol": "calculate_sum", "file": str(source), "body": '    """Calculate sum with logging."""\n    print(f"Calculating {a} + {b}")\n    return a + b'}
    )
    assert replaced["ok"] is True
    content = source.read_text(encoding="utf-8")
    assert "# header comment" in content
    assert "def calculate_product" in content
    assert "Calculate sum with logging" in content


def test_replace_symbol_body_reports_engine(tmp_path: Path):
    source = tmp_path / "engine_check.py"
    source.write_text("def foo():\n    pass\n", encoding="utf-8")
    replaced = replace_symbol_body(
        {"symbol": "foo", "file": str(source), "body": "return 42"}
    )
    assert replaced["ok"] is True
    assert "engine" in replaced
    assert replaced["engine"] in ("libcst", "ast")


def test_find_references_reports_engine(tmp_path: Path):
    source = tmp_path / "refs_check.py"
    source.write_text("def bar():\n    pass\n\nx = bar()\n", encoding="utf-8")
    refs = find_references({"symbol": "bar", "root": str(tmp_path)})
    assert refs["ok"] is True
    assert "engine" in refs
    assert refs["engine"] in ("jedi", "regex")


def test_replace_nonexistent_function(tmp_path: Path):
    source = tmp_path / "nope.py"
    source.write_text("def exists():\n    pass\n", encoding="utf-8")
    replaced = replace_symbol_body(
        {"symbol": "does_not_exist", "file": str(source), "body": "pass"}
    )
    assert replaced["ok"] is False


def test_insert_after_preserves_rest(tmp_path: Path):
    source = tmp_path / "multi.py"
    source.write_text(
        "def first():\n    pass\n\n\ndef second():\n    pass\n",
        encoding="utf-8",
    )
    inserted = insert_after_symbol(
        {"symbol": "first", "file": str(source), "text": "# after first"}
    )
    assert inserted["ok"] is True
    content = source.read_text(encoding="utf-8")
    assert "def second" in content
    assert "# after first" in content
