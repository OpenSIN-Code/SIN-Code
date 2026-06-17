// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

func mockDiffRunner(output string) GitRunner {
	return func(args ...string) (string, error) {
		return output, nil
	}
}

const multiFileDiff = `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -10,4 +10,5 @@
 func main() {
 	fmt.Println("hello")
-	fmt.Println("world")
+	fmt.Println("universe")
+	fmt.Println("!")
 }
diff --git a/README.md b/README.md
index 111..222 100644
--- a/README.md
+++ b/README.md
@@ -1,3 +1,4 @@
 # Project
+New section
 Some text
 More text
`

const newFileDiff = `diff --git a/new.go b/new.go
new file mode 100644
index 000..abc
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
+
+func newFunc() {}
`

func TestGitDiffPanelLoadDiff(t *testing.T) {
	p := NewGitDiffPanel()
	p.SetRunner(mockDiffRunner(multiFileDiff))

	if err := p.LoadDiff(false); err != nil {
		t.Fatalf("LoadDiff failed: %v", err)
	}
	if !p.Loaded() {
		t.Error("expected panel to be loaded")
	}
	if p.TotalFiles() != 2 {
		t.Errorf("expected 2 files, got %d", p.TotalFiles())
	}
	if p.TotalAdded() != 3 {
		t.Errorf("expected 3 added lines, got %d", p.TotalAdded())
	}
	if p.TotalRemoved() != 1 {
		t.Errorf("expected 1 removed line, got %d", p.TotalRemoved())
	}
}

func TestGitDiffPanelLoadStagedDiff(t *testing.T) {
	p := NewGitDiffPanel()
	p.SetRunner(func(args ...string) (string, error) {
		if len(args) >= 2 && args[1] == "--cached" {
			return multiFileDiff, nil
		}
		return "", nil
	})

	if err := p.LoadDiff(true); err != nil {
		t.Fatalf("LoadDiff staged failed: %v", err)
	}
	if p.TotalFiles() != 2 {
		t.Errorf("expected 2 files, got %d", p.TotalFiles())
	}
}

func TestGitDiffPanelLoadFileDiff(t *testing.T) {
	p := NewGitDiffPanel()
	p.SetRunner(func(args ...string) (string, error) {
		if len(args) >= 3 && args[0] == "diff" && args[2] == "main.go" {
			return multiFileDiff, nil
		}
		return "", nil
	})

	if err := p.LoadFileDiff("main.go"); err != nil {
		t.Fatalf("LoadFileDiff failed: %v", err)
	}
	if !p.Loaded() {
		t.Error("expected panel to be loaded")
	}
}

func TestGitDiffPanelRender(t *testing.T) {
	p := NewGitDiffPanel()
	p.SetRunner(mockDiffRunner(multiFileDiff))
	_ = p.LoadDiff(false)

	styles := testStyles()
	output := p.Render(styles, 80, 24)

	stripped := stripANSI(output)
	if !strings.Contains(stripped, "2 files changed") {
		t.Error("expected file count in render output")
	}
	if !strings.Contains(stripped, "+3") {
		t.Error("expected added count in render output")
	}
	if !strings.Contains(stripped, "-1") {
		t.Error("expected removed count in render output")
	}
	if !strings.Contains(stripped, "main.go") {
		t.Error("expected file path in render output")
	}
}

func TestGitDiffPanelScroll(t *testing.T) {
	p := NewGitDiffPanel()
	p.SetRunner(mockDiffRunner(multiFileDiff))
	_ = p.LoadDiff(false)

	p.ScrollDown(5)
	if p.ScrollOffset() != 5 {
		t.Errorf("expected scroll offset 5, got %d", p.ScrollOffset())
	}

	p.ScrollUp(3)
	if p.ScrollOffset() != 2 {
		t.Errorf("expected scroll offset 2, got %d", p.ScrollOffset())
	}

	p.ScrollUp(100)
	if p.ScrollOffset() != 0 {
		t.Errorf("expected scroll offset 0 (clamped), got %d", p.ScrollOffset())
	}
}

func TestGitDiffPanelEmptyDiff(t *testing.T) {
	p := NewGitDiffPanel()
	p.SetRunner(mockDiffRunner(""))

	if err := p.LoadDiff(false); err != nil {
		t.Fatalf("LoadDiff failed: %v", err)
	}
	if !p.Loaded() {
		t.Error("expected panel to be loaded")
	}
	if p.TotalFiles() != 0 {
		t.Errorf("expected 0 files, got %d", p.TotalFiles())
	}

	styles := testStyles()
	output := p.Render(styles, 80, 24)
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "No changes") {
		t.Errorf("expected 'No changes' in output, got %q", stripped)
	}
}

func TestGitDiffPanelParseFileStatsNewFile(t *testing.T) {
	stats := parseFileStats(newFileDiff)
	if len(stats) != 1 {
		t.Fatalf("expected 1 file stat, got %d", len(stats))
	}
	if stats[0].Status != "added" {
		t.Errorf("expected status 'added', got %q", stats[0].Status)
	}
	if stats[0].Path != "new.go" {
		t.Errorf("expected path 'new.go', got %q", stats[0].Path)
	}
	if stats[0].Added != 3 {
		t.Errorf("expected 3 added, got %d", stats[0].Added)
	}
}

func TestGitDiffPanelNotLoadedRender(t *testing.T) {
	p := NewGitDiffPanel()
	styles := testStyles()
	output := p.Render(styles, 80, 24)
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "No diff loaded") {
		t.Errorf("expected 'No diff loaded' message, got %q", stripped)
	}
}

func TestGitDiffPanelConcurrentAccess(t *testing.T) {
	p := NewGitDiffPanel()
	p.SetRunner(mockDiffRunner(multiFileDiff))
	_ = p.LoadDiff(false)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Render(testStyles(), 80, 24)
			p.ScrollDown(1)
			p.ScrollUp(1)
			_ = p.TotalFiles()
			_ = p.RawDiff()
			_ = p.FileStats()
		}()
	}
	wg.Wait()
}
