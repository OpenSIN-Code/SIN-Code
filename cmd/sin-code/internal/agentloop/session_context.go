// SPDX-License-Identifier: MIT
// Purpose: session-start context injection (issue #379). SessionContextBuilder
// assembles a unified preamble from lessons, semantic memory, and active
// goals so the agent loop can inject relevant context at session start.
// All store dependencies are behind small interfaces so callers can supply
// real or mock implementations.
package agentloop

import (
	"context"
	"strings"
)

// LessonsReader returns the n most recent lessons learned by the agent.
type LessonsReader interface {
	Recent(n int) ([]string, error)
}

// MemoryReader runs a semantic query and returns the top n memories.
type MemoryReader interface {
	Query(q string, n int) ([]string, error)
}

// GoalReader returns the currently active goals.
type GoalReader interface {
	Active() ([]string, error)
}

const (
	defaultRecentLessons = 5
	defaultTopKMemories  = 5
	defaultMemoryQuery   = ""
)

// SessionContextBuilder assembles a session-start preamble from lessons,
// memory, and goals. Any nil store is skipped gracefully.
type SessionContextBuilder struct {
	lessonsStore LessonsReader
	memoryStore  MemoryReader
	goalStore    GoalReader
}

// NewSessionContextBuilder returns a builder drawing from the supplied
// stores. Any argument may be nil to disable that context source.
func NewSessionContextBuilder(l LessonsReader, m MemoryReader, g GoalReader) *SessionContextBuilder {
	return &SessionContextBuilder{lessonsStore: l, memoryStore: m, goalStore: g}
}

// Build queries every non-nil store and returns the assembled markdown
// preamble. Context cancellation is honoured between store calls; the
// first store error aborts the build.
func (b *SessionContextBuilder) Build(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var lessons, memories, goals []string
	if b.lessonsStore != nil {
		ls, err := b.lessonsStore.Recent(defaultRecentLessons)
		if err != nil {
			return "", err
		}
		lessons = ls
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}
	if b.memoryStore != nil {
		ms, err := b.memoryStore.Query(defaultMemoryQuery, defaultTopKMemories)
		if err != nil {
			return "", err
		}
		memories = ms
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}
	if b.goalStore != nil {
		gs, err := b.goalStore.Active()
		if err != nil {
			return "", err
		}
		goals = gs
	}

	return b.Format(lessons, memories, goals), nil
}

// Format renders the supplied lessons, memories, and goals as a markdown
// preamble. Empty slices produce no section.
func (b *SessionContextBuilder) Format(lessons, memories, goals []string) string {
	var sb strings.Builder
	writeSection := func(heading string, items []string) {
		if len(items) == 0 {
			return
		}
		sb.WriteString("## ")
		sb.WriteString(heading)
		sb.WriteByte('\n')
		for _, it := range items {
			sb.WriteString("- ")
			sb.WriteString(it)
			sb.WriteByte('\n')
		}
	}
	writeSection("Lessons", lessons)
	writeSection("Memories", memories)
	writeSection("Goals", goals)
	return sb.String()
}
