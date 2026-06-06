## What this PR does

<!-- 1-3 sentences -->

## Skill Creation Charter (if creating or modifying a skill)

If this PR adds/modifies a baseline skill or creates a new
`OpenSIN-Code/SIN-Code-*-Skill` repo, complete the charter:

### Test 1: One-sentence purpose
> This skill does X, distinct from Y, because Z.
>
> *(paste the one-sentence purpose)*

### Test 2: Existing-skill audit
- [ ] I have searched all baseline skills
- [ ] I have searched all baseline MCPs
- [ ] I have searched opencode built-in features
- [ ] Comparison table attached in PR description or linked doc

### Test 3: Repo-fit audit
- [ ] I have read `docs/governance/skill-creation-charter.md`
- [ ] This PR's target repo matches the table for my skill type

### Test 4: Owner + maintenance
- [ ] Owner: @<github-handle>
- [ ] Time budget: <hours/week>

## Checklist

- [ ] Tests pass (`pytest tests/`)
- [ ] CoDocs check passes (`sin-codocs check . --json`)
- [ ] If new skill: `docs/governance/baseline-skills-purpose.md` updated
- [ ] If new skill: `docs/skill-audit-matrix.md` updated
