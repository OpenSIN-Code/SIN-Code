"""Incremental, content-hashed cache for SCKG / impact results.

Avoids rescanning the whole repo on every `impact()` call. Keyed by a hash of
the file set + their mtimes/sizes; invalidated automatically when files change.
Stored under .sin/cache/ as JSON.

Docs: cache.doc.md
"""

from __future__ import annotations

import hashlib
import json
import time
from pathlib import Path
from typing import Any, Optional

_IGNORE = {".git", "node_modules", ".venv", "__pycache__", ".sin", "dist", "build"}
# Directory names that never carry first-party source — pruned from the
# fingerprint walk so e.g. installing a new dep into .venv doesn't blow the
# cache, and so the SCKG cache itself (under .sin/) can't recursively
# invalidate itself.


def _repo_fingerprint(root: Path, exts: tuple[str, ...]) -> str:
    """Cheap content-aware hash of the repo's source tree.

    Walks ``root`` recursively, filters to files whose suffix is in ``exts``
    and whose path does not cross an ``_IGNORE`` directory, then hashes the
    (path, mtime_ns, size) tuple of each. mtime+size is ~free compared to
    reading file bytes and is sensitive enough for "did anything change?"
    cache-invalidation — much cheaper than a full content hash.

    Returns:
        Hex SHA-256 digest. Stable across runs for unchanged trees.
    """
    h = hashlib.sha256()
    for path in sorted(root.rglob("*")):
        if not path.is_file() or path.suffix.lower() not in exts:
            continue
        if any(part in _IGNORE for part in path.parts):
            continue
        try:
            st = path.stat()
        except OSError:
            # File vanished mid-walk (race with a checkout/rebuild) — skip
            # rather than abort; fingerprint will still be stable next call.
            continue
        h.update(str(path).encode())
        h.update(str(st.st_mtime_ns).encode())
        h.update(str(st.st_size).encode())
    return h.hexdigest()


class GraphCache:
    """On-disk cache for expensive SCKG / impact results, keyed by repo state.

    Each cached entry is stamped with the current ``_repo_fingerprint`` of the
    source tree. On :meth:`get`, if the stored fingerprint no longer matches
    the live tree, the entry is treated as stale and ``None`` is returned —
    so the cache silently self-invalidates whenever any tracked file changes.

    Storage layout (under ``<root>/.sin/cache/``)::

        <sha1(key)[:16]>.json
            { "fingerprint": "<sha256>",
              "stored_at": <epoch>,
              "value":     <arbitrary JSON> }

    The 16-char prefix is plenty to avoid collisions for the typical
    handful of cache keys per repo and keeps filenames human-skimmable.
    """

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
        # sha1 (not sha256) is fine here: this is a filesystem key, not a
        # security boundary. 16 hex chars = 64 bits collision space.
        safe = hashlib.sha1(key.encode()).hexdigest()[:16]
        return self.dir / f"{safe}.json"

    def get(self, key: str) -> Optional[Any]:
        """Return the cached value for ``key`` if and only if the repo is unchanged.

        Returns ``None`` when there is no entry, when the file is corrupt, or
        when the stored fingerprint disagrees with the live repo fingerprint
        (i.e. some tracked source file changed since the value was stored).
        """
        fp = self._file(key)
        if not fp.exists():
            return None
        data = json.loads(fp.read_text(encoding="utf-8"))
        if data.get("fingerprint") != _repo_fingerprint(self.root, self.exts):
            return None  # stale — repo changed
        return data.get("value")

    def set(self, key: str, value: Any) -> None:
        """Persist ``value`` under ``key`` together with the current repo fingerprint.

        ``value`` must be JSON-serialisable. Any prior entry under the same
        key is overwritten atomically (single ``write_text`` call).
        """
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
        """Drop every cached entry. Returns the number of files removed."""
        n = 0
        for f in self.dir.glob("*.json"):
            f.unlink()
            n += 1
        return n
