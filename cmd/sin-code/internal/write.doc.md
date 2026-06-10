# write — atomic validated file writing

Replaces native `write`. Pipeline: validate → temp file in target dir →
fsync → chmod (preserves existing perms) → rename. A crash or validation
failure never leaves a partial file. Validation: `.go` via go/parser, `.json`
via encoding/json, otherwise a string/comment-aware bracket-balance heuristic
that catches truncated LLM output; `--no-validate` overrides. `--backup`
keeps `.bak`, `--mkdir` creates parents. MCP: `sin_write` (in-process).
