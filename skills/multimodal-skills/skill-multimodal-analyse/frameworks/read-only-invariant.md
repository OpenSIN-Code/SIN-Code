# Read-Only Invariant

`sanalyse__*` is `allow` policy. It **never mutates the input asset.**
This is a hard invariant under mandate M4 (every tool call must be
gated through the permission engine) and a precondition for letting
the agent invoke these tools unattended in CI / daemon mode.

## Why read-only is the right default

The agent loop's destructive tools (`sin_bash`, `sin_write`,
`sin_edit`, `sin_git_commit`, ...) are `ask` by default. Multimodal
preprocessing is the opposite mode: the user hands the agent a raw
asset, and the agent must surface structure without touching the
source bitstream. If extraction ever mutated the input, the daemon
would silently corrupt source-of-truth artifacts.

## Permission contract

```yaml
# cmd/sin-code/internal/permission_defaults.go
analyse__image_extract:    allow   # M4 — read-only
analyse__pdf_parse:        allow   # M4 — read-only
analyse__log_analyze:      allow   # M4 — read-only
analyse__data_detect:      allow   # M4 — read-only
analyse__audio_transcribe: allow   # M4 — read-only
analyse__video_extract:    allow   # M4 — read-only
```

## Where the boundary lives

`sin-analyse-suite` is a **separate MCP process**. It runs in its own
sandbox and returns JSON payloads only — never writes back to the
file the agent opened. There is no `analyse__write` or
`analyse__delete` tool. There will never be one without a major
version bump and a deprecation cycle.

The skill itself uses `sin_write` / `sin_edit` only to **persist the
extracted payload to a NEW path** (e.g. `assets/<name>.ocr.txt`).
The original asset is never touched.

## Validation

The CI gate `sin-code ceo-audit` (issue #180) verifies this invariant
in two ways:

1. Static grep across the suite's source: every handler must
   `os.Open(path)` (read-only) and must not call `os.WriteFile`,
   `os.Remove`, `ioutil.WriteFile`, or `os.Rename` on a path equal
   to its input.
2. Behavioural test: the suite's own test suite runs every tool
   against a fixture set and asserts the file mtime / sha256 of
   inputs is unchanged before and after invocation.

If either check fails, the suite cannot pass `ceo-audit` and the
release is blocked.
