# SPDX-License-Identifier: MIT
"""Safe parser routing for ``sin review``.

Known IBD code extensions retain the existing semantic AST review. Markdown,
unknown text formats, and mixed file types use a deterministic line-based
fallback instead of being sent to the Python parser.
"""

from __future__ import annotations

import difflib
from pathlib import Path
from typing import Any

from sin_code_bundle.json_utils import jsonable

# These are the extensions currently supported by sin-code-ibd's parser
# registry. Only matching extensions are compared semantically; mixed formats
# intentionally fall back to text so neither side is parsed with the wrong
# language parser.
IBD_EXTENSIONS = frozenset({".py", ".js", ".jsx", ".ts", ".tsx"})


def review_files(file_a: Path, file_b: Path) -> dict[str, Any]:
    """Review two files with explicit parser routing.

    Matching IBD-supported code extensions preserve the historical AST review
    path. Every other pair is handled as UTF-8 text with replacement for
    undecodable bytes, which makes Markdown, extensionless files, and mixed
    code/documentation comparisons safe and deterministic.
    """
    path_a = _validated_file(file_a)
    path_b = _validated_file(file_b)

    if _uses_ibd(path_a, path_b):
        return _review_with_ibd(path_a, path_b)
    return _review_as_text(path_a, path_b)


def _validated_file(path: Path) -> Path:
    candidate = Path(path)
    if not candidate.is_file():
        raise FileNotFoundError(f"Review input is not a file: {candidate}")
    return candidate


def _uses_ibd(file_a: Path, file_b: Path) -> bool:
    extension_a = file_a.suffix.lower()
    extension_b = file_b.suffix.lower()
    return extension_a == extension_b and extension_a in IBD_EXTENSIONS


def _review_with_ibd(file_a: Path, file_b: Path) -> dict[str, Any]:
    from sin_code_ibd import ASTDiff, IntentSummarizer, RiskScorer

    changes = ASTDiff().diff_files(str(file_a), str(file_b))
    intents = IntentSummarizer().summarize(changes)
    risk = RiskScorer().score(changes)
    # Keep the historical Python/JS/TS output contract byte-for-byte at the
    # object level: only ``intents`` and ``risk`` are emitted on the IBD path.
    return {"intents": jsonable(intents), "risk": jsonable(risk)}


def _review_as_text(file_a: Path, file_b: Path) -> dict[str, Any]:
    before = file_a.read_text(encoding="utf-8", errors="replace").splitlines(keepends=True)
    after = file_b.read_text(encoding="utf-8", errors="replace").splitlines(keepends=True)

    added_lines = 0
    removed_lines = 0
    changed_hunks = 0
    matcher = difflib.SequenceMatcher(a=before, b=after, autojunk=False)
    for operation, before_start, before_end, after_start, after_end in matcher.get_opcodes():
        if operation == "equal":
            continue
        changed_hunks += 1
        removed_lines += before_end - before_start
        added_lines += after_end - after_start

    if changed_hunks == 0:
        intents = "No text changes detected."
    else:
        intents = (
            "Text changes detected: "
            f"{_count_phrase(added_lines, 'line')} added, "
            f"{_count_phrase(removed_lines, 'line')} removed across "
            f"{_count_phrase(changed_hunks, 'change hunk')}."
        )

    return {
        "intents": intents,
        # Plain-text review deliberately makes no semantic code-risk claim.
        # Preserve the established risk object shape with a neutral result.
        "risk": {
            "total_risk": 0.0,
            "factors": [],
            "hot_files": [],
            "breakdown": {},
        },
        "text": {
            "strategy": "deterministic-line-diff",
            "added_lines": added_lines,
            "removed_lines": removed_lines,
            "changed_hunks": changed_hunks,
            "before_lines": len(before),
            "after_lines": len(after),
        },
    }


def _count_phrase(count: int, noun: str) -> str:
    suffix = "" if count == 1 else "s"
    return f"{count} {noun}{suffix}"
