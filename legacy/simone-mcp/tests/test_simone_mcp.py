import asyncio
import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

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
from simone_mcp.correlation import ToolCallCorrelation  # noqa: E402
from simone_mcp.a2a_handler import handle_a2a_request  # noqa: E402


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
    assert "sin_simone_mcp_health" in card["capabilities"]
    assert "code.find_symbol" not in card["capabilities"]
    assert "defaultInputModes" in card
    assert "defaultOutputModes" in card
    skill_ids = {s["id"] for s in card["skills"]}
    assert "sin_simone_mcp_symbol_search" in skill_ids
    assert "code.find_symbol" not in skill_ids


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


def test_correlation_generate_and_complete():
    corr = ToolCallCorrelation()
    cid = corr.generate_correlation_id("test_tool", {"arg": "value"})
    status = corr.get_call_status(cid)
    assert status is not None
    assert status["status"] == "in_progress"
    assert status["tool_name"] == "test_tool"

    corr.complete_call(cid, {"ok": True})
    status = corr.get_call_status(cid)
    assert status["status"] == "completed"
    assert status["result"] == {"ok": True}


def test_correlation_with_provided_id():
    corr = ToolCallCorrelation()
    cid = corr.generate_correlation_id("test_tool", {}, provided_id="custom-123")
    assert cid == "custom-123"
    status = corr.get_call_status(cid)
    assert status is not None


def test_correlation_failure_tracking():
    corr = ToolCallCorrelation()
    cid = corr.generate_correlation_id("failing_tool", {})
    corr.complete_call(cid, None, error="something broke")
    status = corr.get_call_status(cid)
    assert status["status"] == "failed"
    assert status["error"] == "something broke"


def test_correlation_cleanup():
    corr = ToolCallCorrelation(max_age_seconds=0)
    cid = corr.generate_correlation_id("old_tool", {})
    removed = corr.cleanup_old_calls()
    assert removed >= 1
    assert corr.get_call_status(cid) is None


def test_correlation_bounded_eviction():
    corr = ToolCallCorrelation(max_calls=5)
    ids = []
    for i in range(10):
        ids.append(corr.generate_correlation_id(f"tool_{i}", {}))
    assert corr.get_call_status(ids[0]) is None
    assert corr.get_call_status(ids[9]) is not None


def test_correlation_auto_cleanup():
    corr = ToolCallCorrelation(max_age_seconds=0, max_calls=1024)
    cid = corr.generate_correlation_id("auto_tool", {})
    corr.complete_call(cid, {"ok": True})
    cid2 = corr.generate_correlation_id("trigger_cleanup", {})
    corr.complete_call(cid2, {"ok": True})
    for _ in range(64):
        cid_n = corr.generate_correlation_id("filler", {})
        corr.complete_call(cid_n, {"ok": True})
    assert corr.get_call_status(cid) is None


def test_a2a_agent_discover():
    result = asyncio.run(handle_a2a_request({"id": "1", "jsonrpc": "2.0", "method": "agent.discover"}, "http://localhost:8234"))
    assert result["id"] == "1"
    assert result["result"]["name"] == "simone-mcp"


def test_a2a_agent_ping():
    result = asyncio.run(handle_a2a_request({"id": "2", "jsonrpc": "2.0", "method": "agent.ping"}, "http://localhost:8234"))
    assert result["result"]["status"] == "alive"
    assert "timestamp" in result["result"]


def test_a2a_tool_list():
    result = asyncio.run(handle_a2a_request({"id": "3", "jsonrpc": "2.0", "method": "tool.list"}, "http://localhost:8234"))
    assert "tools" in result["result"]
    assert len(result["result"]["tools"]) >= 5


def test_a2a_tool_call():
    result = asyncio.run(
        handle_a2a_request(
            {"id": "4", "jsonrpc": "2.0", "method": "tool.call", "params": {"name": "simone.mcp.health"}},
            "http://localhost:8234",
        )
    )
    assert result["result"]["data"]["status"] == "ok"
    assert "correlation_id" in result["result"]


def test_a2a_unknown_method():
    result = asyncio.run(handle_a2a_request({"id": "5", "jsonrpc": "2.0", "method": "nonexistent"}, "http://localhost:8234"))
    assert "error" in result
    assert result["error"]["code"] == -32601


def test_a2a_message_send():
    result = asyncio.run(
        handle_a2a_request(
            {
                "id": "6",
                "jsonrpc": "2.0",
                "method": "message/send",
                "params": {
                    "message": {
                        "parts": [{"type": "text", "text": '{"action": "simone.mcp.health"}'}]
                    }
                },
            },
            "http://localhost:8234",
        )
    )
    assert result["id"] == "6"
    assert "result" in result
    assert result["result"]["kind"] == "task"
    assert result["result"]["status"]["state"] == "completed"
    assert len(result["result"]["artifacts"]) >= 1


def test_a2a_message_send_plain_text():
    result = asyncio.run(
        handle_a2a_request(
            {
                "id": "7",
                "jsonrpc": "2.0",
                "method": "message/send",
                "params": {
                    "message": {
                        "parts": [{"type": "text", "text": "show me the project"}]
                    }
                },
            },
            "http://localhost:8234",
        )
    )
    assert result["result"]["kind"] == "task"


def test_a2a_tasks_get():
    result = asyncio.run(
        handle_a2a_request(
            {"id": "8", "jsonrpc": "2.0", "method": "tasks/get", "params": {"id": "task-123"}},
            "http://localhost:8234",
        )
    )
    assert result["result"]["id"] == "task-123"
    assert result["result"]["status"]["state"] == "completed"


def test_a2a_invalid_jsonrpc_version():
    result = asyncio.run(
        handle_a2a_request(
            {"id": "9", "jsonrpc": "1.0", "method": "agent.ping"},
            "http://localhost:8234",
        )
    )
    assert "error" in result
    assert result["error"]["code"] == -32600


def test_a2a_missing_method():
    result = asyncio.run(
        handle_a2a_request(
            {"id": "10", "jsonrpc": "2.0"},
            "http://localhost:8234",
        )
    )
    assert "error" in result
    assert result["error"]["code"] == -32600


def test_pydantic_schemas_jsonrpc_validation():
    from simone_mcp.schemas import JsonRpcRequest
    valid = JsonRpcRequest(jsonrpc="2.0", id="1", method="test")
    assert valid.jsonrpc == "2.0"
    try:
        JsonRpcRequest(jsonrpc="1.0", id="1", method="test")
        assert False, "Should have raised validation error"
    except Exception:
        pass


def test_pydantic_schemas_tool_call_params():
    from simone_mcp.schemas import ToolCallParams
    valid = ToolCallParams(name="code.find_symbol", arguments={"symbol": "foo"})
    assert valid.tool_name == "code.find_symbol"
    try:
        ToolCallParams(name="", arguments={})
        assert False, "Should have raised validation error"
    except Exception:
        pass


def test_pydantic_schemas_find_symbol_args():
    from simone_mcp.schemas import FindSymbolArgs
    valid = FindSymbolArgs(symbol="my_func")
    assert valid.symbol == "my_func"
    try:
        FindSymbolArgs(symbol="")
        assert False, "Should have raised validation error"
    except Exception:
        pass


def test_treesitter_candidate_files_includes_js_ts(tmp_path: Path):
    (tmp_path / "app.py").write_text("def foo(): pass\n", encoding="utf-8")
    (tmp_path / "app.js").write_text("function bar() {}\n", encoding="utf-8")
    (tmp_path / "app.ts").write_text("function baz() {}\n", encoding="utf-8")
    (tmp_path / "app.mjs").write_text("export const qux = () => {};\n", encoding="utf-8")
    from simone_mcp.core import _candidate_files
    paths = _candidate_files(tmp_path)
    suffixes = {p.suffix for p in paths}
    assert ".py" in suffixes
    assert ".js" in suffixes
    assert ".ts" in suffixes
    assert ".mjs" in suffixes


def test_hybrid_memory_shutdown():
    from simone_mcp.hybrid_memory import shutdown_stores
    shutdown_stores()


def test_correlation_thread_safety():
    import threading
    corr = ToolCallCorrelation(max_calls=100)
    errors: list[str] = []

    def worker(start: int):
        try:
            for i in range(50):
                cid = corr.generate_correlation_id(f"tool_{start+i}", {"idx": i})
                corr.complete_call(cid, {"ok": True})
        except Exception as e:
            errors.append(str(e))

    threads = [threading.Thread(target=worker, args=(s,)) for s in range(0, 200, 50)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    assert not errors


def test_js_regex_fallback(tmp_path: Path):
    js_file = tmp_path / "app.js"
    js_file.write_text(
        "function myFunc() { return 1; }\n"
        "class MyClass {}\n"
        "const myArrow = () => {};\n",
        encoding="utf-8",
    )
    from simone_mcp.core import _extract_symbols_js_regex
    symbols = _extract_symbols_js_regex(js_file)
    names = {s["symbol"] for s in symbols}
    assert "myFunc" in names
    assert "MyClass" in names
    assert "myArrow" in names


def test_a2a_message_send_invalid_params():
    result = asyncio.run(
        handle_a2a_request(
            {
                "id": "20",
                "jsonrpc": "2.0",
                "method": "message/send",
                "params": {"garbage": True},
            },
            "http://localhost:8234",
        )
    )
    assert "error" in result
    assert result["error"]["code"] == -32602


def test_pydantic_replace_symbol_body_requires_body():
    from simone_mcp.schemas import ReplaceSymbolBodyArgs
    try:
        ReplaceSymbolBodyArgs(symbol="foo", file="bar.py", body="")
        assert False, "Should require non-empty body"
    except Exception:
        pass


def test_pydantic_insert_after_requires_text():
    from simone_mcp.schemas import InsertAfterSymbolArgs
    try:
        InsertAfterSymbolArgs(symbol="foo", file="bar.py", text="")
        assert False, "Should require non-empty text"
    except Exception:
        pass


def test_path_traversal_blocked(tmp_path: Path):
    from simone_mcp.core import replace_symbol_body, insert_after_symbol
    source = tmp_path / "safe.py"
    source.write_text("def foo():\n    pass\n", encoding="utf-8")
    result = replace_symbol_body(
        {"symbol": "foo", "file": str(source), "body": "return 1", "root": str(tmp_path)}
    )
    assert result["ok"] is True
    result = replace_symbol_body(
        {"symbol": "foo", "file": "/etc/passwd", "body": "hacked", "root": str(tmp_path)}
    )
    assert result["ok"] is False
    result = insert_after_symbol(
        {"symbol": "foo", "file": "/etc/passwd", "text": "evil", "root": str(tmp_path)}
    )
    assert result["ok"] is False


# ─── SEP-2663: Tasks Extension v2 ──────────────────────────────────────────────

def test_sep2663_protocol_version():
    from simone_mcp.protocol import PROTOCOL_VERSION, SUPPORTED_VERSIONS
    assert PROTOCOL_VERSION == "2026-06-30"
    assert "2026-06-30" in SUPPORTED_VERSIONS
    assert "2025-11-25" in SUPPORTED_VERSIONS


def test_sep2663_initialize_declares_tasks_extension():
    from simone_mcp.protocol import handle_mcp_request
    resp, session_id, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "initialize", "params": {"protocolVersion": "2026-06-30"}},
        session_id=None,
    ))
    caps = resp["result"]["capabilities"]
    assert "tasks" in caps
    assert "update" in caps["tasks"]
    assert "cancel" in caps["tasks"]
    assert "list" not in caps["tasks"]
    assert "extensions" in caps
    ext_uris = [e["uri"] for e in caps["extensions"]]
    assert "io.modelcontextprotocol/tasks" in ext_uris


def test_sep2663_server_creates_task_autonomously():
    from simone_mcp.protocol import _create_task, _build_task_obj
    task = _create_task("sin_simone_mcp_symbol_search", {"query": "foo"}, "test-session")
    task_obj = _build_task_obj(task)
    assert task_obj["status"] == "working"
    assert "io.modelcontextprotocol/tasks" not in task_obj  # _meta only in tools/call response


def test_sep2663_tasks_get_returns_inline_result():
    from simone_mcp.protocol import _create_task, _update_task, handle_mcp_request
    task = _create_task("sin_simone_mcp_symbol_search", {"query": "bar"}, "test-session-inline")
    task_id = task["id"]
    _update_task(task_id, "completed", result={"ok": True, "count": 0, "matches": []})
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "4", "method": "tasks/get", "params": {"taskId": task_id}},
        session_id="test-session-inline",
    ))
    task_obj = resp["result"]
    assert task_obj["status"] == "completed"
    assert task_obj["result"] == {"ok": True, "count": 0, "matches": []}


def test_sep2663_tasks_get_failed_task_has_error():
    from simone_mcp.protocol import _create_task, _update_task, handle_mcp_request
    task = _create_task("sin_simone_mcp_symbol_search", {"query": "baz"}, "test-session-fail")
    task_id = task["id"]
    _update_task(task_id, "failed", error="something broke")
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "6", "method": "tasks/get", "params": {"taskId": task_id}},
        session_id="test-session-fail",
    ))
    task_obj = resp["result"]
    assert task_obj["status"] == "failed"
    assert task_obj["error"] == "something broke"


def test_sep2663_tasks_update_resumes_input_required():
    from simone_mcp.protocol import _create_task, _update_task, handle_mcp_request
    task = _create_task("sin_simone_mcp_symbol_search", {"query": "qux"}, "test-session-update")
    task_id = task["id"]
    _update_task(task_id, "input_required", statusMessage="Need more input")
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "8", "method": "tasks/update", "params": {"taskId": task_id, "input": {"extra": "data"}}},
        session_id="test-session-update",
    ))
    assert resp["result"]["status"] == "working"


def test_sep2663_tasks_update_rejects_non_input_required():
    from simone_mcp.protocol import _create_task, handle_mcp_request
    task = _create_task("sin_simone_mcp_symbol_search", {"query": "quux"}, "test-session-update-reject")
    task_id = task["id"]
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "10", "method": "tasks/update", "params": {"taskId": task_id, "input": {}}},
        session_id="test-session-update-reject",
    ))
    assert "error" in resp
    assert resp["error"]["code"] == -32602
    assert "not awaiting input" in resp["error"]["message"]


def test_sep2663_removed_tasks_result_returns_method_not_found():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "11", "method": "tasks/result", "params": {"taskId": "nonexistent"}},
        session_id="test-removed",
    ))
    assert "error" in resp
    assert resp["error"]["code"] == -32601


def test_sep2663_removed_tasks_list_returns_method_not_found():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "12", "method": "tasks/list", "params": {}},
        session_id="test-removed2",
    ))
    assert "error" in resp
    assert resp["error"]["code"] == -32601


def test_sep2663_notifications_tasks_not_status():
    from simone_mcp.protocol import _run_task, _create_task, _update_task
    notifications_sent = []

    async def capture_notification(n):
        notifications_sent.append(n)

    task = _create_task("sin_simone_mcp_symbol_search", {"query": "test"}, "test-notif-session")
    _update_task(task["id"], "completed", result={"ok": True, "count": 0, "matches": []})
    asyncio.run(_run_task(task["id"], {"action": "sin_simone_mcp_symbol_search", "query": "test"}, capture_notification))
    for n in notifications_sent:
        assert n["method"] == "notifications/tasks"
        assert n["method"] != "notifications/tasks/status"


async def _test_sep2663_tasks_cancel_still_works_async():
    from simone_mcp.protocol import _create_task, _update_task, handle_mcp_request
    task = _create_task("sin_simone_mcp_symbol_search", {"query": "cancel-test"}, "test-cancel-session")
    task_id = task["id"]
    _update_task(task_id, "working")
    resp, _, _ = await handle_mcp_request(
        {"jsonrpc": "2.0", "id": "14", "method": "tasks/cancel", "params": {"taskId": task_id}},
        session_id="test-cancel-session",
    )
    assert resp["result"]["status"] == "cancelled"


def test_sep2663_tasks_cancel_still_works():
    asyncio.run(_test_sep2663_tasks_cancel_still_works_async())


# ─── SEP-2243: HTTP Header Standardization ─────────────────────────────────────

def test_sep2243_mcp_method_header_mismatch():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)

    async def _test():
        async with AsyncClient(transport=transport, base_url="http://test") as client:
            init_resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "1", "method": "initialize", "params": {"protocolVersion": "2026-06-30"}})
            session_id = init_resp.headers.get("mcp-session-id")
            headers = {"Mcp-Session-Id": session_id, "Mcp-Method": "tools/list"}
            resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "2", "method": "tools/call", "params": {"name": "sin_simone_mcp_health"}}, headers=headers)
            body = resp.json()
            assert "error" in body
            assert body["error"]["code"] == -32001
            assert "HeaderMismatch" in body["error"]["message"]

    asyncio.run(_test())


def test_sep2243_mcp_method_header_matches():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)

    async def _test():
        async with AsyncClient(transport=transport, base_url="http://test") as client:
            init_resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "1", "method": "initialize", "params": {"protocolVersion": "2026-06-30"}})
            session_id = init_resp.headers.get("mcp-session-id")
            headers = {"Mcp-Session-Id": session_id, "Mcp-Method": "ping"}
            resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "2", "method": "ping"}, headers=headers)
            body = resp.json()
            assert "result" in body
            assert "error" not in body

    asyncio.run(_test())


def test_sep2243_mcp_name_header_mismatch():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)

    async def _test():
        async with AsyncClient(transport=transport, base_url="http://test") as client:
            init_resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "1", "method": "initialize", "params": {"protocolVersion": "2026-06-30"}})
            session_id = init_resp.headers.get("mcp-session-id")
            headers = {"Mcp-Session-Id": session_id, "Mcp-Method": "tools/call", "Mcp-Name": "wrong_tool"}
            resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "2", "method": "tools/call", "params": {"name": "sin_simone_mcp_health"}}, headers=headers)
            body = resp.json()
            assert "error" in body
            assert body["error"]["code"] == -32001
            assert "Mcp-Name" in body["error"]["message"]

    asyncio.run(_test())


def test_sep2243_mcp_param_header_mismatch():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)

    async def _test():
        async with AsyncClient(transport=transport, base_url="http://test") as client:
            init_resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "1", "method": "initialize", "params": {"protocolVersion": "2026-06-30"}})
            session_id = init_resp.headers.get("mcp-session-id")
            headers = {"Mcp-Session-Id": session_id, "Mcp-Method": "tools/call", "Mcp-Name": "sin_simone_mcp_symbol_search", "Mcp-Param-Query": "wrong_value"}
            resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "2", "method": "tools/call", "params": {"name": "sin_simone_mcp_symbol_search", "arguments": {"query": "actual_query"}}}, headers=headers)
            body = resp.json()
            assert "error" in body
            assert body["error"]["code"] == -32001
            assert "Mcp-Param-" in body["error"]["message"]

    asyncio.run(_test())


def test_sep2243_mcp_name_header_matches():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)

    async def _test():
        async with AsyncClient(transport=transport, base_url="http://test") as client:
            init_resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "1", "method": "initialize", "params": {"protocolVersion": "2026-06-30"}})
            session_id = init_resp.headers.get("mcp-session-id")
            headers = {"Mcp-Session-Id": session_id, "Mcp-Method": "tools/call", "Mcp-Name": "sin_simone_mcp_health"}
            resp = await client.post("/mcp", json={"jsonrpc": "2.0", "id": "2", "method": "tools/call", "params": {"name": "sin_simone_mcp_health"}}, headers=headers)
            body = resp.json()
            assert "result" in body

    asyncio.run(_test())


# ─── SEP-2549: TTL for List Results ────────────────────────────────────────────

def test_sep2549_tools_list_has_ttl():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "20", "method": "tools/list", "params": {}},
        session_id="test-ttl",
    ))
    result = resp["result"]
    assert "ttlMs" in result
    assert "cacheScope" in result
    assert isinstance(result["ttlMs"], int)
    assert result["cacheScope"] == "session"


def test_sep2549_resources_list_has_ttl():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "21", "method": "resources/list", "params": {}},
        session_id="test-ttl2",
    ))
    result = resp["result"]
    assert "ttlMs" in result
    assert result["cacheScope"] == "session"


def test_sep2549_resource_templates_list_has_ttl():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "22", "method": "resources/templates/list", "params": {}},
        session_id="test-ttl3",
    ))
    result = resp["result"]
    assert "ttlMs" in result
    assert result["cacheScope"] == "session"


def test_sep2549_prompts_list_has_ttl():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "23", "method": "prompts/list", "params": {}},
        session_id="test-ttl4",
    ))
    result = resp["result"]
    assert "ttlMs" in result
    assert result["cacheScope"] == "session"


# ─── Schema updates for SEP-2663 ──────────────────────────────────────────────

def test_sep2663_task_update_args_schema():
    from simone_mcp.schemas import TaskUpdateArgs
    valid = TaskUpdateArgs(taskId="abc123", input={"extra": "data"})
    assert valid.taskId == "abc123"
    assert valid.input == {"extra": "data"}


def test_sep2663_task_list_args_removed():
    from simone_mcp.schemas import TOOL_ARG_MODELS
    assert "tasks/list" not in TOOL_ARG_MODELS
    assert "tasks/update" in TOOL_ARG_MODELS


def test_sep2663_task_get_args_schema():
    from simone_mcp.schemas import TaskGetArgs
    valid = TaskGetArgs(taskId="abc")
    assert valid.taskId == "abc"
    alias_valid = TaskGetArgs(id="xyz")
    assert alias_valid.taskId == "xyz"


# ─── Protocol: Resources Subscribe/Unsubscribe ────────────────────────────────

def test_protocol_resources_subscribe():
    from simone_mcp.protocol import handle_mcp_request
    resp, session_id, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "resources/subscribe", "params": {"uri": "file:///test.py"}},
        session_id="test-subscribe",
    ))
    assert resp is not None
    assert "result" in resp
    assert resp["result"] == {}


def test_protocol_resources_unsubscribe():
    from simone_mcp.protocol import handle_mcp_request
    resp, session_id, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "resources/unsubscribe", "params": {"uri": "file:///test.py"}},
        session_id="test-unsubscribe",
    ))
    assert resp is not None
    assert "result" in resp
    assert resp["result"] == {}


# ─── Protocol: Prompts Get with Missing Args ──────────────────────────────────

def test_protocol_prompts_get_missing_required_arg():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "prompts/get", "params": {"name": "code_review"}},
        session_id="test-prompts",
    ))
    assert "error" in resp
    assert resp["error"]["code"] == -32602
    assert "Missing required argument" in resp["error"]["message"]


def test_protocol_prompts_get_unknown():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "prompts/get", "params": {"name": "nonexistent_prompt"}},
        session_id="test-prompts",
    ))
    assert "error" in resp
    assert resp["error"]["code"] == -32602
    assert "Unknown prompt" in resp["error"]["message"]


# ─── Protocol: Logging SetLevel ───────────────────────────────────────────────

def test_protocol_logging_set_level():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "logging/setLevel", "params": {"level": "debug"}},
        session_id="test-logging",
    ))
    assert resp is not None
    assert "result" in resp


def test_protocol_logging_invalid_level():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "logging/setLevel", "params": {"level": "INVALID"}},
        session_id="test-logging",
    ))
    assert "error" in resp
    assert resp["error"]["code"] == -32602
    assert "Invalid log level" in resp["error"]["message"]


# ─── Protocol: Completion ─────────────────────────────────────────────────────

def test_protocol_completion():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "completion/complete", "params": {"ref": {"type": "ref/prompt"}, "argument": {"name": "language", "value": "py"}}},
        session_id="test-completion",
    ))
    assert resp is not None
    assert "result" in resp
    assert "completion" in resp["result"]


# ─── Protocol: Sampling/Elicitation Not Supported ─────────────────────────────

def test_protocol_sampling_not_supported():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "sampling/createMessage", "params": {}},
        session_id="test-sampling",
    ))
    assert "error" in resp
    assert resp["error"]["code"] == -1
    assert "Sampling not supported" in resp["error"]["message"]


def test_protocol_elicitation_not_supported():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "elicitation/create", "params": {"message": "test", "schema": {"enum": ["a", "b"]}}},
        session_id="test-elicitation",
    ))
    assert "error" in resp
    assert resp["error"]["code"] == -1
    assert "Elicitation not supported" in resp["error"]["message"]


# ─── Protocol: Resources Read ─────────────────────────────────────────────────

def test_protocol_resources_read_not_found():
    from simone_mcp.protocol import handle_mcp_request
    resp, _, _ = asyncio.run(handle_mcp_request(
        {"jsonrpc": "2.0", "id": "1", "method": "resources/read", "params": {"uri": "file:///nonexistent.py"}},
        session_id="test-resources",
    ))
    assert "error" in resp
    assert resp["error"]["code"] == -32002


# ─── A2A: Tool Call with Invalid Tool ─────────────────────────────────────────

def test_a2a_tool_call_invalid_tool():
    result = asyncio.run(
        handle_a2a_request(
            {"id": "99", "jsonrpc": "2.0", "method": "tool.call", "params": {"name": "nonexistent_tool"}},
            "http://localhost:8234",
        )
    )
    assert "result" in result
    assert result["result"]["data"]["ok"] is False
    assert result["result"]["data"]["error"] == "unknown_action"


# ─── MCP Stdio: serve_stdio basic flow ────────────────────────────────────────

def test_mcp_stdio_initialization():
    from simone_mcp.mcp_stdio import serve_stdio
    import io
    import sys

    stdin_data = json.dumps({"jsonrpc": "2.0", "id": "1", "method": "initialize", "params": {"protocolVersion": "2026-06-30"}}) + "\n"
    old_stdin = sys.stdin
    old_stdout = sys.stdout
    sys.stdin = io.StringIO(stdin_data)
    sys.stdout = io.StringIO()

    try:
        asyncio.run(serve_stdio())
        output = sys.stdout.getvalue()
        assert "initialize" in output.lower() or "result" in output
    finally:
        sys.stdin = old_stdin
        sys.stdout = old_stdout


# ─── CLI: validate command ────────────────────────────────────────────────────

def test_cli_validate_basic():
    from simone_mcp.cli import _validate_config
    import os
    os.environ.pop("SIMONE_AUTH_REQUIRED", None)
    os.environ.pop("SIMONE_OAUTH_JWKS_URL", None)
    os.environ.pop("QDRANT_URL", None)
    os.environ.pop("NEO4J_URI", None)
    _validate_config()


# ─── Security: Rate Limiting ──────────────────────────────────────────────────

def test_rate_limit_triggers_when_bucket_full():
    import time
    from simone_mcp.http_app import _check_rate_limit, _rate_limit_store, _RATE_LIMIT_WINDOW, _RATE_LIMIT_MAX
    _rate_limit_store.clear()
    client_id = "test-client"
    now = time.monotonic()
    _rate_limit_store[client_id] = [now - 1] * _RATE_LIMIT_MAX
    from fastapi import HTTPException
    try:
        _check_rate_limit(client_id)
        assert False, "Expected HTTPException"
    except HTTPException as e:
        assert e.status_code == 429
        assert "Retry-After" in e.headers
    _rate_limit_store.clear()


def test_rate_limit_allows_under_max():
    import time
    from simone_mcp.http_app import _check_rate_limit, _rate_limit_store
    _rate_limit_store.clear()
    client_id = "test-client"
    _check_rate_limit(client_id)
    assert len(_rate_limit_store[client_id]) == 1
    _rate_limit_store.clear()


def test_rate_limit_window_expires_old_entries():
    import time
    from simone_mcp.http_app import _check_rate_limit, _rate_limit_store, _RATE_LIMIT_WINDOW
    _rate_limit_store.clear()
    client_id = "test-client"
    now = time.monotonic()
    _rate_limit_store[client_id] = [now - _RATE_LIMIT_WINDOW - 1]
    _check_rate_limit(client_id)
    assert len(_rate_limit_store[client_id]) == 1
    _rate_limit_store.clear()


# ─── Security: CORS / Origin Validation ─────────────────────────────────────

def test_cors_blocks_disallowed_origin():
    from fastapi import HTTPException
    from simone_mcp.http_app import _validate_origin
    class FakeRequest:
        def __init__(self, origin):
            self.headers = {"origin": origin}
    try:
        _validate_origin(FakeRequest("https://evil.com"))
        assert False, "Expected HTTPException"
    except HTTPException as e:
        assert e.status_code == 403
        assert "origin_not_allowed" in e.detail


def test_cors_allows_whitelisted_origin():
    from simone_mcp.http_app import _validate_origin
    class FakeRequest:
        def __init__(self, origin):
            self.headers = {"origin": origin}
    _validate_origin(FakeRequest("http://localhost"))


def test_cors_allows_no_origin_header():
    from simone_mcp.http_app import _validate_origin
    class FakeRequest:
        def __init__(self):
            self.headers = {}
    _validate_origin(FakeRequest())


# ─── Security: Auth / Bearer Token ────────────────────────────────────────────

def _reset_auth_cache():
    import os
    os.environ.pop("SIMONE_AUTH_REQUIRED", None)
    os.environ.pop("SIMONE_OAUTH_JWKS_URL", None)
    # Reset the module-level cache
    import simone_mcp.http_app as http_app
    http_app._auth_required_cache = None


def test_auth_bypasses_open_paths():
    from simone_mcp.http_app import _authorize_request, OPEN_PATHS
    _reset_auth_cache()
    os.environ["SIMONE_AUTH_REQUIRED"] = "true"
    os.environ["SIMONE_OAUTH_JWKS_URL"] = "http://localhost:9999/.well-known/jwks.json"
    class FakeRequest:
        def __init__(self, path, headers=None):
            self.url = type("URL", (), {"path": path})
            self.headers = headers or {}
    for path in OPEN_PATHS:
        result = _authorize_request(FakeRequest(path))
        assert result is None, f"Open path {path} should bypass auth"
    _reset_auth_cache()


def test_auth_rejects_missing_bearer():
    from fastapi import HTTPException
    from simone_mcp.http_app import _authorize_request
    _reset_auth_cache()
    os.environ["SIMONE_AUTH_REQUIRED"] = "true"
    os.environ["SIMONE_OAUTH_JWKS_URL"] = "http://localhost:9999/.well-known/jwks.json"
    class FakeRequest:
        def __init__(self, headers=None):
            self.url = type("URL", (), {"path": "/mcp"})
            self.headers = headers or {}
    try:
        _authorize_request(FakeRequest())
        assert False, "Expected HTTPException"
    except HTTPException as e:
        assert e.status_code == 401
        assert "missing_bearer_token" in e.detail
    _reset_auth_cache()


def test_auth_rejects_invalid_scheme():
    from fastapi import HTTPException
    from simone_mcp.http_app import _authorize_request
    _reset_auth_cache()
    os.environ["SIMONE_AUTH_REQUIRED"] = "true"
    os.environ["SIMONE_OAUTH_JWKS_URL"] = "http://localhost:9999/.well-known/jwks.json"
    class FakeRequest:
        def __init__(self, headers=None):
            self.url = type("URL", (), {"path": "/mcp"})
            self.headers = headers or {}
    try:
        _authorize_request(FakeRequest({"authorization": "Basic abc123"}))
        assert False, "Expected HTTPException"
    except HTTPException as e:
        assert e.status_code == 401
    _reset_auth_cache()


# ─── Security: Request Body Size ─────────────────────────────────────────────

def test_body_size_limit():
    from fastapi import HTTPException
    from simone_mcp.http_app import _MAX_REQUEST_BODY
    assert _MAX_REQUEST_BODY == 1048576


# ─── Security: Client IP Extraction ───────────────────────────────────────────

def test_extract_client_ip_from_x_forwarded_for():
    from simone_mcp.http_app import _extract_client_ip
    class FakeRequest:
        def __init__(self, headers, client=None):
            self.headers = headers
            self.client = client
    req = FakeRequest({"x-forwarded-for": "1.2.3.4, 5.6.7.8"})
    assert _extract_client_ip(req) == "5.6.7.8"


def test_extract_client_ip_fallback():
    from simone_mcp.http_app import _extract_client_ip
    class FakeRequest:
        def __init__(self, headers, client=None):
            self.headers = headers
            self.client = client
    req = FakeRequest({}, type("Client", (), {"host": "192.168.1.1"}))
    assert _extract_client_ip(req) == "192.168.1.1"


# ─── HTTP: DELETE Session ───────────────────────────────────────────────────

@pytest.mark.asyncio
async def test_http_delete_session():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.request("DELETE", "/mcp", headers={"Mcp-Session-Id": "test-session-123"})
        assert resp.status_code == 202


@pytest.mark.asyncio
async def test_http_delete_session_missing_id():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.request("DELETE", "/mcp")
        assert resp.status_code == 400
        assert "missing_session_id" in resp.json()["detail"]


# ─── HTTP: Basic Endpoint Smoke Tests ────────────────────────────────────────

@pytest.mark.asyncio
async def test_http_health_endpoint():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.get("/health")
        assert resp.status_code == 200
        data = resp.json()
        assert data["ok"] is True
        assert data["name"] == "simone-mcp"


@pytest.mark.asyncio
async def test_http_root_endpoint():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.get("/")
        assert resp.status_code == 200
        assert resp.json()["name"] == "simone-mcp"


@pytest.mark.asyncio
async def test_http_well_known_agent_card():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.get("/.well-known/agent-card.json")
        assert resp.status_code == 200
        data = resp.json()
        assert data["name"] == "simone-mcp"
        assert "sin_simone_mcp_health" in data["capabilities"]


@pytest.mark.asyncio
async def test_http_a2a_endpoint():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/a2a/v1", json={"jsonrpc": "2.0", "id": "1", "method": "agent.discover"})
        assert resp.status_code == 200
        data = resp.json()
        assert data["result"]["name"] == "simone-mcp"


# ─── HTTP: Invalid JSON Body ─────────────────────────────────────────────────

@pytest.mark.asyncio
async def test_http_invalid_json_body():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/mcp", content=b"not json{{{", headers={"Content-Type": "application/json"})
        assert resp.status_code == 400
        assert "invalid_json" in resp.json()["detail"]


# ─── HTTP: Dashboard ────────────────────────────────────────────────────────

@pytest.mark.asyncio
async def test_http_dashboard():
    from simone_mcp.http_app import create_app
    from httpx import AsyncClient, ASGITransport
    app = create_app()
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.get("/dashboard")
        assert resp.status_code == 200
        assert "Simone MCP" in resp.text
