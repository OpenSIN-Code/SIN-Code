// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when tools are MCP-externalized
// Purpose: git tool implementation — runGit executor and the runGitFn hook
// variable used by coverage tests. Specs and dispatch remain in
// chat_tools_extra.go.
package main

import (
	"context"
	"fmt"
	"os/exec"
)

// runGitFn is injected by coverage tests to mock git subprocess calls.
var runGitFn = runGit

// runGit executes a git command with a bounded timeout and returns the
// combined output. Errors are returned as part of the text (not as Go
// errors) so the agent loop sees them as tool output rather than a hard
// failure.
func runGit(ctx context.Context, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	if err != nil {
		return fmt.Sprintf("git error: %v\n%s", err, text), nil
	}
	return text, nil
}
