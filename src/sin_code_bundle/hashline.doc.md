# hashline.py

Content-hash anchored patching. Instead of brittle string-match edits
("find the literal substring `def login():` somewhere in this 2k-line
file"), each patch carries a 16-char SHA-256 prefix of the target line
and its line number. If the line still hashes to the same value the
patch is safe to apply; if not, we refuse — the code drifted.

## Dependencies

- stdlib only: `hashlib`, `tempfile`, `pathlib`, `typing`

## Touched by

- Future MCP `hashline_patch` tool — would expose `create_semantic_patch` /
  `apply_semantic_patch` to agents
- Any caller that needs atomic, validated file edits without going through
  a full LSP/IDE workflow

## What it does

1. **`_normalize(s)`** — collapses all whitespace runs to a single space
   so a line like `def  foo():` and `def foo():` hash to the same value.
2. **`_line_hash(line)`** — 16-char prefix of SHA-256 over the normalized
   line. 16 hex chars = 64 bits = ~10⁻¹⁹ collision probability in
   practice.
3. **`HashlineAnchor(content)`** — splits into lines (preserving
   endings), precomputes hashes once so multi-anchor workflows stay O(n).
4. **`find_anchor(target)`** — returns 0-indexed line of first matching
   hash, or `None`. Multiple candidates → first one (deterministic).
5. **`create_patch(old, new)`** — returns a dict:
   ```python
   {"type": "hashline_patch",
    "anchor_hash": "9f3a...",
    "anchor_line": 42,
    "old_content": "def foo():",
    "new_content": "def foo(x):",
    "context": {"start": 39, "end": 46, "lines": [...]}}
   ```
6. **`apply_patch(patch)`** — refuses on hash mismatch, preserves the
   line ending of the original file.
7. **`validate_patch(patch)`** — separate check that doesn't mutate;
   useful for pre-flight in CI.
8. **`SINHashlinePatch`** — file-level wrapper: reads the file, builds
   the anchor, builds the patch, and on apply writes atomically
   (`NamedTemporaryFile` + `replace()`).

## Important constants

- **16-char hash prefix** — wide enough to be effectively unique within a
  single file. Truncated for human-readability in patch dicts.
- **context_lines=3** — default; the patch dict carries ±3 lines around
  the anchor so a human reviewer can see *what* is changing.

## Usage

```python
from pathlib import Path
from sin_code_bundle.hashline import HashlineAnchor, SINHashlinePatch

# In-memory
src = "def foo():\n    return 1\n"
anchor = HashlineAnchor(src)
patch = anchor.create_patch("def foo():", "def foo(x=None):")
new = anchor.apply_patch(patch)

# Whole-file (atomic write)
patcher = SINHashlinePatch(Path("/repo"))
patch = patcher.create_semantic_patch(
    Path("auth.py"), "def login():", "def login(user):",
    intent="add user param",
)
ok, msg = patcher.apply_semantic_patch(patch)
# ok=True → msg="Patch applied successfully"
```

## Known caveats

- The patch only replaces the *first line containing* `old_content` in the
  match window — if the same line appears multiple times, the anchor
  hash (not the substring) disambiguates. Make sure your `old_content`
  is the full line.
- The atomic write uses a sibling `.tmp` file in the same directory —
  not safe across filesystems, but safe on any POSIX rename atomic.
- Whitespace normalization is intentionally aggressive (`" ".join(s.split())`)
  — tabs vs spaces and trailing whitespace do not affect the hash. If you
  need whitespace-sensitive matching, normalize the file first.
