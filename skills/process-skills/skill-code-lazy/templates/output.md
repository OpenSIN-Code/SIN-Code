# Template: Output Format

Docs: ../SKILL.md

The output is **code first, three lines max**. If the explanation
is longer than the code, delete the explanation. The byte stable
form (per intensity) is below.

## Lite output template

```
<code block — minimal change, one function>

sin-debt: <ceiling — one short clause>
upgrade:   <trigger — one short clause>
verify:    <one-line gate that proves pass>
```

## Full output template (default)

```
<code block — minimum that works, no abstractions>

sin-debt: <ceiling — what was not built>
upgrade:   <trigger — when to build it>
verify:    <gate — the test/curl/check that proves pass>
meta:      intensity=full, gate-state=pass, debt-markers=1
```

## Ultra output template (YAGNI extremist)

```
<code block — one line, one function, no struct>

sin-debt: <what was deleted this turn>
upgrade:   <never — revisit only if asked>
verify:    <gate>
meta:      intensity=ultra, gate-state=pass, deletions=1
```

## Forbidden output tokens

The renderer **strips** these tokens from the rendered output even
when the model emits them. This keeps the byte-stable render honest.

- "feature tour"
- "for future extensibility"
- "just to be safe"
- "in case we need"
- "let's also add"
- "best practice says"
- "industry standard"
- Any sentence whose `len(words) > 25` is auto-summarised to a
  one-line `sin-debt` clause.

These tokens are exactly the "drift back to over-building" guards
from `ponytail/SKILL.md:36-48`. They are inert in `off` and apply
in `lite`, `full`, and `ultra`.

## Byte-stable header (always emitted)

```yaml
---
skill: skill-code-lazy
intensity: <lite|full|ultra>
gate: pass
source: github.com/DietrichGebert/ponytail
sin-mandate: M3 (verification first)
---
```

The header is byte-identical across runs given the same `intensity`
and `gate` — this is what enables the issue #2 system-prompt hash
metric.

## Sample rendered output (full intensity)

```yaml
---
skill: skill-code-lazy
intensity: full
gate: pass
source: github.com/DietrichGebert/ponytail
sin-mandate: M3 (verification first)
---
```

```go
// sin-debt: no graceful shutdown, upgrade: add ctx-aware http.Server when SIGTERM is wired
mux := http.NewServeMux()
mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
    w.WriteHeader(http.StatusOK)
})
go http.ListenAndServe(":8080", mux) //nolint:errcheck
```

```
sin-debt: no graceful shutdown
upgrade:   when SIGTERM is wired
verify:    curl -fsS localhost:8080/healthz returns 200
meta:      intensity=full, gate-state=pass, debt-markers=1
```

Strips down to **< 1.5 KB** for `lite`, **< 4 KB** for `full`,
**< 2 KB** for `ultra`. Pair with the issue #171 four-arm benchmark
for measurement.
