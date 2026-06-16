# usage (cmd/sin-code/internal/usage)

Persists LLM token usage (issue #168) — the `ChatResponse.Usage` block that
was parsed but dropped in `internal/llm/provider.go:42-46`. One row per
chat completion; aggregations over session, day, lifetime, and model.

## API

```go
// Event is one row.
type Event struct {
    SessionID     string
    Model         string
    Source        Source // "chat" | "verify" | "judge" | "summary" | "plan" | "adhoc"
    InputTokens   int
    OutputTokens  int
    TotalTokens   int
    CostUSD       float64 // derived from per-1k pricing at write time
    CreatedAt     time.Time
    HasUsage      bool
}

// Store is SQLite-backed (modernc/sqlite, CGo-free). Single-writer,
// multiple-readers safe under the package's internal mutex.
func Open(path string) (*Store, error)
func OpenWithPricing(path string, per1K map[string]float64) (*Store, error)
func (s *Store) Record(ctx context.Context, e Event) error
func (s *Store) RecordFromChatUsage(ctx, sessionID, model, source string, input, output, total int) error
func (s *Store) Aggregate(ctx, Filter, groupBy string) (*Aggregation, []Aggregation, error)
func (s *Store) Tail(ctx, Filter, n int) ([]Event, error)
func (s *Store) Count(ctx, Filter) (int, error)
func (s *Store) Pricing() map[string]float64
func (s *Store) SetPricing(map[string]float64)

// DefaultPath returns $XDG_DATA_HOME/sin-code/tokens.db (or
// ~/.local/share/sin-code/tokens.db on Linux/macOS). Override with
// SIN_CODE_TOKENS_DB.
//
// See AGENTS.md §7 for the storage-location mandate (issue #62-style: never
// gitignored subdirs).
func DefaultPath() string
```

## CLI

`sin-code tokens show [--session ID] [--today] [--month] [--cost] [--json]`
`sin-code tokens tail [--session ID] [-n 20]`
`sin-code tokens aggregate [--by day|month|model|source|session] [--json]`

## Pricing

`DefaultPricing()` ships a hardcoded USD-per-1k tokens map covering NIM
(common-open catalogue), Anthropic, OpenAI, Fireworks, and developer
opencode's `accounts/fireworks/models/minimax-m3`. Override per-model via:

```toml
# ~/.config/sin/sin-code.toml
llm.pricing_per_1k."nvidia/llama-3.1-70b-instruct" = 0.0009
llm.pricing_per_1k."gpt-4o" = 0.0050
```

If a model is unknown, `computeCost()` substring-matches against the longest
configured key (so `llama-3.3-70b` matches `meta/llama-3.3-70b-instruct`).
Unknown → 0 (never blocks the user on missing pricing).

## Concurrency

- Single-writer guard via `Store.mu` (ID allocation + pricing lookup); DB
  write itself is a single INSERT under `modernc/org/sqlite`'s single-writer
  constraint.
- Reads (`Aggregate`, `Tail`, `Count`) tolerate concurrent writes; the lock
  is held only long enough to copy a reference to the pricing map.
- Race-safe under `go test -race -count=1`.

## Migration / forward-compatibility

- Schema is versioned (`PRAGMA user_version = 1`); v1 holds all current
  fields.
- The schema is additive — appending columns in v2 will not invalidate v1
  reads.
- `RecordFromChatUsage` drops rows where `input==output==total==0`
  (providers that omit usage do not pollute aggregates).

## Threading SessionID

SessionID is threaded via `context.Context` using the key
`llm.SessionIDContextKey{}` exported from `internal/llm/recorder.go`. The
agentloop sets it; the LLM client reads it; the usage store writes it. No
global variable lookup, no implicit state.
