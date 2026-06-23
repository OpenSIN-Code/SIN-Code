# instinct/evolve.go — cluster → artifact

When a domain accumulates several high-confidence instincts, they can
graduate into a real reusable artifact. The mapping is intentionally
simple — this is not where the design lives; it is where the design
*checks* itself.

## Cluster size → artifact kind

| Members | Kind | Rationale |
|---|---|---|
| 1 | `command` | one sharp behavior = one slash command |
| 2-3 | `skill` | a small coherent capability |
| 4+ | `agent` | a domain specialist worth spinning up |

These thresholds are deliberately low — the goal is to surface *raw
material*, not to ship a finished artifact. The agent that consumes
the proposal is the one that decides what to do with it (create a
file under `~/.config/sin-code/commands/`? install a skill? register
an agent?).

## De-duplication

`MarkEvolved` flips each member to `StatusEvolved`. `EligibleForEvolution`
requires `Status == StatusActive`, so a re-run of `Evolve` will skip
already-evolved members. This makes evolution idempotent.

## Title-casing

Uses `golang.org/x/text/cases` (the maintained successor to the
deprecated `strings.Title`). If you build without `x/text`, the
package compiles but the casing falls back to the bare string.

## Related files

- `types.go` — `EligibleForEvolution`, `StatusEvolved`
- `audit.go` — `MarkEvolved` writes an audit event per member
