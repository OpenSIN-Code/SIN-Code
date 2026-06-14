// SPDX-License-Identifier: MIT
// Purpose: a process-wide registry of agent tools (issue #108). Every tool
// registers its semantic metadata and capabilities here, and the registry
// deterministically generates the system-prompt fragment that lists all
// tools so an LLM agent can never "miss" one. Corrected, race-safe Go
// implementation of the design sketched in issue #108.
package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// ToolMetadata is the interface an agent uses to select a tool.
type ToolMetadata struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Parameters   map[string]interface{} `json:"parameters"`
	Capabilities []string               `json:"capabilities"`
}

// Tool is the contract every SIN tool implements.
type Tool interface {
	Execute(args map[string]interface{}) (interface{}, error)
	GetMetadata() ToolMetadata
}

// Registry holds all active tools and a capability -> tool-name index.
type Registry struct {
	mu         sync.RWMutex
	tools      map[string]Tool
	capability map[string][]string
}

var (
	instance *Registry
	once     sync.Once
)

// GetRegistry returns the process-wide singleton registry.
func GetRegistry() *Registry {
	once.Do(func() {
		instance = NewRegistry()
	})
	return instance
}

// NewRegistry builds an independent registry (used by tests so they don't
// share the global singleton).
func NewRegistry() *Registry {
	return &Registry{
		tools:      make(map[string]Tool),
		capability: make(map[string][]string),
	}
}

// RegisterTool adds a tool. It errors on a duplicate name or empty name.
func (r *Registry) RegisterTool(t Tool) error {
	meta := t.GetMetadata()
	if meta.Name == "" {
		return fmt.Errorf("tools: cannot register tool with empty name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[meta.Name]; exists {
		return fmt.Errorf("tools: tool %q is already registered", meta.Name)
	}
	r.tools[meta.Name] = t
	for _, cap := range meta.Capabilities {
		r.capability[cap] = append(r.capability[cap], meta.Name)
	}
	return nil
}

// GetTool returns a tool by name.
func (r *Registry) GetTool(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// ToolsByCapability returns the names of tools advertising a capability,
// sorted for determinism.
func (r *Registry) ToolsByCapability(cap string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := append([]string(nil), r.capability[cap]...)
	sort.Strings(out)
	return out
}

// Names returns all registered tool names, sorted.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// GenerateAgentPrompt renders a deterministic prompt fragment listing every
// registered tool, so the agent is always aware of the full toolset.
func (r *Registry) GenerateAgentPrompt() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names) // stable output regardless of map iteration order

	prompt := "AUTHORIZED SIN-CODE SYSTEM TOOLS (use whenever the context matches):\n"
	for _, name := range names {
		meta := r.tools[name].GetMetadata()
		paramJSON, _ := json.Marshal(meta.Parameters)
		prompt += fmt.Sprintf("- %s\n  description: %s\n  interface: %s\n",
			meta.Name, meta.Description, string(paramJSON))
	}
	return prompt
}
