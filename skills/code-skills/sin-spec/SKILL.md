---
name: sin-spec
description: Author a technical specification for a feature or change. Use when the user has an idea but no written spec yet.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# sin-spec

## Overview

Write a complete, reviewable technical specification before implementation begins.

## When to Use

- User has an idea or requirement and asks for a spec.
- A feature is large enough that it needs design decisions documented.

## When NOT to Use

- The task is a trivial one-liner or bug fix.
- The implementation approach is already fully understood.

## Core Process

```
GATHER → DECIDE → DOCUMENT → REVIEW → APPROVE
```

1. Gather requirements from the user.
2. Make explicit design decisions (and trade-offs).
3. Document the spec in `.sin/specs/`.
4. Review with Critic or user.
5. Wait for approval before moving to planning.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll spec as I go." | Spec-first prevents design drift. |
| "The user trusts me to pick the design." | Document the choice so it can be reviewed. |
| "Specs slow us down." | Small specs are cheap; bad specs are expensive. |

## Red Flags

- Ambiguous acceptance criteria.
- Missing error handling.
- No trade-offs documented.

## Verification

- [ ] Requirements are complete and unambiguous.
- [ ] Design decisions are explicit.
- [ ] Trade-offs are documented.
- [ ] Acceptance criteria are testable.
- [ ] Spec saved to `.sin/specs/`.
- [ ] Critic or user approval obtained.
