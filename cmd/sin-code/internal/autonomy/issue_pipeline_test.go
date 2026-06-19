// SPDX-License-Identifier: MIT
// Purpose: tests for issue_pipeline.go (issue #391). Uses a mock GHRunner
// so no real gh/git binaries are invoked.
package autonomy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type mockGHRunner struct {
	responses map[string]string
	errs      map[string]error
	calls     [][]string
}

func (m *mockGHRunner) Run(args ...string) (string, error) {
	m.calls = append(m.calls, append([]string(nil), args...))
	key := strings.Join(args, " ")
	if err, ok := m.errs[key]; ok {
		return m.responses[key], err
	}
	if resp, ok := m.responses[key]; ok {
		return resp, nil
	}
	return "", nil
}

func makeIssueJSON(num int, title string) string {
	b, _ := json.Marshal(ghIssueJSON{
		Number: num,
		Title:  title,
		Body:   "issue body",
		Labels: []string{"bug"},
	})
	return string(b)
}

func TestIssuePipelineFetchIssue(t *testing.T) {
	gh := &mockGHRunner{
		responses: map[string]string{
			"gh issue view 42 --json number,title,body,labels": makeIssueJSON(42, "Fix the bug"),
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")

	info, err := p.FetchIssue(42)
	if err != nil {
		t.Fatalf("FetchIssue: %v", err)
	}
	if info.Number != 42 {
		t.Errorf("expected number 42, got %d", info.Number)
	}
	if info.Title != "Fix the bug" {
		t.Errorf("expected title 'Fix the bug', got %q", info.Title)
	}
	if len(info.Labels) != 1 || info.Labels[0] != "bug" {
		t.Errorf("unexpected labels: %v", info.Labels)
	}
}

func TestIssuePipelineFetchIssueError(t *testing.T) {
	gh := &mockGHRunner{
		errs: map[string]error{
			"gh issue view 99 --json number,title,body,labels": fmt.Errorf("not found"),
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")

	_, err := p.FetchIssue(99)
	if err == nil {
		t.Fatal("expected error for failed fetch")
	}
	if !strings.Contains(err.Error(), "fetch issue") {
		t.Errorf("expected 'fetch issue' in error, got %v", err)
	}
}

func TestIssuePipelineCreateBranch(t *testing.T) {
	gh := &mockGHRunner{}
	p := NewIssuePipeline(gh, "/fake/workdir")

	branch, err := p.CreateBranch(7)
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if branch != "fix-issue-7" {
		t.Errorf("expected branch fix-issue-7, got %s", branch)
	}
	if len(gh.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(gh.calls))
	}
	expected := []string{"git", "checkout", "-b", "fix-issue-7"}
	if fmt.Sprint(gh.calls[0]) != fmt.Sprint(expected) {
		t.Errorf("expected %v, got %v", expected, gh.calls[0])
	}
}

func TestIssuePipelineCreatePR(t *testing.T) {
	gh := &mockGHRunner{
		responses: map[string]string{
			"gh pr create --title Fix the bug --body Closes #42 --head fix-issue-42": "https://github.com/org/repo/pull/99",
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")

	url, err := p.CreatePR("fix-issue-42", 42, "Fix the bug")
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if url != "https://github.com/org/repo/pull/99" {
		t.Errorf("unexpected PR URL: %q", url)
	}
}

func TestIssuePipelineCreatePRDefaultTitle(t *testing.T) {
	gh := &mockGHRunner{
		responses: map[string]string{
			"gh pr create --title Fix #10 --body Closes #10 --head fix-issue-10": "https://github.com/org/repo/pull/5",
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")

	url, err := p.CreatePR("fix-issue-10", 10, "")
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if url == "" {
		t.Error("expected non-empty PR URL")
	}
}

func TestIssuePipelineProcessSuccess(t *testing.T) {
	gh := &mockGHRunner{
		responses: map[string]string{
			"gh issue view 1 --json number,title,body,labels":                     makeIssueJSON(1, "Test issue"),
			"git checkout -b fix-issue-1":                                         "",
			"git push -u origin fix-issue-1":                                      "",
			"gh pr create --title Test issue --body Closes #1 --head fix-issue-1": "https://github.com/org/repo/pull/1",
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")

	result, err := p.Process(context.Background(), 1)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.PRURL != "https://github.com/org/repo/pull/1" {
		t.Errorf("unexpected PR URL: %q", result.PRURL)
	}
	expectedSteps := []string{stepFetch, stepBranch, stepImpl, stepTest, stepPush, stepPR}
	if len(result.Steps) != len(expectedSteps) {
		t.Fatalf("expected %d steps, got %d: %+v", len(expectedSteps), len(result.Steps), result.Steps)
	}
	for i, name := range expectedSteps {
		if result.Steps[i].Name != name {
			t.Errorf("step %d: expected %s, got %s", i, name, result.Steps[i].Name)
		}
	}
	if result.Steps[0].Status != statusOK {
		t.Errorf("fetch step should be ok, got %s", result.Steps[0].Status)
	}
	if result.Steps[2].Status != statusSkip {
		t.Errorf("implement step should be skip, got %s", result.Steps[2].Status)
	}
}

func TestIssuePipelineProcessBranchFailure(t *testing.T) {
	gh := &mockGHRunner{
		responses: map[string]string{
			"gh issue view 2 --json number,title,body,labels": makeIssueJSON(2, "Branch fail"),
			"git checkout -b fix-issue-2":                     "branch exists",
		},
		errs: map[string]error{
			"git checkout -b fix-issue-2": fmt.Errorf("branch already exists"),
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")

	result, err := p.Process(context.Background(), 2)
	if err == nil {
		t.Fatal("expected error for branch failure")
	}
	if result.Success {
		t.Error("expected Success=false")
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps (fetch ok + branch fail), got %d", len(result.Steps))
	}
	if result.Steps[1].Status != statusFail {
		t.Errorf("expected branch step to fail, got %s", result.Steps[1].Status)
	}
	if !strings.Contains(result.Steps[1].Error, "branch already exists") {
		t.Errorf("expected error message in step, got %q", result.Steps[1].Error)
	}
}

func TestIssuePipelineProcessInvalidIssue(t *testing.T) {
	gh := &mockGHRunner{}
	p := NewIssuePipeline(gh, "/fake/workdir")

	_, err := p.Process(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error for issue number 0")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Errorf("expected 'positive' in error, got %v", err)
	}
}

func TestExecGHRunnerNoArgs(t *testing.T) {
	r := &ExecGHRunner{Workdir: "/tmp"}
	_, err := r.Run()
	if err == nil {
		t.Fatal("expected error for no args")
	}
}
