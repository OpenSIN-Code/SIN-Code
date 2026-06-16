# [loop-009] Parallel sub-agents via errgroup — delegate N independent subtasks concurrently

**Labels:** `loop-system` `performance` `p1`
**Branch:** `loop-issues`
**Affects:** `agentloop/subagent.go`, `agentloop/loop.go`, `loopbuilder/builder.go`
**Tier:** 1 (highest leverage)
**Depends on:** #153 (sub-agent delegation — done)

---

## Problem

`spawn_subagent` (#153) runs delegations **serially**: each call blocks the
parent until the child loop returns. For genuinely independent subtasks
("write tests for package A", "investigate module B", "update docs for C")
this serializes work that could run concurrently, wasting wall-clock time and
keeping the parent context idle.

We need a `spawn_subagents` (plural) tool that runs N independent subtasks in
parallel with a bounded concurrency limit, while keeping the global token
budget correct under concurrency.

---

## Root cause

```go
// agentloop/subagent.go — current handler runs exactly one child, inline.
func (l *Loop) handleSpawnSubagent(ctx context.Context, args map[string]any) string {
    // ... single SubagentRequest, single SpawnSubagent call, returns when done.
}
```

The token accumulator in `loop.go` (`totalTokens int`) is also **not
concurrency-safe** — parallel children would race on it.

---

## Proposed solution

### 1. Make the token budget concurrency-safe

Replace the plain `int` accumulator with an `atomic.Int64` so parallel
children can charge tokens against a shared budget without a data race.

```go
// agentloop/loop.go — Loop struct (new unexported field, set in Run)
// budgetSpent is the shared, concurrency-safe token counter. Parent and any
// parallel sub-agents charge against the same total so MaxTokens is a true
// global cap, not a per-goroutine one.
budgetSpent atomic.Int64
```

```go
// agentloop/loop.go — replace `totalTokens += ...` with atomic adds.
spent := l.budgetSpent.Add(int64(turnTokens)) // turnTokens computed as before
if l.MaxTokens > 0 && int(spent) >= l.MaxTokens {
    // ... existing exhaustion handling, using int(spent) ...
}
```

### 2. Add a parallel spawn tool spec

```go
// agentloop/subagent.go

// SpawnSubagentsTool delegates MULTIPLE independent subtasks at once. The loop
// runs them concurrently (bounded by SubagentConcurrency) and returns a JSON
// array of results in input order.
const SpawnSubagentsTool = "spawn_subagents"

var subagentsSpec = ToolSpec{
    Name: SpawnSubagentsTool,
    Description: "Delegate several INDEPENDENT subtasks to isolated sub-agents " +
        "that run in parallel. Use only when the subtasks do not depend on each " +
        "other's output. Returns results in the same order as the input tasks.",
    InputSchema: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "tasks": map[string]any{
                "type":        "array",
                "description": "Independent subtasks to run concurrently.",
                "items": map[string]any{
                    "type": "object",
                    "properties": map[string]any{
                        "goal":       map[string]any{"type": "string"},
                        "max_turns":  map[string]any{"type": "integer"},
                        "max_tokens": map[string]any{"type": "integer"},
                    },
                    "required": []any{"goal"},
                },
            },
        },
        "required": []any{"tasks"},
    },
}
```

### 3. Concurrency-bounded handler using errgroup

```go
// agentloop/subagent.go
import (
    "context"
    "encoding/json"
    "fmt"

    "golang.org/x/sync/errgroup"
)

// SubagentConcurrency caps how many sub-agents run at once. Zero means a sane
// default (4). Negative means unbounded (not recommended).
func (l *Loop) subagentConcurrency() int {
    if l.SubagentConcurrency == 0 {
        return 4
    }
    return l.SubagentConcurrency
}

// handleSpawnSubagents runs all tasks concurrently and returns a JSON array of
// results in input order. Individual failures are captured as error strings in
// the corresponding slot — one failed child never aborts its siblings.
func (l *Loop) handleSpawnSubagents(ctx context.Context, args map[string]any) string {
    raw, _ := args["tasks"].([]any)
    if len(raw) == 0 {
        return "SUBAGENT ERROR: 'tasks' must be a non-empty array"
    }

    type slot struct {
        Index  int            `json:"index"`
        Result *SubagentResult `json:"result,omitempty"`
        Error  string          `json:"error,omitempty"`
    }
    results := make([]slot, len(raw))

    g, gctx := errgroup.WithContext(ctx)
    g.SetLimit(l.subagentConcurrency()) // bounded fan-out

    for i, item := range raw {
        i, item := i, item // capture
        g.Go(func() error {
            m, ok := item.(map[string]any)
            if !ok {
                results[i] = slot{Index: i, Error: "task is not an object"}
                return nil
            }
            goal, _ := m["goal"].(string)
            if goal == "" {
                results[i] = slot{Index: i, Error: "'goal' is required"}
                return nil
            }
            req := SubagentRequest{Goal: goal}
            if v, ok := asInt(m["max_turns"]); ok {
                req.MaxTurns = v
            }
            if v, ok := asInt(m["max_tokens"]); ok {
                req.MaxTokens = v
            }
            res, err := l.SpawnSubagent(gctx, l.SubagentStore, req)
            if err != nil {
                results[i] = slot{Index: i, Error: err.Error()}
                return nil // do not cancel siblings
            }
            results[i] = slot{Index: i, Result: res}
            return nil
        })
    }
    _ = g.Wait() // errors are captured per-slot; Wait never returns one here

    b, err := json.Marshal(results)
    if err != nil {
        return "SUBAGENT ERROR: marshal results: " + err.Error()
    }
    return string(b)
}
```

### 4. Advertise + dispatch the plural tool

```go
// agentloop/loop.go — tools()
func (l *Loop) tools() []ToolSpec {
    if !l.subagentEnabled() {
        return l.LocalSpec
    }
    return append(append([]ToolSpec(nil), l.LocalSpec...), subagentSpec, subagentsSpec)
}
```

```go
// agentloop/loop.go — dispatcher, alongside the singular interception
if tc.Name == SpawnSubagentsTool && l.subagentEnabled() {
    out := l.handleSpawnSubagents(ctx, tc.Args)
    l.fire(ctx, hooks.ToolPost, tc.Name, map[string]any{"output_bytes": len(out)})
    l.record(ctx, ledger.TypeToolCall, map[string]any{"tool": tc.Name}, "tool call: "+tc.Name)
    return out, injects
}
```

### 5. New Loop + builder config field

```go
// agentloop/loop.go — Loop struct
// SubagentConcurrency bounds parallel spawn_subagents fan-out. 0 -> default 4.
SubagentConcurrency int
```

```go
// loopbuilder/builder.go — Config + assignment
// SubagentConcurrency caps parallel sub-agent execution. 0 uses the default.
SubagentConcurrency int
// ...
loop.SubagentConcurrency = cfg.SubagentConcurrency
```

---

## Dependency

Requires `golang.org/x/sync/errgroup` (already an indirect dep in most Go
projects; add to `go.mod` if not present: `go get golang.org/x/sync`).

---

## Acceptance criteria

- [ ] `budgetSpent atomic.Int64` replaces the plain accumulator; no data race under `-race`
- [ ] `spawn_subagents` advertised only when `SubagentStore != nil`
- [ ] `handleSpawnSubagents` runs tasks concurrently, bounded by `SubagentConcurrency` (default 4)
- [ ] one failing child does not cancel siblings; failures appear per-slot
- [ ] results returned in input order
- [ ] `SubagentConcurrency` wired through `loopbuilder.Config`
- [ ] test: 3 parallel children complete, results ordered, with a `-race` run
- [ ] test: a child error is isolated to its slot
- [ ] `go test -race ./...` green
