# Token Tracking — `sin-code tokens` (issue #168)

SIN-Code persists every LLM call's `prompt_tokens`, `completion_tokens`,
`total_tokens`, model, and source to a local SQLite database. Aggregations
are exposed via `sin-code tokens` and a one-line TUI badge.

## Data model

One row per LLM call (`internal/usage.Event`):

| Field      | Type     | Notes |
|------------|----------|-------|
| id         | TEXT PK  | SHA-256(content + created_at)[:16] |
| session_id | TEXT     | the agent-loop session ID, set via `context.Context` |
| model      | TEXT     | raw model string from the API response (no normalisation) |
| source     | TEXT     | `chat` / `verify` / `judge` / `summary` / `plan` / `adhoc` |
| input_tokens | INT   | from `ChatResponse.Usage.prompt_tokens` |
| output_tokens | INT  | from `ChatResponse.Usage.completion_tokens` |
| total_tokens | INT    | from `ChatResponse.Usage.total_tokens` |
| cost_usd   | REAL     | computed at write time from per-1k pricing |
| created_at | TEXT     | RFC3339Nano UTC |

Indexes: `session`, `model`, `source`, `created_at`.

## Database location

`$XDG_DATA_HOME/sin-code/tokens.db` (or `~/.local/share/sin-code/tokens.db`).
Override with `SIN_CODE_TOKENS_DB=...` for tests.

> **NEVER** commit this file. AGENTS.md §7 enforces this standard; the file
> is ignored by `.gitignore` automatically because it lives outside any
> tracked directory.

Migrating the legacy `cmd/sin-code/tui/.sin-code/lessons.db`-style hidden
locations is out of scope here — issue #62 owns that broader migration.

## CLI surface

```bash
sin-code tokens show \
    [--session <id>]   # filter to a specific session (default: lifetime)
    [--today]          # since 00:00 UTC today
    [--month]          # since 1st of current month
    [--cost]           # include USD estimate (default true)
    [--share]          # single-line "tweetable" output
    [--json]           # JSON envelope

sin-code tokens tail \
    [--session <id>]
    [-n 20]            # last N events (newest first)

sin-code tokens aggregate \
    [--by day|month|model|source|session]
    [--json]
```

### Examples

```bash
# Lifetime roll-up including USD cost.
sin-code tokens show

# Just one session.
sin-code tokens show --session abc-123-456

# Today only.
sin-code tokens show --today

# Tweetable line — `sin-code ⛏ 12.4k · $1.23 (12 events, 3 sessions)`.
sin-code tokens show --share

# Per-day JSON for a dashboard.
sin-code tokens aggregate --by day --json

# Recent events to debug a runaway loop.
sin-code tokens tail -n 5

# Cost by model.
sin-code tokens aggregate --by model
```

## Pricing

Cost is derived from a built-in price table (`internal/usage.DefaultPricing`,
covers NIM, Anthropic, OpenAI, Fireworks, developer-opencode minimax-m3)
overlaid by user overrides from `~/.config/sin/sin-code.toml`:

```toml
llm.pricing_per_1k."gpt-4o" = 0.0050
llm.pricing_per_1k."anthropic/claude-sonnet-4-5" = 0.0030
```

Both quoted and unquoted keys work. If a model has no exact match,
`computeCost` substring-matches against the longest configured key
(`llama-3.3-70b` matches `meta/llama-3.3-70b-instruct`).
Unknown → 0 (never blocks the user on missing pricing).

## Caveman discipline — no fake numbers

If no LLM call has been recorded for a session, the badge and summary are
silent. The first `chat` or `verify` run populates the row; only then does
the one-liner appear. This mirrors `caveman-statusline.sh`'s "absent until
first run" rule.

## TUI badge (planned v3.18)

A future revision of the `cmd/sin-code/tui/` package will render the
`OneLineToken` summary in the statusline. Today the CLI surface (`tokens
show --share`, `tokens aggregate`) and the summary one-liner cover the
same need.

## Concurrency

- Single-writer via `Store.mu` (ID allocation + pricing lookup); the
  underlying `*sql.DB` is also single-writer (`SetMaxOpenConns(1)` —
  modernc/sqlite convention). Cheap: a single INSERT under 5 ms p99.
- Reads (`Aggregate`, `Tail`, `Count`) tolerate concurrent writes via
  the package mutex + SQLite's own isolation guarantees.
- Race-safe under `go test -race -count=1`.

## Tests

- `cmd/sin-code/internal/usage/usage_test.go` — 84.5% coverage,
  concurrent stress included.
- `cmd/sin-code/tokens_cmd_test.go` — CLI shape, JSON envelope,
  share line, lifecycle of `tokens.db` overrides.
- `cmd/sin-code/internal/agentloop/loop_recorder_test.go` — verifies the
  agent loop threads the SessionID through `context.Context` so the
  recorder sees the right session.

## Forward compatibility

The schema is additive. New columns append in v2, add a `PRAGMA
user_version = 2` migration, ship alongside. The on-disk format is
documented above; reading older rows never fails because new fields are
nullable / zero-default.
