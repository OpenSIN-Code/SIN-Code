// SPDX-License-Identifier: MIT
// Purpose: TemplateStore manages reusable todo templates (issue #333).
// Templates predefine priority, type, tags, and subtask lists so common
// workflows (bug-fix, feature, review, deploy, ...) are one call away.
// All shared state is mutex-guarded (M7).
package todo

import (
	"fmt"
	"sync"
)

// TodoTemplate is a reusable task pattern.
type TodoTemplate struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	DefaultPriority Priority `json:"default_priority"`
	DefaultType     TodoType `json:"default_type"`
	DefaultTags     []string `json:"default_tags"`
	Subtasks        []string `json:"subtasks"`
}

// TemplateStore manages todo templates.
type TemplateStore struct {
	mu        sync.RWMutex
	templates map[string]TodoTemplate
}

// NewTemplateStore creates a store preloaded with the built-in templates.
func NewTemplateStore() *TemplateStore {
	s := &TemplateStore{templates: make(map[string]TodoTemplate)}
	for _, tmpl := range DefaultTemplates() {
		s.Register(tmpl)
	}
	return s
}

// Register adds or replaces a template by name.
func (s *TemplateStore) Register(tmpl TodoTemplate) {
	if s == nil || tmpl.Name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.templates[tmpl.Name] = tmpl
}

// Get returns a template by name.
func (s *TemplateStore) Get(name string) (*TodoTemplate, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	tmpl, ok := s.templates[name]
	if !ok {
		return nil, false
	}
	return &tmpl, true
}

// List returns all templates sorted by name.
func (s *TemplateStore) List() []TodoTemplate {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TodoTemplate, 0, len(s.templates))
	for _, tmpl := range s.templates {
		out = append(out, tmpl)
	}
	sortTemplatesByName(out)
	return out
}

// Apply creates a todo from a named template, applying overrides. Recognized
// override keys: title, description, priority, type, tags, assignee, project.
func (s *TemplateStore) Apply(name string, overrides map[string]string) (*Todo, error) {
	tmpl, ok := s.Get(name)
	if !ok {
		return nil, fmt.Errorf("todo: template %q not found", name)
	}
	td := &Todo{
		Title:       tmpl.Description,
		Description: tmpl.Description,
		Priority:    tmpl.DefaultPriority,
		Type:        tmpl.DefaultType,
		Tags:        append([]string{}, tmpl.DefaultTags...),
		Status:      StatusOpen,
	}
	if v, ok := overrides["title"]; ok && v != "" {
		td.Title = v
	}
	if v, ok := overrides["description"]; ok && v != "" {
		td.Description = v
	}
	if v, ok := overrides["priority"]; ok && Priority(v).Valid() {
		td.Priority = Priority(v)
	}
	if v, ok := overrides["type"]; ok && TodoType(v).Valid() {
		td.Type = TodoType(v)
	}
	if v, ok := overrides["tags"]; ok && v != "" {
		td.Tags = splitList(v)
	}
	if v, ok := overrides["assignee"]; ok {
		td.Assignee = v
	}
	if v, ok := overrides["project"]; ok {
		td.Project = v
	}
	if td.Priority == "" {
		td.Priority = PriorityP2
	}
	if td.Type == "" {
		td.Type = TypeTask
	}
	td.Tags = normalizeTags(td.Tags)
	return td, nil
}

func sortTemplatesByName(ts []TodoTemplate) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && ts[j].Name < ts[j-1].Name; j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}

// DefaultTemplates returns the built-in template set: bug-fix, feature,
// refactor, docs, test, review, deploy.
func DefaultTemplates() []TodoTemplate {
	return []TodoTemplate{
		{
			Name: "bug-fix", Description: "Fix a reported bug",
			DefaultPriority: PriorityP0, DefaultType: TypeBug,
			DefaultTags: []string{"bug", "fix"},
			Subtasks:    []string{"reproduce", "diagnose", "fix", "verify"},
		},
		{
			Name: "feature", Description: "Implement a new feature",
			DefaultPriority: PriorityP1, DefaultType: TypeFeature,
			DefaultTags: []string{"feature"},
			Subtasks:    []string{"design", "implement", "test", "docs"},
		},
		{
			Name: "refactor", Description: "Refactor existing code",
			DefaultPriority: PriorityP2, DefaultType: TypeTask,
			DefaultTags: []string{"refactor"},
			Subtasks:    []string{"analyze", "refactor", "test"},
		},
		{
			Name: "docs", Description: "Write or update documentation",
			DefaultPriority: PriorityP2, DefaultType: TypeChore,
			DefaultTags: []string{"docs"},
			Subtasks:    []string{"outline", "write", "review"},
		},
		{
			Name: "test", Description: "Add or improve tests",
			DefaultPriority: PriorityP2, DefaultType: TypeTask,
			DefaultTags: []string{"test"},
			Subtasks:    []string{"identify", "write", "run"},
		},
		{
			Name: "review", Description: "Review a pull request",
			DefaultPriority: PriorityP1, DefaultType: TypeTask,
			DefaultTags: []string{"review", "pr"},
			Subtasks:    []string{"read-diff", "run-tests", "comment"},
		},
		{
			Name: "deploy", Description: "Deploy to production",
			DefaultPriority: PriorityP0, DefaultType: TypeChore,
			DefaultTags: []string{"deploy"},
			Subtasks:    []string{"changelog", "tag", "build", "publish"},
		},
	}
}
