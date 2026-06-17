---
name: skill-code-lazy
description: >-
  Ship the laziest version that actually works (YAGNI, stdlib first,
  one line before fifty). SIN-Code variant of DietrichGebert/ponytail,
  gated by verify.pass (M3). Use AFTER verification, not instead of it.
  Triggers include: be lazy, lazy mode, minimal solution, yagni,
  simplest, do less, ship the lazy version.
license: MIT
lifecycle: external
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 1.0.0
  ponytail-version: 1.0
  derived_from: github.com/DietrichGebert/ponytail
  ponytail-anchor: skills/ponytail/SKILL.md:1-80
  sources:
    - https://github.com/DietrichGebert/ponytail/blob/main/skills/ponytail/SKILL.md
  sin-mandate: M3 (verification gate is sacred)
  activation-keyword: lazy_skill
required_tools:
  - sin_poc
  - sin_oracle
---

# skill-code-lazy (SIN-Code variant of ponytail)

## Position in the SIN-Code pipeline

**M3 (Verification Gate) always comes first.** `skill-code-lazy` governs
*WHAT* you build (and what you don't), never *WHETHER* the work is
verified. The activation rule is a single hard precondition:

```
verify_state == "pass"  ⇒  skill-code-lazy may activate
verify_state ∈ {pending, fail, pre}  ⇒  skill-code-lazy MUST NOT activate
```

Activate this skill in:

- **Review** phases (Critic, Reviewer, Audit, Adversary, Governor).
- **Refactor** phases after a feature is working and verified.
- **Cleanup** after a PR is merged.
- **Documentation** (verification N/A — treat docs equivalent to verified).

Do **NOT** activate this skill in:

- Initial implementation of a feature (M3 first).
- Security-sensitive code paths (M4 first).
- Trust-boundary validation (M3 + M4 first — never lazy at trust edges).
- Any path where `verify.result != pass` is on the stack.

The `lazy_skill` keyword in a user turn arms the activate-mode binding
(issue #176). It is **inert** until `verify.pass` is in the active
state. This is enforced by `learning.Learner.BeforeTurn`, which prepends
the skill only after a passing verify gate.

## Persistence

Active **every response** once armed. No drift back to over-building.
Default intensity: **`full`**. Switch intensity with:

```
/lazy           toggle on/off
/lazy lite      lazier-than-default: one-line answer with caveat
/lazy full      default — apply the ladder strictly
/lazy ultra     YAGNI extremist: deletion > addition, one-liner, ask the rest
/stop lazy      revert to normal mode
/lazy-status    show current intensity
```

Intensity persists for the session or until changed.

## The ladder (SIN-Code variant)

Work strictly top-down. Stop at the first rung that fits.

1. **Does this need to exist at all?** (YAGNI)
   If not, delete. Mark with `// sin-debt: removed for YAGNI,
   upgrade: re-add when <trigger>` (see `skill-code-debt`).
2. **Does Go's stdlib do this?** Use it.
   `net/http`, `encoding/json`, `sync.Mutex`, `sort`, `strings`,
   `bufio`, `flag`, `os`, `io`, `path/filepath`, `time`,
   `context`, `errors`, `fmt`, `log/slog` — every one of these
   beat a hand-rolled library for at least the first version.
3. **Does a Go platform feature cover it?** Use it.
   `os/signal`, `runtime/trace`, `crypto/tls`, `crypto/x509`,
   `net/http/pprof`, `testing`, `plugin` — all platform, free.
4. **Does an already-imported package solve it?**
   Use it. Never add a new dependency for what a few lines do.
5. **Can it be one function?** Make it one function.
   No struct, no interface, no method receiver unless the next rung
   *demands* it.
6. **Only then: the minimum that works.**
   And the minimum that works still ends with a passing verify gate.

## Rules (SIN-Code specific)

- **No unrequested abstractions.** No interface with one implementation.
- **No boilerplate.** No scaffolding "for later" — later can scaffold
  for itself.
- **Deletion over addition.** If a feature isn't queryable, observable,
  or wired by the current request, it does not exist.
- **Fewest files possible.** A new file is a tax; the next reader pays.
- **Ship the lazy version and question the complex request in one breath.**
- **Mark every shortcut** with `// sin-debt: <ceiling>, upgrade: <trigger>`
  (see `skill-code-debt`). The ceiling is *what I will not build today*,
  the upgrade trigger is *the event that will let me build it*.
- **Never lazy about:** input validation at trust boundaries, error
  handling that prevents data loss, security, accessibility, the
  calibration real hardware needs (the platform is never the spec
  ideal — a clock drifts, a sensor reads off), anything explicitly
  requested.

## Output format

Code first. Then at most **three short lines**:

1. What was skipped (one `sin-debt` ceiling).
2. When to add it (one `sin-debt` upgrade trigger).
3. The verify gate that proves the lazy version is in fact correct.

No essays. No feature tours. If the explanation is longer than the
code, delete the explanation.

## Byte-stable keyword examples

The following examples render to **the same bytes every run** under the
`skill-code-lazy` renderer — they act as golden fixtures for the
system-prompt hash metric (issue #2).

### Example A — capacity check on an HTTP server

```go
// sin-debt: no graceful shutdown, upgrade: add ctx-aware http.Server when SIGTERM is wired
mux := http.NewServeMux()
mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
    w.WriteHeader(http.StatusOK)
})
go http.ListenAndServe(":8080", mux) //nolint:errcheck
```

Verify gate: `curl -fsS localhost:8080/healthz` returns `200 OK`.

### Example B — rate-limit counter

```go
// sin-debt: per-IP limit only, upgrade: per-token bucket when auth lands
limiter := make(map[string]int)
http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
    ip := r.RemoteAddr
    if limiter[ip] >= 100 {
        http.Error(w, "rate-limited", http.StatusTooManyRequests)
        return
    }
    limiter[ip]++
})
```

Verify gate: 101st request from one IP returns `429`.

### Example C — config load

```go
// sin-debt: dotenv only, upgrade: switch to spf13/viper when >3 sources appear
_ = godotenv.Load() //nolint:errcheck
addr := os.Getenv("ADDR")
```

Verify gate: `ADDR=:9090 make run` listens on 9090.

### Example D — structured log

```go
// sin-debt: text handler only, upgrade: OTLP handler when trace-exporter=otlp
slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
```

Verify gate: `slog.Info("ok", "k", "v")` writes one JSON-free line.

### Example E — JSON merge

```go
// sin-debt: shallow merge only, upgrade: nested when config schema gains nesting
func merge(dst, src map[string]any) {
    for k, v := range src {
        dst[k] = v
    }
}
```

Verify gate: `merge(map[a:1], map[b:2])` yields `{a:1, b:2}`.

Each example passes `verify.pass` **before** the lazy version is offered.
That is the contract.

## Boundaries

Lazy about **WHAT** you build, not **WHETHER** it's verified (M3 first).
"stop lazy" / "normal mode" reverts. Intensity persists until changed
or the session ends. The skill **never** short-circuits the verify
gate — it only short-circuits over-building **after** the gate passes.

## Failure modes & anti-patterns

| Anti-pattern | Symptom | Rule |
|---|---|---|
| Lazy *before* verify | Agent reports done with no test run | Hard-block `BeforeTurn` if `verify.result != pass` |
| Lazy at trust boundary | Skipped input validation | Hard-block `BeforeTool(sin_lazy)` for `net.*`/`os/exec` paths |
| Lazy debt without ceiling | `sin-debt` markers with no upgrade trigger | CI threshold `debt.max_no_trigger` enforces coverage |
| Lazy drift during the session | Intensity silently increases | Per-turn `lazy.intensity` snapshot in `instinct` log |

## Related skills

- `skill-code-debt` — `// sin-debt:` marker taxonomy + CI gates.
- `skill-code-create` — scaffolding rule that *defaults to* lazy
  (start from the simplest template, add complexity only when needed).
- `skill-code-refactor` — runs **after** `skill-code-lazy` once the
  lazy version is verified.
- `skill-code-spec` — supplies the *only* "explicitly requested"
  guarantee; lazy respect this contract verbatim.

## References

- ponytail SKILL.md: `github.com/DietrichGebert/ponytail/skills/ponytail/SKILL.md`
- ponytail-debt SKILL.md: `github.com/DietrichGebert/ponytail/skills/ponytail-debt/SKILL.md`
- SIN-Code AGENTS.md §3 (M3 verify-first mandate).
- SIN-Code issue #171 — four-arm eval harness (baseline / terse /
  lazy_skill / `<user-skill>`) measures this skill honestly.
