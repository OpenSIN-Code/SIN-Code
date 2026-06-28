// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

const goDiff = `diff --git a/cmd/sin-code/tui/git_view.go b/cmd/sin-code/tui/git_view.go
index abc..def 100644
--- a/cmd/sin-code/tui/git_view.go
+++ b/cmd/sin-code/tui/git_view.go
@@ -1,5 +1,7 @@
 package tui
+import "fmt"
+var x = 1
 func main() {}
`

const mdDiff = `diff --git a/docs/guide.md b/docs/guide.md
index abc..def 100644
--- a/docs/guide.md
+++ b/docs/guide.md
@@ -1,3 +1,4 @@
 # Guide
+New section
 Content
`

const commitTestDiff = `diff --git a/cmd/sin-code/tui/git_view_test.go b/cmd/sin-code/tui/git_view_test.go
index abc..def 100644
--- a/cmd/sin-code/tui/git_view_test.go
+++ b/cmd/sin-code/tui/git_view_test.go
@@ -1,3 +1,5 @@
 package tui
+func TestNew(t *testing.T) {}
+func TestOld(t *testing.T) {}
`

const configDiff = `diff --git a/go.mod b/go.mod
index abc..def 100644
--- a/go.mod
+++ b/go.mod
@@ -1,2 +1,3 @@
 module test
+require foo
`

func mockCommitRunner(stagedCount int, diff string) GitRunner {
	return func(args ...string) (string, error) {
		if len(args) >= 1 && args[0] == "status" {
			var b strings.Builder
			for i := 0; i < stagedCount; i++ {
				b.WriteString("M  file")
				b.WriteString("\n")
			}
			return b.String(), nil
		}
		if len(args) >= 2 && args[0] == "diff" && args[1] == "--cached" {
			return diff, nil
		}
		if len(args) >= 1 && args[0] == "commit" {
			return "committed", nil
		}
		return "", nil
	}
}

func TestGitCommitFlowStart(t *testing.T) {
	f := NewGitCommitFlow()
	f.SetRunner(mockCommitRunner(3, goDiff))

	if err := f.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !f.Active() {
		t.Error("expected flow to be active")
	}
	if f.StagedCount() != 3 {
		t.Errorf("expected 3 staged, got %d", f.StagedCount())
	}
	if f.Message() == "" {
		t.Error("expected non-empty message")
	}
}

func TestGitCommitFlowGenerateMessageGo(t *testing.T) {
	f := NewGitCommitFlow()
	msg := f.GenerateMessage(goDiff)
	if !strings.HasPrefix(msg, "feat") {
		t.Errorf("expected 'feat' prefix for .go files, got %q", msg)
	}
}

func TestGitCommitFlowGenerateMessageMd(t *testing.T) {
	f := NewGitCommitFlow()
	msg := f.GenerateMessage(mdDiff)
	if !strings.HasPrefix(msg, "docs") {
		t.Errorf("expected 'docs' prefix for .md files, got %q", msg)
	}
}

func TestGitCommitFlowGenerateMessageTest(t *testing.T) {
	f := NewGitCommitFlow()
	msg := f.GenerateMessage(commitTestDiff)
	if !strings.HasPrefix(msg, "test") {
		t.Errorf("expected 'test' prefix for _test.go files, got %q", msg)
	}
}

func TestGitCommitFlowGenerateMessageConfig(t *testing.T) {
	f := NewGitCommitFlow()
	msg := f.GenerateMessage(configDiff)
	if !strings.HasPrefix(msg, "chore") {
		t.Errorf("expected 'chore' prefix for config files, got %q", msg)
	}
}

func TestGitCommitFlowGenerateMessageScope(t *testing.T) {
	f := NewGitCommitFlow()
	msg := f.GenerateMessage(goDiff)
	if !strings.Contains(msg, "(") {
		t.Errorf("expected scope in message, got %q", msg)
	}
	if !strings.Contains(msg, "tui") {
		t.Errorf("expected 'tui' scope, got %q", msg)
	}
}

func TestGitCommitFlowExecute(t *testing.T) {
	f := NewGitCommitFlow()
	f.SetRunner(mockCommitRunner(1, goDiff))

	_ = f.Start()
	msg := f.Message()

	if err := f.Execute(msg); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if f.Active() {
		t.Error("expected flow to be inactive after execute")
	}
}

func TestGitCommitFlowRender(t *testing.T) {
	f := NewGitCommitFlow()
	f.SetRunner(mockCommitRunner(3, goDiff))
	_ = f.Start()

	styles := testStyles()
	output := f.Render(styles, 80, 24)
	stripped := stripANSI(output)

	if !strings.Contains(stripped, "Commit") {
		t.Error("expected 'Commit' header in render")
	}
	if !strings.Contains(stripped, "Staged") {
		t.Error("expected 'Staged' label in render")
	}
	if !strings.Contains(stripped, "Message") {
		t.Error("expected 'Message' label in render")
	}
	if !strings.Contains(stripped, "[Enter]") {
		t.Error("expected keybinding hint in render")
	}
}

func TestGitCommitFlowFilter(t *testing.T) {
	f := NewGitCommitFlow()
	f.SetFilter(&mockCommitFilter{filter: func(msg string) string {
		return strings.ReplaceAll(msg, "AI", "")
	}})

	msg := f.GenerateMessage("diff --git a/file.go b/file.go\n+++ b/file.go\n+AI code")
	if strings.Contains(msg, "AI") {
		t.Errorf("expected AI to be filtered, got %q", msg)
	}
}

func TestGitCommitFlowActiveCancel(t *testing.T) {
	f := NewGitCommitFlow()
	f.SetRunner(mockCommitRunner(1, goDiff))

	_ = f.Start()
	if !f.Active() {
		t.Error("expected active after start")
	}

	f.Cancel()
	if f.Active() {
		t.Error("expected inactive after cancel")
	}
}

func TestGitCommitFlowRenderInactive(t *testing.T) {
	f := NewGitCommitFlow()
	styles := testStyles()
	output := f.Render(styles, 80, 24)
	if output != "" {
		t.Errorf("expected empty render when inactive, got %q", output)
	}
}

func TestGitCommitFlowConcurrentAccess(t *testing.T) {
	f := NewGitCommitFlow()
	f.SetRunner(mockCommitRunner(2, goDiff))
	_ = f.Start()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = f.GenerateMessage(goDiff)
			_ = f.Render(testStyles(), 80, 24)
			_ = f.Active()
			_ = f.Message()
			_ = f.StagedCount()
		}()
	}
	wg.Wait()
}

type mockCommitFilter struct {
	filter func(msg string) string
}

func (m *mockCommitFilter) FilterCommitMessage(msg string) string {
	return m.filter(msg)
}
