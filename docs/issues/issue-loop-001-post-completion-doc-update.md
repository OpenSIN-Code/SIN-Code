# [loop-001] Post-completion doc updater — agent must never leave docs stale

**Labels:** `loop-engineering` `autonomy` `p0`
**Branch:** `sincode-loop-system`
**Status:** DONE
**Affects:** `daemon_cmd.go`, `goalcontract/goalcontract.go`, `agentloop/loop.go`

---

## Problem

The agent finishes its code changes, the verify-gate passes, the stop-gate
confirms the contract, `queue.Complete` fires — and **nobody updates
README.md, CHANGELOG.md, AGENTS.md, BACKLOG.md, or the package `doc.md`**.
A human must still open a PR comment and say "please update the docs". That
is babysitting. Every completed goal that touches code should atomically
trigger a doc-update sub-goal before the parent is marked verified.

---

## Root cause

`executeGoal` in `daemon_cmd.go` calls `queue.Complete` immediately after a
verified `Result`:

```go
// daemon_cmd.go — current (incomplete)
_ = queue.Complete(ctx, goal.ID, sess.ID)
hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalVerified, ...})
```

There is no post-completion phase. The `GoalContract` struct has a
`SemanticCriteria` slice but no `PostCompletionGoals` field. The discovery
scanner (`autonomy/discover.go`) only runs on a timer trigger, not after
each successful goal.

---

## Proposed solution

### 1. Add `PostCompletionGoals` to `GoalContract`

```go
// internal/goalcontract/goalcontract.go
type GoalContract struct {
    GoalID              string               `json:"goal_id,omitempty"`
    DeterministicChecks []orchestrator.Check `json:"deterministic_checks,omitempty"`
    SemanticCriteria    []string             `json:"semantic_criteria,omitempty"`
    MaxFilesChanged     int                  `json:"max_files_changed,omitempty"`
    MaxLinesChanged     int                  `json:"max_lines_changed,omitempty"`

    // PostCompletionGoals are sub-goals auto-spawned AFTER the main goal
    // verifies. Each one runs as a child and the parent only truly
    // finalizes when all post-completion children are verified too.
    // This is how the loop ensures docs, CHANGELOG, BACKLOG, and README
    // are ALWAYS updated without a human reminder.
    PostCompletionGoals []PostGoal `json:"post_completion_goals,omitempty"`

    // DisablePostGoals persists a per-goal opt-out set via
    // `goal add --no-post-goals`. Survives restarts because it is stored
    // in the contract JSON alongside the goal in the queue DB.
    DisablePostGoals bool `json:"disable_post_goals,omitempty"`
}

// PostGoal is one automatically spawned follow-up goal.
type PostGoal struct {
    // PromptTemplate is a Go text/template rendered with the parent
    // Result as its data (fields: .Summary, .SessionID, .Turns).
    PromptTemplate string   `json:"prompt_template"`
    // Criteria are acceptance criteria for this post-goal's stop-gate.
    Criteria       []string `json:"criteria,omitempty"`
    // OnlyIfChanged is a glob pattern; the post-goal is skipped when no
    // files matching the pattern were modified by the parent goal. Avoids
    // spawning a CHANGELOG update when no user-visible code changed.
    // Uses filepath.Match against the base name of each changed file.
    OnlyIfChanged  string   `json:"only_if_changed,omitempty"`
}
```

### 2. Auto-detect standard post-completion goals in `Resolve`

```go
// internal/goalcontract/goalcontract.go — autoDetectChecks extension
func autoDetectPostGoals(workspace string) []PostGoal {
    var goals []PostGoal
    // Always: update CHANGELOG.md to record the change.
    if fileExists(filepath.Join(workspace, "CHANGELOG.md")) {
        goals = append(goals, PostGoal{
            PromptTemplate: `Update CHANGELOG.md to record the following completed work:
{{ .Summary }}
Add it under the [Unreleased] section with today's date. Follow the
existing format exactly. Ensure the build and tests still pass.`,
            Criteria:      []string{"CHANGELOG.md updated with the completed work"},
            OnlyIfChanged: "*.go",
        })
    }
    // If a MASTER_TODO.md exists: check off any item the goal addressed.
    if fileExists(filepath.Join(workspace, "MASTER_TODO.md")) {
        goals = append(goals, PostGoal{
            PromptTemplate: `Review MASTER_TODO.md and check off (change "- [ ]" to "- [x]")
any items that were completed by this work:
{{ .Summary }}
Do not add new items. Ensure the build and tests still pass.`,
            Criteria:      []string{"all relevant MASTER_TODO items checked off"},
        })
    }
    // If a docs/ directory exists: update relevant doc.md files.
    if dirExists(filepath.Join(workspace, "docs")) {
        goals = append(goals, PostGoal{
            PromptTemplate: `Review all doc.md files under docs/ and cmd/ that relate to
the following completed work and update them to reflect any API,
flag, or behavioral changes:
{{ .Summary }}
Do not change unrelated docs. Ensure the build and tests still pass.`,
            Criteria:      []string{"all affected doc.md files reflect the change"},
            OnlyIfChanged: "*.go",
        })
    }
    return goals
}
```

### 3. Spawn post-completion sub-goals in `executeGoal`

```go
// daemon_cmd.go — after verifying, before queue.Complete
if !opt.noPostGoals && contract != nil && len(contract.PostCompletionGoals) > 0 {
    spawnPostGoals(ctx, queue, goal, res, contract.PostCompletionGoals)
}
_ = queue.Complete(ctx, goal.ID, sess.ID)

// ---

func spawnPostGoals(ctx context.Context, q *autonomy.Queue,
    parent *autonomy.Goal, res *agentloop.Result, posts []goalcontract.PostGoal) {

    tmplData := map[string]any{
        "Summary":   res.Summary,
        "SessionID": res.SessionID,
        "Turns":     res.Turns,
    }
    for _, pg := range posts {
        if pg.OnlyIfChanged != "" && !changedFilesMatch(parent.Workspace, pg.OnlyIfChanged) {
            continue
        }
        prompt, err := renderTemplate(pg.PromptTemplate, tmplData)
        if err != nil {
            fmt.Fprintf(os.Stderr, "post-goal template error: %v\n", err)
            continue
        }
        var contractJSON string
        if len(pg.Criteria) > 0 {
            c := &goalcontract.GoalContract{SemanticCriteria: pg.Criteria}
            contractJSON, _ = c.Marshal()
        }
        id, err := q.AddSub(ctx, parent.ID, prompt, parent.Priority, parent.MaxRetries, contractJSON)
        if err != nil {
            fmt.Fprintf(os.Stderr, "warn: could not spawn post-goal: %v\n", err)
            continue
        }
        fmt.Printf("daemon: spawned post-completion goal %d (docs/changelog) under goal %d\n", id, parent.ID)
    }
}
```

### 4. `changedFilesMatch` — detect what changed in the last commit

```go
// daemon_cmd.go
// changedFilesMatch reports whether any file changed in the last commit
// matches the glob pattern. Fail-open: returns true when the diff cannot
// be determined (no prior commit, not a git repo) so post-goals always
// run in ambiguous situations rather than silently skip.
func changedFilesMatch(workspace, pattern string) bool {
    cmd := exec.Command("git", "diff", "--name-only", "HEAD~1", "HEAD")
    cmd.Dir = workspace
    out, err := cmd.Output()
    if err != nil {
        return true // fail-open: when we can't detect, always spawn
    }
    for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
        if ok, _ := filepath.Match(pattern, filepath.Base(line)); ok {
            return true
        }
        if ok, _ := filepath.Match(pattern, line); ok {
            return true
        }
    }
    return false
}
```

---

## Implementation notes (added after build)

**Why `queue.Complete` fires AFTER post-goals are spawned, not before:**
The queue's tree semantics treat a parent as "blocked" until all children are
verified. If `queue.Complete` were called first, the parent would immediately
become verified and the queue could prune it before the children are picked up
by a worker. The ordering `spawnPostGoals → queue.Complete` is load-bearing.

**Template data is intentionally minimal (Summary, SessionID, Turns):**
Passing the full `agentloop.Result` would couple this to the internal loop API.
Three fields are enough for every post-goal prompt we need. If more context is
required in the future, add fields to a dedicated `PostGoalData` struct.

**`OnlyIfChanged` uses `filepath.Match` not full glob expansion:**
`filepath.Match` only handles a single path segment per `*`. For cross-directory
patterns the check falls back to matching against the full relative path. This
means `*.go` matches `foo.go` in any directory, which is the intended behaviour.
Use `**/*.go` style paths with caution — `filepath.Match` does not expand `**`.

**`DisablePostGoals` vs `--no-post-goals` daemon flag:**
The daemon flag (`opt.noPostGoals`) is a runtime global override. The per-goal
`DisablePostGoals` field is stored in the contract JSON in the queue DB, so it
survives daemon restarts. Both are checked: `opt.noPostGoals || persisted.DisablePostGoals`.

**Post-goals are not recursive:**
Post-completion goals themselves do not spawn further post-completion goals.
`autoDetectPostGoals` is only called in the `Resolve` path for the initial
contract, not for goals spawned from `spawnPostGoals`. This prevents infinite
doc-update chains.

**Relation to loop-007 (auto-commit):**
Auto-commit runs BEFORE post-goals are spawned. This gives the doc-update
child goals a clean baseline (the main work is already committed as HEAD),
so `git diff HEAD~1..HEAD` inside the doc-goal correctly shows only the parent's
changes and not the doc-update changes themselves.

---

## Acceptance criteria

- [x] `GoalContract` has `PostCompletionGoals []PostGoal`
- [x] `autoDetectChecks` auto-populates post-goals for CHANGELOG, MASTER_TODO, docs when files exist
- [x] `executeGoal` spawns post-completion sub-goals before `queue.Complete`
- [x] post-goals run as tree children; parent finalizes only once all post-goals are verified
- [x] `--no-post-goals` flag on daemon to opt out
- [x] `goal add --no-post-goals` flag
- [x] unit tests for template rendering and `changedFilesMatch`
- [x] `go test -race` green on `goalcontract` and `autonomy` packages
