# Skill Creation Charter

**Effective:** 2026-06-06
**Owner:** @agent-opensin
**Review cadence:** quarterly

## Purpose

OpenSIN-Code has 17 baseline skills. Without a written policy on
when to create a new skill, we will accumulate duplicates and
misplaced files (see audit-matrix for the prior 5-week drift).

This charter is the **only** gate for creating a new skill.

## The 4 Tests

Every new skill must pass all 4 tests before creation.

### Test 1: One-sentence purpose

> "This skill does X, distinct from Y, because Z."

If the sentence can't be written in <30 words, the skill is too vague.
Decompose or don't create.

**Example (pass):**
> "This skill provides a SerpAPI multi-key pool with cache+history,
> distinct from the built-in websearch, because we need >100 req/day
> capacity."

**Example (fail):**
> "This skill helps with documentation." — Too vague.

### Test 2: Existing-skill audit

> "I have searched all baseline skills + baseline MCPs + opencode
> built-in features. No existing skill covers the same ground."

Required artifact: a 1-paragraph comparison table showing the new
skill vs. the 3 closest existing ones, with explicit differentiators.

### Test 3: Repo-fit audit

| Skill type | Repo path |
|---|---|
| CoDocs / docs / skill infrastructure | `OpenSIN-Code/SIN-Code-Codocs-Skill` |
| MCP server scaffolding | `OpenSIN-Code/SIN-Code-Bundle` (`sin mcp-server`) |
| Skill catalog / install / discover | `OpenSIN-Code/SIN-Code-Bundle` (`sin marketplace`) |
| Slash commands | `OpenSIN-Code/SIN-Code-Bundle` (`sin slash`) |
| Domain-specific tool | `OpenSIN-Code/SIN-Code-<Name>-Skill` (own repo) |
| Cross-cutting (memory, context, goals) | `OpenSIN-Code/SIN-Code-<Name>-Skill` (own repo) |
| v0.dev API proxy | `SIN-Rotator/SINator-v0` (PRIVATE org) |
| Anything else | STOP. Write a one-pager. CEO approval. |

### Test 4: Owner + maintenance budget

> "A specific person is on the hook for: weekly test runs, monthly
> dep updates, responding to issues within 7 days."

If no owner, no skill. The "owner" can be a team, but the team must
be named and reachable.

## PR template

Every PR that touches `opencode.json` baseline or creates a new
`OpenSIN-Code/SIN-Code-*-Skill` repo must:

1. Link to a charter checklist (Test 1, 2, 3, 4 all passed)
2. Update `docs/baseline-skills-purpose.md` with the new skill's
   "distinct from X, Y, Z" note
3. Add the new skill to the Skill-Audit-Matrix

## Quarterly review

Every quarter (Q1, Q2, Q3, Q4), re-run the audit-matrix and:

- Identify new redundancy classes
- Mark skills that crossed the "should be deprecated" line
- Update the cap (currently **target 14, hard cap 16**)

## Exceptions

None. The CEO can override, but the override must be documented in
`docs/governance/overrides.md` with a date and reason.

## See also

- `docs/skill-audit-matrix.md` — the running inventory
- Issue #28 — Bundle sub-projects cleanup (7 vendored sub-projects)
- Issue #29 — Phase 3 Konsolidierungen (slash, mcp-builder, marketplace)
- Today's audit findings (2026-06-06)
