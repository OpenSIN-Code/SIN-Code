# [loop-015] Structured run-trace & replay — visualize turns, tokens, stalls, sub-agent trees

**Labels:** `loop-system` `observability` `dx` `p2`
**Branch:** `loop-issues`
**Affects:** new `cmd/sin-code/trace_cmd.go`, `ledger/store.go`, `agentloop/loop.go`
**Tier:** 3

---

## Problem

The ledger already records rich per-run events (`stop_continue`, `verify_pass`,
`stall_detected`, `token_budget_exhausted`, `reflection`, tool calls). But
there is no way to **read** a run back. Debugging a long autonomous run means
grepping raw ledger rows. We need a `sin-code trace <runID>` command that
renders a run as a readable timeline, including the sub-agent tree and a token
curve.

---

## Root cause

Ledger entries exist but have no run-scoped query/render path:

```go
// ledger/store.go — entries are appended but only low-level reads exist.
func (s *Store) Append(e Entry) error { /* ... */ }
// No `ByRun(runID)` / ordered timeline accessor.
```

---

## Proposed solution

### 1. Run-scoped query on the ledger

```go
// ledger/store.go

// Entry gains a RunID + Span so events can be grouped and nested (sub-agents).
type Entry struct {
    // ... existing fields (Type, Data, Message, Timestamp) ...
    RunID    string `json:"run_id"`
    ParentID string `json:"parent_run_id,omitempty"` // set for sub-agent runs
}

// ByRun returns all entries for a run (and, if includeChildren, its sub-agent
// runs) ordered by timestamp ascending.
func (s *Store) ByRun(runID string, includeChildren bool) ([]Entry, error) {
    all, err := s.All()
    if err != nil {
        return nil, err
    }
    children := map[string]bool{runID: true}
    if includeChildren {
        for _, e := range all { // one pass to collect child run ids
            if e.ParentID == runID {
                children[e.RunID] = true
            }
        }
    }
    var out []Entry
    for _, e := range all {
        if children[e.RunID] {
            out = append(out, e)
        }
    }
    sort.Slice(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
    return out, nil
}
```

### 2. Loop stamps RunID/ParentID on every record

```go
// agentloop/loop.go — Loop struct
// RunID identifies this run in the ledger; sub-agents inherit it as ParentID.
RunID string
// ParentRunID is set on sub-agent loops (see SpawnSubagent) to nest them.
ParentRunID string
```

```go
// agentloop/loop.go — record() stamps the ids
func (l *Loop) record(ctx context.Context, t ledger.EntryType, data map[string]any, msg string) {
    if l.Ledger == nil {
        return
    }
    _ = l.Ledger.Append(ledger.Entry{
        Type: t, Data: data, Message: msg, Timestamp: time.Now().UnixNano(),
        RunID: l.RunID, ParentID: l.ParentRunID,
    })
}
```

```go
// agentloop/subagent.go — SpawnSubagent sets the child's ParentRunID
child := *l            // shallow copy of wiring
child.RunID = newRunID()
child.ParentRunID = l.RunID
```

### 3. The `trace` command

```go
// cmd/sin-code/trace_cmd.go
package main

import (
    "fmt"
    "sort"
    "strings"
    "time"

    "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

// runTrace renders a run timeline: turns, gate decisions, stalls, reflections,
// token curve, and a nested sub-agent tree.
func runTrace(store *ledger.Store, runID string) (string, error) {
    entries, err := store.ByRun(runID, true /* include sub-agents */)
    if err != nil {
        return "", err
    }
    if len(entries) == 0 {
        return "", fmt.Errorf("no trace for run %s", runID)
    }

    var b strings.Builder
    fmt.Fprintf(&b, "RUN %s — %d events\n", runID, len(entries))
    b.WriteString(strings.Repeat("=", 60) + "\n")

    cumTokens := 0
    for _, e := range entries {
        indent := ""
        if e.ParentID != "" && e.RunID != runID {
            indent = "    | " // nest sub-agent events
        }
        ts := time.Unix(0, e.Timestamp).Format("15:04:05")
        line := fmt.Sprintf("%s%s  %-22s %s", indent, ts, e.Type, e.Message)

        // Annotate token events with a running total + sparkline-ish bar.
        if e.Type == ledger.TypeTurn {
            if tt, ok := e.Data["tokens"].(float64); ok {
                cumTokens += int(tt)
                line += fmt.Sprintf("  [Σ %d tok]", cumTokens)
            }
        }
        b.WriteString(line + "\n")
    }

    b.WriteString(strings.Repeat("=", 60) + "\n")
    fmt.Fprintf(&b, "Total tokens: %d\n", cumTokens)
    return b.String(), nil
}

// traceCmd wires `sin-code trace <runID>` (flag parsing omitted for brevity).
func traceCmd(args []string) error {
    if len(args) < 1 {
        return fmt.Errorf("usage: sin-code trace <runID>")
    }
    store, err := ledger.Open(defaultLedgerPath())
    if err != nil {
        return err
    }
    out, err := runTrace(store, args[0])
    if err != nil {
        return err
    }
    fmt.Print(out)
    return nil
}
```

### 4. `--json` output for tooling

```go
// trace_cmd.go — when --json is passed, marshal []ledger.Entry directly so a
// UI/dashboard (see loop-004) can consume the structured timeline.
```

---

## Acceptance criteria

- [ ] `ledger.Entry` gains `RunID` + `ParentID`; `record()` stamps them
- [ ] sub-agent loops inherit `ParentRunID` so they nest under the parent
- [ ] `Store.ByRun(runID, includeChildren)` returns an ordered timeline
- [ ] `sin-code trace <runID>` renders a readable timeline with token totals
- [ ] sub-agent events are visually nested under the parent
- [ ] `--json` emits the structured entry list
- [ ] test: `ByRun` orders entries and includes children when asked
- [ ] test: `runTrace` renders without error for a synthetic run
- [ ] `go test -race ./...` green
