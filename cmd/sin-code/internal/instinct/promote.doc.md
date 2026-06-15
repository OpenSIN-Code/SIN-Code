# instinct/promote.go — project → global

The "promotion" gate. An instinct that has been observed in N distinct
projects is probably universal — not a quirk of one repo. We surface
those as candidates for global scope.

## Threshold

Default `PromotionThreshold = 2`. Set via `SIN_INSTINCT_PROMOTE_N`.

Two is a deliberately low bar:
- we want to surface *candidates*, not auto-promote
- false positives are cheap (an unused global instinct is inert)
- the operator can `sin instinct forget` any false positive in one step

## What "global" means in practice

A global instinct:
- survives project deletion
- shows up in `Manager.Active()` for every project
- wins on signature collision in `LoadEffective`

The promotion preserves the strongest copy's `Action` and `Evidence`
and re-derives a new `ID` from the global scope (so it doesn't collide
with the per-project ID space).

## Audit

Every promotion writes an `AuditEvent` of kind `promoted`. See
`audit.go` and `sin instinct history`.

## Related files

- `evolve.go` — the "graduate" gate; this is the "universalize" gate
- `audit.go` — event log
