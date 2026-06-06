# history.doc.md

In-memory ring buffer of recent search terms.

## What this file does

Provides `History` — a small, dependency-free ring buffer with a single
cursor. Used by the search input in `app.go` so `↑` / `↓` recalls prior
queries the same way a shell does.

## Dependencies

None. Pure Go standard library.

## API

| Function          | Purpose                                            |
|-------------------|----------------------------------------------------|
| `NewHistory()`    | Construct an empty history (cursor inactive)       |
| `(*H).Push(q)`    | Append `q`. Ignores blanks and exact-of-last dupes |
| `(*H).Prev()`     | One step older. First call returns the newest.     |
| `(*H).Next()`     | One step newer. Past the newest returns `""`.      |
| `(*H).Reset()`    | Clear the cursor without dropping items            |
| `(*H).Len()`      | Number of stored items                             |
| `(*H).Items()`    | Snapshot copy, oldest → newest                     |

## Config / magic constants

```go
const MaxHistoryItems = 20  // matches typical shell HISTSIZE
```

## Design decisions

- **Cursor sentinel `-1`** — distinguishes "no recall active" from
  "currently on the oldest entry". Avoids two booleans or a separate
  state enum.
- **Push resets cursor to `-1`** — after the user runs a query, the
  next `Prev` starts over from the newest entry. Matches readline.
- **Adjacent dedup only** — re-running the same query twice doesn't
  fill history; running A→B→A keeps both A entries because the user
  alternated intentionally. Matches `HISTCONTROL=ignoredups` in bash.
- **Not persisted to disk** — search terms can hold paths or secrets a
  user mistyped. The TUI is a dev tool; if persistence is wanted later,
  add an opt-in flag in `config.go` and serialize on `Push`.
- **Items() returns a copy** — callers can iterate or render without
  worrying about a Push mutating their slice.

## Example

```go
h := NewHistory()
h.Push("scout")
h.Push("audit")
h.Prev()  // "audit"
h.Prev()  // "scout"
h.Push("map")
h.Prev()  // "map"   (cursor reset)
```

## Caveats

- Pure in-memory: lost on process exit.
- Eviction is O(n) (slice reslice). At n=20 this is irrelevant.
- Not thread-safe. The TUI is single-goroutine in the Bubbletea event
  loop, so this is fine today. Add a `sync.Mutex` if a Cmd ever calls
  `Push` from a background goroutine.
