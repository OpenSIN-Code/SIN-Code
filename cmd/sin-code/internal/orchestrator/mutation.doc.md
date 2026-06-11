# orchestrator/mutation.go

Mutation Probe — verify tests actually OBSERVE the change. Injects k
small mutations into changed lines only and re-runs the affected tests.
Surviving mutations mean the tests are blind to the change — the result
is flagged regardless of the green verdict.

## Public surface

- `Mutation{File, Line, Before, After, Rule, Killed}`
- `ProbeResult{Mutations, Killed, Survived, ObservabilityScore}`
  - `Diagnosis() string` — repair context for blind lines
- `MutationProbe{Workdir, TestCmd, MaxMutations}`
  - `Run(ctx, lines) *ProbeResult`
- `ChangedLine{File, Line, Text}` — extracted from a unified diff
- `ParseAddedLines(diff) []ChangedLine`

## Mutation rules (deliberately tiny + language-portable)

| Rule         | Pattern       | Substitution |
|--------------|---------------|--------------|
| negate-eq    | `==`          | `!=`         |
| negate-lt    | `\B<\B`       | `>=`         |
| and-to-or    | `&&`          | `\|\|`       |
| true-to-false| `\btrue\b`    | `false`      |
| plus-to-minus| `+ 1`         | `- 1`        |

## Behavior

- One mutation per line is enough signal; `MaxMutations` bounds total cost.
- If the file drifted between diff parse and probe, the probe skips
  that line (returns `Killed=true` to avoid false-positive survival).
- A working tree with no `Workdir` is a no-op (assumes kill) — useful for tests.
- The probe restores the file via `defer` after each mutation.

## Caveats

- Textual mutators are language-portable but not language-aware — they
  can mutate strings, comments (filtered), or non-executable lines.
  The `*.go` filter in `ParseAddedLines` keeps this manageable.
- Probe cost is `O(MaxMutations × test-time)`. Default cap: 5 mutations.
