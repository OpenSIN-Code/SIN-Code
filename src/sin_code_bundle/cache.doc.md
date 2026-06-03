# cache.py

Incremental, content-hashed cache for SCKG / impact results. Keyed by a
fingerprint of the file set + their mtimes/sizes; invalidated automatically
when the repo changes. Stores JSON blobs under `.sin/cache/`.

## Dependencies

- stdlib: `hashlib`, `json`, `time`, `pathlib`

## Touched by

- `lsp_backend.py` — wraps every `compute_impact()` call in `GraphCache.get/set`
- `gitnexus.py` — could be reused for the GitNexus bridge

## What it does

1. **`_repo_fingerprint(root, exts)`** — SHA-256 over `(path, mtime_ns, size)`
   for every file under `root` whose suffix is in `exts` and whose path parts
   do not contain a sentinel (`.git`, `node_modules`, `.venv`, `__pycache__`,
   `.sin`, `dist`, `build`).
2. **`GraphCache.get(key)`** — returns the cached value or `None` if either
   the file is missing *or* the stored fingerprint doesn't match the
   current repo fingerprint (i.e. something changed).
3. **`GraphCache.set(key, value)`** — writes `{fingerprint, stored_at, value}`
   to `.sin/cache/<sha1(key)[:16]>.json`.
4. **`GraphCache.clear()`** — wipes all cache files, returns the count.

## Important config

- `_IGNORE` — directories never included in the fingerprint; keep in sync
  with `.gitignore` for the target repo type
- `exts` default — `.py .ts .tsx .js .go .rs`; extend for additional langs
- Cache directory: `<root>/.sin/cache/` — gitignored automatically

## Usage

```python
from pathlib import Path
from sin_code_bundle.cache import GraphCache

cache = GraphCache(Path("."))
if (cached := cache.get("impact:foo:bar:10:0")) is not None:
    return cached
# ... expensive computation ...
cache.set("impact:foo:bar:10:0", result)
```

## Known caveats

- Fingerprint is **all-or-nothing** — a 1-byte change anywhere invalidates
  every key, not just the affected ones. Fine for most repos (< 10k files).
- File mtime *resolution* is platform-dependent (ns on Linux/Mac, ~100ms
  on Windows); rapid successive edits may share a fingerprint.
- Cache is **unencrypted** — never cache values that contain raw secrets.
