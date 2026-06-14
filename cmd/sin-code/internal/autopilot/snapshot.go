// SPDX-License-Identifier: MIT
// Purpose: git-backed keep/revert. Every experiment is reversible: snapshot
// the baseline before acting, commit on keep, hard-reset on revert. This is
// what makes unattended autonomy safe — no half-applied bad change survives.
package autopilot

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Snapshotter wraps git operations scoped to a workspace.
type Snapshotter struct {
	Workspace string
}

// NewSnapshotter returns a git snapshotter for the workspace.
func NewSnapshotter(workspace string) *Snapshotter {
	return &Snapshotter{Workspace: workspace}
}

func (s *Snapshotter) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204
	cmd.Dir = s.Workspace
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, errb.String())
	}
	return strings.TrimSpace(out.String()), nil
}

// IsRepo reports whether the workspace is a git work tree.
func (s *Snapshotter) IsRepo(ctx context.Context) bool {
	out, err := s.git(ctx, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// Baseline returns the current HEAD commit hash (the revert target).
func (s *Snapshotter) Baseline(ctx context.Context) (string, error) {
	return s.git(ctx, "rev-parse", "HEAD")
}

// Keep stages all changes and commits them with the experiment message.
// Returns the new commit hash. If there is nothing to commit, returns the
// baseline unchanged.
func (s *Snapshotter) Keep(ctx context.Context, message string) (string, error) {
	if _, err := s.git(ctx, "add", "-A"); err != nil {
		return "", err
	}
	status, err := s.git(ctx, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if status == "" {
		return s.Baseline(ctx)
	}
	if _, err := s.git(ctx,
		"-c", "user.name=sin-code-autopilot",
		"-c", "user.email=autopilot@sin-code.local",
		"commit", "-m", message, "--no-verify"); err != nil {
		return "", err
	}
	return s.Baseline(ctx)
}

// Revert discards all working-tree changes and resets hard to baseline.
func (s *Snapshotter) Revert(ctx context.Context, baseline string) error {
	if _, err := s.git(ctx, "reset", "--hard", baseline); err != nil {
		return err
	}
	// Remove untracked files/dirs the experiment may have created.
	_, err := s.git(ctx, "clean", "-fd")
	return err
}
