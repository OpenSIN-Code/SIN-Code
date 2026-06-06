"""
Purpose: Generate pytest test cases for an MCP tool.
Docs: test_gen.doc.md
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path

# ── Test templates ──────────────────────────────────────

PYTEST_TEST_TEMPLATES: dict[str, str] = {
    "basic": '''
def test_{tool_name}():
    """Smoke test: `{tool_name}` returns a JSON object."""
    result = {tool_name}({args})
    data = json.loads(result)
    assert isinstance(data, dict)
''',
    "parametrized": '''
@pytest.mark.parametrize(
    {parametrize_args},
)
def test_{tool_name}({param_names}):
    """Parametrized test for `{tool_name}`."""
    result = {tool_name}({args})
    data = json.loads(result)
    assert isinstance(data, dict)
''',
    "error": '''
def test_{tool_name}_error():
    """Ensure `{tool_name}` surfaces errors as a JSON envelope."""
    result = {tool_name}({error_args})
    data = json.loads(result)
    assert "error" in data
''',
}


@dataclass
class TestGenerator:
    """Generate pytest test cases for an MCP tool from its signature.

    The generator inspects the source of `mcp_server.py`, finds the
    `@mcp.tool()`-decorated function matching `tool_name`, and emits a
    small set of tests: smoke, parametrized (one per param), and error.
    """

    # pytest class-collector hint — this is not a test class.
    __test__ = False

    def generate(
        self,
        server_path: Path,
        tool_name: str,
        output_path: Path | None = None,
    ) -> str:
        """
        Generate tests for `tool_name` defined in `server_path`.

        Args:
            server_path: Path to the `mcp_server.py` file.
            tool_name: Name of the tool function.
            output_path: If set, append to this file. Otherwise return string.

        Returns:
            The generated test code (always; also written if `output_path`
            is given).
        """
        if not server_path.is_file():
            raise FileNotFoundError(f"Server file not found: {server_path}")
        text = server_path.read_text(encoding="utf-8")
        params = self._extract_params(text, tool_name)
        test_code = self._render_tests(tool_name, params)
        if output_path is not None:
            existing = (
                output_path.read_text(encoding="utf-8") if output_path.is_file() else ""
            )
            output_path.write_text(
                existing.rstrip() + "\n" + test_code, encoding="utf-8"
            )
        return test_code

    # ── Internals ──────────────────────────────────────

    _DEF_RE = re.compile(
        r"@mcp\.tool\(\)\s*\n\s*def\s+(?P<name>\w+)\((?P<params>[^)]*)\)",
        re.MULTILINE,
    )

    def _extract_params(self, source: str, tool_name: str) -> list[tuple[str, str]]:
        """Return [(name, type_str), ...] for the tool's signature."""
        m = self._DEF_RE.search(source)
        if not m or m.group("name") != tool_name:
            return []
        raw = m.group("params").strip()
        if not raw:
            return []
        out: list[tuple[str, str]] = []
        for piece in raw.split(","):
            piece = piece.strip()
            if not piece or piece in ("self", "cls"):
                continue
            if ":" in piece:
                name_part, type_part = piece.split(":", 1)
                type_str = type_part.split("=")[0].strip()
            else:
                name_part, type_str = piece, "str"
            out.append((name_part.strip(), type_str))
        return out

    def _render_tests(self, tool_name: str, params: list[tuple[str, str]]) -> str:
        """Render the basic + parametrize + error tests for a tool."""
        args_str = ", ".join(f"{n}={_sample_value(t)}" for n, t in params)
        blocks: list[str] = []
        blocks.append(
            PYTEST_TEST_TEMPLATES["basic"].format(
                tool_name=tool_name,
                args=args_str,
            )
        )
        if params:
            parametrize_rows = []
            param_names = []
            for n, t in params:
                parametrize_rows.append(f'    ("{_sample_value(t)}",),')
                param_names.append(n)
            blocks.append(
                PYTEST_TEST_TEMPLATES["parametrized"].format(
                    tool_name=tool_name,
                    parametrize_args="\n".join(parametrize_rows),
                    param_names=", ".join(param_names),
                    args=", ".join(f"{n}={n}" for n in param_names),
                )
            )
        # Error test: pass empty/None to see if tool degrades gracefully.
        error_args = ", ".join(f"{n}={_empty_value(t)}" for n, t in params)
        blocks.append(
            PYTEST_TEST_TEMPLATES["error"].format(
                tool_name=tool_name,
                error_args=error_args or "",
            )
        )
        return "\n".join(blocks)


def _sample_value(type_str: str) -> str:
    """Default sample value for a type (used in basic + parametrize)."""
    t = type_str.lower().strip()
    if t in ("str", "string"):
        return '"x"'
    if t in ("int", "integer"):
        return "1"
    if t in ("float", "double"):
        return "0.5"
    if t in ("bool", "boolean"):
        return "True"
    if t in ("list", "list[str]"):
        return '["a", "b"]'
    if t in ("dict", "dict[str, str]", "dict[str, any]", "dict"):
        return '{"key": "val"}'
    return '"x"'


def _empty_value(type_str: str) -> str:
    """Empty / boundary value used in the error test."""
    t = type_str.lower().strip()
    if "int" in t or "float" in t:
        return "0"
    if "bool" in t:
        return "False"
    if "list" in t or "dict" in t:
        return "[]" if "list" in t else "{}"
    return '""'
