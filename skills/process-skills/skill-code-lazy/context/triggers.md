# Context: Triggers & Boundaries

Docs: ../SKILL.md

## Trigger Phrases

User-facing trigger phrases that arm `skill-code-lazy`:

- "be lazy"
- "lazy mode"
- "minimal solution"
- "yagni"
- "simplest"
- "do less"
- "ship the lazy version"
- "stop being clever"
- "one function please"
- "just stdlib"
- "delete the abstraction"
- "/lazy"
- "/stop lazy"
- "/lazy-status"

Keyword for `autoactivate` (issue #176):

- `lazy_skill` — full intensity (default)
- `lazy_skill lite` — minimal mode
- `lazy_skill ultra` — yagni extremist

## Boundaries (HARD)

| State of `verify.result` | `lazy_skill` activation | Reason |
|---|---|---|
| `pass` | **allow** | M3 satisfied; lazy review is safe |
| `pending` | **deny** | Verification hasn't run yet |
| `pre` | **deny** | Verification not yet entered |
| `fail` | **deny** | Failing tests → fix first, lazy later |

The activation rule is enforced by `learning.Learner.BeforeTurn`
in `cmd/sin-code/internal/learning/`. A `lazy_skill` keyword in a
user prompt is treated as inert until `verify.pass` enters the
session state.

## Domain Boundaries

Skill activates in:

- Review (Critic, Reviewer, Adversary, Governor)
- Refactor (after a feature works)
- Cleanup (after a PR is merged)
- Documentation (verification N/A — equivalent to verified)

Skill does **NOT** activate in:

- Initial feature implementation (M3 first)
- Security-sensitive code paths (M4 first)
- Trust-boundary validation (M3 + M4 first)
- Any path where `// TODO: write tests` is still in the diff

## Required Input

- An intensity level (`off` / `lite` / `full` / `ultra`).
- A clear subject of laziness: which file, which function, which PR.
- An acknowledged `verify.pass` precondition (silently held by the
  learner, not required from the user).

## Never-Lazy List

These blocks are **non-negotiable**, regardless of intensity:

1. Input validation at trust boundaries (HTTP entry points,
   `os/exec`, `database/sql` query inputs, environment-derived
   config).
2. Error handling that prevents silent data loss.
3. Permissions and authorization checks.
4. Cryptographic primitives and key handling.
5. Accessibility (WCAG 2.2 AA on the public output surface).
6. Calibration constants that map platform behaviour to physical
   reality (clocks drift, sensors read off).
7. Anything explicitly requested in the contract.

## Tone

Quiet, declarative, judgment-bearing. Like a senior dev who has
seen every abstraction six times. Never preachy; the explanation is
shorter than the code, or it's deleted.
