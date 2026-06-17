// SPDX-License-Identifier: MIT
// Purpose: ConflictPredictor predicts merge conflicts before they happen
// (issue #319). It wraps `git merge-tree --write-tree --name-only` via
// os/exec to detect conflicts between a branch and main without performing
// an actual merge, and falls back to overlap detection on the diff name-only
// output when merge-tree is unavailable. All git commands are routed through
// hookable function fields so tests can mock git without a real repository
// (mirroring the WorktreeManager pattern in worktree_mgr.go).
//
// M7 invariant: the only shared mutable field is `mu`, which serializes
// access to the git runners. The runners themselves are set once at
// construction time and never reassigned after NewConflictPredictor.
package autonomy

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
)

// ConflictType classifies how a file came to be flagged by the predictor.
type ConflictType string

const (
	// ConflictMergeTree means git merge-tree reported the file as
	// conflicted during a virtual merge.
	ConflictMergeTree ConflictType = "merge-tree"
	// ConflictOverlap means the fallback overlap heuristic flagged the
	// file because it was modified on both branch and main since their
	// merge base.
	ConflictOverlap ConflictType = "overlap"
)

// SeverityLevel is the coarse risk band assigned to a conflict count.
type SeverityLevel string

const (
	SeverityNone   SeverityLevel = "none"
	SeverityLow    SeverityLevel = "low"
	SeverityMedium SeverityLevel = "medium"
	SeverityHigh   SeverityLevel = "high"
)

// ConflictPrediction describes a single file that is likely to conflict
// when the branch is merged into main.
type ConflictPrediction struct {
	File         string        `json:"file"`
	BranchLines  int           `json:"branch_lines"`
	MainLines    int           `json:"main_lines"`
	ConflictType ConflictType  `json:"conflict_type"`
	Severity     SeverityLevel `json:"severity"`
}

// mergeTreeResult is the raw outcome of a `git merge-tree` invocation.
// exitCode is 0 for a clean merge, 1 for a conflicted merge, and any
// other value (with err set) for a failure such as an unsupported flag.
type mergeTreeResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

// ConflictPredictor predicts merge conflicts between a branch and main
// using git merge-tree, with a diff-based overlap fallback. It is safe
// for concurrent use (M7).
type ConflictPredictor struct {
	root string
	mu   sync.Mutex

	// gitMergeTree runs `git merge-tree --write-tree --name-only <base>
	// <branch>`. Tests override this to mock git.
	gitMergeTree func(root, base, branch string) mergeTreeResult
	// gitDiffNameOnly runs `git diff --name-only <spec>` and returns the
	// trimmed output (one file path per line).
	gitDiffNameOnly func(root, spec string) (string, error)
}

// cpGitRunner is the default command runner that shells out to git.
type cpGitRunner struct{}

func (cpGitRunner) mergeTree(root, base, branch string) mergeTreeResult {
	cmd := exec.Command("git", "merge-tree", "--write-tree", "--name-only", base, branch)
	cmd.Dir = root
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
		err = nil // exit 1 is a legitimate "conflicts present" signal
	}
	return mergeTreeResult{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: exitCode,
		err:      err,
	}
}

func (cpGitRunner) diffNameOnly(root, spec string) (string, error) {
	cmd := exec.Command("git", "diff", "--name-only", spec)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

var defaultCPRunner = cpGitRunner{}

// NewConflictPredictor creates a ConflictPredictor rooted at the given git
// repository path. The root should be the toplevel of the git work tree.
func NewConflictPredictor(root string) *ConflictPredictor {
	return &ConflictPredictor{
		root:            root,
		gitMergeTree:    defaultCPRunner.mergeTree,
		gitDiffNameOnly: defaultCPRunner.diffNameOnly,
	}
}

// Predict runs git merge-tree between main (base) and the given branch and
// returns one ConflictPrediction per conflicting file. When merge-tree is
// unavailable (command fails with a non-conflict error), it falls back to
// OverlappingFiles and reports each overlap as a ConflictOverlap prediction.
// An empty branch is rejected.
func (p *ConflictPredictor) Predict(branch string) ([]ConflictPrediction, error) {
	if branch == "" {
		return nil, fmt.Errorf("conflict_predict: branch name required")
	}

	p.mu.Lock()
	res := p.gitMergeTree(p.root, "main", branch)
	p.mu.Unlock()

	// merge-tree succeeded with no conflicts.
	if res.err == nil && res.exitCode == 0 {
		return nil, nil
	}

	// exitCode == 1 means conflicts were detected; parse them.
	if res.err == nil && res.exitCode == 1 {
		files := parseMergeTreeConflicts(res.stdout)
		preds := make([]ConflictPrediction, 0, len(files))
		sev := p.Severity(len(files))
		for _, f := range files {
			preds = append(preds, ConflictPrediction{
				File:         f,
				ConflictType: ConflictMergeTree,
				Severity:     SeverityLevel(sev),
			})
		}
		return preds, nil
	}

	// Any other error (unsupported flag, missing repo, etc.) -> fallback.
	overlaps, oerr := p.OverlappingFiles(branch)
	if oerr != nil {
		return nil, fmt.Errorf("conflict_predict: merge-tree (%v) and overlap fallback (%w) both failed", res.err, oerr)
	}
	sev := p.Severity(len(overlaps))
	preds := make([]ConflictPrediction, 0, len(overlaps))
	for _, f := range overlaps {
		preds = append(preds, ConflictPrediction{
			File:         f,
			ConflictType: ConflictOverlap,
			Severity:     SeverityLevel(sev),
		})
	}
	return preds, nil
}

// parseMergeTreeConflicts extracts the conflicted file paths from
// `git merge-tree --write-tree --name-only` output. The first non-empty
// line is the written tree SHA; subsequent non-empty lines are the
// conflicted paths.
func parseMergeTreeConflicts(stdout string) []string {
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	files := make([]string, 0, len(lines))
	skippedTree := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// The first line is always the tree object SHA (40 hex chars).
		if !skippedTree {
			skippedTree = true
			continue
		}
		files = append(files, line)
	}
	return files
}

// Severity maps a conflict count to a coarse risk band. 0 -> none,
// 1-3 -> low, 4-9 -> medium, 10+ -> high.
func (p *ConflictPredictor) Severity(conflicts int) string {
	switch {
	case conflicts <= 0:
		return string(SeverityNone)
	case conflicts <= 3:
		return string(SeverityLow)
	case conflicts <= 9:
		return string(SeverityMedium)
	default:
		return string(SeverityHigh)
	}
}

// SafeToMerge reports whether the branch can be merged into main without
// conflicts. Returns (true, reason) when clean and (false, reason) when
// conflicts are predicted. The reason string summarises the count and
// severity so it can be surfaced in a CLI / TUI.
func (p *ConflictPredictor) SafeToMerge(branch string) (bool, string) {
	preds, err := p.Predict(branch)
	if err != nil {
		return false, fmt.Sprintf("prediction failed: %v", err)
	}
	if len(preds) == 0 {
		return true, "no conflicts detected"
	}
	sev := p.Severity(len(preds))
	files := make([]string, 0, len(preds))
	for _, pr := range preds {
		files = append(files, pr.File)
	}
	return false, fmt.Sprintf("%d conflict(s) [%s]: %s", len(preds), sev, strings.Join(files, ", "))
}

// OverlappingFiles returns the set of files modified on both the branch
// and main since their merge base. It runs `git diff --name-only main...branch`
// (branch-side changes) and `git diff --name-only branch...main` (main-side
// changes) and returns the intersection. This is the fallback path for
// Predict when git merge-tree is unavailable, and is also useful directly
// for a "merge queue" view. An empty branch is rejected.
func (p *ConflictPredictor) OverlappingFiles(branch string) ([]string, error) {
	if branch == "" {
		return nil, fmt.Errorf("conflict_predict: branch name required")
	}

	p.mu.Lock()
	branchOut, err := p.gitDiffNameOnly(p.root, "main..."+branch)
	p.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("conflict_predict: diff main...%s: %w", branch, err)
	}
	branchFiles := parseDiffNameOnly(branchOut)

	p.mu.Lock()
	mainOut, err := p.gitDiffNameOnly(p.root, branch+"...main")
	p.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("conflict_predict: diff %s...main: %w", branch, err)
	}
	mainFiles := parseDiffNameOnly(mainOut)

	mainSet := make(map[string]struct{}, len(mainFiles))
	for _, f := range mainFiles {
		mainSet[f] = struct{}{}
	}

	overlap := make([]string, 0)
	seen := make(map[string]struct{}, len(branchFiles))
	for _, f := range branchFiles {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		if _, ok := mainSet[f]; ok {
			overlap = append(overlap, f)
		}
	}
	sort.Strings(overlap)
	return overlap, nil
}

// parseDiffNameOnly turns `git diff --name-only` output into a deduplicated,
// trimmed slice of file paths. Empty lines are skipped.
func parseDiffNameOnly(out string) []string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	files := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		files = append(files, line)
	}
	return files
}
