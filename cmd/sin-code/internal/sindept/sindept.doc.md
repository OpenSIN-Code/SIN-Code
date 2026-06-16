# sin-dept Marker Convention (`internal/sindept`)

Docs: `parser.go`, `stats.go`, `report.go`, `policy.go`

## What

A first-class, parseable convention for marking **intentional** complexity
in source code. Each marker names the ceiling of the current shortcut and
the trigger to revisit it. The marker scanner (`internal/sindept`) is
byte-stable, language-agnostic, and feeds two downstream consumers:

| Consumer | Issue | Role |
|----------|-------|------|
| `sin-code debt list / stats / check` | #177 (this package) | Human-readable reports + CI gate |
| complexity auditor | #179 | Auto-tagging related complexity findings as "approved shortcut" |
| audit-engine | #180 | Aggregate one-shot boolean of "do you have rot?" |

## Format

```
// sin-debt: <ceiling>, upgrade: <trigger>
// sin-debt: <ceiling>   // upgrade is OPTIONAL but RECOMMENDED
```

- **Comment families** recognised out of the box: `//`, `#`, `--`, `/* … */`,
  `<!-- … -->`. The opener and any trailing closer are stripped silently
  from the captured values.
- **Reason / Upgrade** values are trimmed, surrounding punctuation stripped,
  internal whitespace collapsed — so two markers authored with different
  whitespace produce the same `Marker.Reason` byte-for-byte.
- **Upgrade clause is optional** — markers without one are tagged
  `rot-risk` in the report and trigger a non-zero exit on
  `sin-code debt check` when policy acceptance is configured.

### Examples from real source code

```go
// sin-debt: global mutex, upgrade: per-account locks when throughput > 1k req/s
// sin-debt: O(n²) scan, upgrade: switch to map lookup when n > 100
// sin-debt: hand-rolled retry, upgrade: use cenkalti/backoff when context cancellation matters
// sin-debt: this exists
```

```python
# sin-debt: polling timer ticks, upgrade: switch to fsnotify when file count > 100
```

```markdown
<!-- sin-debt: hand-rolled pipeline rendering, upgrade: replace with text/template when content grows 10x -->
```

## Files

| Path | Purpose |
|------|---------|
| `parser.go` | `Marker`, regex, `ParseFile`, `ParseDir` |
| `stats.go` | `Stats` aggregator, `ByFile/ByReason/ByLanguage/BySymbol` views |
| `report.go` | byte-deterministic `Render*` markdown functions |
| `policy.go` | configurable default reasons + upgrade triggers + rot thresholds |

## Usage

```go
import "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sindept"

mk, err := sindept.ParseFile("cmd/sin-code/internal/foo/foo.go")
stats := sindept.AggregateStats(mk)
fmt.Println(sindept.RenderStatsString(stats))
```

```bash
# CLI surface
sin-code debt list  --path .                 # markdown table of every marker
sin-code debt stats --by reason              # report grouped by reason
sin-code debt stats --by file                # report grouped by file
sin-code debt stats --by age                 # chronological: oldest first
sin-code debt check                          # CI gate, exits 1 when rot > threshold
```

## Policy file (`.sin-code/debt-policy.toml`)

```toml
[sin-debt]
max_no_upgrade  = 50     # soft ceiling; above this `debt check` fails
require_upgrade = false  # when true, ANY marker without upgrade fails

[sin-debt.upgrade_triggers]
throughput = "when throughput exceeds threshold"   # suggest keyword `throughput` as a known trigger
main       = "when the upstream API stabilises"
```

The walk looks up the policy from the scan root upwards — the closest
`.sin-code/debt-policy.toml` wins. A missing file = default policy.

## Byte-stability promise

`RenderStatsString` and `RenderListString` are byte-deterministic for the
same Stats value. Two scans of the same tree (different process, same
machine) MUST emit the same bytes. The `FormatVersion` const
(`sin-debt/v1`) is embedded so tests can pin the format and reviewers
can `rg "sin-debt/v1"` to find a generated report.

## Maintenance

- When adding a new comment family (e.g. `%` for Erlang), extend
  `markerRe` in parser.go and add a parse test in sindept_test.go.
- Renaming a `Marker` field is a breaking change; bump the format
  version and add a deprecation alias on the JSON tag.
- The `DefaultReasons` and `UpgradeTriggers` lists mirror ponytail's
  recommended tags so existing adopters have a familiar starting point.
  Add new entries alphabetically; never delete existing ones.
