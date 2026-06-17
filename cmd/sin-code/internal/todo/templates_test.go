// SPDX-License-Identifier: MIT
// Purpose: tests for todo templates (issue #333).
package todo

import (
	"sync"
	"testing"
)

func TestDefaultTemplatesCount(t *testing.T) {
	ts := DefaultTemplates()
	want := []string{"bug-fix", "feature", "refactor", "docs", "test", "review", "deploy"}
	if len(ts) != len(want) {
		t.Fatalf("expected %d templates, got %d", len(want), len(ts))
	}
	names := make(map[string]bool, len(ts))
	for _, tmpl := range ts {
		names[tmpl.Name] = true
	}
	for _, w := range want {
		if !names[w] {
			t.Errorf("missing template %q", w)
		}
	}
}

func TestRegisterAndGet(t *testing.T) {
	s := NewTemplateStore()
	custom := TodoTemplate{Name: "custom", Description: "Custom task", DefaultPriority: PriorityP3, DefaultType: TypeTask}
	s.Register(custom)
	got, ok := s.Get("custom")
	if !ok {
		t.Fatal("expected to find custom template")
	}
	if got.DefaultPriority != PriorityP3 {
		t.Errorf("Priority = %q, want P3", got.DefaultPriority)
	}
}

func TestGetNotFound(t *testing.T) {
	s := NewTemplateStore()
	if _, ok := s.Get("nonexistent"); ok {
		t.Error("expected not found for nonexistent template")
	}
}

func TestListSortedByName(t *testing.T) {
	s := NewTemplateStore()
	list := s.List()
	for i := 1; i < len(list); i++ {
		if list[i].Name < list[i-1].Name {
			t.Errorf("list not sorted: %q before %q", list[i-1].Name, list[i].Name)
		}
	}
}

func TestApplyDefaults(t *testing.T) {
	s := NewTemplateStore()
	td, err := s.Apply("bug-fix", nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if td.Title != "Fix a reported bug" {
		t.Errorf("Title = %q", td.Title)
	}
	if td.Priority != PriorityP0 {
		t.Errorf("Priority = %q, want P0", td.Priority)
	}
	if td.Type != TypeBug {
		t.Errorf("Type = %q, want bug", td.Type)
	}
	if td.Status != StatusOpen {
		t.Errorf("Status = %q, want open", td.Status)
	}
}

func TestApplyOverrides(t *testing.T) {
	s := NewTemplateStore()
	td, err := s.Apply("feature", map[string]string{
		"title":    "Add OAuth2 login",
		"priority": "P0",
		"tags":     "auth,oauth",
		"assignee": "alice",
		"project":  "webapp",
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if td.Title != "Add OAuth2 login" {
		t.Errorf("Title = %q", td.Title)
	}
	if td.Priority != PriorityP0 {
		t.Errorf("Priority = %q, want P0", td.Priority)
	}
	if td.Assignee != "alice" {
		t.Errorf("Assignee = %q", td.Assignee)
	}
	if td.Project != "webapp" {
		t.Errorf("Project = %q", td.Project)
	}
	hasAuth := false
	for _, tag := range td.Tags {
		if tag == "auth" {
			hasAuth = true
		}
	}
	if !hasAuth {
		t.Errorf("Tags = %v, want 'auth' present", td.Tags)
	}
}

func TestApplyNotFound(t *testing.T) {
	s := NewTemplateStore()
	if _, err := s.Apply("nonexistent", nil); err == nil {
		t.Error("expected error for nonexistent template")
	}
}

func TestTemplateStoreConcurrent(t *testing.T) {
	s := NewTemplateStore()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Register(TodoTemplate{Name: "tmp", Description: "x"})
			_, _ = s.Get("bug-fix")
			_ = s.List()
			_, _ = s.Apply("review", map[string]string{"title": "PR"})
		}()
	}
	wg.Wait()
}
