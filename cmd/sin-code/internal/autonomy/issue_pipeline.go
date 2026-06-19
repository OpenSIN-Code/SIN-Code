// SPDX-License-Identifier: MIT
// Purpose: IssuePipeline — automated issue-to-PR pipeline (issue #391).
// Orchestrates the full flow: fetch issue → create branch → implement →
// test → push → create PR. Each step is recorded as a PipelineStep so
// the caller can inspect where the pipeline failed. The gh/git command
// surface is abstracted behind GHRunner so tests inject a mock without
// spawning real processes (M2: no CGO; M7: no shared mutable state).
package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GHRunner executes a command (gh or git) and returns captured output.
// Production code uses ExecGHRunner (real os/exec); tests inject a mock.
type GHRunner interface {
	Run(args ...string) (string, error)
}

// ExecGHRunner shells out to real binaries via os/exec. It is stateless
// and safe to share across goroutines.
type ExecGHRunner struct {
	Workdir string
}

// Run executes the command given by args (args[0] is the program name)
// in the runner's Workdir and returns combined stdout+stderr.
func (r *ExecGHRunner) Run(args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("issue_pipeline: no command given")
	}
	cmd := exec.Command(args[0], args[1:]...)
	if r.Workdir != "" {
		cmd.Dir = r.Workdir
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// PipelineStep records the outcome of one stage of the pipeline.
type PipelineStep struct {
	Name   string
	Status string
	Error  string
}

// PipelineResult is the output of Process. Steps are ordered by execution.
// Success is true only when every step completed and a PR was created.
type PipelineResult struct {
	IssueNumber int
	Steps       []PipelineStep
	PRURL       string
	PRNumber    int
	Success     bool
}

// IssueInfo holds the parsed representation of a GitHub issue.
type IssueInfo struct {
	Number int
	Title  string
	Body   string
	Labels []string
}

// IssuePipeline orchestrates the issue-to-PR flow. It is stateless aside
// from the injected runner and workdir and is safe for concurrent use
// provided the runner itself is (M7).
type IssuePipeline struct {
	ghRunner GHRunner
	workdir  string
}

// NewIssuePipeline creates an IssuePipeline with the given runner and
// working directory.
func NewIssuePipeline(gh GHRunner, workdir string) *IssuePipeline {
	return &IssuePipeline{ghRunner: gh, workdir: workdir}
}

const (
	stepFetch   = "fetch-issue"
	stepBranch  = "create-branch"
	stepImpl    = "implement"
	stepTest    = "test"
	stepPush    = "push"
	stepPR      = "create-pr"
	statusOK    = "ok"
	statusFail  = "fail"
	statusSkip  = "skip"
)

// Process runs the full pipeline for the given issue number. The
// "implement" and "test" steps are placeholders that the daemon wires to
// the agent loop; in this package they are recorded as skipped so the
// pipeline structure is visible. Steps after a failure are not executed.
func (p *IssuePipeline) Process(ctx context.Context, issueNumber int) (*PipelineResult, error) {
	if issueNumber <= 0 {
		return nil, fmt.Errorf("issue_pipeline: issue number must be positive")
	}
	if p.ghRunner == nil {
		return nil, fmt.Errorf("issue_pipeline: nil GHRunner")
	}

	result := &PipelineResult{IssueNumber: issueNumber}

	var issue *IssueInfo
	if _, err := p.recordStep(result, stepFetch, func() error {
		info, err := p.FetchIssue(issueNumber)
		if err != nil {
			return err
		}
		issue = info
		return nil
	}); err != nil {
		return result, err
	}

	var branch string
	if _, err := p.recordStep(result, stepBranch, func() error {
		b, err := p.CreateBranch(issueNumber)
		if err != nil {
			return err
		}
		branch = b
		return nil
	}); err != nil {
		return result, err
	}

	// implement — placeholder (daemon wires agent loop here)
	result.Steps = append(result.Steps, PipelineStep{Name: stepImpl, Status: statusSkip})

	// test — placeholder (daemon wires verify gate here)
	result.Steps = append(result.Steps, PipelineStep{Name: stepTest, Status: statusSkip})

	if _, err := p.recordStep(result, stepPush, func() error {
		_, err := p.ghRunner.Run("git", "push", "-u", "origin", branch)
		return err
	}); err != nil {
		return result, err
	}

	if _, err := p.recordStep(result, stepPR, func() error {
		url, err := p.CreatePR(branch, issueNumber, issue.Title)
		if err != nil {
			return err
		}
		result.PRURL = url
		return nil
	}); err != nil {
		return result, err
	}

	result.Success = true
	return result, nil
}

// recordStep runs fn, appends a PipelineStep to result, and returns the
// error from fn so the caller can short-circuit.
func (p *IssuePipeline) recordStep(result *PipelineResult, name string, fn func() error) (string, error) {
	if err := fn(); err != nil {
		result.Steps = append(result.Steps, PipelineStep{Name: name, Status: statusFail, Error: err.Error()})
		return "", err
	}
	result.Steps = append(result.Steps, PipelineStep{Name: name, Status: statusOK})
	return "", nil
}

// ghIssueJSON is the subset of `gh issue view --json` output we parse.
type ghIssueJSON struct {
	Number int      `json:"number"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
}

// FetchIssue retrieves issue metadata via `gh issue view <number> --json`.
func (p *IssuePipeline) FetchIssue(number int) (*IssueInfo, error) {
	if number <= 0 {
		return nil, fmt.Errorf("issue_pipeline: issue number must be positive")
	}
	out, err := p.ghRunner.Run("gh", "issue", "view", fmt.Sprint(number), "--json", "number,title,body,labels")
	if err != nil {
		return nil, fmt.Errorf("issue_pipeline: fetch issue %d: %s: %w", number, strings.TrimSpace(out), err)
	}

	var raw ghIssueJSON
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("issue_pipeline: parse issue %d: %w", number, err)
	}

	return &IssueInfo{
		Number: raw.Number,
		Title:  raw.Title,
		Body:   raw.Body,
		Labels: raw.Labels,
	}, nil
}

// CreateBranch creates and checks out a branch named "fix-issue-<number>"
// via git. Returns the branch name.
func (p *IssuePipeline) CreateBranch(issueNumber int) (string, error) {
	if issueNumber <= 0 {
		return "", fmt.Errorf("issue_pipeline: issue number must be positive")
	}
	branch := fmt.Sprintf("fix-issue-%d", issueNumber)
	if _, err := p.ghRunner.Run("git", "checkout", "-b", branch); err != nil {
		return "", fmt.Errorf("issue_pipeline: create branch %s: %w", branch, err)
	}
	return branch, nil
}

// CreatePR opens a pull request via `gh pr create` referencing the issue.
// Returns the PR URL.
func (p *IssuePipeline) CreatePR(branch string, issueNumber int, title string) (string, error) {
	if branch == "" {
		return "", fmt.Errorf("issue_pipeline: branch required")
	}
	if issueNumber <= 0 {
		return "", fmt.Errorf("issue_pipeline: issue number must be positive")
	}
	if title == "" {
		title = fmt.Sprintf("Fix #%d", issueNumber)
	}
	body := fmt.Sprintf("Closes #%d", issueNumber)

	out, err := p.ghRunner.Run("gh", "pr", "create",
		"--title", title,
		"--body", body,
		"--head", branch,
	)
	if err != nil {
		return "", fmt.Errorf("issue_pipeline: create PR: %s: %w", strings.TrimSpace(out), err)
	}
	return strings.TrimSpace(out), nil
}
