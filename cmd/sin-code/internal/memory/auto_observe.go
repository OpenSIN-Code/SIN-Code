// SPDX-License-Identifier: MIT
// Purpose: auto-observation capture from tool calls (issue #349).
// The AutoObserver hooks into tool.pre/tool.post events and records
// memory observations for mutating tools (edit, write, execute, test).
// Read-only tools (discover, scout, map) are skipped. Similar
// observations (same tool + same file) are grouped into one memory
// with updated content. Thread-safe (mandate M7).
package memory

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

var mutatingTools = map[string]bool{
	"edit":        true,
	"write":       true,
	"execute":     true,
	"test":        true,
	"sin_edit":    true,
	"sin_write":   true,
	"sin_execute": true,
	"sin_test":    true,
}

var readOnlyTools = map[string]bool{
	"discover":     true,
	"scout":        true,
	"map":          true,
	"read":         true,
	"sin_discover": true,
	"sin_scout":    true,
	"sin_map":      true,
	"sin_read":     true,
}

// AutoObserver captures observations from tool calls and stores them
// as memory entries. It filters read-only tools and groups similar
// observations (same tool + same file) into a single memory.
type AutoObserver struct {
	store *Store
	mu    sync.Mutex
	// cache maps "tool\x00file" → memory ID for grouping.
	cache map[string]string
}

// NewAutoObserver creates an observer backed by the given memory store.
func NewAutoObserver(store *Store) *AutoObserver {
	return &AutoObserver{
		store: store,
		cache: make(map[string]string),
	}
}

// ShouldObserve returns true if the tool is a mutating tool whose
// calls should be captured as observations.
func (o *AutoObserver) ShouldObserve(toolName string) bool {
	base := normalizeToolName(toolName)
	return mutatingTools[base]
}

func normalizeToolName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(name, "sin_") {
		return name
	}
	// Check if the base name (without sin_ prefix) is known.
	if mutatingTools[name] || readOnlyTools[name] {
		return name
	}
	// Try with sin_ prefix.
	return "sin_" + name
}

// Observe records a memory observation for a mutating tool call. If the
// same tool has already been observed on the same file, the existing
// memory is updated rather than duplicated. Read-only tools are silently
// skipped.
func (o *AutoObserver) Observe(toolName string, args map[string]any, result string, success bool) {
	if o == nil || o.store == nil {
		return
	}
	if !o.ShouldObserve(toolName) {
		return
	}

	base := normalizeToolName(toolName)
	filePath := extractFilePath(args)
	insight := buildObservationInsight(base, filePath, result, success)
	tags := []string{"observation", base}
	if filePath != "" {
		tags = append(tags, "file:"+filePath)
	}

	cacheKey := base + "\x00" + filePath

	o.mu.Lock()
	defer o.mu.Unlock()

	if existingID, found := o.cache[cacheKey]; found && existingID != "" {
		existing, err := o.store.Get(existingID)
		if err == nil && existing != nil {
			existing.Insight = insight
			existing.Tags = tags
			existing.AccessCount++
			_ = o.store.Update(existing)
			return
		}
	}

	m := &Memory{
		Insight:    insight,
		Tags:       tags,
		Importance: 0.3,
		Created:    time.Now().UTC(),
	}
	if err := o.store.Add(m); err != nil {
		return
	}
	o.cache[cacheKey] = m.ID
}

func extractFilePath(args map[string]any) string {
	if args == nil {
		return ""
	}
	for _, key := range []string{"path", "file", "filePath", "filename", "dest"} {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func buildObservationInsight(tool, filePath, result string, success bool) string {
	status := "succeeded"
	if !success {
		status = "failed"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Tool %s %s", tool, status)
	if filePath != "" {
		fmt.Fprintf(&b, " on %s", filePath)
	}
	trunc := truncate(result, 120)
	if trunc != "" {
		fmt.Fprintf(&b, ": %s", trunc)
	}
	return b.String()
}

// ObservableTools returns the list of tool names that trigger
// observation. Useful for documentation and testing.
func ObservableTools() []string {
	return []string{"edit", "write", "execute", "test", "sin_edit", "sin_write", "sin_execute", "sin_test"}
}

// ReadOnlyTools returns the list of tool names that are explicitly
// skipped by the observer.
func ReadOnlyTools() []string {
	return []string{"discover", "scout", "map", "read", "sin_discover", "sin_scout", "sin_map", "sin_read"}
}
