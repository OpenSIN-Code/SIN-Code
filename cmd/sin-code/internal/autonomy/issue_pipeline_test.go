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

func TestExecGHRunnerRunEcho(t *testing.T) {
	r := &ExecGHRunner{Workdir: ""}
	out, err := r.Run("echo", "hello")
	if err != nil {
		t.Fatalf("Run echo: %v", err)
	}
	if out != "hello" {
		t.Errorf("expected 'hello', got %q", out)
	}
}

func TestExecGHRunnerRunError(t *testing.T) {
	r := &ExecGHRunner{Workdir: ""}
	_, err := r.Run("false")
	if err == nil {
		t.Fatal("expected error from 'false' command")
	}
}

func TestExecGHRunnerRunWithWorkdir(t *testing.T) {
	dir := t.TempDir()
	r := &ExecGHRunner{Workdir: dir}
	out, err := r.Run("pwd")
	if err != nil {
		t.Fatalf("Run pwd: %v", err)
	}
	if !strings.Contains(out, dir) {
		t.Errorf("expected pwd to contain %q, got %q", dir, out)
	}
}

func TestIssuePipelineProcessNilRunner(t *testing.T) {
	p := NewIssuePipeline(nil, "/fake/workdir")
	_, err := p.Process(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for nil runner")
	}
	if !strings.Contains(err.Error(), "nil GHRunner") {
		t.Errorf("expected 'nil GHRunner' in error, got %v", err)
	}
}

func TestIssuePipelineProcessFetchFailure(t *testing.T) {
	gh := &mockGHRunner{
		errs: map[string]error{
			"gh issue view 5 --json number,title,body,labels": fmt.Errorf("network error"),
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")
	result, err := p.Process(context.Background(), 5)
	if err == nil {
		t.Fatal("expected error for fetch failure")
	}
	if result.Success {
		t.Error("expected Success=false")
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step (fetch fail), got %d", len(result.Steps))
	}
	if result.Steps[0].Status != statusFail {
		t.Errorf("expected fetch step to fail, got %s", result.Steps[0].Status)
	}
}

func TestIssuePipelineProcessPushFailure(t *testing.T) {
	gh := &mockGHRunner{
		responses: map[string]string{
			"gh issue view 3 --json number,title,body,labels": makeIssueJSON(3, "Test"),
			"git checkout -b fix-issue-3":                     "",
		},
		errs: map[string]error{
			"git push -u origin fix-issue-3": fmt.Errorf("push rejected"),
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")
	result, err := p.Process(context.Background(), 3)
	if err == nil {
		t.Fatal("expected error for push failure")
	}
	if result.Success {
		t.Error("expected Success=false")
	}
	// fetch(ok) + branch(ok) + implement(skip) + test(skip) + push(fail) = 5 steps
	if len(result.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d: %+v", len(result.Steps), result.Steps)
	}
	if result.Steps[4].Status != statusFail {
		t.Errorf("expected push step to fail, got %s", result.Steps[4].Status)
	}
	if !strings.Contains(result.Steps[4].Error, "push rejected") {
		t.Errorf("expected 'push rejected' in error, got %q", result.Steps[4].Error)
	}
}

func TestIssuePipelineProcessPRFailure(t *testing.T) {
	gh := &mockGHRunner{
		responses: map[string]string{
			"gh issue view 4 --json number,title,body,labels": makeIssueJSON(4, "PR fail"),
			"git checkout -b fix-issue-4":                     "",
			"git push -u origin fix-issue-4":                  "",
		},
		errs: map[string]error{
			"gh pr create --title PR fail --body Closes #4 --head fix-issue-4": fmt.Errorf("pr creation failed"),
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")
	result, err := p.Process(context.Background(), 4)
	if err == nil {
		t.Fatal("expected error for PR failure")
	}
	if result.Success {
		t.Error("expected Success=false")
	}
	// 6 steps, last one is PR failure
	if len(result.Steps) != 6 {
		t.Fatalf("expected 6 steps, got %d: %+v", len(result.Steps), result.Steps)
	}
	if result.Steps[5].Status != statusFail {
		t.Errorf("expected PR step to fail, got %s", result.Steps[5].Status)
	}
}

func TestIssuePipelineCreatePREmptyBranch(t *testing.T) {
	gh := &mockGHRunner{}
	p := NewIssuePipeline(gh, "/fake/workdir")
	_, err := p.CreatePR("", 1, "title")
	if err == nil {
		t.Fatal("expected error for empty branch")
	}
	if !strings.Contains(err.Error(), "branch required") {
		t.Errorf("expected 'branch required', got %v", err)
	}
}

func TestIssuePipelineCreatePRInvalidIssue(t *testing.T) {
	gh := &mockGHRunner{}
	p := NewIssuePipeline(gh, "/fake/workdir")
	_, err := p.CreatePR("fix-issue-0", 0, "title")
	if err == nil {
		t.Fatal("expected error for issue 0")
	}
	if !strings.Contains(err.Error(), "positive") {
		t.Errorf("expected 'positive' in error, got %v", err)
	}
}

func TestIssuePipelineCreatePRGHError(t *testing.T) {
	gh := &mockGHRunner{
		errs: map[string]error{
			"gh pr create --title T --body Closes #1 --head fix-issue-1": fmt.Errorf("gh error"),
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")
	_, err := p.CreatePR("fix-issue-1", 1, "T")
	if err == nil {
		t.Fatal("expected error from gh")
	}
	if !strings.Contains(err.Error(), "gh error") {
		t.Errorf("expected 'gh error' in message, got %v", err)
	}
}

func TestIssuePipelineFetchIssueInvalidNumber(t *testing.T) {
	gh := &mockGHRunner{}
	p := NewIssuePipeline(gh, "/fake/workdir")
	_, err := p.FetchIssue(0)
	if err == nil {
		t.Fatal("expected error for issue 0")
	}
}

func TestIssuePipelineFetchIssueBadJSON(t *testing.T) {
	gh := &mockGHRunner{
		responses: map[string]string{
			"gh issue view 8 --json number,title,body,labels": "not valid json",
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")
	_, err := p.FetchIssue(8)
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
	if !strings.Contains(err.Error(), "parse issue") {
		t.Errorf("expected 'parse issue' in error, got %v", err)
	}
}

func TestIssuePipelineCreateBranchInvalid(t *testing.T) {
	gh := &mockGHRunner{}
	p := NewIssuePipeline(gh, "/fake/workdir")
	_, err := p.CreateBranch(0)
	if err == nil {
		t.Fatal("expected error for issue 0")
	}
}

func TestIssuePipelineCreateBranchGitError(t *testing.T) {
	gh := &mockGHRunner{
		errs: map[string]error{
			"git checkout -b fix-issue-9": fmt.Errorf("checkout failed"),
		},
	}
	p := NewIssuePipeline(gh, "/fake/workdir")
	_, err := p.CreateBranch(9)
	if err == nil {
		t.Fatal("expected error from git")
	}
	if !strings.Contains(err.Error(), "create branch") {
		t.Errorf("expected 'create branch' in error, got %v", err)
	}
}
