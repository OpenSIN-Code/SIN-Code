# autoactivate/triggers.go — TOML-ish parser

Pure-stdlib parser for `.sin-code/autoactivate.toml`. Two sections:

```toml
[rule]
name       = "terse-mode"     # required
body       = "be terse"       # optional, the system-prompt block
trigger    = "/compact"       # optional native-language phrase
no_trigger = false            # when true, prompt triggers are ignored

[default]
auto_on    = false            # when true, every session starts active
no_trigger = false            # when true, prompt triggers are never honoured
```

Comments are `#` outside quoted strings. Keys may be unquoted (bare)
or quoted with `"…"`. Booleans are `true|yes|1`.

## Parsing rules

- Multi-rule files use a new `[rule]` section per rule. A second
  `name = "x"` inside the **same** `[rule]` block flushes the prior
  rule with its body so the pattern

  ```
  [rule]
  name = "a"
  body = "first"
  name = "b"
  body = "second"
  ```

  produces rule `a` with body `first` and rule `b` with body `second`.

- Unknown sections are ignored silently — open for forward compatibility.
- Bad lines (no `=`, empty key) are skipped, not fatal.

## Multi-line bodies

The current grammar is single-line; multi-line bodies are a future
extension. Use a `[rule]` block per body for now.

## Why hand-rolled

SIN-Code's existing config parser (`internal/config`) is dependency-free
and rejects complex shapes. The autoactivate file is small enough (rule
+ body + 2 toggles, max) that adding a TOML dependency would dwarf the
text it parses (mandate M2: single static binary, `CGO_ENABLED=0`).
