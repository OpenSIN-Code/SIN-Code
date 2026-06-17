// SPDX-License-Identifier: MIT
// Purpose: Cross-harness MCP inventory — normalized view of MCP tools
// across different agent harnesses (issue #336). Allows comparison of
// tool sets, identification of unique/common tools, and unified
// querying.
//
// M7 invariant: CrossHarnessInventory is guarded by sync.RWMutex.
package hub

import (
	"sort"
	"sync"
)

// ToolSummary is a normalized, harness-agnostic description of a
// single MCP tool.
type ToolSummary struct {
	Name        string `json:"name"`
	Category    string `json:"category,omitempty"`
	Description string `json:"description,omitempty"`
	ReadOnly    bool   `json:"read_only,omitempty"`
}

// HarnessTools associates a harness name with its tool set.
type HarnessTools struct {
	Harness string        `json:"harness"`
	Tools   []ToolSummary `json:"tools"`
}

// ToolDiff describes the presence of a single tool in two harnesses
// being compared.
type ToolDiff struct {
	Tool string `json:"tool"`
	InA  bool   `json:"in_a"`
	InB  bool   `json:"in_b"`
	Diff string `json:"diff"` // "only_a", "only_b", "both"
}

// CrossHarnessInventory maintains a normalized MCP tool inventory
// across multiple agent harnesses. Safe for concurrent use (M7).
type CrossHarnessInventory struct {
	mu       sync.RWMutex
	harnesses map[string][]ToolSummary
}

// NewCrossHarnessInventory creates an empty inventory.
func NewCrossHarnessInventory() *CrossHarnessInventory {
	return &CrossHarnessInventory{
		harnesses: make(map[string][]ToolSummary),
	}
}

// RegisterHarness adds or replaces the tool set for a named harness.
func (i *CrossHarnessInventory) RegisterHarness(name string, tools []ToolSummary) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.harnesses[name] = tools
}

// AllTools returns the merged set of tools from all registered
// harnesses, sorted by name. Duplicate tool names (same name across
// harnesses) are collapsed into a single entry.
func (i *CrossHarnessInventory) AllTools() []ToolSummary {
	i.mu.RLock()
	defer i.mu.RUnlock()
	seen := make(map[string]ToolSummary)
	for _, tools := range i.harnesses {
		for _, t := range tools {
			seen[t.Name] = t
		}
	}
	out := make([]ToolSummary, 0, len(seen))
	for _, t := range seen {
		out = append(out, t)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// Compare returns the per-tool diff between two harnesses. Tools
// present in both are marked "both"; tools only in harnessA are
// "only_a"; tools only in harnessB are "only_b".
func (i *CrossHarnessInventory) Compare(harnessA, harnessB string) []ToolDiff {
	i.mu.RLock()
	defer i.mu.RUnlock()
	toolsA := toolNameSet(i.harnesses[harnessA])
	toolsB := toolNameSet(i.harnesses[harnessB])
	all := make(map[string]bool)
	for name := range toolsA {
		all[name] = true
	}
	for name := range toolsB {
		all[name] = true
	}
	out := make([]ToolDiff, 0, len(all))
	for name := range all {
		inA := toolsA[name]
		inB := toolsB[name]
		diff := "both"
		if inA && !inB {
			diff = "only_a"
		} else if !inA && inB {
			diff = "only_b"
		}
		out = append(out, ToolDiff{Tool: name, InA: inA, InB: inB, Diff: diff})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Tool < out[b].Tool })
	return out
}

// Unique returns tools that exist only in the named harness (not in
// any other registered harness), sorted by name.
func (i *CrossHarnessInventory) Unique(harness string) []ToolSummary {
	i.mu.RLock()
	defer i.mu.RUnlock()
	target, ok := i.harnesses[harness]
	if !ok {
		return nil
	}
	otherNames := make(map[string]bool)
	for name, tools := range i.harnesses {
		if name == harness {
			continue
		}
		for _, t := range tools {
			otherNames[t.Name] = true
		}
	}
	var out []ToolSummary
	for _, t := range target {
		if !otherNames[t.Name] {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// Common returns tools present in ALL registered harnesses, sorted by
// name. If fewer than two harnesses are registered, returns all tools
// from the single harness (or nil if none).
func (i *CrossHarnessInventory) Common() []ToolSummary {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if len(i.harnesses) == 0 {
		return nil
	}
	if len(i.harnesses) == 1 {
		for _, tools := range i.harnesses {
			out := make([]ToolSummary, len(tools))
			copy(out, tools)
			sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
			return out
		}
	}
	count := make(map[string]int)
	latest := make(map[string]ToolSummary)
	for _, tools := range i.harnesses {
		seenInHarness := make(map[string]bool)
		for _, t := range tools {
			if !seenInHarness[t.Name] {
				seenInHarness[t.Name] = true
				count[t.Name]++
				latest[t.Name] = t
			}
		}
	}
	total := len(i.harnesses)
	var out []ToolSummary
	for name, c := range count {
		if c == total {
			out = append(out, latest[name])
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// Harnesses returns the names of all registered harnesses, sorted.
func (i *CrossHarnessInventory) Harnesses() []string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]string, 0, len(i.harnesses))
	for name := range i.harnesses {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// --- helpers ------------------------------------------------------------

func toolNameSet(tools []ToolSummary) map[string]bool {
	set := make(map[string]bool, len(tools))
	for _, t := range tools {
		set[t.Name] = true
	}
	return set
}
