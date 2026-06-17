// SPDX-License-Identifier: MIT
// Purpose: tests for GitHub issue<->todo sync (issue #324).
package todo

import (
	"sync"
	"testing"
)

func TestImportIssuesBasic(t *testing.T) {
	s := NewGitHubSync()
	todos := s.ImportIssues([]GitHubIssue{
		{Number: 1, Title: "Fix crash", Body: "steps", State: "open", Assignee: "alice"},
	})
	if len(todos) != 1 {
		t.Fatalf("expected 1 todo, got %d", len(todos))
	}
	td := todos[0]
	if td.Title != "Fix crash" {
		t.Errorf("Title = %q", td.Title)
	}
	if td.Description != "steps" {
		t.Errorf("Description = %q", td.Description)
	}
	if td.ExternalRef != "issue:1" {
		t.Errorf("ExternalRef = %q", td.ExternalRef)
	}
	if td.Status != StatusOpen {
		t.Errorf("Status = %q", td.Status)
	}
	if td.Assignee != "alice" {
		t.Errorf("Assignee = %q", td.Assignee)
	}
}

func TestImportIssuesClosedState(t *testing.T) {
	s := NewGitHubSync()
	todos := s.ImportIssues([]GitHubIssue{{Number: 5, Title: "Done", State: "closed"}})
	if todos[0].Status != StatusDone {
		t.Errorf("Status = %q, want done", todos[0].Status)
	}
}

func TestImportIssuesLabelMapsPriorityAndType(t *testing.T) {
	s := NewGitHubSync()
	todos := s.ImportIssues([]GitHubIssue{
		{Number: 1, Title: "X", Labels: []string{"bug", "critical", "ui"}},
	})
	td := todos[0]
	if td.Type != TypeBug {
		t.Errorf("Type = %q, want bug", td.Type)
	}
	if td.Priority != PriorityP0 {
		t.Errorf("Priority = %q, want P0", td.Priority)
	}
	found := false
	for _, tag := range td.Tags {
		if tag == "ui" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'ui' tag preserved, got %v", td.Tags)
	}
}

func TestImportIssuesDefaults(t *testing.T) {
	s := NewGitHubSync()
	todos := s.ImportIssues([]GitHubIssue{{Number: 1, Title: "Plain"}})
	td := todos[0]
	if td.Priority != PriorityP2 {
		t.Errorf("Priority = %q, want P2", td.Priority)
	}
	if td.Type != TypeTask {
		t.Errorf("Type = %q, want task", td.Type)
	}
}

func TestExportTodosBasic(t *testing.T) {
	s := NewGitHubSync()
	todos := []*Todo{
		{Title: "Fix", Description: "desc", Priority: PriorityP1, Type: TypeBug, Status: StatusOpen, ExternalRef: "issue:7", Tags: []string{"ui"}},
	}
	issues := s.ExportTodos(todos)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	iss := issues[0]
	if iss.Title != "Fix" {
		t.Errorf("Title = %q", iss.Title)
	}
	if iss.Body != "desc" {
		t.Errorf("Body = %q", iss.Body)
	}
	if iss.Number != 7 {
		t.Errorf("Number = %d, want 7", iss.Number)
	}
	if iss.State != "open" {
		t.Errorf("State = %q", iss.State)
	}
}

func TestExportTodosClosedState(t *testing.T) {
	s := NewGitHubSync()
	issues := s.ExportTodos([]*Todo{{Title: "X", Status: StatusDone}})
	if issues[0].State != "closed" {
		t.Errorf("State = %q, want closed", issues[0].State)
	}
}

func TestExportTodosLabelsIncludePriorityAndType(t *testing.T) {
	s := NewGitHubSync()
	issues := s.ExportTodos([]*Todo{{Title: "X", Priority: PriorityP0, Type: TypeBug, Tags: []string{"ui"}}})
	labels := issues[0].Labels
	hasP0 := false
	hasBug := false
	hasUI := false
	for _, l := range labels {
		switch l {
		case "P0":
			hasP0 = true
		case "bug":
			hasBug = true
		case "ui":
			hasUI = true
		}
	}
	if !hasP0 || !hasBug || !hasUI {
		t.Errorf("labels = %v, want P0+bug+ui", labels)
	}
}

func TestMapLabelKnownAndUnknown(t *testing.T) {
	s := NewGitHubSync()
	cases := []struct{ in, want string }{
		{"bug", string(TypeBug)},
		{"enhancement", string(TypeFeature)},
		{"critical", string(PriorityP0)},
		{"P1", string(PriorityP1)},
		{"unknown-label", "unknown-label"},
	}
	for _, c := range cases {
		if got := s.MapLabel(c.in); got != c.want {
			t.Errorf("MapLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMapPriorityAllLevels(t *testing.T) {
	s := NewGitHubSync()
	if labels := s.MapPriority("P0"); len(labels) != 2 || labels[0] != "P0" || labels[1] != "critical" {
		t.Errorf("MapPriority(P0) = %v", labels)
	}
	if labels := s.MapPriority("P3"); len(labels) != 2 || labels[0] != "P3" {
		t.Errorf("MapPriority(P3) = %v", labels)
	}
	if labels := s.MapPriority("invalid"); labels != nil {
		t.Errorf("MapPriority(invalid) = %v, want nil", labels)
	}
}

func TestGitHubSyncConcurrent(t *testing.T) {
	s := NewGitHubSync()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.ImportIssues([]GitHubIssue{{Number: 1, Title: "X", Labels: []string{"bug"}}})
			_ = s.ExportTodos([]*Todo{{Title: "Y", Priority: PriorityP1}})
			_ = s.MapLabel("bug")
			_ = s.MapPriority("P1")
		}()
	}
	wg.Wait()
}

func TestExtractIssueNumber(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"issue:42", 42},
		{"https://github.com/owner/repo/issues/99", 99},
		{"goal:5", 0},
		{"", 0},
	}
	for _, c := range cases {
		if got := extractIssueNumber(c.in); got != c.want {
			t.Errorf("extractIssueNumber(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
