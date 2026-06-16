# `sin-code debt` — sin-debt marker manager (issue #177)

Source: `cmd/sin-code/debt_cmd.go`  
Package: `cmd/sin-code/internal/sindept/`

## What

`sin-code debt` is the user-facing surface for the **`// sin-debt:` marker
convention** adopted from ponytail v4.7.0 (DietrichGebert/ponytail) and
hardened into SIN-Code as a first-class concept. Every intentional
shortcut in the codebase can (and should) carry a marker naming its
ceiling and the upgrade trigger.

## Subcommands

| Subcommand | Purpose | Exits non-zero on |
|------------|---------|-------------------|
| `list`     | One row per marker | nothing (always 0) |
| `stats`    | Aggregated report grouped by reason/file/language/symbol/age | nothing |
| `check`    | CI gate against the configured policy | rot > threshold OR require_upgrade |
| `policy`   | Print the active policy (defaults + on-disk overlay) | nothing |
| `fix`      | List rot-risk markers in sed-friendly `path:line\treason` form | nothing |
| `export`   | Write the canonical `SIN-DEBT.md` ledger | nothing |

## Common flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--path` | `.` | Root directory or file to scan |
| `--format` | `table` | `table` (markdown) or `json` |
| `--no-trigger` | `false` | When set, returns markers WITHOUT an `upgrade:` clause only |
| `--json` | `false` | Same as `--format=json` (cobra convention) |
| `--by` (stats only) | `file` | `file\|reason\|language\|symbol\|summary\|age` |

## Examples

```bash
# render a markdown report to stdout
sin-code debt list

# show all rot-risk markers
sin-code debt list --no-trigger

# stats grouped by reason (rot + ceiling buckets)
sin-code debt stats --by reason

# chronological — oldest markers first, great for triage Monday morning
sin-code debt stats --by age

# CI gate — exits 1 when more than 50 markers lack an upgrade clause
sin-code debt check

# strict CI gate — exits 1 if ANY marker lacks `upgrade:`
sin-code debt check --require-upgrade

# dump the active policy (defaults merged with the on-disk .sin-code/debt-policy.toml)
sin-code debt policy --json

# write the ledger to ops/backlog/SIN-DEBT.md
sin-code debt export ops/backlog/SIN-DEBT.md
```

## Policy file (`.sin-code/debt-policy.toml`)

```toml
[sin-debt]
max_no_upgrade  = 50     # soft ceiling — `check` fails above this
require_upgrade = false  # when true, ANY marker without upgrade fails `check`
default_reasons = ["global mutex", "O(n²) scan"]

[sin-debt.upgrade_triggers]
throughput = "when throughput exceeds threshold"
main       = "when the upstream API stabilises"
```

The walk looks up the policy from the scan root upwards. The closest
`.sin-code/debt-policy.toml` wins; missing file = out-of-the-box defaults.

## Marker format

```
// sin-debt: <ceiling>, upgrade: <trigger>
// sin-debt: <ceiling>            # upgrade is OPTIONAL but RECOMMENDED
```

Reconised comment families: `//`, `#`, `--`, `/* */`, `<!-- -->`.

## Permissions

Registered in `cmd/sin-code/internal/permission_defaults.go`:

| Policy | Layer | Reason |
|--------|-------|--------|
| `sindept__list`  | allow | read-only scanner |
| `sindept__stats` | allow | read-only aggregation |
| `sindept__check` | ask   | may exit non-zero, visible in CI |
| `sindept__policy` | allow | read-only policy dump |
| `sindept__fix`   | ask   | outputs patch instructions (manual edit) |
| `sindept__export` | ask   | writes SIN-DEBT.md |

## Byte-stability promise

`RenderStatsString` and `RenderListString` are byte-deterministic for
the same `Stats` / marker set. Two scans of the same tree on different
days MUST emit the same bytes — that is the precondition for the
golden-file snapshot that will gate issue #171's four-arm comparator.

## See also

- `docs/sin-debt-convention.md` — author-facing convention + examples
- `cmd/sin-code/internal/sindept/sindept.doc.md` — package-level docs
- Issue #179 — downstream complexity auditor ("approved shortcut" check)
- Issue #180 — audit-engine (`sin-code review --complexity`)
