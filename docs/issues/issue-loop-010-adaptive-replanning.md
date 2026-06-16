# [loop-010] Adaptive re-planning on stall — turn stagnation into a new strategy, not an abort

**Labels:** `loop-system` `autonomy` `p1`
**Branch:** `loop-issues`
**Affects:** `agentloop/loop.go`, `hooks/hooks.go`, `ledger/store.go`
**Tier:** 1
**Depends on:** #150 (stall detection — done)

---

## Problem

Stall detection (#150) currently **aborts** with an error when the stop-gate
returns identical open criteria `StallThreshold` times in a row. But a stall
often means the agent's *current strategy* is wrong, not that the goal is
impossible. Aborting throws away a recoverable run.

Better: when a stall is detected, give the agent **one re-plan turn** that
forces a different decomposition/strategy before escalating. Only if the
re-plan also stalls do we abort.

---

## Root cause

```go
// agentloop/loop.go — current stall handling escalates immediately.
if l.StallThreshold > 0 && stallCount >= l.StallThreshold {
    // ... fire StopStalled, record TypeStallDetected ...
    return nil, fmt.Errorf("stop-gate stalled: ...")
}
```

There is no intermediate "change approach" step.

---

## Proposed solution

### 1. Replanner type and Loop field

```go
// agentloop/loop.go

// StallSnapshot is the context handed to a Replanner when progress stalls.
type StallSnapshot struct {
    Prompt       string
    OpenCriteria []string // criteria stuck unchanged
    StallCount   int
    Turns        int
    ToolsUsed    []string
    SessionID    string
}

// Replanner proposes a fresh strategy when the loop stalls. Returning a
// non-empty directive injects it as a user message and resets the stall
// counter, granting the agent another attempt with new guidance. Returning
// an empty string lets the loop escalate (abort/checkpoint) as before.
type Replanner func(ctx context.Context, snap StallSnapshot) string
```

```go
// agentloop/loop.go — Loop struct
// Replanner, if set, is consulted on stall BEFORE escalation. It returns a
// new strategy directive; an empty return means "no new idea, escalate".
// Limited to ReplanBudget attempts per run to avoid infinite re-planning.
Replanner   Replanner
// ReplanBudget caps how many stall-triggered re-plans a single run may use.
// 0 -> default 1. Each successful re-plan resets stall tracking once.
ReplanBudget int
```

### 2. Hook + ledger entry

```go
// hooks/hooks.go
// Replan fires when a stall triggers a strategy change instead of an abort.
Replan = "loop.replan"
```

```go
// ledger/store.go
// TypeReplan records a stall-triggered strategy change (adaptive recovery).
TypeReplan EntryType = "replan"
```

### 3. Wire re-planning into the stall branch

```go
// agentloop/loop.go — inside the stall escalation block
if l.StallThreshold > 0 && stallCount >= l.StallThreshold {
    budget := l.ReplanBudget
    if budget == 0 {
        budget = 1
    }
    // Try to recover with a fresh strategy before giving up.
    if l.Replanner != nil && replansUsed < budget {
        directive := l.Replanner(ctx, StallSnapshot{
            Prompt: prompt, OpenCriteria: lastOpen, StallCount: stallCount,
            Turns: turn + 1, ToolsUsed: toolsUsed, SessionID: sess.ID,
        })
        if strings.TrimSpace(directive) != "" {
            replansUsed++
            stallCount = 0           // give the new strategy a clean slate
            lastCritFingerprint = "" // forget the old fingerprint
            l.fire(ctx, hooks.Replan, "", map[string]any{
                "replans_used": replansUsed, "open_criteria": lastOpen,
            })
            l.record(ctx, ledger.TypeReplan,
                map[string]any{"replans_used": replansUsed, "open_criteria": lastOpen},
                "stall recovered via re-plan; new strategy injected")
            msgs = append(msgs, session.Message{
                Role: "user",
                Content: "STRATEGY CHANGE — the current approach is not making " +
                    "progress. Abandon it and try a different decomposition:\n\n" + directive,
            })
            if err := sess.SaveHistory(msgs); err != nil {
                return nil, err
            }
            continue
        }
    }
    // No replanner, exhausted budget, or no new idea -> escalate as before.
    if serr := sess.SaveHistory(msgs); serr != nil {
        return nil, serr
    }
    l.fire(ctx, hooks.StopStalled, "", map[string]any{
        "stall_count": stallCount, "open_criteria": lastOpen,
    })
    l.record(ctx, ledger.TypeStallDetected,
        map[string]any{"stall_count": stallCount, "open_criteria": lastOpen},
        fmt.Sprintf("no progress after %d re-plans; escalating", replansUsed))
    return nil, fmt.Errorf(
        "stop-gate stalled after %d re-plan(s): %s",
        replansUsed, strings.Join(lastOpen, "; "))
}
```

### 4. New run-state counter

```go
// agentloop/loop.go — alongside stallCount etc.
replansUsed := 0
```

---

## Acceptance criteria

- [ ] `Replanner` type + `ReplanBudget` field added (default 1)
- [ ] `hooks.Replan` and `ledger.TypeReplan` added
- [ ] stall first attempts re-plan (if Replanner set and budget remains), only then aborts
- [ ] a successful re-plan resets `stallCount` and the fingerprint
- [ ] re-plans are capped by `ReplanBudget`
- [ ] nil Replanner preserves current abort-on-stall behavior exactly
- [ ] test: stall -> replan -> completion succeeds
- [ ] test: stall -> replan -> still stalls -> abort after budget
- [ ] `go test -race ./...` green
