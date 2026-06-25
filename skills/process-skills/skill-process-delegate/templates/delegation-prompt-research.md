# Delegation Prompt Template — Research/Exploration Task

Use this template when delegating research or codebase exploration to an
`explore` or `scout` subagent.

## Template

```
You are exploring the [PROJECT NAME] repository at [ABSOLUTE PATH].
[For scout: "You are researching the external dependency [NAME]."]

Your task: [ONE SENTENCE — what to understand/find/analyze]

## Where to look
- Start with: [specific files or directories to read first]
- Then check: [secondary locations — configs, tests, docs]
- Also look at: [related modules, upstream repos, documentation]

## What to find
1. [Specific question 1]
2. [Specific question 2]
3. [Specific question 3]

## Output format
Return a structured markdown report with:
- **Summary**: 2-3 sentence overview
- **Findings**: One section per question, with file:line references
- **Architecture**: How the relevant code is structured
- **Recommendations**: What should be done next (if applicable)

## Scope
- Focus only on [specific area — don't explore the entire codebase]
- Do NOT modify any files
- Time budget: [estimated 5-15 minutes of exploration]
```

## Example

```
You are exploring the SIN-Code repository at /Users/jeremy/dev/SIN-Code-Bundle.

Your task: Understand how the agent loop's verification gate works.

## Where to look
- Start with: cmd/sin-code/internal/verify/ (the verify package)
- Then check: cmd/sin-code/internal/agentloop/ (how the loop calls verify)
- Also look at: AGENTS.md section 3 (M3 mandate), docs/ for verify docs

## What to find
1. What verification modes exist (poc, oracle, off)?
2. How does the gate decide pass/fail?
3. What happens when verification fails? (retry logic)
4. How does the stop-gate (decoupled completion) relate to the verify gate?

## Output format
Return a structured markdown report with file:line references for each finding.

## Scope
- Focus only on the verification system
- Do NOT modify any files
```
