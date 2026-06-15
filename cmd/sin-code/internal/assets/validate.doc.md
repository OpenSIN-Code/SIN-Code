# assets/validate.go — schema gate

`Validate(asset)` produces a list of `Issue`s — either `error`
(fail CI / `sin assets validate`) or `warn` (informational).

## Per-kind rules

| Kind | Required | Warnings |
|---|---|---|
| all | `name`, `description` | body < 20 chars |
| agent | — | no `model` hint, no `tools` |
| command | — | `argument-hint` set but no `$` placeholder |
| skill | — | no markdown section |

## Cross-cutting rules

- **Duplicate detection** — same `(kind, name)` twice → error
- **Unsafe unicode** — bidi overrides, zero-width, isolates → error
  (prompt-injection defense)

## Why bidi/zero-width are errors

These are routinely used in prompt-injection payloads to make a
malicious instruction look like benign text. An asset that ships
with them is either compromised or being shipped by someone who
does not understand the threat model.

## Related files

- `asset.go` — the schema being validated
- `cli.go` — `assets validate` runs `ValidateAll`
- `importer.go` — uses `Validate` to drop invalid skills
