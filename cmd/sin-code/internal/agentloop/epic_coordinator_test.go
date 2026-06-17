// SPDX-License-Identifier: MIT
// Purpose: unit tests for the epic coordinator (issue #318, M7).
package agentloop

import (
	"testing"
)

// mockLoader is a deterministic in-memory EpicLoader for testing.
type mockLoader struct {
	epics map[int]*Epic
	deps  map[int][]int
}

func (m mockLoader) LoadEpic(issueNumber int) (*Epic, error) {
	if e, ok := m.epics[issueNumber]; ok {
		return e, nil
	}
	return nil, ErrEpicNotFound
}

func (m mockLoader) LoadDependencies(issueNumber int) ([]int, error) {
	return m.deps[issueNumber], nil
}

func TestEpicCoordinator_LoadEpic(t *testing.T) {
	c := NewEpicCoordinator()
	c.SetLoader(mockLoader{
		epics: map[int]*Epic{
			100: {IssueNumber: 100, Title: "Feature X", SubIssues: []int{101, 102, 103}, Completed: []int{101}},
		},
	})
	epic, err := c.LoadEpic(100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if epic.Title != "Feature X" {
		t.Errorf("title: got %q, want 'Feature X'", epic.Title)
	}
	if len(epic.SubIssues) != 3 {
		t.Errorf("sub-issues: got %d, want 3", len(epic.SubIssues))
	}
}

func TestEpicCoordinator_LoadEpic_NotFound(t *testing.T) {
	c := NewEpicCoordinator()
	c.SetLoader(mockLoader{epics: map[int]*Epic{}})
	_, err := c.LoadEpic(999)
	if err != ErrEpicNotFound {
		t.Fatalf("expected ErrEpicNotFound, got: %v", err)
	}
}

func TestEpicCoordinator_LoadEpic_Cached(t *testing.T) {
	c := NewEpicCoordinator()
	ml := mockLoader{
		epics: map[int]*Epic{
			50: {IssueNumber: 50, Title: "Cached Epic", SubIssues: []int{51}, Completed: nil},
		},
	}
	c.SetLoader(ml)
	first, err := c.LoadEpic(50)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	second, err := c.LoadEpic(50)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if first != second {
		t.Error("expected same epic pointer on cache hit")
	}
}

func TestEpicCoordinator_NextIssue(t *testing.T) {
	c := NewEpicCoordinator()
	c.SetLoader(mockLoader{
		epics: map[int]*Epic{
			200: {IssueNumber: 200, Title: "Epic", SubIssues: []int{201, 202, 203}, Completed: []int{201}},
		},
	})
	epic, _ := c.LoadEpic(200)
	next, err := c.NextIssue(epic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != 202 {
		t.Errorf("next issue: got %d, want 202", next)
	}
	c.MarkComplete(202)
	next, err = c.NextIssue(epic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != 203 {
		t.Errorf("next issue after 202 done: got %d, want 203", next)
	}
}

func TestEpicCoordinator_NextIssue_AllComplete(t *testing.T) {
	c := NewEpicCoordinator()
	epic := &Epic{IssueNumber: 300, Title: "Done Epic", SubIssues: []int{301, 302}, Completed: []int{}}
	c.MarkComplete(301)
	c.MarkComplete(302)
	_, err := c.NextIssue(epic)
	if err != ErrNoRemainingIssues {
		t.Fatalf("expected ErrNoRemainingIssues, got: %v", err)
	}
}

func TestEpicCoordinator_Progress(t *testing.T) {
	c := NewEpicCoordinator()
	c.SetLoader(mockLoader{
		epics: map[int]*Epic{
			400: {IssueNumber: 400, Title: "Epic", SubIssues: []int{401, 402, 403, 404}, Completed: []int{401}},
		},
	})
	epic, _ := c.LoadEpic(400)
	pct := c.Progress(epic)
	if pct < 0.24 || pct > 0.26 {
		t.Errorf("progress 1/4: got %.2f, want ~0.25", pct)
	}
	c.MarkComplete(402)
	c.MarkComplete(403)
	pct = c.Progress(epic)
	if pct < 0.74 || pct > 0.76 {
		t.Errorf("progress 3/4: got %.2f, want ~0.75", pct)
	}
}

func TestEpicCoordinator_Dependencies(t *testing.T) {
	c := NewEpicCoordinator()
	c.SetLoader(mockLoader{
		epics: map[int]*Epic{
			500: {IssueNumber: 500, Title: "Epic", SubIssues: []int{501, 502}, Completed: nil},
		},
		deps: map[int][]int{
			501: {502},
		},
	})
	_, _ = c.LoadEpic(500)
	deps, err := c.Dependencies(501)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 1 || deps[0] != 502 {
		t.Errorf("dependencies: got %v, want [502]", deps)
	}
	cached, _ := c.Dependencies(501)
	if len(cached) != 1 || cached[0] != 502 {
		t.Errorf("cached dependencies: got %v, want [502]", cached)
	}
}

func TestEpicCoordinator_Summary(t *testing.T) {
	c := NewEpicCoordinator()
	c.SetLoader(mockLoader{
		epics: map[int]*Epic{
			600: {IssueNumber: 600, Title: "Big Feature", SubIssues: []int{601, 602, 603}, Completed: []int{601}},
		},
	})
	epic, _ := c.LoadEpic(600)
	summary := c.Summary(epic)
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if !contains(summary, "Big Feature") {
		t.Errorf("summary should contain title, got: %s", summary)
	}
	if !contains(summary, "1/3") {
		t.Errorf("summary should show 1/3, got: %s", summary)
	}
}

func TestEpicCoordinator_MarkComplete_UpdatesEpic(t *testing.T) {
	c := NewEpicCoordinator()
	c.SetLoader(mockLoader{
		epics: map[int]*Epic{
			700: {IssueNumber: 700, Title: "Epic", SubIssues: []int{701, 702}, Completed: nil},
		},
	})
	epic, _ := c.LoadEpic(700)
	if err := c.MarkComplete(701); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, num := range epic.Completed {
		if num == 701 {
			found = true
			break
		}
	}
	if !found {
		t.Error("MarkComplete should add issue to epic.Completed")
	}
	pct := c.Progress(epic)
	if pct < 0.49 || pct > 0.51 {
		t.Errorf("progress after 1/2: got %.2f, want ~0.50", pct)
	}
}

func TestEpicCoordinator_NilSafe(t *testing.T) {
	var c *EpicCoordinator
	_, err := c.LoadEpic(1)
	if err != ErrEpicNotFound {
		t.Errorf("nil LoadEpic should return ErrEpicNotFound, got %v", err)
	}
	_, err = c.NextIssue(nil)
	if err != ErrNoRemainingIssues {
		t.Errorf("nil NextIssue should return ErrNoRemainingIssues, got %v", err)
	}
	if c.Progress(nil) != 0 {
		t.Error("nil Progress should return 0")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
