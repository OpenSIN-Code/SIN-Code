// SPDX-License-Identifier: MIT
// Purpose: Speculative Best-of-N — N agents in parallel, verifier picks the winner.
// Each candidate runs in its own worktree; losers are destroyed post-merge.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Candidate struct {
	ID       string
	Agent    Agent
	Worktree string
	Output   string
	Verdict  *Verdict
	Err      error
}

type SpeculativeRunner struct {
	RepoRoot    string
	Checks      []Check
	MaxParallel int
	KeepLosers  bool
	WorkdirBase string // optional override (defaults to os.TempDir()/sin-spec)
}

func NewSpeculativeRunner(repoRoot string, checks []Check) *SpeculativeRunner {
	return &SpeculativeRunner{
		RepoRoot:    repoRoot,
		Checks:      checks,
		MaxParallel: 3,
	}
}

type SpecResult struct {
	Winner     *Candidate
	Candidates []*Candidate
}

func (s *SpeculativeRunner) Run(ctx context.Context, task *Task, agents []Agent, scratch *Scratchpad) (*SpecResult, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("speculative: no agents")
	}

	sem := make(chan struct{}, maxInt(1, s.MaxParallel))
	var wg sync.WaitGroup
	candidates := make([]*Candidate, len(agents))

	for i, ag := range agents {
		i, ag := i, ag
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			candidates[i] = s.runCandidate(ctx, task, ag, i, scratch)
		}()
	}
	wg.Wait()

	verdicts := make([]*Verdict, 0, len(candidates))
	for _, c := range candidates {
		if c != nil && c.Verdict != nil {
			verdicts = append(verdicts, c.Verdict)
		}
	}
	best := BestVerdict(verdicts)

	result := &SpecResult{Candidates: candidates}
	for _, c := range candidates {
		if c != nil && best != nil && c.Verdict == best {
			result.Winner = c
			break
		}
	}

	if !s.KeepLosers {
		for _, c := range candidates {
			if c == nil || c == result.Winner {
				continue
			}
			s.removeWorktree(c.Worktree)
		}
	}
	return result, nil
}

func (s *SpeculativeRunner) runCandidate(ctx context.Context, task *Task, ag Agent, idx int, scratch *Scratchpad) *Candidate {
	id := fmt.Sprintf("%s-cand%d-%s", task.ID, idx, ag.Name())
	c := &Candidate{ID: id, Agent: ag}

	wt, err := s.addWorktree(ctx, id)
	if err != nil {
		c.Err = fmt.Errorf("worktree: %w", err)
		c.Verdict = &Verdict{TaskID: task.ID, Candidate: id, Passed: false, CreatedAt: timeNow()}
		return c
	}
	c.Worktree = wt

	localTask := *task
	out, err := ag.Run(ctx, &localTask, scratch)
	c.Output, c.Err = out, err

	vf := NewVerifier(wt)
	c.Verdict = vf.Verify(ctx, task.ID, id, s.Checks)
	return c
}

func (s *SpeculativeRunner) worktreeDir(id string) string {
	base := s.WorkdirBase
	if base == "" {
		base = filepath.Join(os.TempDir(), "sin-spec")
	}
	return filepath.Join(base, id)
}

func (s *SpeculativeRunner) addWorktree(ctx context.Context, id string) (string, error) {
	dir := s.worktreeDir(id)
	if err := os.MkdirAll(filepath.Dir(dir), 0o750); err != nil {
		return "", err
	}
	if s.RepoRoot == "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return "", err
		}
		return dir, nil
	}
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "--detach", dir)
	cmd.Dir = s.RepoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %s: %w", string(out), err)
	}
	return dir, nil
}

func (s *SpeculativeRunner) removeWorktree(dir string) {
	if dir == "" {
		return
	}
	if s.RepoRoot != "" {
		cmd := exec.Command("git", "worktree", "remove", "--force", dir)
		cmd.Dir = s.RepoRoot
		_ = cmd.Run()
	}
	_ = os.RemoveAll(dir)
}

func (s *SpeculativeRunner) MergeWinner(ctx context.Context, winner *Candidate) (string, error) {
	if winner == nil || winner.Worktree == "" {
		return "", fmt.Errorf("speculative: no winner to merge")
	}
	if s.RepoRoot == "" {
		return "", fmt.Errorf("speculative: RepoRoot unset; cannot diff against base")
	}
	diffCmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	diffCmd.Dir = winner.Worktree
	diff, err := diffCmd.Output()
	if err != nil {
		return "", fmt.Errorf("diff winner: %w", err)
	}
	if len(diff) == 0 {
		return "", nil
	}
	apply := exec.CommandContext(ctx, "git", "apply", "--3way", "-")
	apply.Dir = s.RepoRoot
	apply.Stdin = strings.NewReader(string(diff))
	if out, err := apply.CombinedOutput(); err != nil {
		return "", fmt.Errorf("apply winner diff: %s: %w", string(out), err)
	}
	if !s.KeepLosers {
		s.removeWorktree(winner.Worktree)
	}
	return string(diff), nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
