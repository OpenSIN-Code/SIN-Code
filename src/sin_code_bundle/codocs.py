"""CoDocs — Co-located Docs Standard validator.

Each code file may declare a companion ``.doc.md`` file via a first-line
reference comment, e.g.::

    # Docs: router.doc.md      (Python, shell, YAML, Makefile, ...)
    // Docs: types.doc.md      (TypeScript, Rust, Go, C, ...)

This module finds those references and verifies the referenced doc file
actually exists next to the source file. It replaces the original fragile
``grep | sed`` one-liner with a robust, testable implementation that ignores
matches inside multi-line strings/heredocs by only inspecting the first
non-shebang lines of each file.

It is intentionally dependency-free (stdlib only) so it works even when the
optional SIN-Code subsystems are not installed.
"""
from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path

# Directories never scanned.
DEFAULT_EXCLUDE = {
    ".git",
    ".hg",
    ".svn",
    "__pycache__",
    "node_modules",
    "venv",
    ".venv",
    "dist",
    "build",
    ".mypy_cache",
    ".pytest_cache",
    ".ruff_cache",
}

# File extensions we consider "code" and therefore eligible for a Docs: ref.
# Makefile and Dockerfile are matched by name in ``_is_code_file``.
CODE_SUFFIXES = {
    ".py", ".pyi",
    ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs",
    ".rs", ".go", ".java", ".kt", ".kts", ".scala",
    ".c", ".h", ".cc", ".cpp", ".hpp", ".cs",
    ".rb", ".php", ".swift", ".sh", ".bash", ".zsh",
    ".yaml", ".yml", ".toml",
}

CODE_FILENAMES = {"Makefile", "Dockerfile", "Justfile"}

# How many leading lines to inspect for a reference. The standard places it on
# the first line; we allow a small window to tolerate a shebang / encoding
# cookie / license header line above it.
_HEAD_LINES = 5

# Matches: optional comment leader, then "Docs:" then a path ending in .doc.md
_DOCS_RE = re.compile(
    r"""^\s*
        (?:\#|//|/\*|\*|--|;)?      # optional comment leader
        \s*Docs:\s*
        (?P<doc>[^\s*]+?\.doc\.md)  # the referenced doc path
        \s*\*?/?\s*$                # optional closing comment
    """,
    re.VERBOSE,
)


@dataclass(frozen=True)
class DocReference:
    """A ``Docs:`` reference discovered in a source file."""

    source: Path
    doc: str          # raw referenced path, as written
    resolved: Path    # absolute path the reference resolves to
    exists: bool

    def to_dict(self) -> dict:
        return {
            "source": str(self.source),
            "doc": self.doc,
            "resolved": str(self.resolved),
            "exists": self.exists,
        }


def _is_code_file(path: Path) -> bool:
    if path.name in CODE_FILENAMES:
        return True
    return path.suffix in CODE_SUFFIXES


def _iter_code_files(root: Path, exclude: set[str]):
    for path in sorted(root.rglob("*")):
        if not path.is_file():
            continue
        if any(part in exclude for part in path.parts):
            continue
        if _is_code_file(path):
            yield path


def _extract_reference(path: Path) -> str | None:
    """Return the referenced ``.doc.md`` path from a file's head, or None."""
    try:
        with path.open("r", encoding="utf-8", errors="ignore") as fh:
            for _ in range(_HEAD_LINES):
                line = fh.readline()
                if line == "":
                    break
                match = _DOCS_RE.match(line)
                if match:
                    return match.group("doc")
    except OSError:
        return None
    return None


def scan(root: str | Path = ".", exclude: set[str] | None = None) -> list[DocReference]:
    """Scan ``root`` and return every CoDocs reference found."""
    root_path = Path(root).resolve()
    excl = DEFAULT_EXCLUDE | (exclude or set())
    references: list[DocReference] = []
    for source in _iter_code_files(root_path, excl):
        doc = _extract_reference(source)
        if doc is None:
            continue
        resolved = (source.parent / doc).resolve()
        references.append(
            DocReference(
                source=source.relative_to(root_path),
                doc=doc,
                resolved=resolved,
                exists=resolved.is_file(),
            )
        )
    return references


def find_broken(root: str | Path = ".", exclude: set[str] | None = None) -> list[DocReference]:
    """Return only the references whose target doc file is missing."""
    return [ref for ref in scan(root, exclude) if not ref.exists]
