// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"sync"
	"testing"
)

const sampleLogOutput = `a1b2c3d4e5f6|John Doe|2024-01-15|feat: add git diff panel
b2c3d4e5f6a1|Jane Smith|2024-01-14|fix: resolve scroll bug
c3d4e5f6a1b2|Bob Wilson|2024-01-13|docs: update README
d4e5f6a1b2c3|Alice Brown|2024-01-12|chore: bump dependencies
e5f6a1b2c3d4|Charlie Davis|2024-01-11|refactor: clean up imports
`

func mockLogRunner(output string) GitRunner {
	return func(args ...string) (string, error) {
		return output, nil
	}
}

func TestGitLogViewLoad(t *testing.T) {
	v := NewGitLogView()
	v.SetRunner(mockLogRunner(sampleLogOutput))

	if err := v.Load(5); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !v.Loaded() {
		t.Error("expected view to be loaded")
	}
	entries := v.Entries()
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

func TestGitLogViewRender(t *testing.T) {
	v := NewGitLogView()
	v.SetRunner(mockLogRunner(sampleLogOutput))
	_ = v.Load(5)

	styles := testStyles()
	output := v.Render(styles, 80, 24)
	stripped := stripANSI(output)

	if !strings.Contains(stripped, "git log") {
		t.Error("expected 'git log' header in render")
	}
	if !strings.Contains(stripped, "a1b2c3d") {
		t.Error("expected short hash in render")
	}
	if !strings.Contains(stripped, "feat: add git diff panel") {
		t.Error("expected commit message in render")
	}
	if !strings.Contains(stripped, "John Doe") {
		t.Error("expected author in render")
	}
}

func TestGitLogViewMoveUp(t *testing.T) {
	v := NewGitLogView()
	v.SetRunner(mockLogRunner(sampleLogOutput))
	_ = v.Load(5)

	v.MoveDown()
	v.MoveDown()
	if v.Selected() != 2 {
		t.Errorf("expected selected 2 after two down, got %d", v.Selected())
	}

	v.MoveUp()
	if v.Selected() != 1 {
		t.Errorf("expected selected 1 after up, got %d", v.Selected())
	}
}

func TestGitLogViewMoveDown(t *testing.T) {
	v := NewGitLogView()
	v.SetRunner(mockLogRunner(sampleLogOutput))
	_ = v.Load(5)

	for i := 0; i < 10; i++ {
		v.MoveDown()
	}
	if v.Selected() != 4 {
		t.Errorf("expected selected 4 (clamped at last), got %d", v.Selected())
	}
}

func TestGitLogViewMoveUpClamped(t *testing.T) {
	v := NewGitLogView()
	v.SetRunner(mockLogRunner(sampleLogOutput))
	_ = v.Load(5)

	for i := 0; i < 10; i++ {
		v.MoveUp()
	}
	if v.Selected() != 0 {
		t.Errorf("expected selected 0 (clamped at first), got %d", v.Selected())
	}
}

func TestGitLogViewParseEntries(t *testing.T) {
	entries := parseLogEntries(sampleLogOutput)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	first := entries[0]
	if first.Hash != "a1b2c3d4e5f6" {
		t.Errorf("expected hash 'a1b2c3d4e5f6', got %q", first.Hash)
	}
	if first.Author != "John Doe" {
		t.Errorf("expected author 'John Doe', got %q", first.Author)
	}
	if first.Date != "2024-01-15" {
		t.Errorf("expected date '2024-01-15', got %q", first.Date)
	}
	if first.Message != "feat: add git diff panel" {
		t.Errorf("expected message, got %q", first.Message)
	}
}

func TestGitLogViewEmpty(t *testing.T) {
	v := NewGitLogView()
	v.SetRunner(mockLogRunner(""))

	if err := v.Load(5); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	styles := testStyles()
	output := v.Render(styles, 80, 24)
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "No commits") {
		t.Errorf("expected 'No commits' message, got %q", stripped)
	}
}

func TestGitLogViewNotLoaded(t *testing.T) {
	v := NewGitLogView()
	styles := testStyles()
	output := v.Render(styles, 80, 24)
	stripped := stripANSI(output)
	if !strings.Contains(stripped, "No log loaded") {
		t.Errorf("expected 'No log loaded' message, got %q", stripped)
	}
}

func TestGitLogViewConcurrentAccess(t *testing.T) {
	v := NewGitLogView()
	v.SetRunner(mockLogRunner(sampleLogOutput))
	_ = v.Load(5)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = v.Render(testStyles(), 80, 24)
			v.MoveUp()
			v.MoveDown()
			_ = v.Entries()
			_ = v.Selected()
			_ = v.SelectedEntry()
		}()
	}
	wg.Wait()
}
