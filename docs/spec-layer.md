# Spec-Layer (issue #122)

The **Spec-Layer** sits between free-text intent and machine verification. A
`*.spec.md` file is the single human-edited contract a change must satisfy.
The agent and the autopilot consume it; CI can check it.

```
author spec.md  ->  sin-code spec validate  ->  agent / autopilot acts  ->  verify criteria
```

## Where it fits in SIN-Code

| Layer        | Artifact        | Owner            | Purpose                              |
|--------------|-----------------|------------------|--------------------------------------|
| Intent       | issue / prompt  | human            | what & why, informal                 |
| **Spec**     | `*.spec.md`     | human (reviewed) | the checkable contract               |
| Program      | `program.md`    | human            | autonomous objective + metric/budget |
| Execution    | agent loop      | SIN-Code         | edits + tools                        |
| Verification | acceptance cmds | CI / autopilot   | pass/fail gate                       |

`program.md` (autopilot) optimizes a single metric under a budget; a
`*.spec.md` instead enumerates discrete requirements and acceptance criteria.
They compose: a spec can be the human-readable contract that an autopilot run
must not violate (its criteria become invariants/gates).

## File format

```markdown
# <Spec Title>

# Objective
Free-text description of the goal.

# Requirements
- [must]  R1: an allocation-free steady-state path
- [should] support nested structs
- [may]   expose a streaming API

# Acceptance Criteria
- A1: benchmark shows 0 allocs/op  verify: go test -bench=Encode -benchmem ./...
- A2: existing tests still pass     verify: go test ./encoder/...

# Invariants
- Public API must not change
- No new third-party dependencies
```

Parsing rules:

- The first `#` heading that is not a known section becomes the **Title**.
- Section names are flexible: `Objective`/`Goal`/`Summary`,
  `Acceptance Criteria`/`Criteria`/`Done When`, `Invariants`/`Constraints`.
- Requirements: optional `[must]`/`[should]`/`[may]` prefix (default `must`)
  and optional `Rn:` id (auto-assigned `R1..Rn` otherwise).
- Criteria: optional `An:` id and an optional trailing `verify: <command>`
  (zero exit = passed). Auto-assigned `A1..An` otherwise.

## CLI

```sh
# structural validation — non-zero exit on error (CI-friendly)
sin-code spec validate feature.spec.md
sin-code spec validate -q feature.spec.md      # exit code only

# inspect parsed spec
sin-code spec show feature.spec.md
sin-code spec show --json feature.spec.md      # machine-readable
```

## Validation rules

| Severity | Rule                                                        |
|----------|-------------------------------------------------------------|
| error    | missing `# Objective`                                       |
| error    | no requirements                                             |
| error    | no acceptance criteria (a spec must be checkable)           |
| error    | duplicate requirement / criterion id                        |
| warning  | empty requirement / criterion text                          |
| warning  | no criterion has a `verify:` command (cannot be auto-checked) |

## Roadmap

- **Phase 1 (this change):** parser, validator, `sin-code spec` CLI.
- Phase 2: `sin-code spec check` runs the `verify:` commands and reports a
  pass/fail matrix.
- Phase 3: autopilot consumes a spec's criteria as hard gates per experiment.
