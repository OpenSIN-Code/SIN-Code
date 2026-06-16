# [loop-016] Cost-aware model routing — cheap model for routine turns, strong model for hard ones

**Labels:** `loop-system` `cost` `p2`
**Branch:** `loop-issues`
**Affects:** `agentloop/loop.go`, `loopbuilder/builder.go`
**Tier:** 3
**Depends on:** #151 (token budget — done)

---

## Problem

Every turn uses the same (expensive) model — reflection passes, simple
tool-call follow-ups, and the stop-gate evaluation all pay top-tier prices.
Routine turns rarely need the strongest model. With `BudgetWarnRatio` (#151)
we already know when we're running hot; we should **route turns to a cheaper
model** for routine work and reserve the strong model for planning and the
stop-gate.

---

## Root cause

```go
// agentloop/loop.go — a single Completion func, one model for everything.
Completion func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error)
```

---

## Proposed solution

### 1. Turn classification

```go
// agentloop/loop.go

// TurnRole classifies what a turn is for, so the router can pick a model.
type TurnRole string

const (
    RolePlanning   TurnRole = "planning"   // first turn / post-replan
    RoleWork       TurnRole = "work"       // ordinary tool-using turns
    RoleReflection TurnRole = "reflection" // self-review (cheap is fine)
    RoleStopGate   TurnRole = "stopgate"   // completion judgement (use strong)
)

// ModelRouter picks a Completion implementation for a given turn role and
// budget pressure (0..1, where 1 means budget fully spent). Returning nil
// falls back to the default Completion.
type ModelRouter func(role TurnRole, budgetPressure float64) CompletionFunc

// CompletionFunc is the existing completion signature, named for reuse.
type CompletionFunc func(ctx context.Context, msgs []session.Message, tools []ToolSpec) (*Completion, error)
```

### 2. Loop field + resolver

```go
// agentloop/loop.go — Loop struct
// Router, if set, selects the model per turn. Falls back to Completion when
// it returns nil. Enables cost-aware routing without changing call sites.
Router ModelRouter
```

```go
// agentloop/loop.go — helper used at every model call site.
func (l *Loop) completionFor(role TurnRole) CompletionFunc {
    if l.Router != nil {
        pressure := 0.0
        if l.MaxTokens > 0 {
            pressure = float64(l.budgetSpentInt()) / float64(l.MaxTokens)
        }
        if fn := l.Router(role, pressure); fn != nil {
            return fn
        }
    }
    return l.Completion
}
```

### 3. Use the router at each call site

```go
// agentloop/loop.go — main work turn
role := RoleWork
if turn == 0 || justReplanned {
    role = RolePlanning
}
resp, err := l.completionFor(role)(ctx, reqMsgs, tools)
```

```go
// Reflector and StopGate, if they make model calls, should likewise be
// constructed against completionFor(RoleReflection) / completionFor(RoleStopGate).
// (The builder wires these — see below.)
```

### 4. Builder config: model names per role

```go
// loopbuilder/builder.go — Config
// Models maps turn roles to model identifiers. Empty roles fall back to
// Config.Model. Enables `.sin-code.yml` to declare routing (see loop-017).
Models map[string]string // e.g. {"work":"gpt-5-mini","stopgate":"claude-opus-4.6"}
```

```go
// loopbuilder/builder.go — build a ModelRouter from Models.
if len(cfg.Models) > 0 {
    loop.Router = func(role agentloop.TurnRole, pressure float64) agentloop.CompletionFunc {
        name := cfg.Models[string(role)]
        if name == "" {
            // Under high budget pressure, downgrade work turns automatically.
            if role == agentloop.RoleWork && pressure > 0.8 {
                name = cfg.Models["work_cheap"]
            }
        }
        if name == "" {
            return nil // fall back to default Completion
        }
        return makeCompletion(name) // existing factory that binds a model id
    }
}
```

### 5. Default policy (when no Models configured)

```go
// A sensible built-in: reflection -> cheap, stopgate -> strong, work/planning
// -> default. Documented so users understand routing without config.
```

---

## Acceptance criteria

- [ ] `TurnRole`, `ModelRouter`, `CompletionFunc` types added
- [ ] `Loop.Router` + `completionFor(role)` resolver
- [ ] every model call site tagged with a role (planning/work/reflection/stopgate)
- [ ] budget pressure passed to the router; auto-downgrade above 0.8
- [ ] `loopbuilder.Config.Models` maps roles -> model ids; builds a router
- [ ] nil Router preserves single-model behavior exactly
- [ ] test: router returns role-specific funcs; fallback when nil/empty
- [ ] test: pressure > 0.8 downgrades work turns when `work_cheap` set
- [ ] `go test -race ./...` green
