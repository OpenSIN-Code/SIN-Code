"""Incremental, content-hashed cache for SCKG / impact results.

Avoids rescanning the whole repo on every `impact()` call. Keyed by a hash of
the file set + their mtimes/sizes; invalidated automatically when files change.
Stored under .sin/cache/ as JSON.
"""
from __future__ import annotations

import hashlib
import json
import time
from pathlib import Path
from typing import Any, Optional

_IGNORE = {".git", "node_modules", ".venv", "__pycache__", ".sin", "dist", "build"}


def _repo_fingerprint(root: Path, exts: tuple[str, ...]) -> str:
    h = hashlib.sha256()
    for path in sorted(root.rglob("*")):
        if not path.is_file() or path.suffix.lower() not in exts:
            continue
        if any(part in _IGNORE for part in path.parts):
            continue
        try:
            st = path.stat()
        except OSError:
            continue
        h.update(str(path).encode())
        h.update(str(st.st_mtime_ns).encode())
        h.update(str(st.st_size).encode())
    return h.hexdigest()


class GraphCache:
    def __init__(
        self,
        root: Path = Path("."),
        exts: tuple[str, ...] = (".py", ".ts", ".tsx", ".js", ".go", ".rs"),
    ) -> None:
        self.root = Path(root).resolve()
        self.exts = exts
        self.dir = self.root / ".sin" / "cache"
        self.dir.mkdir(parents=True, exist_ok=True)

    def _file(self, key: str) -> Path:
        safe = hashlib.sha1(key.encode()).hexdigest()[:16]
        return self.dir / f"{safe}.json"

    def get(self, key: str) -> Optional[Any]:
        fp = self._file(key)
        if not fp.exists():
            return None
        data = json.loads(fp.read_text(encoding="utf-8"))
        if data.get("fingerprint") != _repo_fingerprint(self.root, self.exts):
            return None  # stale — repo changed
        return data.get("value")

    def set(self, key: str, value: Any) -> None:
        fp = self._file(key)
        fp.write_text(
            json.dumps(
                {
                    "fingerprint": _repo_fingerprint(self.root, self.exts),
                    "stored_at": time.time(),
                    "value": value,
                },
                indent=2,
            ),
            encoding="utf-8",
        )

    def clear(self) -> int:
        n = 0
        for f in self.dir.glob("*.json"):
            f.unlink()
            n += 1
        return n
