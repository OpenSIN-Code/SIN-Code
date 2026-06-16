# [loop-005] Semantic goal decomposition — agent must split large goals automatically, not wait

**Labels:** `loop-engineering` `autonomy` `p1`
**Branch:** `agent-loop-engineering`
**Affects:** `daemon_cmd.go` (`wrapWithSpawn`), `agentloop/loop.go`, `goalcontract/goalcontract.go`

---

## Problem

`spawn_subgoal` exists as a tool but the agent only uses it when it decides to.
For large goals ("refactor the entire auth system", "add full test coverage to
all packages") the agent often just starts writing code without decomposing,
then hits `max-turns`, checkpoints, and resumes — losing context between
continuations. The loop should proactively detect "this goal is too large" and
force decomposition before work begins, not after max-turns is hit.

---

## Root cause

```go
// daemon_cmd.go — current executeGoal — builds the loop immediately
loop, cleanup, err := loopbuilder.Build(ctx, loopbuilder.Config{
    // ...
    ToolFactory: func(mgr *mcpclient.Manager) (...) {
        return wrapWithSpawn(queue, goal, opt.maxDepth, ...)
    },
})
// No pre-flight decomposition step; the agent discovers the tool mid-run.
res, err := loop.Run(ctx, sess, goal.Prompt)
```

`wrapWithSpawn` is registered as a tool but its description does not tell the
model *when* to use it proactively. The system prompt injected by the loop also
does not mention decomposition as a first step.

---

## Proposed solution

### 1. Pre-flight decomposition prompt injection

Before running the main loop for a goal that has no parent (depth == 0) and no
session history (first attempt), inject a decomposition-first directive:

```go
// daemon_cmd.go — executeGoal, before loop.Run
decompositionDirective := ""
if goal.Depth == 0 && goal.Attempts == 1 && len(sess.History()) == 0 {
    decompositionDirective = buildDecompositionDirective(goal.Prompt, opt.maxDepth)
}

var effectivePrompt string
if decompositionDirective != "" {
    effectivePrompt = decompositionDirective + "\n\n---\nORIGINAL GOAL:\n" + goal.Prompt
} else {
    effectivePrompt = goal.Prompt
}
res, err := loop.Run(ctx, sess, effectivePrompt)
```

```go
func buildDecompositionDirective(prompt string, maxDepth int) string {
    return fmt.Sprintf(`AUTONOMOUS EXECUTION PROTOCOL — read before starting:

You are an autonomous coding agent. Your job is to FULLY complete the goal
below without any human intervention. Before writing a single line of code:

1. ASSESS SCOPE: Estimate how many distinct units of work this goal requires.
   - If it requires changes in 3+ independent packages or concerns: USE spawn_subgoal
     to decompose it into child goals FIRST, then work on each child.
   - If it is a single self-contained change: proceed directly.

2. FOR EVERY CODE CHANGE YOU MAKE, the following are NON-NEGOTIABLE:
   a. Write or update _test.go files for every changed package.
   b. Ensure go build ./... passes.
   c. Ensure go test -race ./... passes.
   d. Ensure go vet ./... is clean.
   e. Remove any TODO/FIXME you introduced.

3. DON'T STOP EARLY. The stop-gate is independent of you. Even if you think
   you're done, continue working until it confirms completion.

4. spawn_subgoal is available (max depth %d). Use it freely for independent
   units of work — child goals run in parallel and are verified independently.

5. When ALL work is done: summarize exactly what changed and why.`, maxDepth)
}
```

### 2. `GoalContract.MaxSubGoals` to express expected decomposition

```go
// internal/goalcontract/goalcontract.go
type GoalContract struct {
    // ... existing fields ...

    // MaxSubGoals, when > 0, is a semantic criterion injected into the
    // stop-gate: it tells the LLM judge to confirm the goal was decomposed
    // into at least MinSubGoals child goals when the scope warranted it.
    MinSubGoals int `json:"min_sub_goals,omitempty"`
}
```

### 3. Scope estimator in `Resolve`

When a goal prompt contains size signals ("all packages", "entire", "full",
"comprehensive"), auto-add a semantic criterion and lower the max-turns
default via a hint:

```go
// internal/goalcontract/goalcontract.go
func autoDetectScopeHints(prompt string) []string {
    largeScope := []string{
        "all packages", "entire", "full coverage", "comprehensive",
        "refactor", "migrate", "all tests", "every",
    }
    low := strings.ToLower(prompt)
    for _, hint := range largeScope {
        if strings.Contains(low, hint) {
            return []string{
                "The goal was large in scope. If it required 3+ independent units, " +
                    "it was decomposed into sub-goals via spawn_subgoal. " +
                    "All sub-goals are verified before the parent finalizes.",
            }
        }
    }
    return nil
}

// In Resolve, after existing criteria:
if opts.AutoDetect {
    for _, cr := range autoDetectScopeHints(opts.GoalID) { // pass prompt via GoalID field or new Prompt field
        c.SemanticCriteria = append(c.SemanticCriteria, cr)
    }
}
```

### 4. Add `Prompt` to `ResolveOptions` so scope hints have access to it

```go
// internal/goalcontract/goalcontract.go
type ResolveOptions struct {
    Workspace    string
    GoalID       string
    Prompt       string  // NEW: used for scope heuristics
    ContractFile string
    Criteria     []string
    DoneWhen     string
    VerifyCmd    string
    AutoDetect   bool
}
```

---

## Acceptance criteria

- [ ] `buildDecompositionDirective` injected for depth-0 first-attempt goals
- [ ] `ResolveOptions.Prompt` added and passed from `executeGoal`
- [ ] `autoDetectScopeHints` detects large-scope prompts and adds semantic criterion
- [ ] `GoalContract.MinSubGoals` field added
- [ ] decomposition directive mentions all non-negotiables (tests, build, vet, no-todos, stop-gate)
- [ ] directive is not injected for child goals (depth > 0) or resumptions
- [ ] unit tests for `buildDecompositionDirective` and `autoDetectScopeHints`
- [ ] `go test -race` green
