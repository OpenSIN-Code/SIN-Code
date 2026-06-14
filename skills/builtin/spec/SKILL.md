# spec

## Overview
Generate a detailed specification for a feature or change before any code is written. Ensures alignment and prevents rework.

## Steps
1. Understand the problem: Ask clarifying questions about the feature request.
2. Define scope: List what is included and what is explicitly excluded.
3. Write acceptance criteria: Bullet points that define "done".
4. Create technical design: Describe architecture changes, new components.
5. Review spec with user: Output the spec and await approval.

## Verification
- [ ] All steps completed in order.
- [ ] Acceptance criteria are testable.
- [ ] Technical design mentions affected modules.
- [ ] User has approved the spec.

## Anti-Rationalization
| Excuse | Rebuttal |
|--------|----------|
| "The feature is small, we don't need a spec." | Specs prevent misunderstandings even for small changes. |
| "I'll just start coding, it's faster." | Coding without spec leads to 3x more rework. |
| "The user already explained it." | Write it down to ensure shared understanding. |

## Quality Gates
- Must not proceed to `/plan` without user approval.
- Spec must be stored in `.sin/specs/<feature>.md`.
- No code generation in this step.
