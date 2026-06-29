// SPDX-License-Identifier: MIT
// Purpose: auto-commit after verified task (issue #487).
// M3: git commit is created ONLY after the verification gate passes.
// M2: uses os/exec for git — no CGO.
package agentloop

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
)

// detectCommitPrefix infers a conventional-commit prefix from the prompt
// content. Returns "feat" as the default when no keyword matches.
func detectCommitPrefix(prompt string) string {
	lower := strings.ToLower(prompt)
	switch {
	case strings.Contains(lower, "security") || strings.Contains(lower, "vuln"):
		return "security"
	case strings.Contains(lower, "refactor"):
		return "refactor"
	case strings.Contains(lower, "doc") || strings.Contains(lower, "readme"):
		return "docs"
	case strings.Contains(lower, "test"):
		return "test"
	case strings.Contains(lower, "fix") || strings.Contains(lower, "bug"):
		return "fix"
	case strings.Contains(lower, "perf") || strings.Contains(lower, "optim"):
		return "perf"
	case strings.Contains(lower, "chore") || strings.Contains(lower, "cleanup"):
		return "chore"
	case strings.Contains(lower, "style") || strings.Contains(lower, "format"):
		return "style"
	case strings.Contains(lower, "ci") || strings.Contains(lower, "pipeline"):
		return "ci"
	case strings.Contains(lower, "build"):
		return "build"
	default:
		return "feat"
	}
}

// buildCommitMessage assembles a conventional-commit message from the
// prefix and the session summary / prompt.
func buildCommitMessage(prefix, prompt, summary string) string {
	msg := summary
	if strings.TrimSpace(msg) == "" {
		msg = prompt
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "auto-commit: verified task"
	}
	firstLine := strings.SplitN(msg, "\n", 2)[0]
	full := fmt.Sprintf("%s: %s", prefix, firstLine)
	if len(full) > 72 {
		full = full[:69] + "..."
	}
	return full
}

// hasStagedChanges runs `git status --porcelain` in the workspace and
// returns true if there are any changes (staged or unstaged).
func hasStagedChanges(ctx context.Context, workspace string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", workspace, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// doAutoCommit runs `git add -A && git commit` in the workspace after
// the verification gate has passed. If the tree is clean (nothing to
// commit) it skips silently. It fires the commit.pre and commit.post
// hooks around the commit operation.
func (l *Loop) doAutoCommit(ctx context.Context, prompt, summary string) {
	if !l.AutoCommit {
		return
	}

	dirty, err := hasStagedChanges(ctx, l.Workspace)
	if err != nil {
		l.fire(ctx, hooks.TaskAbort, "", map[string]any{
			"auto_commit": true, "error": err.Error(),
		})
		return
	}
	if !dirty {
		return
	}

	prefix := l.CommitPrefix
	if prefix == "" {
		prefix = detectCommitPrefix(prompt)
	}
	msg := buildCommitMessage(prefix, prompt, summary)

	l.fire(ctx, hooks.CommitPre, "", map[string]any{
		"auto_commit": true, "message": msg,
	})

	addCmd := exec.CommandContext(ctx, "git", "-C", l.Workspace, "add", "-A")
	if err := addCmd.Run(); err != nil {
		l.fire(ctx, hooks.TaskAbort, "", map[string]any{
			"auto_commit": true, "error": fmt.Sprintf("git add: %v", err),
		})
		return
	}

	commitCmd := exec.CommandContext(ctx, "git", "-C", l.Workspace, "commit", "-m", msg)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		l.fire(ctx, hooks.TaskAbort, "", map[string]any{
			"auto_commit": true,
			"error":       fmt.Sprintf("git commit: %v: %s", err, string(out)),
		})
		return
	}

	l.fire(ctx, hooks.CommitPost, "", map[string]any{
		"auto_commit": true, "message": msg,
	})
}
