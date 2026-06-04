# SPDX-License-Identifier: MIT
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

Docs: codocs.doc.md
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path

# ── Scanner Configuration ────────────────────────────────────────────

# Directories never scanned. Mirrors common build/VCS/tooling caches so the
# scanner stays fast on large repos (these folders balloon quickly).
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
# Limited to languages the SIN-Code stack actively targets; add carefully
# because the regex below is tuned to C-style/line/Python comment leaders.
# Makefile and Dockerfile are matched by name in ``_is_code_file`` (no suffix).
CODE_SUFFIXES = {
    ".py",
    ".pyi",
    ".ts",
    ".tsx",
    ".js",
    ".jsx",
    ".mjs",
    ".cjs",
    ".rs",
    ".go",
    ".java",
    ".kt",
    ".kts",
    ".scala",
    ".c",
    ".h",
    ".cc",
    ".cpp",
    ".hpp",
    ".cs",
    ".rb",
    ".php",
    ".swift",
    ".sh",
    ".bash",
    ".zsh",
    ".yaml",
    ".yml",
    ".toml",
}

# Extensionless files that still count as code (matched by exact basename).
CODE_FILENAMES = {"Makefile", "Dockerfile", "Justfile"}

# How many leading lines to inspect for a reference. The standard places it on
# the first line; we allow a small window (5) to tolerate a shebang / encoding
# cookie / license header line above it. Keep small — false positives grow
# linearly with this value.
_HEAD_LINES = 5

# Matches: optional comment leader, then "Docs:" then a path ending in .doc.md.
# Regex is VERBOSE so the comment leaders are easy to extend when new
# languages are added. The final `\*?/?\s*$` swallows closing block-comment
# tokens like `*/` so ``/* Docs: foo.doc.md */`` matches.
_DOCS_RE = re.compile(
    r"""^\s*
        (?:\#|//|/\*|\*|--|;)?      # optional comment leader
        \s*Docs:\s*
        (?P<doc>[^\s*]+?\.doc\.md)  # the referenced doc path
        \s*\*?/?\s*$                # optional closing comment
    """,
    re.VERBOSE,
)


# ── CoDocsReference: Parsed Reference ────────────────────────────────


@dataclass(frozen=True)
class DocReference:
    """A ``Docs:`` reference discovered in a source file.

    Attributes:
        source: Path of the code file containing the reference, relative to
            the scan root.
        doc: The raw referenced path as written in the source (e.g.
            ``"router.doc.md"``). Unvalidated.
        resolved: Absolute path the reference resolves to (source parent +
            doc), computed at scan time.
        exists: Whether ``resolved`` points to a regular file on disk.
    """

    source: Path
    doc: str  # raw referenced path, as written
    resolved: Path  # absolute path the reference resolves to
    exists: bool

    def to_dict(self) -> dict:
        """Serialize to a JSON-friendly dict for CLI/JSON output."""
        return {
            "source": str(self.source),
            "doc": self.doc,
            "resolved": str(self.resolved),
            "exists": self.exists,
        }


# ── Scanner: Find All # Docs: References ──────────────────────────────


def _is_code_file(path: Path) -> bool:
    """True if ``path`` is a code file eligible for CoDocs scanning."""
    if path.name in CODE_FILENAMES:
        return True
    return path.suffix in CODE_SUFFIXES


def _iter_code_files(root: Path, exclude: set[str]):
    """Yield eligible code files under ``root``, skipping ``exclude`` dirs."""
    for path in sorted(root.rglob("*")):
        if not path.is_file():
            continue
        if any(part in exclude for part in path.parts):
            continue
        if _is_code_file(path):
            yield path


def _extract_reference(path: Path) -> str | None:
    """Return the referenced ``.doc.md`` path from a file's head, or None.

    Reads at most ``_HEAD_LINES`` lines (shebang/encoding tolerant). Returns
    None on either "no match" or "file unreadable" so the scanner keeps going.
    """
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
        # Permission denied, binary file, etc. — treat as "no reference" so
        # one bad file does not abort the whole scan.
        return None
    return None


# ── Validation: Check References Resolve ──────────────────────────────


def scan(root: str | Path = ".", exclude: set[str] | None = None) -> list[DocReference]:
    """Scan ``root`` and return every CoDocs reference found.

    Walks the tree, reads each code file's head, parses the ``Docs:`` line,
    and resolves the target relative to the source file's directory. Files
    without a reference are skipped silently; unreachable references are
    still returned with ``exists=False`` so callers can report them.

    Args:
        root: Filesystem path to scan. Defaults to current working directory.
        exclude: Additional directory basenames to skip (merged with
            ``DEFAULT_EXCLUDE``).

    Returns:
        A list of ``DocReference`` (one per file that declares a Docs line),
        sorted by source path.
    """
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


# ── SOTA Inline Doc checks ─────────────────────────────────────────────


@dataclass(frozen=True)
class InlineDocIssue:
    """A missing or deficient inline doc element.

    Attributes:
        path: Source file the issue was found in, relative to the scan root.
        kind: Machine-readable issue category. Currently one of:
            ``"missing_purpose"`` — file lacks a Purpose/module-docstring
            header in its first few lines.
        detail: Human-readable explanation suitable for CLI output.
    """

    path: Path
    kind: str  # "missing_purpose", "missing_docstring", "missing_section"
    detail: str

    def to_dict(self) -> dict:
        """Serialize to a JSON-friendly dict for CLI/JSON output."""
        return {"path": str(self.path), "kind": self.kind, "detail": self.detail}


# Detects a SOTA-compliant file header: ``# Purpose: ...`` line or a
# Python module docstring (triple-single or triple-double quotes) appearing
# in the first _HEAD_LINES lines of the file.
_INLINE_HEAD_RE = re.compile(r"^\s*(?:#\s*Purpose\s*:|'''|\"\"\")")


def check_inline_docs(
    root: str | Path = ".",
    exclude: set[str] | None = None,
) -> list[InlineDocIssue]:
    """Check files for SOTA inline doc compliance.

    Currently checks:
    - File header with ``Purpose`` line or module docstring.

    The check is intentionally narrow: false positives in a docs linter
    create noise, so we only flag files with no Purpose line AND no
    Python module docstring in the first ``_HEAD_LINES`` lines.

    Args:
        root: Filesystem path to scan. Defaults to current working directory.
        exclude: Additional directory basenames to skip (merged with
            ``DEFAULT_EXCLUDE`` plus ``debug``/``tmp``).

    Returns:
        A list of ``InlineDocIssue`` (one per non-compliant file), sorted by
        path.
    """
    root_path = Path(root).resolve()
    # ``debug``/``tmp`` are extra ignores on top of the standard excludes —
    # these folders are explicitly for throwaway experiments and are exempt
    # from the docs standard per the AGENTS.md exception list.
    excl = DEFAULT_EXCLUDE | {"debug", "tmp"} | (exclude or set())
    issues: list[InlineDocIssue] = []

    for path in sorted(root_path.rglob("*")):
        if not path.is_file():
            continue
        if any(part in excl for part in path.parts):
            continue
        if not _is_code_file(path):
            continue
        # Inline doc header is currently only defined for the languages with
        # line comments or Python module docstrings. YAML/TOML/SH etc. are
        # scanned for Docs: refs but not for inline headers.
        if path.suffix not in (".py", ".pyi", ".ts", ".tsx", ".js", ".jsx", ".rs", ".go"):
            continue

        try:
            text = path.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            continue

        head = "\n".join(text.splitlines()[:_HEAD_LINES])
        rel = path.relative_to(root_path)
        if not _INLINE_HEAD_RE.search(head):
            issues.append(
                InlineDocIssue(
                    path=rel,
                    kind="missing_purpose",
                    detail="Missing Purpose/header comment in first lines",
                )
            )

    return issues


def _check_inline_docs_json(root: str = ".", exclude: set[str] | None = None) -> str:
    """Inline doc check as JSON string, for CLI use.

    Wraps :func:`check_inline_docs` in a JSON serialization so the CLI can
    pipe the result without each caller re-importing :mod:`json`.
    """
    import json
    return json.dumps(
        [issue.to_dict() for issue in check_inline_docs(root, exclude)],
        indent=2,
    )
