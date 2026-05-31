---
name: sin-codocs
description: SOTA Code Documentation — every code file gets both a `.doc.md` companion AND proper inline `#` comments. Create both for new files, update both on changes, verify with `sin codocs check`. Use when the user says "document this", "add docs", "explain the code", "comment this", "add inline documentation", "self-documenting code", or "SOTA docs".
---

## Code Documentation Standard

Every meaningful code file needs **two documentation layers**:

1. **`.doc.md` companion** — the "what and why" overview
2. **Inline `#` comments** — the "how and why here" detail in the code itself

Both layers must exist. Neither replaces the other.

---

## Layer 1: CoDocs (.doc.md companion)

Every code file gets a `.doc.md` companion file in the same directory.

### Naming

```
router.py         → router.doc.md
config.yaml       → config.doc.md
api/types.ts      → api/types.doc.md
Makefile          → Makefile.doc.md
```

### Code reference

First line of the code file:

```python
# Docs: router.doc.md
```

```ts
// Docs: types.doc.md
```

```makefile
# Docs: Makefile.doc.md
```

### What belongs in a `.doc.md`

- What does this file do? (1 sentence)
- Which other files import / touch it? (dependency map)
- Important config values & limits
- Why certain decisions were made (e.g. "no async here because X")
- Usage examples (1-2 lines)
- Known caveats or footguns

### What does NOT belong in a `.doc.md`

- Implementation details (inline comments handle that)
- Git history (that's what `git log` is for)

---

## Layer 2: SOTA Inline Documentation

Every code file must also have professional inline `#`/`//`/`#:` comments. This is **not** about "comment every line" — it is about providing **semantic context** that an agent can't infer from the code alone.

### SOTA Inline Doc Rules

#### 1. File header (mandatory)

Every code file starts with:

```
# Purpose: <what this file does in 1 line>
# Docs: <companion .doc.md path>
```

For Python use `"""` module docstrings instead of `#`:

```python
"""Handle user authentication.

Docs: auth.doc.md
"""
```

For TypeScript/Rust/etc use doc-comment style:

```ts
/**
 * Handle user authentication.
 * Docs: auth.doc.md
 */
```

#### 2. Public API: docstrings (mandatory)

Every public function, method, class, type, and constant needs a docstring:

```python
def calculate_route(
    origin: Coordinate,
    dest: Coordinate,
    traffic: bool = False,
) -> Route:
    """Shortest path between two coordinates.

    Uses A* with Manhattan heuristic. Raises if both coords
    are identical (avoids zero-length route).
    """
    ...
```

```ts
/** Shortest path between two coordinates.
 *
 * Uses A* with Manhattan heuristic. Throws if both coords
 * are identical (avoids zero-length route).
 */
function calculateRoute(
  origin: Coordinate,
  dest: Coordinate,
  traffic: boolean = false,
): Route { ... }
```

#### 3. Non-obvious logic: inline context comments

Add a comment when the code does something surprising:

- **Why NOT the obvious approach**: `# not using dict comprehension because ...`
- **Why this value**: `# 50ms timeout — must be < retry-after of upstream (60ms)`
- **Why this ordering**: `# flush before close — close may skip unflushed data`
- **Edge case**: `# handles None because protocol allows null fields`
- **Performance note**: `# O(n²) but n ≤ 10 in practice`
- **Security note**: `# sanitize_input() prevents SQL injection here`

#### 4. Section separators (recommended for 100+ line files)

```
# ── Auth ──────────────────────────────────────
```

Visually group related blocks. The long line makes sections scannable.

#### 5. Magic values & config keys

Always explain:

```python
MAX_RETRIES = 3    # upstream SLA guarantees < 2 failures per 1000
WAIT_SECONDS = 60  # must match upstream rate-limit window
```

```ts
const MAX_RETRIES = 3   // upstream SLA guarantees < 2 per 1000
const WAIT_SECONDS = 60 // must match upstream rate-limit window
```

#### 6. Tests: describe scenario + expected behavior

```python
def test_retry_exhaustion():
    """After 3 retries, route should raise UpstreamError."""
```

Test names plus docstrings = executable documentation.

#### 7. Deprecation & migration markers

```python
def old_login():  # DEPRECATED(v2): use authenticate() instead
```

### When to update inline docs

- **Every change to a function's signature**: update its docstring
- **Every change to non-obvious logic**: add/update the context comment
- **Every new module**: file header + section separators
- **Every new public API**: docstring on add

### When NOT to comment

- `i += 1` — obvious code needs no comment
- `x = 1` — unless 1 is a meaningful constant
- Getter/setter boilerplate
- Standard library calls with obvious semantics

---

## Validation

After changes, verify with the bundle CLI:

```bash
sin codocs check            # exit 1 if any .doc.md reference is broken
sin codocs check --json     # machine-readable output
sin codocs list             # list every reference and whether it resolves
```

For inline docs, use manual review with:

```bash
# Check files that have NO module-level docstring/Purpose line
python3 -c "
import ast, sys
for f in sys.argv[1:]:
    try:
        tree = ast.parse(open(f).read())
        if not (isinstance(tree.body[0], ast.Expr) and hasattr(tree.body[0].value, 'value') and 'Purpose' in tree.body[0].value.s if hasattr(tree.body[0].value, 's') else isinstance(tree.body[0], ast.Expr) and isinstance(tree.body[0].value, ast.Constant)):
            print(f'MISSING PURPOSE: {f}')
    except: print(f'PARSE ERROR: {f}')
"
```

## Exceptions

- `docs/` folder — architecture docs, ADRs, setup guides
- `README.md` — project overview
- No `.doc.md` for pure config files without logic (`.gitignore`, `.prettierrc`, etc.)
- No inline docs required for throwaway scripts in `debug/`, `tmp/`, experimental branches

---

## MarkItDown Integration (Microsoft)

**Converts everything to Markdown** for LLM consumption: PDF, DOCX, PPTX, XLSX,
Images (OCR), Audio, HTML, CSV/JSON/XML, ZIP, YouTube, EPUB, Outlook MSG.

### Installation

```bash
pipx install markitdown                          # recommended (CLI + library)
pip install 'markitdown[pdf, docx, pptx, xlsx]'  # minimal
pip install 'markitdown[all]'                    # everything
```

### CLI

```bash
markitdown file.pdf > file.md                     # stdout
markitdown file.pdf -o file.md                    # output file
cat file.pdf | markitdown                         # pipe
markitdown --use-plugins file.pdf                 # with plugins (OCR)
markitdown file.pdf --use-cu --cu-endpoint "<e>"  # Azure Content Understanding
```

### Python API

```python
from markitdown import MarkItDown
md = MarkItDown()
result = md.convert("document.pdf")
print(result.text_content)

# With LLM vision (image descriptions in PPTX/Images)
from openai import OpenAI
md = MarkItDown(llm_client=OpenAI(), llm_model="gpt-4o")

# Security: local files only
result = md.convert_local("document.pdf")
```

### CoDocs pipeline

```bash
for f in docs/*.pdf docs/*.docx docs/*.pptx; do
    markitdown "$f" -o "${f%.*}.doc.md"
done
# then add `# Docs: filename.doc.md` to the matching code file
```

### Security

- `convert()` runs with the calling process's full file-IO rights. Never pass
  untrusted input directly.
- Prefer `convert_local()` / `convert_stream()` for controlled access.

### Reference

https://github.com/microsoft/markitdown | `pipx install markitdown`
