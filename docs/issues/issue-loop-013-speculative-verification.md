# [loop-013] Speculative verification — run cheap checks mid-run, not just at the stop-gate

**Labels:** `loop-system` `performance` `quality` `p2`
**Branch:** `loop-issues`
**Affects:** `agentloop/loop.go`, `orchestrator/verifier.go`, `hooks/hooks.go`
**Tier:** 2

---

## Problem

Deterministic checks (`go build`, lint, vet) run only when the agent proposes
completion and the stop-gate evaluates. By then the agent has often built on
top of broken code for several turns. Each stop-gate reject then costs a full
re-work cycle.

If we run **cheap** checks opportunistically after each tool-edit sequence and
feed failures back immediately, the agent fixes problems while context is
fresh — drastically reducing stop-gate rejects.

---

## Root cause

```go
// agentloop/loop.go — checks only happen via the Gate at proposed completion.
res := l.Gate.Verify(ctx, l.Workspace)   // only on no-tool-call turns
```

No verification happens during the tool-using portion of a turn.

---

## Proposed solution

### 1. Declare which checks are "fast"

```go
// orchestrator/verifier.go
// Check gains a Speculative flag: fast, side-effect-free checks safe to run
// opportunistically between turns (e.g. build, vet, lint — NOT full test
// suites or anything that mutates state).
type Check struct {
    // ... existing fields ...
    Speculative bool // safe + cheap to run mid-run
}
```

Mark the obvious ones in `DetectChecks` (#154):

```go
// orchestrator/detect.go — when building Go checks
{Kind: CheckPredicate, Name: "go build", Cmd: []string{"go", "build", "./..."}, Speculative: true},
{Kind: CheckPredicate, Name: "go vet",   Cmd: []string{"go", "vet", "./..."},   Speculative: true},
// "go test ./..." stays Speculative:false (too slow / side-effecting).
```

### 2. A speculative runner on the Loop

```go
// agentloop/loop.go — Loop struct
// SpeculativeChecks, when set, runs the subset of checks flagged Speculative
// after each tool-edit turn. Failures are injected as feedback before the
// agent proposes completion. Nil disables speculative verification.
SpeculativeChecks func(ctx context.Context, workspace string) (ok bool, report string)
// SpeculativeEvery throttles how often speculative checks run (every N
// tool turns). 0 -> default 1 (every tool turn).
SpeculativeEvery int
```

### 3. Run after tool sequences, before the next model call

```go
// agentloop/loop.go — after executing tool calls for a turn, before looping.
if l.SpeculativeChecks != nil && len(resp.ToolCalls) > 0 {
    every := l.SpeculativeEvery
    if every == 0 {
        every = 1
    }
    toolTurns++
    if toolTurns%every == 0 {
        if ok, report := l.SpeculativeChecks(ctx, l.Workspace); !ok {
            l.fire(ctx, hooks.SpeculativeFail, "", map[string]any{"report": report})
            l.record(ctx, ledger.TypeSpeculativeCheck,
                map[string]any{"passed": false}, "speculative check failed mid-run")
            msgs = append(msgs, session.Message{
                Role: "user",
                Content: "FAST CHECK FAILED after your last edits — fix before " +
                    "continuing:\n\n" + report,
            })
            if err := sess.SaveHistory(msgs); err != nil {
                return nil, err
            }
            continue
        }
        l.record(ctx, ledger.TypeSpeculativeCheck,
            map[string]any{"passed": true}, "speculative check passed mid-run")
    }
}
```

### 4. Helper to build the runner from a verifier

```go
// orchestrator/verifier.go
// SpeculativeRunner returns a function that runs only the Speculative-flagged
// checks of v, suitable for agentloop.Loop.SpeculativeChecks.
func (v *Verifier) SpeculativeRunner() func(context.Context, string) (bool, string) {
    return func(ctx context.Context, ws string) (bool, string) {
        var failed []string
        for _, c := range v.checks {
            if !c.Speculative {
                continue
            }
            if ok, out := c.run(ctx, ws); !ok {
                failed = append(failed, c.Name+": "+out)
            }
        }
        if len(failed) == 0 {
            return true, ""
        }
        return false, strings.Join(failed, "\n")
    }
}
```

### 5. Hook + ledger + run-state

```go
// hooks/hooks.go
// SpeculativeFail fires when a mid-run fast check fails.
SpeculativeFail = "verify.speculative_fail"
```

```go
// ledger/store.go
// TypeSpeculativeCheck records a mid-run speculative verification result.
TypeSpeculativeCheck EntryType = "speculative_check"
```

```go
// agentloop/loop.go — run state
toolTurns := 0
```

---

## Acceptance criteria

- [ ] `Check.Speculative` flag added; build/vet marked speculative in detect.go
- [ ] `Loop.SpeculativeChecks` + `SpeculativeEvery` (default 1) fields
- [ ] speculative checks run after tool turns, throttled by `SpeculativeEvery`
- [ ] failures injected as feedback and the turn loop continues (no completion)
- [ ] `Verifier.SpeculativeRunner()` builds the runner from flagged checks
- [ ] `hooks.SpeculativeFail` + `ledger.TypeSpeculativeCheck` added
- [ ] nil runner = exact legacy behavior
- [ ] test: a failing speculative check injects feedback before completion
- [ ] test: throttling respects `SpeculativeEvery`
- [ ] `go test -race ./...` green
