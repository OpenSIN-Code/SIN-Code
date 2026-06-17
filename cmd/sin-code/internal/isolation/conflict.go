// SPDX-License-Identifier: MIT
// Purpose: conflict prediction for git worktree isolation using
// `git merge-tree` (issue #319). This lets callers abort or warn before
// spending time on a worktree that cannot cleanly merge with a target
// integration branch.
package isolation

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ConflictReport is the result of predicting a merge between two refs.
type ConflictReport struct {
	Target        string
	Source        string
	Clean         bool
	Tree          string
	ConflictPaths []string
	Messages      []string
}

// treeHashRe matches the 40-character SHA-1 object ids Git writes for trees.
// Accepting 40-64 hex chars future-proofs the parser against SHA-256 repos.
var treeHashRe = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

// PredictConflicts runs `git merge-tree --write-tree --name-only --messages`
// between `target` and `source` in the repository rooted at `repoRoot`. It
// returns a ConflictReport that is Clean=true when the two refs can merge
// without path conflicts, and Clean=false with ConflictPaths populated when
// conflicts are detected. Non-conflict git failures (unknown refs, broken
// repository, etc.) are returned as errors.
func PredictConflicts(repoRootDir, target, source string) (ConflictReport, error) {
	if target == "" || source == "" {
		return ConflictReport{}, fmt.Errorf("isolation: target and source refs required")
	}
	root, err := repoRoot(repoRootDir)
	if err != nil {
		return ConflictReport{}, err
	}

	cmd := exec.Command("git", "merge-tree", "--write-tree", "--name-only", "--messages", target, source)
	cmd.Dir = root
	out, err := cmd.Output()
	text := strings.TrimRight(string(out), "\n")

	report, ok := parseMergeTreeOutput(target, source, text)
	if err != nil {
		// If git exited non-zero but produced a valid conflict report, the
		// non-zero status is Git's way of signalling conflicts rather than
		// a command error. In that case we return the report.
		if ok && !report.Clean {
			return report, nil
		}
		var errMsg string
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			errMsg = strings.TrimSpace(string(exitErr.Stderr))
		} else {
			errMsg = strings.TrimSpace(text)
		}
		return ConflictReport{}, fmt.Errorf("isolation: git merge-tree: %s: %w", errMsg, err)
	}
	if !ok {
		// A zero exit status with unparseable output is unexpected; treat it
		// as an error so callers never silently assume a clean merge.
		return ConflictReport{}, fmt.Errorf("isolation: git merge-tree produced unexpected output: %q", text)
	}
	return report, nil
}

// PredictWorktreeConflicts predicts the merge result between the configured
// target branch and the worktree for `worktreeName`. If a branch named
// `worktree-<worktreeName>` already exists, it is used as the source; otherwise
// the current HEAD is used as the source.
func PredictWorktreeConflicts(repoRootDir, worktreeName, target string) (ConflictReport, error) {
	if worktreeName == "" {
		return ConflictReport{}, fmt.Errorf("isolation: worktree name required")
	}
	if target == "" {
		return ConflictReport{}, fmt.Errorf("isolation: target branch required")
	}
	root, err := repoRoot(repoRootDir)
	if err != nil {
		return ConflictReport{}, err
	}

	source := "worktree-" + worktreeName
	if !branchExists(root, source) {
		source = "HEAD"
	}
	return PredictConflicts(root, target, source)
}

// branchExists reports whether a local branch named `name` exists in the
// repo rooted at `root`.
func branchExists(root, name string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	cmd.Dir = root
	return cmd.Run() == nil
}

// parseMergeTreeOutput parses the stdout of `git merge-tree --write-tree
// --name-only --messages`. It returns (report, true) when the first line is a
// valid tree hash. For a clean merge the output is a single tree-hash line; for
// a conflict it is a tree hash, the conflicted paths (one per line), a blank
// line, and then the human-readable messages.
func parseMergeTreeOutput(target, source, text string) (ConflictReport, bool) {
	if text == "" {
		return ConflictReport{}, false
	}
	lines := strings.Split(text, "\n")
	if !treeHashRe.MatchString(lines[0]) {
		return ConflictReport{}, false
	}

	report := ConflictReport{
		Target: target,
		Source: source,
		Tree:   lines[0],
	}
	if len(lines) == 1 {
		report.Clean = true
		return report, true
	}

	// The first blank line separates the conflicted path list from the
	// informational messages added by --messages.
	boundary := len(lines)
	for i := 1; i < len(lines); i++ {
		if lines[i] == "" {
			boundary = i
			break
		}
	}
	report.ConflictPaths = lines[1:boundary]
	if boundary < len(lines) {
		report.Messages = lines[boundary+1:]
	}
	report.Clean = len(report.ConflictPaths) == 0
	return report, true
}
