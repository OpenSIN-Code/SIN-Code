# [loop-011] Diff-based progress score — detect *real* stagnation, not just identical criteria text

**Labels:** `loop-system` `robustness` `p1`
**Branch:** `loop-issues`
**Affects:** `agentloop/loop.go`, new `agentloop/progress.go`, `ledger/store.go`
**Tier:** 1
**Depends on:** #150 (stall detection — done), complements #010

---

## Problem

Stall detection (#150) fingerprints the **text** of the open criteria. If the
stop-gate slightly rephrases criteria each turn (LLM nondeterminism), the
fingerprint changes and the stall guard never fires — even though zero real
progress is being made. Conversely, criteria text can stay identical while the
agent *is* making progress (e.g. partial work toward the same criterion).

We need a signal grounded in **actual workspace change** rather than gate text.

---

## Root cause

```go
// agentloop/loop.go — fingerprint is purely textual.
fp := strings.Join(dec.OpenCriteria, "\x1f")
if fp != "" && fp == lastCritFingerprint {
    stallCount++
}
```

---

## Proposed solution

### 1. A progress probe over the git working tree

```go
// agentloop/progress.go
package agentloop

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "os/exec"
    "strconv"
    "strings"
)

// ProgressProbe measures real change in the workspace between turns. The
// default implementation hashes `git diff` stat output; any monotonic
// non-change across turns is treated as a true stall regardless of how the
// stop-gate phrases its criteria.
type ProgressProbe func(ctx context.Context, workspace string) ProgressSignal

// ProgressSignal captures a cheap, comparable snapshot of workspace progress.
type ProgressSignal struct {
    DiffHash    string // hash of the diff content; identical => no new edits
    LinesChanged int   // additions+deletions in the working tree
}

// GitProgressProbe is the default probe. It is best-effort: if git is missing
// or the workspace is not a repo, it returns a zero signal (which disables
// diff-based stall detection gracefully).
func GitProgressProbe(ctx context.Context, workspace string) ProgressSignal {
    // `git diff --numstat` over tracked+staged changes; cheap and stable.
    out, err := runGit(ctx, workspace, "diff", "--numstat", "HEAD")
    if err != nil {
        return ProgressSignal{}
    }
    lines := 0
    for _, ln := range strings.Split(strings.TrimSpace(out), "\n") {
        fields := strings.Fields(ln)
        if len(fields) >= 2 {
            add, _ := strconv.Atoi(fields[0])
            del, _ := strconv.Atoi(fields[1])
            lines += add + del
        }
    }
    sum := sha256.Sum256([]byte(out))
    return ProgressSignal{DiffHash: hex.EncodeToString(sum[:]), LinesChanged: lines}
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
    cmd := exec.CommandContext(ctx, "git", args...)
    cmd.Dir = dir
    b, err := cmd.Output()
    return string(b), err
}
```

### 2. Loop field + run-state

```go
// agentloop/loop.go — Loop struct
// ProgressProbe, if set, augments text-based stall detection with a measure
// of real workspace change. When the probe reports an identical diff hash for
// NoProgressThreshold consecutive stop-gate rejects, the run is considered
// stalled even if the criteria text changed. Nil disables it (legacy).
ProgressProbe ProgressProbe
// NoProgressThreshold: consecutive identical-diff rejects before a hard stall.
// 0 -> default 3.
NoProgressThreshold int
```

```go
// agentloop/loop.go — run state
lastDiffHash := ""
noProgressCount := 0
```

### 3. Combine textual + diff signals in the reject branch

```go
// agentloop/loop.go — inside `if !dec.Complete {` after stopRejects++
// Diff-based progress check (robust to criteria rephrasing).
if l.ProgressProbe != nil {
    sig := l.ProgressProbe(ctx, l.Workspace)
    threshold := l.NoProgressThreshold
    if threshold == 0 {
        threshold = 3
    }
    if sig.DiffHash != "" && sig.DiffHash == lastDiffHash {
        noProgressCount++
    } else {
        noProgressCount = 0
        lastDiffHash = sig.DiffHash
    }
    if noProgressCount >= threshold {
        if serr := sess.SaveHistory(msgs); serr != nil {
            return nil, serr
        }
        l.fire(ctx, hooks.StopStalled, "", map[string]any{
            "reason": "no workspace change", "no_progress_count": noProgressCount,
        })
        l.record(ctx, ledger.TypeNoProgress,
            map[string]any{"no_progress_count": noProgressCount, "lines_changed": sig.LinesChanged},
            fmt.Sprintf("no workspace change for %d rejects; escalating", noProgressCount))
        // Hand off to the Replanner (#010) if present, else abort.
        // (If #010 landed, call the shared escalate path here.)
        return nil, fmt.Errorf("no workspace progress for %d consecutive rejects", noProgressCount)
    }
}
```

### 4. Ledger entry

```go
// ledger/store.go
// TypeNoProgress records a diff-based stall: the working tree stopped changing
// across consecutive stop-gate rejects.
TypeNoProgress EntryType = "no_progress"
```

---

## Interaction with #010 and #150

- #150 (textual fingerprint) and this issue (diff hash) are **complementary**:
  either can trigger a stall. Whichever fires first wins.
- When #010 (Replanner) is present, both stall paths should route through the
  same escalate-or-replan helper rather than returning directly. Extract a
  small `(l *Loop) escalateStall(...)` method to avoid duplication.

---

## Acceptance criteria

- [ ] `agentloop/progress.go` with `ProgressProbe`, `ProgressSignal`, `GitProgressProbe`
- [ ] `ProgressProbe` + `NoProgressThreshold` (default 3) on `Loop`
- [ ] reject branch increments `noProgressCount` on identical diff hash, resets otherwise
- [ ] `ledger.TypeNoProgress` added
- [ ] nil probe = exact legacy behavior; missing git = graceful no-op
- [ ] shared escalate path with #010 if both are present
- [ ] test: identical diff across N rejects triggers stall
- [ ] test: changing diff resets the counter
- [ ] `go test -race ./...` green
