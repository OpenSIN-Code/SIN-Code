# [loop-004] Loop health dashboard — `sin-code status` shows everything at a glance

**Labels:** `loop-engineering` `dx` `p1`
**Branch:** `agent-loop-engineering`
**Affects:** `goal_cmd.go`, new `status_cmd.go`

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
// status_cmd.go (new file)
package main

import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/spf13/cobra"
    "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
    "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

func NewStatusCmd() *cobra.Command {
    var json bool
    cmd := &cobra.Command{
        Use:   "status",
        Short: "Show the current state of the autonomous loop: queue, recent completions, failures",
        RunE: func(cmd *cobra.Command, args []string) error {
            return runStatus(cmd.Context(), json)
        },
    }
    cmd.Flags().BoolVar(&json, "json", false, "machine-readable JSON output")
    return cmd
}

func runStatus(ctx context.Context, asJSON bool) error {
    q, err := autonomy.Open(autonomy.DefaultPath())
    if err != nil {
        return err
    }
    defer q.Close()

    all, err := q.List(ctx, "")
    if err != nil {
        return err
    }

    // Build tree map
    roots := []autonomy.Goal{}
    byID := map[int64]autonomy.Goal{}
    children := map[int64][]autonomy.Goal{}
    for _, g := range all {
        byID[g.ID] = g
        if g.ParentID == 0 {
            roots = append(roots, g)
        } else {
            children[g.ParentID] = append(children[g.ParentID], g)
        }
    }

    if asJSON {
        return printStatusJSON(ctx, roots, children)
    }

    // Summary line
    counts := map[autonomy.GoalStatus]int{}
    for _, g := range all { counts[g.Status]++ }
    fmt.Printf("SIN-Code Loop Status  (%s)\n", time.Now().Format("2006-01-02 15:04:05"))
    fmt.Printf("  pending=%-4d  running=%-4d  blocked=%-4d  verified=%-4d  failed=%-4d  exhausted=%d\n\n",
        counts["pending"], counts["running"], counts["blocked"],
        counts["verified"], counts["failed"], counts["exhausted"])

    // Tree view
    var printTree func(g autonomy.Goal, indent string)
    printTree = func(g autonomy.Goal, indent string) {
        icon := statusIcon(g.Status)
        conts := ""
        if g.Continuations > 0 {
            conts = fmt.Sprintf(" [cont:%d]", g.Continuations)
        }
        prompt := g.Prompt
        if len(prompt) > 72 {
            prompt = prompt[:69] + "..."
        }
        fmt.Printf("%s%s #%-5d %-10s%s  %s\n", indent, icon, g.ID, g.Status, conts, prompt)
        if g.LastError != "" && g.Status != "verified" {
            fmt.Printf("%s       err: %s\n", indent, truncate(g.LastError, 80))
        }
        for _, ch := range children[g.ID] {
            printTree(ch, indent+"  ")
        }
    }
    if len(roots) == 0 {
        fmt.Println("  (no goals in queue)")
        return nil
    }
    for _, g := range roots {
        printTree(g, "  ")
    }

    // Recent ledger events
    l, err := ledger.Open(ledger.DefaultPath())
    if err == nil {
        defer l.Close()
        recent, _ := l.Recent(ctx, 5)
        if len(recent) > 0 {
            fmt.Printf("\nRecent events:\n")
            for _, e := range recent {
                fmt.Printf("  %-22s  %-20s  %s\n",
                    e.CreatedAt.Format("01-02 15:04:05"), e.Type, truncate(e.Summary, 60))
            }
        }
    }
    return nil
}

func statusIcon(s autonomy.GoalStatus) string {
    switch s {
    case "verified":   return "✓"
    case "running":    return "→"
    case "blocked":    return "⏸"
    case "failed":     return "✗"
    case "exhausted":  return "!"
    default:           return "·"
    }
}
func truncate(s string, n int) string {
    if len(s) <= n { return s }
    return s[:n-3] + "..."
}
```

### 2. `ledger.Recent` — new query needed

```go
// internal/ledger/store.go — add method
func (s *Store) Recent(ctx context.Context, n int) ([]Entry, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, session_id, type, data, summary, created_at
         FROM entries ORDER BY id DESC LIMIT ?`, n)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    return scanEntries(rows)
}
```

### 3. Wire into root command

```go
// main.go or root_cmd.go
rootCmd.AddCommand(NewStatusCmd())
```

---

## Acceptance criteria

- [ ] `sin-code status` prints a tree view of all goals with icons and continuation counts
- [ ] `sin-code status --json` emits machine-readable JSON for CI/monitoring
- [ ] shows last 5 ledger events below the tree
- [ ] `ledger.Store.Recent(ctx, n)` method added
- [ ] unit tests for tree rendering and JSON output
- [ ] `go test -race` green
