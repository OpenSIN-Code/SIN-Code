# [loop-004] Loop health dashboard — `sin-code status` shows everything at a glance

**Labels:** `loop-engineering` `dx` `p1`
**Branch:** `sincode-loop-system`
**Status:** DONE
**Affects:** `goal_cmd.go`, `status_cmd.go` (new), `internal/ledger/store.go`

---

## Problem

There is no single command to see "is the loop running, what is it doing, what
is queued, what failed, how many continuations happened, which goals are
blocked waiting for children". An operator must run `goal list`, look at the
daemon log, and grep the ledger separately. This makes debugging why a goal
never completes extremely hard and time-consuming.

---

## Root cause

`goal list` exists but shows only raw rows. The queue has all necessary data
(`parent_id`, `depth`, `continuations`, `status`, `last_error`, `attempts`)
but no command renders it as a human-readable tree with context.

---

## Proposed solution

### 1. New `status` command

```go
// status_cmd.go (new top-level command)
func NewStatusCmd() *cobra.Command {
    var asJSON bool
    cmd := &cobra.Command{
        Use:   "status",
        Short: "Show queue tree, recent events, and loop health",
        Long: `Prints a tree view of all queued goals with status icons,
continuation counts, and last errors. Appends the 10 most recent
ledger events so you can see what the loop has been doing.

Use --json for machine-readable output (CI, monitoring, scripts).`,
        RunE: func(cmd *cobra.Command, args []string) error {
            return runStatus(cmd.Context(), asJSON)
        },
    }
    cmd.Flags().BoolVar(&asJSON, "json", false,
        "emit machine-readable JSON (suitable for monitoring scripts)")
    return cmd
}
```

### 2. Tree renderer

```go
// status_cmd.go — printTree helper
// Icons:  ✓ verified  → running  · pending  ⏸ blocked  ✗ failed  ! exhausted
var statusIcons = map[autonomy.GoalStatus]string{
    "verified":  "✓",
    "running":   "→",
    "pending":   "·",
    "blocked":   "⏸",
    "failed":    "✗",
    "exhausted": "!",
}

var printTree func(g autonomy.Goal, indent string)
printTree = func(g autonomy.Goal, indent string) {
    icon := statusIcons[g.Status]
    if icon == "" { icon = "?" }

    conts := ""
    if g.Continuations > 0 {
        conts = fmt.Sprintf(" [cont:%d]", g.Continuations)
    }
    prompt := g.Prompt
    if len(prompt) > 72 { prompt = prompt[:69] + "..." }

    fmt.Printf("%s%s #%-5d %-10s%s  %s\n",
        indent, icon, g.ID, g.Status, conts, prompt)
    if g.LastError != "" && g.Status != "verified" {
        fmt.Printf("%s       err: %s\n", indent, truncate(g.LastError, 80))
    }
    for _, ch := range children[g.ID] {
        printTree(ch, indent+"  ")
    }
}
```

### 3. `ledger.Store.Recent` — new query

```go
// internal/ledger/store.go
// Recent returns the n most recent ledger entries ordered newest-first.
// It is used by `sin-code status` to show a tail of loop activity without
// requiring the caller to know the schema.
func (s *Store) Recent(ctx context.Context, n int) ([]Entry, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, session_id, type, data, summary, created_at
         FROM entries ORDER BY id DESC LIMIT ?`, n)
    if err != nil { return nil, err }
    defer rows.Close()
    return scanEntries(rows)
}
```

### 4. JSON output shape

```json
{
  "generated_at": "2025-06-16T15:04:05Z",
  "summary": {
    "pending": 2, "running": 1, "blocked": 0,
    "verified": 47, "failed": 1, "exhausted": 0
  },
  "goals": [
    {
      "id": 42, "status": "running", "depth": 0,
      "continuations": 2, "attempts": 1,
      "prompt": "Implement the stop-gate hybrid evaluator...",
      "children": [
        { "id": 43, "status": "verified", "depth": 1, "prompt": "Update CHANGELOG.md..." }
      ]
    }
  ],
  "recent_events": [
    { "id": 301, "type": "goal_verified", "summary": "goal #41 done in 9 turns", "created_at": "..." }
  ]
}
```

---

## Implementation notes (added after build)

**Tree construction is O(n) with a two-pass map build:**
First pass builds `byID` and `children` maps. Second pass recursively walks
roots (goals with `ParentID == 0`). This avoids nested SQL queries per node
and keeps the output consistent regardless of the order `q.List` returns rows.

**`ledger.Recent` orders by `id DESC` not `created_at DESC`:**
SQLite's `id` is a monotonically increasing integer. Ordering by `id` is safer
than by timestamp because two goals completing in the same millisecond could
produce identical `created_at` values, causing non-deterministic row order.
`id` is always unique and strictly ordered by insertion time.

**`--json` is the canonical machine interface:**
The text tree format is for humans at a terminal. Its exact layout is not
considered stable — spacing, icons, and column widths can change between
versions. JSON output (`--json`) is the stable contract for CI scripts,
monitoring dashboards, or piping into `jq`. Both share the same data source.

**`goal list` is not deprecated:**
`sin-code status` is additive. `goal list` continues to exist for scripts
that need raw tabular data (e.g., filtering by status with `awk`). The two
commands are complementary, not redundant.

**`ledger.DefaultPath()` falls back to `~/.sin-code/ledger.db`:**
When the workspace has no explicit ledger path configured, the status command
still works by reading the global ledger. This means `sin-code status` can be
run from any directory and will always show something meaningful.

**Error handling: ledger open failure is non-fatal:**
If the ledger cannot be opened (file locked, path wrong, first run), the queue
tree is still printed and the "Recent events" section is simply omitted. The
primary value of `status` is the queue tree, not the event tail.

**Continuation count `[cont:N]`:**
A high continuation count (> 5) on a single goal is a red flag — it usually
means the agent is stuck in a loop, the verify-gate keeps rejecting its work,
or the goal is genuinely too large and should be decomposed. The `status`
command surfaces this at a glance without digging through logs.

---

## Acceptance criteria

- [x] `sin-code status` prints a tree view of all goals with icons and continuation counts
- [x] `sin-code status --json` emits machine-readable JSON
- [x] shows last 10 ledger events below the tree
- [x] `ledger.Store.Recent(ctx, n)` method added
- [x] unit tests for tree rendering and JSON output
- [x] `go test -race` green
