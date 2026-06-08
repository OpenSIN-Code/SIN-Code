// SPDX-License-Identifier: MIT
// Purpose: orchestrator data model — Plan, Task, Agent, Result, ScratchpadEntry.
package orchestrator

import (
	"crypto/sha1"
	"fmt"
	"sync"
	"time"
)

type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
	TaskBlocked   TaskStatus = "blocked"
)

type TaskType string

const (
	TaskCode      TaskType = "code"
	TaskTest      TaskType = "test"
	TaskReview    TaskType = "review"
	TaskDocs      TaskType = "docs"
	TaskSecurity  TaskType = "security"
	TaskArchitect TaskType = "architect"
	TaskGeneral   TaskType = "general"
)

type Intent string

const (
	IntentCodebase     Intent = "codebase_change"
	IntentTest         Intent = "test_work"
	IntentReview       Intent = "code_review"
	IntentDocs         Intent = "documentation"
	IntentSecurity     Intent = "security_audit"
	IntentArchitecture Intent = "architecture"
	IntentGeneral      Intent = "general_query"
)

type Task struct {
	ID          string     `json:"id"`
	Type        TaskType   `json:"type"`
	Description string     `json:"description"`
	AgentName   string     `json:"agent"`
	DependsOn   []string   `json:"depends_on,omitempty"`
	Status      TaskStatus `json:"status"`
	Result      string     `json:"result,omitempty"`
	Error       string     `json:"error,omitempty"`
	Created     time.Time  `json:"created"`
	Started     *time.Time `json:"started,omitempty"`
	Completed   *time.Time `json:"completed,omitempty"`
	TokensUsed  int        `json:"tokens_used"`
	Cost        float64    `json:"cost"`
}

type Plan struct {
	ID         string    `json:"id"`
	Prompt     string    `json:"prompt"`
	Intent     Intent    `json:"intent"`
	Tasks      []*Task   `json:"tasks"`
	Created    time.Time `json:"created"`
	Started    time.Time `json:"started"`
	Completed  time.Time `json:"completed"`
	TotalCost  float64   `json:"total_cost"`
	TokensUsed int       `json:"tokens_used"`
	Success    bool      `json:"success"`
}

type AgentConfig struct {
	Name         string   `toml:"name"`
	Description  string   `toml:"description"`
	Type         TaskType `toml:"type"`
	Provider     string   `toml:"provider"`
	BaseURL      string   `toml:"base_url"`
	Model        string   `toml:"model"`
	MaxTokens    int      `toml:"max_tokens"`
	Temperature  float64  `toml:"temperature"`
	SystemFile   string   `toml:"system_file"`
	MaxContext   int      `toml:"max_context_tokens"`
	ToolsAllow   []string `toml:"tools_allow"`
	ToolsDeny    []string `toml:"tools_deny"`
	MemoryNS     string   `toml:"memory_namespace"`
	RetentionDays int     `toml:"retention_days"`
}

type ScratchpadEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Agent     string    `json:"agent"`
	Section   string    `json:"section"`
	Content   string    `json:"content"`
	Version   int       `json:"version"`
}

type Scratchpad struct {
	mu      sync.RWMutex
	entries map[string]*ScratchpadEntry
}

func NewScratchpad() *Scratchpad {
	return &Scratchpad{entries: map[string]*ScratchpadEntry{}}
}

func (s *Scratchpad) Write(agent, section, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := section
	existing, ok := s.entries[key]
	version := 1
	if ok {
		version = existing.Version + 1
	}
	s.entries[key] = &ScratchpadEntry{
		Timestamp: time.Now().UTC(),
		Agent:     agent,
		Section:   section,
		Content:   content,
		Version:   version,
	}
}

func (s *Scratchpad) Read(section string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[section]
	if !ok {
		return "", false
	}
	return e.Content, true
}

func (s *Scratchpad) ReadAll() map[string]*ScratchpadEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]*ScratchpadEntry, len(s.entries))
	for k, v := range s.entries {
		out[k] = v
	}
	return out
}

func (s *Scratchpad) Merge(other *Scratchpad) {
	if other == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range other.entries {
		existing, ok := s.entries[k]
		if !ok {
			s.entries[k] = v
			continue
		}
		if v.Version > existing.Version || v.Timestamp.After(existing.Timestamp) {
			s.entries[k] = v
		}
	}
}

var idCounter uint64

func GenerateID(prefix string) string {
	idCounter++
	h := sha1.Sum([]byte(fmt.Sprintf("%d-%d-%s", time.Now().UnixNano(), idCounter, prefix)))
	return fmt.Sprintf("%s-%x", prefix, h[:6])
}
