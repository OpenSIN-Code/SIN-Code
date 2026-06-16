# The sin-debt Marker Convention (issue #177)

This document is the author-facing reference for the `sin-debt:` marker
convention adopted in SIN-Code. SIN-Code adopted ponytail's `ponytail:`
marker convention as the canonical way to name **intentional complexity**
in source code. The scanner (`internal/sindept`) and the CLI
(`sin-code debt`) operate on the format below.

## Format

```text
// sin-debt: <ceiling>, upgrade: <trigger>
// sin-debt: <ceiling>            # upgrade is OPTIONAL but RECOMMENDED
```

Where:

- `<ceiling>` describes **what is limited**. Free-text but should match
  one of the [default reasons](#default-reasons) when possible:
  `global mutex`, `O(n²) scan`, `hand-rolled retry`, `in-memory cache`,
  `synchronous I/O`, `polling`, etc.
- `<trigger>` describes **when to revisit**. Free-text but should match
  one of the [default upgrade triggers](#default-upgrade-triggers) when
  possible:
  `when throughput > N req/s`, `when n > 100`,
  `when the upstream API stabilises`, etc.

The two clauses are separated by a single comma. The upgrade clause is
optional — markers without one are flagged as `rot-risk` and trip a CI
gate at the configured threshold.

## Comment families

The parser recognises every common comment family:

| Family | Languages |
|--------|-----------|
| `// ...` | Go, Rust, JavaScript, TypeScript, Kotlin, Swift, ... |
| `# ...`  | Python, Shell, YAML, TOML, Ruby (line), ... |
| `-- ...` | SQL, Lua, Haskell |
| `/* ... */` | C, C++, CSS, Java, Go (rare), ... |
| `<!-- ... -->` | HTML, XML, Markdown |

Block-comment closers (`*/`, `-->`) are stripped silently from the
captured reason/upgrade clauses so the same byte sequence is matched
regardless of comment style.

## Examples

### Go — acknowledged shortcut

```go
// Recompute the entire cache on each request. Cheap for now; expensive
// above 1k users. Switch to incremental updates when the load profile
// shows it.
// sin-debt: global mutex, upgrade: per-account locks when throughput > 1k req/s
func recomputeCache() { ... }
```

### Python — known ceiling

```python
# sin-debt: polling timer ticks, upgrade: switch to fsnotify when file count > 100
def watch_config_dir():
    while not_stopped():
        refresh()
        time.sleep(0.5)
```

### Rust — short-form (no upgrade; flagged rot-risk)

```rust
// sin-debt: hand-rolled retry loop (TODO: revisit)
fn send_with_retry(...) { ... }
```

This is recognised but immediately flagged in `sin-code debt stats`
under `Rot-risk markers`. The CI threshold (`max_no_upgrade = 50`
default) controls when such markers fail the build.

## Markers in prose comments

The parser is line-anchored and multi-line aware. A line that *talks
about* the convention (e.g. inside a docs block or README) is NOT a
marker because it does not satisfy the comment-opener regex. Quotes in
backticks (`// sin-debt:`) inside still-running prose also do not match
because the surrounding `// <prose>` line does not start with the marker
token.

## Author checklist

Before committing a shortcut, make sure the marker:

1. Names the **ceiling** — what cannot grow.
2. Names the **trigger** — when to revisit. Avoid "soon" / "when needed"
   unless "needed" is operationalised in the trigger.
3. Sits above (or below, ≤40 lines) the declaration it documents.
4. Lists a Symbol — the `nextSymbol` resolver will pick up the nearest
   function/class automatically; you do not need to duplicate it.

## CI integration

```yaml
# .github/workflows/ci.yml — humans don't need to know; CI does.
- run: sin-code debt check --require-upgrade
```

Or with a soft threshold:

```toml
# .sin-code/debt-policy.toml
[sin-debt]
max_no_upgrade  = 50    # soft ceiling — fail above this
require_upgrade = false
```

## See also

- `cmd/sin-code/debt_cmd.doc.md` — CLI reference.
- `cmd/sin-code/internal/sindept/sindept.doc.md` — package internals.
- Issue #179 — downstream complexity auditor ("approved shortcut" check).
- Issue #180 — audit-engine (`sin-code review --complexity`).

## Default reasons

| Reason | When to use |
|--------|-------------|
| `global mutex` | Single mutex protects multi-key state; per-key locks are the upgrade. |
| `O(n²) scan` | Linear scan over <100 elements; map lookup is the upgrade. |
| `hand-rolled retry` | Loop + sleep; cenkalti/backoff is the upgrade. |
| `hand-rolled backoff` | In-house retry scaling; cenkalti/backoff is the upgrade. |
| `in-memory cache` | In-process map; redis is the upgrade. |
| `in-process queue` | Go channel; redis stream / NATS is the upgrade. |
| `tied loop` | Two loops coupled across files; one function with rich return is the upgrade. |
| `manual json encode` | fmt.Sprintf; json.Marshal is the upgrade. |
| `manual json decode` | strings.Split; json.Unmarshal is the upgrade. |
| `disabled keepalive` | TCP layer off; connection cost is the upgrade. |
| `single-threaded worker` | One goroutine drains the queue; worker pool is the upgrade. |
| `polling` | sleep + check; fsnotify / inotify is the upgrade. |
| `polling-only watcher` | Same as `polling`. |
| `synchronous I/O` | Inline web calls; async pipeline is the upgrade. |
| `synchronous fan-out` | Sequential calls; goroutines + errgroup is the upgrade. |
| `blocking sleep` | time.Sleep; ticker or wake callback is the upgrade. |
| `rebuild-on-every-call` | Object rebuilt per call; pool / sync.Once is the upgrade. |
| `reload-on-import` | Re-import side effects; lazy construction is the upgrade. |
| `polling timer ticks` | Same as `polling`. |
| `best-effort retry` | When retry success is best-effort (cheap, idempotent). |
| `best-effort dedup` | When de-duplication is best-effort (cheap, idempotent). |

## Default upgrade triggers

| Slug | Phrase |
|------|--------|
| `throughput` | when throughput exceeds threshold |
| `scale`      | when N exceeds threshold |
| `latency`    | when latency exceeds threshold |
| `errors`     | when error rate is non-trivial |
| `main`       | when the upstream API stabilises |
| `stable`     | when the upstream API stabilises |
| `context`    | when context cancellation is required |
| `rswitch`    | switch to <alternative> when threshold breached |

The `dswitch` / `rswitch` family invites the healthy habit of writing
the upgrade as a concrete switch instruction, not a vague "improve
later". A specific upgrade (`switch to redis when instances > 1`) gives
the next developer a clear target.
