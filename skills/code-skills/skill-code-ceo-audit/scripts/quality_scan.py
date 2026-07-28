#!/usr/bin/env python3
"""Evidence-focused static quality scan for the CEO Audit quality axis.

The scanner favors concrete, reproducible source locations over repository-wide
heuristic counters. Unsupported analyses are reported as skipped rather than
invented findings.
"""

from __future__ import annotations

import ast
import json
import re
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

MAX_FILE_BYTES = 2 * 1024 * 1024
MAX_EXAMPLES = 12
COMPLEXITY_THRESHOLD = 15
FUNCTION_LINES_THRESHOLD = 120
FILE_LINES_THRESHOLD = 1200
TODO_AGE_DAYS = 90
SOURCE_SUFFIXES = {".py", ".go", ".js", ".jsx", ".ts", ".tsx", ".rs", ".java", ".kt"}
EXCLUDED_PARTS = {
    ".git",
    ".venv",
    "venv",
    "env",
    "node_modules",
    "vendor",
    "dist",
    "build",
    "coverage",
    ".pytest_cache",
    "__pycache__",
    "tests",
    "test",
    "testdata",
    "fixtures",
    "fixture",
    "examples",
    "docs",
    "third_party",
    "ceo-audit-output",
}
TODO_RE = re.compile(r"\b(TODO|FIXME|HACK|XXX)\b")
PY_CAMEL_RE = re.compile(r"^[a-z]+(?:[A-Z][A-Za-z0-9]*)+$")
GO_FUNC_RE = re.compile(r"(?m)^\s*func\s+(?:\([^\n)]*\)\s*)?([A-Za-z_]\w*)\s*\([^\n]*?\)")


@dataclass(frozen=True)
class Gate:
    gate_id: str
    severity: str
    code: str
    title: str
    fix: str


GATES = {
    "3.1": Gate(
        "3.1",
        "MEDIUM",
        "QUALITY-COMPLEXITY",
        "Functions exceed complexity threshold",
        "Split decision-heavy functions and reduce nested control flow.",
    ),
    "3.2": Gate(
        "3.2",
        "MEDIUM",
        "QUALITY-FUNCTION-SIZE",
        "Functions exceed 120 lines",
        "Extract cohesive helpers and keep one responsibility per function.",
    ),
    "3.3": Gate(
        "3.3",
        "MEDIUM",
        "QUALITY-FILE-SIZE",
        "Production files exceed 1,200 lines",
        "Split oversized modules along stable domain boundaries.",
    ),
    "3.4": Gate(
        "3.4",
        "MEDIUM",
        "QUALITY-DUPLICATION",
        "Code duplication",
        "Run a token-aware duplicate-code engine and consolidate verified clones.",
    ),
    "3.5": Gate(
        "3.5",
        "MEDIUM",
        "QUALITY-DEAD",
        "Dead code",
        "Use a language-aware call graph before removing unreachable symbols.",
    ),
    "3.6": Gate(
        "3.6",
        "LOW",
        "QUALITY-NAMING",
        "Python functions violate snake_case",
        "Rename Python functions to snake_case and preserve compatibility aliases where needed.",
    ),
    "3.7": Gate(
        "3.7",
        "LOW",
        "QUALITY-TODO",
        "Production TODO markers older than 90 days",
        "Resolve, remove, or convert aged markers into tracked issues.",
    ),
}

SKIPPED_GATES = {
    "3.4": "token-aware duplicate-code engine is not bundled with this audit",
    "3.5": "dead-code claims require a complete language-aware call graph",
}


@dataclass(frozen=True)
class Source:
    path: Path
    text: str


@dataclass(frozen=True)
class Metric:
    location: str
    label: str
    value: int


def is_production_source(root: Path, path: Path) -> bool:
    try:
        rel = path.relative_to(root)
    except ValueError:
        return False
    parts = {part.lower() for part in rel.parts[:-1]}
    if parts & EXCLUDED_PARTS:
        return False
    name = path.name.lower()
    if path.suffix.lower() not in SOURCE_SUFFIXES:
        return False
    return not (
        name.startswith("test_")
        or name.endswith("_test.py")
        or name.endswith("_test.go")
        or ".test." in name
        or ".spec." in name
        or name.endswith(".min.js")
        or name.endswith(".generated.go")
        or name.endswith("_generated.go")
    )


def iter_sources(root: Path) -> Iterable[Source]:
    for path in root.rglob("*"):
        if not path.is_file() or not is_production_source(root, path):
            continue
        try:
            if path.stat().st_size > MAX_FILE_BYTES:
                continue
            yield Source(path, path.read_text(encoding="utf-8", errors="replace"))
        except OSError:
            continue


def rel_location(root: Path, path: Path, line: int) -> str:
    return f"{path.relative_to(root).as_posix()}:{line}"


class ComplexityVisitor(ast.NodeVisitor):
    """Count decision points while excluding nested function bodies."""

    def __init__(self, root: ast.AST) -> None:
        self.root = root
        self.score = 1

    def visit_FunctionDef(self, node: ast.FunctionDef) -> None:
        if node is self.root:
            self.generic_visit(node)

    def visit_AsyncFunctionDef(self, node: ast.AsyncFunctionDef) -> None:
        if node is self.root:
            self.generic_visit(node)

    def visit_If(self, node: ast.If) -> None:
        self.score += 1
        self.generic_visit(node)

    def visit_For(self, node: ast.For) -> None:
        self.score += 1
        self.generic_visit(node)

    visit_AsyncFor = visit_For

    def visit_While(self, node: ast.While) -> None:
        self.score += 1
        self.generic_visit(node)

    def visit_IfExp(self, node: ast.IfExp) -> None:
        self.score += 1
        self.generic_visit(node)

    def visit_BoolOp(self, node: ast.BoolOp) -> None:
        self.score += max(1, len(node.values) - 1)
        self.generic_visit(node)

    def visit_Try(self, node: ast.Try) -> None:
        self.score += len(node.handlers) + int(bool(node.orelse)) + int(bool(node.finalbody))
        self.generic_visit(node)

    def visit_Match(self, node: ast.Match) -> None:
        self.score += max(1, len(node.cases))
        self.generic_visit(node)

    def visit_comprehension(self, node: ast.comprehension) -> None:
        self.score += 1 + len(node.ifs)
        self.generic_visit(node)


def python_metrics(root: Path, source: Source) -> tuple[list[Metric], list[Metric], list[Metric]]:
    complex_functions: list[Metric] = []
    long_functions: list[Metric] = []
    naming: list[Metric] = []
    try:
        tree = ast.parse(source.text, filename=str(source.path))
    except SyntaxError:
        return complex_functions, long_functions, naming
    for node in ast.walk(tree):
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue
        visitor = ComplexityVisitor(node)
        visitor.visit(node)
        loc = rel_location(root, source.path, node.lineno)
        if visitor.score > COMPLEXITY_THRESHOLD:
            complex_functions.append(Metric(loc, node.name, visitor.score))
        end_line = getattr(node, "end_lineno", node.lineno)
        length = max(1, end_line - node.lineno + 1)
        if length > FUNCTION_LINES_THRESHOLD:
            long_functions.append(Metric(loc, node.name, length))
        if PY_CAMEL_RE.fullmatch(node.name):
            naming.append(Metric(loc, node.name, 1))
    return complex_functions, long_functions, naming


def mask_go_noncode(text: str) -> str:
    """Replace comments and string contents while preserving lines/braces."""
    out = list(text)
    i = 0
    state = "code"
    quote = ""
    while i < len(out):
        ch = out[i]
        nxt = out[i + 1] if i + 1 < len(out) else ""
        if state == "code":
            if ch == "/" and nxt == "/":
                out[i] = out[i + 1] = " "
                i += 2
                state = "line"
                continue
            if ch == "/" and nxt == "*":
                out[i] = out[i + 1] = " "
                i += 2
                state = "block"
                continue
            if ch in {'"', "'", "`"}:
                quote = ch
                out[i] = " "
                state = "string"
        elif state == "line":
            if ch == "\n":
                state = "code"
            else:
                out[i] = " "
        elif state == "block":
            if ch == "*" and nxt == "/":
                out[i] = out[i + 1] = " "
                i += 2
                state = "code"
                continue
            if ch != "\n":
                out[i] = " "
        elif state == "string":
            if quote != "`" and ch == "\\" and i + 1 < len(out):
                out[i] = out[i + 1] = " "
                i += 2
                continue
            if ch == quote:
                out[i] = " "
                state = "code"
            elif ch != "\n":
                out[i] = " "
        i += 1
    return "".join(out)


def go_metrics(root: Path, source: Source) -> tuple[list[Metric], list[Metric]]:
    complex_functions: list[Metric] = []
    long_functions: list[Metric] = []
    masked = mask_go_noncode(source.text)
    for match in GO_FUNC_RE.finditer(masked):
        brace = masked.find("{", match.end())
        if brace < 0:
            continue
        depth = 0
        end = -1
        for index in range(brace, len(masked)):
            if masked[index] == "{":
                depth += 1
            elif masked[index] == "}":
                depth -= 1
                if depth == 0:
                    end = index
                    break
        if end < 0:
            continue
        body = masked[brace + 1 : end]
        line = source.text.count("\n", 0, match.start()) + 1
        loc = rel_location(root, source.path, line)
        name = match.group(1)
        complexity = 1
        complexity += len(re.findall(r"\b(?:if|for|case|select)\b", body))
        complexity += body.count("&&") + body.count("||")
        if complexity > COMPLEXITY_THRESHOLD:
            complex_functions.append(Metric(loc, name, complexity))
        length = body.count("\n") + 1
        if length > FUNCTION_LINES_THRESHOLD:
            long_functions.append(Metric(loc, name, length))
    return complex_functions, long_functions


def blame_times(root: Path, path: Path) -> dict[int, int] | None:
    try:
        rel = path.relative_to(root).as_posix()
        proc = subprocess.run(
            ["git", "blame", "--line-porcelain", "--", rel],
            cwd=root,
            capture_output=True,
            text=True,
            timeout=15,
            check=False,
        )
    except (OSError, ValueError, subprocess.TimeoutExpired):
        return None
    if proc.returncode != 0:
        return None
    result: dict[int, int] = {}
    current_line: int | None = None
    current_time: int | None = None
    for raw in proc.stdout.splitlines():
        header = re.match(r"^[0-9a-f^]{7,40}\s+\d+\s+(\d+)(?:\s+\d+)?$", raw)
        if header:
            current_line = int(header.group(1))
            current_time = None
        elif raw.startswith("committer-time "):
            try:
                current_time = int(raw.split()[1])
            except (IndexError, ValueError):
                current_time = None
        elif raw.startswith("\t") and current_line is not None and current_time is not None:
            result[current_line] = current_time
            current_line = None
            current_time = None
    return result


def aged_todos(root: Path, sources: list[Source]) -> tuple[list[Metric], bool]:
    threshold = int(time.time()) - TODO_AGE_DAYS * 86400
    findings: list[Metric] = []
    had_candidates = False
    blame_available = False
    for source in sources:
        candidates = [
            line_no
            for line_no, line in enumerate(source.text.splitlines(), start=1)
            if TODO_RE.search(line)
        ]
        if not candidates:
            continue
        had_candidates = True
        times = blame_times(root, source.path)
        if times is None:
            continue
        blame_available = True
        for line_no in candidates:
            committed = times.get(line_no)
            if committed is not None and committed < threshold:
                age_days = max(0, (int(time.time()) - committed) // 86400)
                findings.append(
                    Metric(rel_location(root, source.path, line_no), "TODO/FIXME/HACK", age_days)
                )
    return findings, (not had_candidates or blame_available)


def render_examples(metrics: list[Metric], unit: str) -> str:
    ordered = sorted(metrics, key=lambda item: (-item.value, item.location, item.label))
    return ", ".join(
        f"{item.location} ({item.label} {unit}={item.value})" for item in ordered[:MAX_EXAMPLES]
    )


def add_finding(findings: list[dict], gate_id: str, metrics: list[Metric], unit: str) -> None:
    if not metrics:
        return
    gate = GATES[gate_id]
    ordered = sorted(metrics, key=lambda item: (-item.value, item.location, item.label))
    locations = list(dict.fromkeys(item.location for item in ordered))
    findings.append(
        {
            "gate": gate_id,
            "severity": gate.severity,
            "cwe": gate.code,
            "title": gate.title,
            "description": f"{len(metrics)} concrete production-source match(es): {render_examples(metrics, unit)}",
            "fix": gate.fix,
            "locations": locations,
            "occurrence_count": len(metrics),
            "metrics": [
                {"location": item.location, "label": item.label, "value": item.value, "unit": unit}
                for item in ordered
            ],
        }
    )


def scan(root: Path) -> dict:
    sources = list(iter_sources(root))
    complex_functions: list[Metric] = []
    long_functions: list[Metric] = []
    naming: list[Metric] = []
    large_files: list[Metric] = []

    for source in sources:
        line_count = source.text.count("\n") + int(bool(source.text))
        if line_count > FILE_LINES_THRESHOLD:
            large_files.append(
                Metric(rel_location(root, source.path, 1), source.path.name, line_count)
            )
        if source.path.suffix.lower() == ".py":
            complexity, long, names = python_metrics(root, source)
            complex_functions.extend(complexity)
            long_functions.extend(long)
            naming.extend(names)
        elif source.path.suffix.lower() == ".go":
            complexity, long = go_metrics(root, source)
            complex_functions.extend(complexity)
            long_functions.extend(long)

    todo_metrics, todo_check_available = aged_todos(root, sources)
    metrics_by_gate = {
        "3.1": complex_functions,
        "3.2": long_functions,
        "3.3": large_files,
        "3.6": naming,
        "3.7": todo_metrics,
    }
    units = {
        "3.1": "complexity",
        "3.2": "lines",
        "3.3": "lines",
        "3.6": "violation",
        "3.7": "age_days",
    }

    findings: list[dict] = []
    gates: list[dict] = []
    for gate_id, gate in GATES.items():
        if gate_id in SKIPPED_GATES:
            gates.append(
                {
                    "id": gate_id,
                    "severity": gate.severity,
                    "status": "skipped",
                    "reason": SKIPPED_GATES[gate_id],
                }
            )
            continue
        if gate_id == "3.7" and not todo_check_available:
            gates.append(
                {
                    "id": gate_id,
                    "severity": gate.severity,
                    "status": "skipped",
                    "reason": "git blame metadata unavailable for TODO age verification",
                }
            )
            continue
        metrics = metrics_by_gate.get(gate_id, [])
        gates.append(
            {"id": gate_id, "severity": gate.severity, "status": "finding" if metrics else "pass"}
        )
        add_finding(findings, gate_id, metrics, units[gate_id])

    return {
        "axis": "quality",
        "gates": gates,
        "findings": findings,
        "scanned_files": len(sources),
        "coverage": {
            "complexity": ["python", "go"],
            "function_length": ["python", "go"],
            "file_length": sorted({source.path.suffix.lower() for source in sources}),
            "naming": ["python"],
            "todo_age": "git blame",
        },
        "thresholds": {
            "complexity": COMPLEXITY_THRESHOLD,
            "function_lines": FUNCTION_LINES_THRESHOLD,
            "file_lines": FILE_LINES_THRESHOLD,
            "todo_age_days": TODO_AGE_DAYS,
        },
    }


def main() -> int:
    if len(sys.argv) != 3:
        print("Usage: quality_scan.py <repo> <output.json>", file=sys.stderr)
        return 2
    root = Path(sys.argv[1]).resolve()
    output = Path(sys.argv[2])
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(scan(root), indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
