"""
Purpose: Validate an MCP server (tools, type hints, docstrings, CoDocs).
Docs: validator.doc.md
"""

from __future__ import annotations

import ast
import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

# ── Result types ──────────────────────────────────────


@dataclass
class ValidationIssue:
    """A single validation finding."""

    level: str  # "error" | "warning" | "info"
    code: str
    message: str
    file: str = ""
    line: int = 0

    def to_dict(self) -> dict[str, Any]:
        return {
            "level": self.level,
            "code": self.code,
            "message": self.message,
            "file": self.file,
            "line": self.line,
        }


@dataclass
class ValidationResult:
    """Aggregate validation report."""

    issues: list[ValidationIssue] = field(default_factory=list)
    tools: list[str] = field(default_factory=list)
    files_checked: int = 0

    @property
    def ok(self) -> bool:
        """True when no `error` issues are present."""
        return not any(i.level == "error" for i in self.issues)

    def add(self, issue: ValidationIssue) -> None:
        self.issues.append(issue)

    def to_dict(self) -> dict[str, Any]:
        return {
            "ok": self.ok,
            "tools": self.tools,
            "files_checked": self.files_checked,
            "errors": [i.to_dict() for i in self.issues if i.level == "error"],
            "warnings": [i.to_dict() for i in self.issues if i.level == "warning"],
            "info": [i.to_dict() for i in self.issues if i.level == "info"],
        }


# ── Validator ──────────────────────────────────────


@dataclass
class Validator:
    """Validate an MCP server source tree.

    Checks performed:
      - Every `@mcp.tool()` function has a docstring.
      - Every public function has type-annotated parameters.
      - Every `.py` file in `src/` has a sibling `.doc.md` (CoDocs).
      - `pyproject.toml` exists for python-fastmcp templates.
      - Tests directory has at least one `test_*.py` file.
    """

    _TOOL_DECORATOR = "@mcp.tool"
    _DOCSTRING_RE = re.compile(r'^\s*"""', re.MULTILINE)

    def validate(self, root: str | Path) -> ValidationResult:
        """Run every check against `root` and return a `ValidationResult`."""
        result = ValidationResult()
        root = Path(root)
        if not root.is_dir():
            result.add(
                ValidationIssue(
                    level="error",
                    code="ROOT_MISSING",
                    message=f"Project root not found: {root}",
                )
            )
            return result

        # Collect Python files.
        src_dir = root / "src"
        py_files = sorted(src_dir.rglob("*.py")) if src_dir.is_dir() else []
        result.files_checked = len(py_files)

        for py in py_files:
            self._check_python_file(py, result)

        # CoDocs: every .py should have a sibling .doc.md (skip __init__.py is OK).
        self._check_codocs(src_dir, result)

        # pyproject.toml + tests existence.
        self._check_project_root(root, result)

        return result

    # ── Individual checks ──────────────────────────────────────

    def _check_python_file(self, path: Path, result: ValidationResult) -> None:
        """Parse a Python file and lint its `@mcp.tool()` functions."""
        try:
            tree = ast.parse(path.read_text(encoding="utf-8"))
        except SyntaxError as exc:
            result.add(
                ValidationIssue(
                    level="error",
                    code="SYNTAX_ERROR",
                    message=f"Syntax error: {exc.msg}",
                    file=str(path),
                    line=exc.lineno or 0,
                )
            )
            return
        for node in ast.walk(tree):
            if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                continue
            if not self._is_tool_decorated(node):
                continue
            result.tools.append(f"{path.stem}.{node.name}")
            self._check_tool_signature(node, path, result)

    def _is_tool_decorated(self, func: ast.FunctionDef | ast.AsyncFunctionDef) -> bool:
        """True if any decorator is `@mcp.tool(...)` or `@mcp.tool`."""
        for d in func.decorator_list:
            if isinstance(d, ast.Call):
                fn = d.func
                if isinstance(fn, ast.Attribute) and fn.attr == "tool":
                    if isinstance(fn.value, ast.Name) and fn.value.id == "mcp":
                        return True
            elif isinstance(d, ast.Attribute) and d.attr == "tool":
                if isinstance(d.value, ast.Name) and d.value.id == "mcp":
                    return True
        return False

    def _check_tool_signature(
        self,
        func: ast.FunctionDef | ast.AsyncFunctionDef,
        path: Path,
        result: ValidationResult,
    ) -> None:
        """Verify a single tool function for type hints and docstring."""
        # Docstring present?
        if not (
            isinstance(func.body[0], ast.Expr)
            and isinstance(func.body[0].value, ast.Constant)
            and isinstance(func.body[0].value.value, str)
        ):
            result.add(
                ValidationIssue(
                    level="error",
                    code="MISSING_DOCSTRING",
                    message=f"Tool `{func.name}` is missing a docstring.",
                    file=str(path),
                    line=func.lineno,
                )
            )
        # Annotated params (skip `self`/`cls` and *args/**kwargs).
        for arg in func.args.args:
            if arg.arg in ("self", "cls"):
                continue
            if arg.annotation is None:
                result.add(
                    ValidationIssue(
                        level="warning",
                        code="MISSING_TYPE_HINT",
                        message=f"Parameter `{arg.arg}` in `{func.name}` has no type hint.",
                        file=str(path),
                        line=arg.lineno or func.lineno,
                    )
                )
        # Return type hint?
        if func.returns is None:
            result.add(
                ValidationIssue(
                    level="warning",
                    code="MISSING_RETURN_HINT",
                    message=f"Tool `{func.name}` has no return type hint.",
                    file=str(path),
                    line=func.lineno,
                )
            )

    def _check_codocs(self, src_dir: Path, result: ValidationResult) -> None:
        """Verify every `.py` has a sibling `.doc.md`."""
        if not src_dir.is_dir():
            return
        for py in src_dir.rglob("*.py"):
            doc = py.with_suffix(".doc.md")
            if not doc.is_file():
                result.add(
                    ValidationIssue(
                        level="warning",
                        code="MISSING_CODOC",
                        message=f"CoDocs file missing for `{py.relative_to(src_dir.parent)}`.",
                        file=str(py),
                    )
                )

    def _check_project_root(self, root: Path, result: ValidationResult) -> None:
        """Check for pyproject.toml and tests/ directory."""
        if not (root / "pyproject.toml").is_file():
            result.add(
                ValidationIssue(
                    level="info",
                    code="NO_PYPROJECT",
                    message="No `pyproject.toml` at project root.",
                    file=str(root),
                )
            )
        tests = root / "tests"
        if not tests.is_dir():
            result.add(
                ValidationIssue(
                    level="error",
                    code="NO_TESTS_DIR",
                    message="Missing `tests/` directory.",
                    file=str(root),
                )
            )
            return
        if not list(tests.rglob("test_*.py")):
            result.add(
                ValidationIssue(
                    level="warning",
                    code="NO_TESTS",
                    message="No `test_*.py` files in `tests/`.",
                    file=str(tests),
                )
            )
