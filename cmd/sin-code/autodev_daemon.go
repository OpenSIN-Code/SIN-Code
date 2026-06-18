// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
)

func autoCreatePR(ctx context.Context, goal *autonomy.Goal, res *agentloop.Result) error {
	branch := fmt.Sprintf("goal-%d", goal.ID)
	diffOut, err := exec.CommandContext(ctx, "git", "-C", goal.Workspace, "diff", "--stat", "HEAD").CombinedOutput()
	if err != nil || strings.TrimSpace(string(diffOut)) == "" { fmt.Printf("daemon: goal %d no changes\n", goal.ID); return nil }
	statusOut, _ := exec.CommandContext(ctx, "git", "-C", goal.Workspace, "status", "--porcelain").CombinedOutput()
	if strings.TrimSpace(string(statusOut)) != "" {
		exec.CommandContext(ctx, "git", "-C", goal.Workspace, "add", "-A").Run()
		exec.CommandContext(ctx, "git", "-C", goal.Workspace, "commit", "-m", fmt.Sprintf("feat: resolve goal %d", goal.ID)).Run()
	}
	title := fmt.Sprintf("[Autodev] Goal #%d: %s", goal.ID, autodevTruncate(goal.Prompt, 60))
	body := fmt.Sprintf("## Automated PR\n**Goal:** %d\n**Verified:** %v\n**Turns:** %d\n\n%s", goal.ID, res.Verified, res.Turns, res.Summary)
	prOut, err := exec.CommandContext(ctx, "gh", "pr", "create", "--title", title, "--body", body, "--head", branch, "--base", "main").CombinedOutput()
	if err != nil { return fmt.Errorf("gh pr create: %v: %s", err, string(prOut)) }
	fmt.Printf("daemon: goal %d PR created: %s\n", goal.ID, strings.TrimSpace(string(prOut)))
	return nil
}

func autodevTruncate(s string, maxLen int) string {
	if len(s) <= maxLen { return s }
	return s[:maxLen-3] + "..."
}
