"""
Purpose: Add a new tool to an existing MCP server, preserving CoDocs structure.
Docs: tool_adder.doc.md
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from pathlib import Path

# ── Constants ──────────────────────────────────────
PYTHON_TOOL_TEMPLATE = '''

@mcp.tool()
def {tool_name}({params}) -> str:
    """
    {docstring_summary}

    Args:
{param_docs}

    Returns:
        JSON string with the result.
    """
    {body}
    return json.dumps(result)
'''

PYTHON_TEST_TEMPLATE = '''

def test_{tool_name}():
    """Test that `{tool_name}` returns the expected JSON envelope."""
    result = {tool_name}({args})
    data = json.loads(result)
    assert isinstance(data, dict)
'''


@dataclass
class ToolSpec:
    """
    Specification for a new tool to add.

    Attributes:
        name: Snake-case tool name (must be a valid Python identifier).
        description: One-line docstring for the tool.
        params: List of (name, type, default) tuples. type is e.g. "str", "int".
        returns: Return type description for the docstring.
        body: Raw Python body (statements) executed inside the tool.
    """

    name: str
    description: str
    params: list[tuple[str, str, str]] = field(default_factory=list)
    returns: str = "dict"
    body: str = 'result = {"ok": True}'

    def __post_init__(self) -> None:
        if not self.name.isidentifier():
            raise ValueError(f"Tool name must be a valid identifier: {self.name!r}")


@dataclass
class ToolAdder:
    """Add a new tool to an existing Python FastMCP server.

    The adder is deliberately conservative: it reads the existing
    `mcp_server.py`, locates the `mcp = FastMCP(...)` declaration and any
    existing `@mcp.tool()` block, and appends the new tool after the last
    `@mcp.tool()` decorator. CoDocs (`@file.doc.md`) are NOT touched.
    """

    def add_to_python(self, server_path: Path, spec: ToolSpec) -> str:
        """
        Append a new tool to `server_path` (a `mcp_server.py`).

        Returns:
            The new tool's full source code.
        """
        if not server_path.is_file():
            raise FileNotFoundError(f"Server file not found: {server_path}")
        new_tool = self.render_python_tool(spec)
        existing = server_path.read_text(encoding="utf-8")
        insertion_point = self._find_last_tool_block(existing)
        if insertion_point is None:
            # No existing @mcp.tool() — insert after the `mcp = FastMCP(...)` line.
            insertion_point = self._find_mcp_declaration(existing)
        new_content = (
            existing[:insertion_point] + new_tool + "\n\n" + existing[insertion_point:]
        )
        server_path.write_text(new_content, encoding="utf-8")
        return new_tool

    def add_test(self, test_path: Path, spec: ToolSpec) -> str:
        """Append a new pytest test for `spec` to `test_path`."""
        if not test_path.is_file():
            raise FileNotFoundError(f"Test file not found: {test_path}")
        new_test = PYTHON_TEST_TEMPLATE.format(
            tool_name=spec.name,
            args=", ".join(f"{p[0]}={_sample_value(p[1])}" for p in spec.params) or "",
        )
        existing = test_path.read_text(encoding="utf-8")
        test_path.write_text(existing.rstrip() + "\n" + new_test, encoding="utf-8")
        return new_test

    # ── Rendering helpers ──────────────────────────────────────

    def render_python_tool(self, spec: ToolSpec) -> str:
        """Render the Python source code for `spec`."""
        param_strs = []
        param_docs = []
        for name, type_, default in spec.params:
            if default:
                param_strs.append(f"{name}: {type_} = {default}")
            else:
                param_strs.append(f"{name}: {type_}")
            param_docs.append(f"        {name}: {type_} parameter.")
        if not param_strs:
            param_strs.append("")  # Empty parens.
        return PYTHON_TOOL_TEMPLATE.format(
            tool_name=spec.name,
            params=", ".join(param_strs),
            docstring_summary=spec.description or f"Tool: {spec.name}",
            param_docs="\n".join(param_docs) if param_docs else "        (none)",
            body=spec.body,
        )

    # ── String finders ──────────────────────────────────────

    _TOOL_DECORATOR_RE = re.compile(r"@mcp\.tool\(\)", re.MULTILINE)
    _FASTMCP_RE = re.compile(r"^mcp\s*=\s*FastMCP\(", re.MULTILINE)

    def _find_last_tool_block(self, text: str) -> int | None:
        """Return the index right after the last `@mcp.tool()` decorator."""
        matches = list(self._TOOL_DECORATOR_RE.finditer(text))
        if not matches:
            return None
        last = matches[-1]
        # Skip to the end of the next function signature ("def name(...):").
        rest = text[last.end() :]
        m = re.search(r"^def\s+\w+\([^)]*\)\s*->\s*[^:]+:\s*$", rest, re.MULTILINE)
        if not m:
            return last.end()
        # Skip past the docstring ("""...""") if present.
        after_def = rest[m.end() :]
        doc = re.match(r'\s*"""[\s\S]*?"""', after_def, re.DOTALL)
        if doc:
            return last.start() + (m.end() + doc.end())
        return last.start() + m.end()

    def _find_mcp_declaration(self, text: str) -> int:
        """Find the line right after `mcp = FastMCP(...)` declaration."""
        m = self._FASTMCP_RE.search(text)
        if not m:
            return 0
        # Advance to end-of-line.
        return text.find("\n", m.end()) + 1


def _sample_value(type_str: str) -> str:
    """Pick a reasonable default value for a sample pytest argument."""
    t = type_str.lower().strip()
    if t in ("str", "string"):
        return '"x"'
    if t in ("int", "integer"):
        return "1"
    if t in ("float", "double"):
        return "0.0"
    if t in ("bool", "boolean"):
        return "True"
    if t in ("list", "list[str]"):
        return "[]"
    if t in ("dict", "dict[str, str]"):
        return "{}"
    return '"x"'
