# [loop-014] Confidence-weighted stop-gate — clarify instead of re-working when nearly done

**Labels:** `loop-system` `quality` `p2`
**Branch:** `loop-issues`
**Affects:** `agentloop/loop.go`, `hooks/hooks.go`, `ledger/store.go`
**Tier:** 2

---

## Problem

`StopDecision` is binary (`Complete bool`). When the gate is *almost* satisfied
(say it believes the work is 80% done with one ambiguous criterion), the loop
treats it exactly like a 0%-done rejection and throws the agent back into a
full work cycle. That is wasteful and can ping-pong.

A **confidence score** lets the loop take a cheaper middle path: when
confidence is in a configurable band, ask the agent a targeted clarification
question instead of re-working everything.

---

## Root cause

```go
// agentloop/loop.go — StopDecision today
type StopDecision struct {
    Complete     bool
    OpenCriteria []string
    Report       string
}
```

No notion of "how close are we".

---

## Proposed solution

### 1. Add confidence to the decision

```go
// agentloop/loop.go
type StopDecision struct {
    Complete     bool
    OpenCriteria []string
    Report       string
    // Confidence in [0,1]: the gate's certainty that the work is complete.
    // 0 means "definitely not done", 1 means "definitely done". Optional;
    // a zero value with Complete=false keeps the legacy reject behavior.
    Confidence float64
}
```

### 2. Confidence band config on the Loop

```go
// agentloop/loop.go — Loop struct
// ClarifyBand defines a confidence range [Lo, Hi) in which a NOT-complete
// decision triggers a single targeted clarification turn instead of a full
// re-work. E.g. {Lo: 0.7, Hi: 1.0}. Zero-value band disables this path.
ClarifyBand struct{ Lo, Hi float64 }
// MaxClarifications caps clarify turns per run. 0 -> default 2.
MaxClarifications int
```

### 3. Take the clarify path in the reject branch

```go
// agentloop/loop.go — inside `if !dec.Complete {`, before the generic reject.
if l.ClarifyBand.Hi > 0 &&
    dec.Confidence >= l.ClarifyBand.Lo && dec.Confidence < l.ClarifyBand.Hi &&
    clarifications < clarifyBudget(l) {
    clarifications++
    l.fire(ctx, hooks.Clarify, "", map[string]any{
        "confidence": dec.Confidence, "open_criteria": dec.OpenCriteria,
    })
    l.record(ctx, ledger.TypeClarify,
        map[string]any{"confidence": dec.Confidence}, "near-complete: targeted clarification")
    var b strings.Builder
    fmt.Fprintf(&b, "ALMOST DONE (gate confidence %.0f%%). Only these remain — "+
        "address them precisely without redoing finished work:\n", dec.Confidence*100)
    for i, c := range dec.OpenCriteria {
        fmt.Fprintf(&b, "  %d. %s\n", i+1, c)
    }
    msgs = append(msgs, session.Message{Role: "user", Content: b.String()})
    if err := sess.SaveHistory(msgs); err != nil {
        return nil, err
    }
    continue
}
// else: fall through to the existing stall/reject handling.
```

### 4. Budget helper + run-state + hook/ledger

```go
// agentloop/loop.go
func clarifyBudget(l *Loop) int {
    if l.MaxClarifications == 0 {
        return 2
    }
    return l.MaxClarifications
}

// run state
clarifications := 0
```

```go
// hooks/hooks.go
// Clarify fires when a near-complete decision triggers a targeted question.
Clarify = "stop.clarify"
```

```go
// ledger/store.go
// TypeClarify records a confidence-banded clarification turn.
TypeClarify EntryType = "clarify"
```

### 5. Stop-gate prompt should emit confidence

The LLM-backed stop-gate prompt must be updated to return a confidence value
alongside its verdict (e.g. JSON `{"complete": false, "confidence": 0.82,
"open_criteria": [...]}`). Parsers default `Confidence` to 0 when absent so
older gates remain compatible.

---

## Acceptance criteria

- [ ] `StopDecision.Confidence` field added (default 0 = legacy)
- [ ] `Loop.ClarifyBand` + `MaxClarifications` (default 2)
- [ ] in-band not-complete decisions trigger a clarify turn, not a full re-work
- [ ] clarify turns capped by budget; out-of-band falls through to stall/reject
- [ ] `hooks.Clarify` + `ledger.TypeClarify` added
- [ ] stop-gate JSON parser reads optional `confidence`
- [ ] zero-band / absent confidence preserves legacy behavior
- [ ] test: confidence 0.82 with band [0.7,1.0) clarifies, then completes
- [ ] test: confidence 0.3 falls through to normal reject
- [ ] `go test -race ./...` green
