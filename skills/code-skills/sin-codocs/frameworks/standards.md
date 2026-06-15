# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## CoDocs Standard

- Every meaningful code file gets a `.doc.md` companion.
- Code file references doc in the first line.
- Keep implementation details in inline comments, not doc.

## Inline Comment Standard

- File header: Purpose + Docs.
- Public API: docstrings.
- Non-obvious logic: context comments.
- Section separators for files >100 lines.
- Magic values: explain.
- Tests: scenario + expected behavior.

## Exemptions

- `docs/` folder.
- `README.md`.
- Pure config files without logic.
- Throwaway scripts in `debug/`, `tmp/`, experimental branches.
