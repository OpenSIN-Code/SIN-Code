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
// default implementation hashes `git diff` numstat output; an unchanged hash
// across consecutive turns is treated as a true stall regardless of how the
// stop-gate phrases its criteria. A nil probe disables diff-based stall
// detection (legacy behavior).
type ProgressProbe func(ctx context.Context, workspace string) ProgressSignal

// ProgressSignal captures a cheap, comparable snapshot of workspace progress.
// A zero value (empty DiffHash) means "unknown" and never triggers a stall.
type ProgressSignal struct {
	DiffHash     string // hash of the diff content; identical => no new edits
	LinesChanged int    // additions+deletions in the working tree
}

// GitProgressProbe is the default probe. It is best-effort: if git is missing
// or the workspace is not a repo, it returns a zero signal (which disables
// diff-based stall detection gracefully rather than aborting the run).
func GitProgressProbe(ctx context.Context, workspace string) ProgressSignal {
	// `git diff --numstat HEAD` covers tracked changes (staged + unstaged)
	// relative to the last commit; cheap and stable across turns.
	out, err := runGit(ctx, workspace, "diff", "--numstat", "HEAD")
	if err != nil {
		return ProgressSignal{}
	}
	lines := 0
	for _, ln := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(ln)
		if len(fields) >= 2 {
			// numstat reports "-" for binary files; Atoi fails -> counts as 0.
			add, _ := strconv.Atoi(fields[0])
			del, _ := strconv.Atoi(fields[1])
			lines += add + del
		}
	}
	sum := sha256.Sum256([]byte(out))
	return ProgressSignal{DiffHash: hex.EncodeToString(sum[:]), LinesChanged: lines}
}

// runGit runs a git subcommand in dir and returns its stdout.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	b, err := cmd.Output()
	return string(b), err
}
