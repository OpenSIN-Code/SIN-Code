#!/usr/bin/env python3
"""Fail on duplicate definitions, duplicate statements and unreachable code."""

from __future__ import annotations

import ast
import json
import sys
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Iterable

TERMINATORS = (ast.Return, ast.Raise, ast.Break, ast.Continue)
SKIPPED_PARTS = {".git", ".venv", "venv", "__pycache__", "node_modules"}


@dataclass(frozen=True)
class Finding:
    path: str
    line: int
    code: str
    message: str
    scope: str


def python_files(root: Path) -> Iterable[Path]:
    for path in root.rglob("*.py"):
        if any(part in SKIPPED_PARTS for part in path.parts):
            continue
        yield path


def fingerprint(statement: ast.stmt) -> str:
    return ast.dump(statement, include_attributes=False)


def scan_block(
    body: list[ast.stmt],
    path: Path,
    scope: str,
    findings: list[Finding],
    *,
    function_scope: bool,
) -> None:
    definitions: dict[str, ast.AST] = {}
    previous: str | None = None
    terminated = False

    for statement in body:
        line = getattr(statement, "lineno", 0)
        if terminated:
            findings.append(
                Finding(
                    str(path),
                    line,
                    "UNREACHABLE_STATEMENT",
                    "Statement follows an unconditional terminator.",
                    scope,
                )
            )

        if isinstance(statement, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
            original = definitions.get(statement.name)
            if original is not None:
                findings.append(
                    Finding(
                        str(path),
                        line,
                        "DUPLICATE_DEFINITION",
                        f"{statement.name!r} already defined at line "
                        f"{getattr(original, 'lineno', 0)}.",
                        scope,
                    )
                )
            else:
                definitions[statement.name] = statement

        current = fingerprint(statement)
        harmless = isinstance(statement, ast.Pass) or (
            isinstance(statement, ast.Expr)
            and isinstance(statement.value, ast.Constant)
            and isinstance(statement.value.value, str)
        )
        if not harmless and current == previous:
            findings.append(
                Finding(
                    str(path),
                    line,
                    "DUPLICATE_ADJACENT_STATEMENT",
                    "Adjacent statement duplicates its predecessor.",
                    scope,
                )
            )
        previous = current

        child_scope = f"{scope}.{getattr(statement, 'name', type(statement).__name__)}"
        if isinstance(statement, (ast.FunctionDef, ast.AsyncFunctionDef)):
            scan_block(
                statement.body,
                path,
                child_scope,
                findings,
                function_scope=True,
            )
        elif isinstance(statement, ast.ClassDef):
            scan_block(
                statement.body,
                path,
                child_scope,
                findings,
                function_scope=False,
            )
        elif isinstance(statement, ast.If):
            scan_block(statement.body, path, f"{scope}.if", findings, function_scope=function_scope)
            scan_block(statement.orelse, path, f"{scope}.else", findings, function_scope=function_scope)
        elif isinstance(statement, (ast.For, ast.AsyncFor, ast.While)):
            scan_block(statement.body, path, f"{scope}.loop", findings, function_scope=function_scope)
            scan_block(statement.orelse, path, f"{scope}.loop_else", findings, function_scope=function_scope)
        elif isinstance(statement, ast.Try):
            scan_block(statement.body, path, f"{scope}.try", findings, function_scope=function_scope)
            for index, handler in enumerate(statement.handlers):
                scan_block(handler.body, path, f"{scope}.except[{index}]", findings, function_scope=function_scope)
            scan_block(statement.orelse, path, f"{scope}.try_else", findings, function_scope=function_scope)
            scan_block(statement.finalbody, path, f"{scope}.finally", findings, function_scope=function_scope)

        terminated = function_scope and isinstance(statement, TERMINATORS)


def scan_file(path: Path) -> list[Finding]:
    try:
        tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    except (OSError, UnicodeDecodeError, SyntaxError) as error:
        return [
            Finding(
                str(path),
                getattr(error, "lineno", 0) or 0,
                "PARSE_ERROR",
                str(error),
                "<module>",
            )
        ]

    findings: list[Finding] = []
    scan_block(tree.body, path, "<module>", findings, function_scope=False)
    return findings


def main() -> int:
    root = Path(sys.argv[1] if len(sys.argv) > 1 else "src").resolve()
    findings = [
        finding
        for path in python_files(root)
        for finding in scan_file(path)
    ]
    print(
        json.dumps(
            {
                "ok": not findings,
                "root": str(root),
                "finding_count": len(findings),
                "findings": [asdict(item) for item in findings],
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return 1 if findings else 0


if __name__ == "__main__":
    raise SystemExit(main())
