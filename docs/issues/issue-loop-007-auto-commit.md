# [loop-007] Daemon must auto-commit verified work — no human `git commit` needed

**Labels:** `loop-engineering` `autonomy` `p2`
**Branch:** `agent-loop-engineering`
**Affects:** `daemon_cmd.go`, new `internal/gitops/commit.go`

---

## Problem

After the stop-gate confirms a goal is done and `queue.Complete` fires, the
changes are sitting in the working tree as unstaged or staged files. A human
must still run `git add && git commit && git push`. This is babysitting at the
final step — the loop is 99% autonomous and then stops at the most mechanical
task imaginable.

---

## Root cause

`executeGoal` calls `queue.Complete` and that is the end:

```go
// daemon_cmd.go — current end of executeGoal
_ = queue.Complete(ctx, goal.ID, sess.ID)
hookEngine.Fire(ctx, hooks.Payload{Event: hooks.GoalVerified, ...})
fmt.Printf("daemon: goal %d VERIFIED in %d turns (session %s)\n", ...)
// NOTHING ELSE. No commit, no push, no PR.
```

There is no `internal/gitops` package. The daemon has no git awareness.

---

## Proposed solution

### 1. New `internal/gitops` package

```go
// internal/gitops/commit.go (new file)
package gitops

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "strings"
    "time"
)

type CommitOptions struct {
    Workspace   string
    Message     string
    Author      string // "Name <email>" — default: SIN-Code Agent <agent@sin-code.local>
    PushRemote  string // "origin" when non-empty triggers a push
    PushBranch  string // current branch when empty
    CreatePR    bool   // open a GitHub PR after push (requires GH_TOKEN)
    PRTitle     string
    PRBody      string
}

// AutoCommit stages all modified tracked files, commits, and optionally pushes.
// It is a no-op when the working tree is clean (no changes to commit).
func AutoCommit(ctx context.Context, opts CommitOptions) error {
    if opts.Workspace == "" {
        return fmt.Errorf("gitops: workspace must be set")
    }
    if opts.Author == "" {
        opts.Author = "SIN-Code Agent <agent@sin-code.local>"
    }

    // Is the tree dirty?
    status, err := runGit(opts.Workspace, "status", "--porcelain")
    if err != nil {
        return fmt.Errorf("gitops: status: %w", err)
    }
    if strings.TrimSpace(status) == "" {
        return nil // nothing to commit
    }

    // Stage all changes (tracked + untracked code files).
    if _, err := runGit(opts.Workspace, "add", "-A"); err != nil {
        return fmt.Errorf("gitops: add: %w", err)
    }

    msg := opts.Message
    if msg == "" {
        msg = "chore(agent): autonomous goal completion [sin-code]"
    }
    args := []string{
        "commit",
        "--author=" + opts.Author,
        "-m", msg,
        "--no-verify", // skip local hooks — our verification already ran
    }
    if _, err := runGit(opts.Workspace, args...); err != nil {
        return fmt.Errorf("gitops: commit: %w", err)
    }
    fmt.Fprintf(os.Stderr, "gitops: committed: %s\n", truncate(msg, 72))

    if opts.PushRemote == "" {
        return nil
    }
    branch := opts.PushBranch
    if branch == "" {
        branch, err = currentBranch(opts.Workspace)
        if err != nil {
            return fmt.Errorf("gitops: current branch: %w", err)
        }
    }
    // Never push directly to main/master without explicit opt-in.
    if (branch == "main" || branch == "master") && !isMainPushAllowed() {
        return fmt.Errorf("gitops: refusing to push directly to %s; "+
            "set SIN_ALLOW_MAIN_PUSH=1 or use a feature branch", branch)
    }
    if _, err := runGit(opts.Workspace, "push", opts.PushRemote, branch); err != nil {
        return fmt.Errorf("gitops: push: %w", err)
    }
    fmt.Fprintf(os.Stderr, "gitops: pushed %s → %s/%s\n", branch, opts.PushRemote, branch)

    if opts.CreatePR && os.Getenv("GH_TOKEN") != "" {
        return openPR(ctx, opts, branch)
    }
    return nil
}

func runGit(workspace string, args ...string) (string, error) {
    cctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()
    cmd := exec.CommandContext(cctx, "git", args...)
    cmd.Dir = workspace
    out, err := cmd.CombinedOutput()
    return string(out), err
}

func currentBranch(workspace string) (string, error) {
    out, err := runGit(workspace, "rev-parse", "--abbrev-ref", "HEAD")
    return strings.TrimSpace(out), err
}

func isMainPushAllowed() bool {
    return os.Getenv("SIN_ALLOW_MAIN_PUSH") == "1"
}

func truncate(s string, n int) string {
    if len(s) <= n { return s }
    return s[:n-3] + "..."
}
```

### 2. Wire into `executeGoal`

```go
// daemon_cmd.go — after queue.Complete, when SIN_AUTO_COMMIT=1
_ = queue.Complete(ctx, goal.ID, sess.ID)

if os.Getenv("SIN_AUTO_COMMIT") == "1" || opt.autoCommit {
    commitMsg := fmt.Sprintf(
        "feat(agent): complete goal #%d in %d turns\n\n%s\n\n[sin-code goal-id: %d]",
        goal.ID, res.Turns, res.Summary, goal.ID)
    if err := gitops.AutoCommit(ctx, gitops.CommitOptions{
        Workspace:  goal.Workspace,
        Message:    commitMsg,
        PushRemote: opt.pushRemote, // "" = no push; "origin" = push
        CreatePR:   opt.openPR,
        PRTitle:    fmt.Sprintf("Agent: goal #%d — %s", goal.ID, truncate(goal.Prompt, 60)),
        PRBody:     "Autonomously completed by SIN-Code daemon.\n\n" + res.Summary,
    }); err != nil {
        fmt.Fprintf(os.Stderr, "warn: auto-commit failed: %v\n", err)
        // Non-fatal: goal is verified regardless of commit status.
    }
}
```

### 3. New daemon flags

```go
cmd.Flags().BoolVar(&autoCommit, "auto-commit", false,
    "automatically git commit verified work (env: SIN_AUTO_COMMIT=1)")
cmd.Flags().StringVar(&pushRemote, "push-remote", "",
    "git remote to push to after commit (e.g. origin); empty = no push")
cmd.Flags().BoolVar(&openPR, "open-pr", false,
    "open a GitHub PR after push (requires GH_TOKEN, implies --push-remote=origin)")
```

### 4. Commit message format

```
feat(agent): complete goal #42 in 17 turns

Implemented the stop-gate hybrid evaluator with deterministic checks
and LLM judge. All tests pass, coverage ≥ 80%, CHANGELOG updated.

Closes #42
[sin-code goal-id: 42]
```

---

## Acceptance criteria

- [ ] `internal/gitops` package with `AutoCommit` function
- [ ] `AutoCommit` is a no-op on clean tree; never errors on clean tree
- [ ] refuses to push to `main`/`master` without `SIN_ALLOW_MAIN_PUSH=1`
- [ ] `--auto-commit`, `--push-remote`, `--open-pr` flags on daemon
- [ ] `SIN_AUTO_COMMIT=1` env var as alternative to flag
- [ ] commit message embeds goal ID, summary, turn count
- [ ] `AutoCommit` failure is non-fatal (logs warning, goal still verified)
- [ ] unit tests for clean-tree no-op, dirty-tree commit, main-branch refusal
- [ ] `go test -race` green
