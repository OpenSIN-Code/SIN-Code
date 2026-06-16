# Template: Sin-Debt Marker Cookbook

Docs: ../SKILL.md

A `sin-debt:` marker is the lazy-ship's audit trail. It pairs every
shortcut with an **upgrade trigger** so reviewers can decide whether
the shortcut is still appropriate or ripe to undo.

The pair is mandatory. A `sin-debt:` line without an `upgrade:` line
on the next line is a CI-blocker (`debt.max_no_trigger` from
issue #177).

## Canonical form

```
// sin-debt: <ceiling — what was not built today>,
// upgrade:   <trigger — the event that will let me build it>
```

Both clauses are **one short sentence**, in **present tense**, with
**no hedging**. The word `upgrade` is the literal identifier expected
by the parser (issue #177).

## Copy/paste templates

### HTTP

```
// sin-debt: no graceful shutdown, upgrade: add ctx-aware http.Server when SIGTERM is wired
// sin-debt: no per-IP rate limit, upgrade: switch to token bucket when auth lands
// sin-debt: no retry policy, upgrade: add exponential backoff when 429 is observed
// sin-debt: no request logging, upgrade: enable access log when audit-on-call is wired
```

### Config

```
// sin-debt: flags only, no config file, upgrade: switch to viper when >5 flags appear
// sin-debt: dotenv only, upgrade: switch to viper when >3 sources appear
// sin-debt: no hot-reload, upgrade: add fsnotify when SIGHUP is wired
```

### Logging

```
// sin-debt: text handler only, upgrade: switch to OTLP handler when trace-exporter=otlp
// sin-debt: no level filtering, upgrade: add level when >3 components log
// sin-debt: no structured fields, upgrade: switch to slog.Attr when >6 fields emit
```

### Persistence

```
// sin-debt: in-memory only, upgrade: add bolt when restart-survival is required
// sin-debt: no migration, upgrade: add migration when schema v2 is drafted
// sin-debt: no transactions, upgrade: switch to sqlx when multi-row writes appear
```

### Auth

```
// sin-debt: trust upstream header X-User-Id, upgrade: add JWT verify when intermediate proxy is wired
// sin-debt: no token expiry check, upgrade: add exp audit when refresh tokens appear
```

### Concurrency

```
// sin-debt: single goroutine, upgrade: add worker pool when QPS >100 observed
// sin-debt: no backpressure, upgrade: add bounded channel when queue depth >1000
```

### Documentation

```
// sin-debt: README only, upgrade: add godoc when public API >3 funcs
// sin-debt: no examples, upgrade: add runnable example when >2 callers ask
```

## Anti-patterns

```
// sin-debt: could be better                  ← ceiling is missing the WHAT
// upgrade: later                             ← trigger is missing the WHEN
// sin-debt: refactor soon                    ← "refactor" is not a trigger
// upgrade: if needed                         ← "if needed" is forbidden
// sin-debt: nice to have                     ← not a real shortcut
// upgrade: when Johnny asks                  ← personal-name triggers denied
```

## CI gate

`scripts/validate_skill.py --all-bundled --strict` does not enforce
`sin-debt` formatting (that lives in lint for the source tree). The
expected follow-up is `cmd/sin-code/internal/debt/lint.go` running
across the repo and reporting `debt.missing_upgrade_count` to
`debt.max_no_trigger` baseline.
