# [loop-012] Persistent lesson-learning across runs — the loop gets better at a repo over time

**Labels:** `loop-system` `learning` `p2`
**Branch:** `loop-issues`
**Affects:** `agentloop/loop.go`, `internal/lessons` (existing `Lessons` store), `loopbuilder/builder.go`
**Tier:** 2

---

## Problem

The `Loop` already has a `Lessons` store, but it is only read mid-run. We never
**write back** what went wrong (stalls, repeated stop-gate rejects, reflection
findings) as durable lessons. Each run starts cold and repeats the same
mistakes in the same repo.

We want the loop to **harvest lessons at the end of each run** and **inject
relevant ones at the start** of future runs as system-prompt context.

---

## Root cause

```go
// agentloop/loop.go — Lessons is consulted for injection but never appended to
// from terminal run outcomes (stall, reject, reflection patterns).
type Loop struct {
    // ...
    Lessons LessonStore // read-only in practice today
}
```

---

## Proposed solution

### 1. Extend the lesson store interface (additive)

```go
// agentloop/loop.go (or internal/lessons) — interface the loop relies on.
type LessonStore interface {
    // Relevant returns lessons applicable to the given goal/workspace, most
    // useful first, capped to `limit`.
    Relevant(workspace, goal string, limit int) []Lesson
    // Record persists a new lesson harvested from a finished run.
    Record(l Lesson) error
}

// Lesson is a durable, reusable insight scoped to a workspace.
type Lesson struct {
    Workspace string   `json:"workspace"`
    Kind      string   `json:"kind"`     // "stall" | "reject" | "reflection" | "success"
    Trigger   string   `json:"trigger"`  // what situation it applies to
    Guidance  string   `json:"guidance"` // what to do about it
    Tags      []string `json:"tags,omitempty"`
    CreatedAt int64    `json:"created_at"`
}
```

### 2. Inject relevant lessons at run start

```go
// agentloop/loop.go — near the top of Run, before the turn loop.
if l.Lessons != nil {
    if ls := l.Lessons.Relevant(l.Workspace, prompt, 5); len(ls) > 0 {
        var b strings.Builder
        b.WriteString("LESSONS FROM PRIOR RUNS IN THIS REPO (apply proactively):\n")
        for i, ln := range ls {
            fmt.Fprintf(&b, "  %d. [%s] %s -> %s\n", i+1, ln.Kind, ln.Trigger, ln.Guidance)
        }
        // Prepend as a system-style user message so it conditions the whole run.
        msgs = append([]session.Message{{Role: "user", Content: b.String()}}, msgs...)
    }
}
```

### 3. Harvest lessons at terminal outcomes

```go
// agentloop/loop.go — helper called from stall/reject/reflection/success paths.

// harvestLesson persists a reusable insight from the current run. Best-effort:
// a store error is logged via the ledger but never fails the run.
func (l *Loop) harvestLesson(ctx context.Context, kind, trigger, guidance string, tags ...string) {
    if l.Lessons == nil {
        return
    }
    les := Lesson{
        Workspace: l.Workspace, Kind: kind, Trigger: trigger,
        Guidance: guidance, Tags: tags, CreatedAt: time.Now().Unix(),
    }
    if err := l.Lessons.Record(les); err != nil {
        l.record(ctx, ledger.TypeNote,
            map[string]any{"lesson_error": err.Error()}, "failed to persist lesson")
        return
    }
    l.record(ctx, ledger.TypeLessonLearned,
        map[string]any{"kind": kind, "trigger": trigger}, "lesson recorded: "+kind)
}
```

Call sites:

```go
// On stall escalation:
l.harvestLesson(ctx, "stall",
    "open criteria stuck: "+strings.Join(lastOpen, "; "),
    "decompose this kind of goal earlier; verify incrementally", "stall")

// On reflection finding issues:
l.harvestLesson(ctx, "reflection",
    "self-review caught: "+strings.Join(ref.Issues, "; "),
    "address these checks before declaring done", "quality")

// On successful completion (in the DONE path):
l.harvestLesson(ctx, "success",
    "goal completed in "+strconv.Itoa(turn+1)+" turns",
    "this approach worked for similar goals", "success")
```

### 4. Ledger entries

```go
// ledger/store.go
// TypeLessonLearned records that a reusable lesson was persisted for the repo.
TypeLessonLearned EntryType = "lesson_learned"
// TypeNote is a generic informational entry (used for best-effort failures).
TypeNote EntryType = "note"
```

### 5. Default file-backed implementation

```go
// internal/lessons/store.go (new) — JSONL file under the workspace .sin-code dir.
// One lesson per line; Relevant() does simple tag/substring matching scored by
// recency. Intentionally dependency-free and human-readable for inspection.
```

---

## Acceptance criteria

- [ ] `LessonStore` gains `Record`; `Lesson` struct defined
- [ ] relevant lessons injected at run start (capped, most-useful-first)
- [ ] lessons harvested on stall, reflection-issues, and success
- [ ] `ledger.TypeLessonLearned` + `TypeNote` added
- [ ] file-backed JSONL store implementation under `.sin-code/lessons.jsonl`
- [ ] harvesting is best-effort and never fails a run
- [ ] test: a recorded lesson is returned by `Relevant` for a matching goal
- [ ] test: injection adds the lesson block to the first message
- [ ] `go test -race ./...` green
