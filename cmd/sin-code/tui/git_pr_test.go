// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

func mockPRRunner(branch string, baseExists bool, logOutput string) GitRunner {
	return func(args ...string) (string, error) {
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
			return branch + "\n", nil
		}
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--verify" {
			if baseExists && (args[2] == "main" || args[2] == "master" || args[2] == "develop") {
				return "abc123\n", nil
			}
			return "", errMock
		}
		if len(args) >= 1 && args[0] == "log" {
			return logOutput, nil
		}
		return "", nil
	}
}

var errMock = &mockError{"mock error"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }

func mockGHRunner(output string) GHRunner {
	return func(args ...string) (string, error) {
		return output, nil
	}
}

const prLogOutput = `a1b2c3d|John Doe|2024-01-15|feat(tui): add git diff panel
b2c3d4e|John Doe|2024-01-14|feat(tui): add commit flow
c3d4e5f|John Doe|2024-01-13|feat(tui): add git log view
`

func TestPRCreateFlowStart(t *testing.T) {
	f := NewPRCreateFlow()
	f.SetRunner(mockPRRunner("feat/tui-git", true, prLogOutput))
	f.SetGHRunner(mockGHRunner("https://github.com/repo/pull/1"))

	if err := f.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !f.Active() {
		t.Error("expected flow to be active")
	}
	info := f.Info()
	if info.Branch != "feat/tui-git" {
		t.Errorf("expected branch 'feat/tui-git', got %q", info.Branch)
	}
	if info.BaseBranch != "main" {
		t.Errorf("expected base 'main', got %q", info.BaseBranch)
	}
	if len(info.Commits) != 3 {
		t.Errorf("expected 3 commits, got %d", len(info.Commits))
	}
}

func TestPRCreateFlowDetectInfo(t *testing.T) {
	f := NewPRCreateFlow()
	f.SetRunner(mockPRRunner("feature/test", true, prLogOutput))

	if err := f.DetectInfo(); err != nil {
		t.Fatalf("DetectInfo failed: %v", err)
	}
	info := f.Info()
	if info.Branch != "feature/test" {
		t.Errorf("expected branch 'feature/test', got %q", info.Branch)
	}
	if info.BaseBranch == "" {
		t.Error("expected non-empty base branch")
	}
}

func TestPRCreateFlowGenerateTitle(t *testing.T) {
	f := NewPRCreateFlow()
	f.SetRunner(mockPRRunner("feat/tui-git", true, prLogOutput))
	_ = f.DetectInfo()

	title := f.GenerateTitle()
	if !strings.Contains(title, "feat") {
		t.Errorf("expected title to contain 'feat', got %q", title)
	}
	if !strings.Contains(title, "git diff") {
		t.Errorf("expected title to contain commit message, got %q", title)
	}
}

func TestPRCreateFlowGenerateBody(t *testing.T) {
	f := NewPRCreateFlow()
	f.SetRunner(mockPRRunner("feat/tui-git", true, prLogOutput))
	_ = f.DetectInfo()

	body := f.GenerateBody()
	if !strings.Contains(body, "feat(tui): add git diff panel") {
		t.Errorf("expected first commit in body, got %q", body)
	}
	if !strings.Contains(body, "feat(tui): add commit flow") {
		t.Errorf("expected second commit in body, got %q", body)
	}
}

func TestPRCreateFlowRender(t *testing.T) {
	f := NewPRCreateFlow()
	f.SetRunner(mockPRRunner("feat/tui-git", true, prLogOutput))
	f.SetGHRunner(mockGHRunner(""))
	_ = f.Start()

	styles := testStyles()
	output := f.Render(styles, 80, 24)
	stripped := stripANSI(output)

	if !strings.Contains(stripped, "Create PR") {
		t.Error("expected 'Create PR' header in render")
	}
	if !strings.Contains(stripped, "Branch:") {
		t.Error("expected 'Branch:' label in render")
	}
	if !strings.Contains(stripped, "Commits:") {
		t.Error("expected 'Commits:' label in render")
	}
	if !strings.Contains(stripped, "Title:") {
		t.Error("expected 'Title:' label in render")
	}
	if !strings.Contains(stripped, "Body:") {
		t.Error("expected 'Body:' label in render")
	}
}

func TestPRCreateFlowExecute(t *testing.T) {
	executed := false
	f := NewPRCreateFlow()
	f.SetRunner(mockPRRunner("feat/tui-git", true, prLogOutput))
	f.SetGHRunner(func(args ...string) (string, error) {
		if len(args) >= 1 && args[0] == "pr" {
			executed = true
			return "https://github.com/repo/pull/1", nil
		}
		return "", nil
	})

	_ = f.Start()
	if err := f.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !executed {
		t.Error("expected gh pr create to be called")
	}
	if f.Active() {
		t.Error("expected flow to be inactive after execute")
	}
}

func TestPRCreateFlowActiveCancel(t *testing.T) {
	f := NewPRCreateFlow()
	f.SetRunner(mockPRRunner("feat/tui-git", true, prLogOutput))
	f.SetGHRunner(mockGHRunner(""))

	_ = f.Start()
	if !f.Active() {
		t.Error("expected active after start")
	}

	f.Cancel()
	if f.Active() {
		t.Error("expected inactive after cancel")
	}
}

func TestPRCreateFlowRenderInactive(t *testing.T) {
	f := NewPRCreateFlow()
	styles := testStyles()
	output := f.Render(styles, 80, 24)
	if output != "" {
		t.Errorf("expected empty render when inactive, got %q", output)
	}
}

func TestPRCreateFlowConcurrentAccess(t *testing.T) {
	f := NewPRCreateFlow()
	f.SetRunner(mockPRRunner("feat/tui-git", true, prLogOutput))
	f.SetGHRunner(mockGHRunner(""))
	_ = f.Start()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = f.Render(testStyles(), 80, 24)
			_ = f.GenerateTitle()
			_ = f.GenerateBody()
			_ = f.Active()
			_ = f.Info()
		}()
	}
	wg.Wait()
}

func TestPRCreateFlowSetTitleBody(t *testing.T) {
	f := NewPRCreateFlow()
	f.SetTitle("custom title")
	f.SetBody("custom body")

	info := f.Info()
	if info.Title != "custom title" {
		t.Errorf("expected 'custom title', got %q", info.Title)
	}
	if info.Body != "custom body" {
		t.Errorf("expected 'custom body', got %q", info.Body)
	}
}
