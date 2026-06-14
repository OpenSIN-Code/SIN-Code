# plan

## Overview
Break down a specification into concrete, executable tasks for the agent.

## Steps
1. Load the approved specification from `.sin/specs/`.
2. Identify atomic work units (each unit: one file change or test).
3. Order tasks by dependencies.
4. For each task, define input/output contracts.
5. Output the plan as a checklist.

## Verification
- [ ] Each task references a spec requirement.
- [ ] Tasks can be executed by an autonomous agent.
- [ ] No task takes more than 10 minutes of agent time.
- [ ] Plan is saved to `.sin/plans/`.

## Anti-Rationalization
| Excuse | Rebuttal |
|--------|----------|
| "I can keep the plan in my head." | Written plan enables parallel work and review. |
| "Just generate code directly." | Plan first reduces hallucinations. |

## Quality Gates
- Must have an approved spec present.
- Plan must be validated by the Critic agent.
