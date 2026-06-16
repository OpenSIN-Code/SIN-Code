# autoactivate/rules.go — deterministic rule sets

`RuleSet` is a `map[string]Rule` whose iteration order is **lexicographic
by name** — `Render()` depends on this. Any two RuleSets with the same
name+body+trigger tuples produce identical bytes regardless of insertion
order, so the system-prompt hash metric (issue #2) is stable.

## Render contract

`(*RuleSet).Render()` returns:

```
## Active rules
### terse-mode
be terse

### skill-x
use skill-x carefully
```

Empty RuleSets return `""` — callers must treat that as "do nothing".

## Public API

| Method | Purpose |
|---|---|
| `Add(Rule)` | Upsert by `Rule.Name`. Empty-name rules are dropped silently. |
| `Remove(name)` | Drop by name. Missing-name Remove is a no-op. |
| `Has(name)` | Membership test, defensively nil-safe. |
| `Len()` | Number of rules. |
| `Names()` | Sorted names (the canonical iteration order). |
| `Clone()` | Defensive copy — required before returning from the activator. |
| `Equal(other)` | Structural equality; used in tests. |
| `Render()` | Deterministic concatenation header+body+body+… |

## Why a map (not a slice)

Map deduplicates by name so a double-Activate is safe. The sorted-`Names()`
helper then gives the byte-stable iteration order. A slice would force
callers to dedupe; a sorted-`[]string` is a reasonable alternative but
the lookup-heavy use cases (Activate / Deactivate) need O(1) checks.
