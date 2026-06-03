# ast_edit.py

AST-based code editing with **lazy tree-sitter** import and optional
**POC** verification. Tree-sitter is not a hard dep: when missing,
:class:`SINASTEdit` still constructs and reports
:meth:`is_available` as ``False`` so callers can degrade gracefully.

## Dependencies

- stdlib: `tempfile`, `pathlib`
- optional: `tree_sitter`, `tree_sitter_languages`
- optional: `sin_code_poc` (for ``property_metadata``)

## Touched by

- (none yet — leaf module exposed for agent use)

## What it does

1. **`SINASTEdit(repo_root)`** — constructor probes for tree-sitter
   at *import* time of the helper functions
   (:func:`_try_import_tree_sitter`,
   :func:`_try_import_tree_sitter_languages`). If both succeed, it
   eagerly initialises parsers for ``python``, ``javascript``,
   ``typescript``, ``go``, ``rust``. Failures per language are
   swallowed and that language is simply skipped.
2. **`is_available(language=None)`** — ``False`` if tree-sitter is
   missing, otherwise ``True`` (or ``True`` only for the requested
   language if one was specified).
3. **`edit(file, old, new, verify_with_poc=True)`** — parses the
   file with the right language parser to prove the file is
   syntactically valid, locates the line containing ``old``, and
   emits a single ``ast_replacement`` change with the line index,
   old line, and new line. Then best-effort calls
   ``sin_code_poc.property_metadata()`` for verification.
4. **`resolve(file, changes)`** — applies a list of change dicts
   in reverse line order, atomically (write tmp → ``Path.replace``).
   Returns ``False`` on any I/O failure.
5. **`_detect_language(path)`** — pure-Python extension-to-language
   map; returns ``None`` for unknown suffixes.

## Important config

- `SUPPORTED_LANGS` — set of language names that get a parser
  preloaded; missing grammars are silently skipped.
- `verify_with_poc` — defaults to True; flips to a no-op when POC
  is not installed (never raises).

## Usage

```python
from pathlib import Path
from sin_code_bundle.ast_edit import SINASTEdit

ast = SINASTEdit(Path("/path/to/repo"))

if not ast.is_available():
    raise SystemExit("pip install tree-sitter tree-sitter-languages")

result = ast.edit(Path("foo.py"), "def old_func():", "def new_func():")
if result.success and result.poc_verified:
    ast.resolve(Path("foo.py"), result.proposed_changes)
```

## Known caveats

- **Line-based replacement.** v1 uses tree-sitter to *validate*
  syntax and pick the language, but the actual swap is the whole
  line containing the old substring. For multi-line or
  context-aware edits, extend the query API.
- **No undo.** :meth:`resolve` overwrites the file in place; the
  caller is responsible for VCS backups or a `git stash`.
- **POC is best-effort.** Missing `sin_code_poc` only flips
  ``poc_verified`` to ``False`` and writes a ``skipped`` report;
  it never aborts an edit.
- **Atomic write assumes same-filesystem tmp dir.** The tmp file
  is created in the *target file's* parent dir so ``Path.replace``
  is atomic; on exotic filesystems this may not hold.
