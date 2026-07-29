# Review parser routing

`review.py` is the parser-selection layer behind `sin review <before> <after>`.
It prevents documentation and other plain-text inputs from reaching the Python
AST parser.

## Routing contract

- Matching `.py`, `.js`, `.jsx`, `.ts`, and `.tsx` pairs retain the existing
  `sin-code-ibd` semantic review path (`ASTDiff` → intent → risk).
- Markdown, extensionless files, unknown text extensions, and pairs with
  different extensions use a deterministic line diff.
- Text is decoded as UTF-8 with replacement for undecodable bytes. The fallback
  counts added lines, removed lines, and non-equal diff hunks with
  `difflib.SequenceMatcher(autojunk=False)`.
- The IBD result contract remains `{"intents": ..., "risk": ...}`. Text reviews
  add a `text` object with deterministic line statistics and return a neutral
  risk object because no code-semantic risk analysis was performed.

The fallback is intentionally standard-library-only, so Markdown review works
when the optional `sin_code_ibd` package is not installed.
