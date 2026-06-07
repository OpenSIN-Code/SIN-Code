# SPDX-License-Identifier: MIT
"""Detect repo languages and ensure the matching language servers are present.

`sin doctor` uses this to tell users exactly what to install for accurate
impact analysis. We never silently install global tooling; we report and offer
the exact install command.

Docs: lsp_bootstrap.doc.md
"""

from __future__ import annotations

import shutil
from collections import Counter
from pathlib import Path

# language -> (server binary, install hint)
SERVERS: dict[str, tuple[str, str]] = {
    "python": (
        "pyright-langserver",
        "npm i -g pyright   (or: pip install pyright)",
    ),
    "typescript": (
        "typescript-language-server",
        "npm i -g typescript typescript-language-server",
    ),
    "javascript": (
        "typescript-language-server",
        "npm i -g typescript typescript-language-server",
    ),
    "go": (
        "gopls",
        "go install golang.org/x/tools/gopls@latest",
    ),
    "rust": (
        "rust-analyzer",
        "rustup component add rust-analyzer",
    ),
    "java": (
        "jdtls",
        "see: https://github.com/eclipse-jdtls/eclipse.jdt.ls",
    ),
}

_EXT_LANG: dict[str, str] = {
    ".py": "python",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".js": "javascript",
    ".jsx": "javascript",
    ".go": "go",
    ".rs": "rust",
    ".java": "java",
}
_IGNORE = {".git", "node_modules", ".venv", "__pycache__", ".sin"}


def detect_languages(root: Path) -> list[tuple[str, int]]:
    """Return (language, file_count) pairs, most frequent first."""
    counter: Counter[str] = Counter()
    for p in root.rglob("*"):
        if not p.is_file() or any(part in _IGNORE for part in p.parts):
            continue
        lang = _EXT_LANG.get(p.suffix.lower())
        if lang:
            counter[lang] += 1
    return counter.most_common()


def server_status(root: Path) -> list[dict]:
    """Return a list of dicts with language server availability info."""
    rows: list[dict] = []
    for lang, count in detect_languages(root):
        entry = SERVERS.get(lang)
        binary, hint = entry if entry else (None, "no LSP integration yet")
        installed = bool(binary and shutil.which(binary))
        rows.append(
            {
                "language": lang,
                "files": count,
                "server": binary,
                "installed": installed,
                "install_hint": hint,
            }
        )
    return rows
