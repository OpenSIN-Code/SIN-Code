# Template: Prompt Snippet

Docs: ../SKILL.md

## User asks to refactor a symbol

```markdown
You are performing a SAFE REFACTOR of `{symbol}` in SIN-Code.

Goal: preserve behavior exactly.

Required steps:
1. Run `impact("{symbol}")` and report blast radius.
2. Make the smallest possible change.
3. Run `semantic_diff(before, after)` for each edited file.
4. Run `architectural_debt()` and check for regression.
5. Run `verify_tests(...)` (and `prove(...)` for pure functions).
6. Do NOT report done until Oracle verdict is `pass`.

Produce the Refactor Report from templates/output.md.
```
