---
name: skill-code-codocs
description: Maintain the two-layer documentation standard (CoDocs .doc.md companions + inline comments) for every meaningful code file.
license: MIT
compatibility: 
metadata: 
lifecycle: external
---

# skill-code-codocs

## Overview

Ensure every meaningful code file has two documentation layers: a `.doc.md` companion and professional inline comments.

## When to Use

- User asks to add documentation.
- After a behavioral change that needs docs.
- When creating or modifying a significant code file.

## When NOT to Use

- For throwaway scripts in `debug/` or `tmp/`.
- For pure config files without logic.

## Core Process

```
AUDIT FILES → ADD/UPDATE .doc.md → ADD INLINE COMMENTS → VALIDATE
```

1. Identify files that changed or were created.
2. For each, add or update its `.doc.md` companion.
3. Add inline comments for non-obvious logic.
4. Validate that `.doc.md` references resolve.

## CoDocs Companion

- File naming: `router.py` → `router.doc.md`.
- First line of code file: `// Docs: router.doc.md`.
- Contents: what, why, dependencies, limits, examples, caveats.

## Inline Comments

- File header with `Purpose` and `Docs`.
- Public API docstrings.
- Context comments for non-obvious logic.
- Section separators for large files.
- Explain magic values and config keys.
- Tests describe scenario + expected behavior.

## Verification

- [ ] Every changed file has a `.doc.md` or is exempt.
- [ ] Code file first line references the doc.
- [ ] Inline comments explain non-obvious logic.
- [ ] `sin codocs check` passes (or equivalent manual check).
