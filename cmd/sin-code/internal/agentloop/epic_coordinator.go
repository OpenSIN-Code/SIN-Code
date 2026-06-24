// SPDX-License-Identifier: MIT
// Purpose: Epic coordination workflow for GitHub issues (issue #318).
// Coordinates work across multiple sub-issues in an epic: tracks
// completion, finds the next uncompleted sub-issue, resolves
// dependencies, and produces human-readable progress summaries.
//
// The EpicLoader interface is mockable so tests can inject deterministic
// epic data without hitting the GitHub API.
//
// Thread-safe (mandate M7).
package agentloop

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ErrEpicNotFound is returned when an epic cannot be loaded.
var ErrEpicNotFound = errors.New("agentloop: epic not found")

// ErrNoRemainingIssues is returned when all sub-issues in an epic are
// completed.
var ErrNoRemainingIssues = errors.New("agentloop: no remaining sub-issues")

// Epic represents a GitHub issue that coordinates multiple sub-issues.
type Epic struct {
	IssueNumber int
	Title       string
	SubIssues   []int
	Completed   []int
}

// sin-debt: yagni, upgrade: when a second implementation lands, remove this marker
// EpicLoader loads epic data from an external source (GitHub API in
// production, a mock in tests). Implementations must be safe for
// concurrent use.
type EpicLoader interface {
	LoadEpic(issueNumber int) (*Epic, error)
	LoadDependencies(issueNumber int) ([]int, error)
}

// EpicCoordinator coordinates work across multiple sub-issues in an
// epic. It caches loaded epics, tracks completion state, and resolves
// dependencies.
type EpicCoordinator struct {
	mu        sync.Mutex
	loader    EpicLoader
	epics     map[int]*Epic
	completed map[int]bool
	deps      map[int][]int
}

// NewEpicCoordinator creates a coordinator with the given loader. If
// loader is nil, a nilLoader is used (LoadEpic always returns
// ErrEpicNotFound).
func NewEpicCoordinator() *EpicCoordinator {
	return &EpicCoordinator{
		loader:    nilLoader{},
		epics:     make(map[int]*Epic),
		completed: make(map[int]bool),
		deps:      make(map[int][]int),
	}
}

// SetLoader replaces the epic loader. Allows injecting a custom loader
// after construction (e.g. for testing).
func (c *EpicCoordinator) SetLoader(loader EpicLoader) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.loader = loader
	c.mu.Unlock()
}

// LoadEpic loads an epic from the configured loader and caches it.
func (c *EpicCoordinator) LoadEpic(issueNumber int) (*Epic, error) {
	if c == nil {
		return nil, ErrEpicNotFound
	}
	c.mu.Lock()
	cached, ok := c.epics[issueNumber]
	c.mu.Unlock()
	if ok {
		return cached, nil
	}
	c.mu.Lock()
	loader := c.loader
	c.mu.Unlock()
	if loader == nil {
		return nil, ErrEpicNotFound
	}
	epic, err := loader.LoadEpic(issueNumber)
	if err != nil {
		return nil, err
	}
	if epic == nil {
		return nil, ErrEpicNotFound
	}
	c.mu.Lock()
	c.epics[issueNumber] = epic
	for _, num := range epic.Completed {
		c.completed[num] = true
	}
	c.mu.Unlock()
	return epic, nil
}

// NextIssue returns the first uncompleted sub-issue in the epic. Sub-issues
// are checked in order; the first one not in the completed set is returned.
// Returns ErrNoRemainingIssues if all sub-issues are completed.
func (c *EpicCoordinator) NextIssue(epic *Epic) (int, error) {
	if c == nil || epic == nil {
		return 0, ErrNoRemainingIssues
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, num := range epic.SubIssues {
		if !c.completed[num] {
			return num, nil
		}
	}
	return 0, ErrNoRemainingIssues
}

// MarkComplete records an issue as completed and updates any cached epic
// that contains it.
func (c *EpicCoordinator) MarkComplete(issueNumber int) error {
	if c == nil {
		return ErrEpicNotFound
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.completed[issueNumber] = true
	for _, epic := range c.epics {
		already := false
		for _, num := range epic.Completed {
			if num == issueNumber {
				already = true
				break
			}
		}
		if !already {
			isSub := false
			for _, num := range epic.SubIssues {
				if num == issueNumber {
					isSub = true
					break
				}
			}
			if isSub {
				epic.Completed = append(epic.Completed, issueNumber)
			}
		}
	}
	return nil
}

// Progress returns the completion percentage of the epic as a float64
// between 0.0 and 1.0. If the epic has no sub-issues, returns 1.0
// (vacuously complete).
func (c *EpicCoordinator) Progress(epic *Epic) float64 {
	if c == nil || epic == nil {
		return 0
	}
	if len(epic.SubIssues) == 0 {
		return 1.0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	completed := 0
	for _, num := range epic.SubIssues {
		if c.completed[num] {
			completed++
		}
	}
	return float64(completed) / float64(len(epic.SubIssues))
}

// Dependencies returns the issue numbers that depend on the given issue.
// The coordinator caches dependency lookups from the loader.
func (c *EpicCoordinator) Dependencies(issueNumber int) ([]int, error) {
	if c == nil {
		return nil, ErrEpicNotFound
	}
	c.mu.Lock()
	cached, ok := c.deps[issueNumber]
	c.mu.Unlock()
	if ok {
		return append([]int(nil), cached...), nil
	}
	c.mu.Lock()
	loader := c.loader
	c.mu.Unlock()
	if loader == nil {
		return nil, nil
	}
	deps, err := loader.LoadDependencies(issueNumber)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.deps[issueNumber] = deps
	c.mu.Unlock()
	return append([]int(nil), deps...), nil
}

// Summary produces a human-readable progress summary for the epic.
func (c *EpicCoordinator) Summary(epic *Epic) string {
	if c == nil || epic == nil {
		return "epic: <unknown>"
	}
	progress := c.Progress(epic)
	pct := progress * 100
	completed := 0
	c.mu.Lock()
	for _, num := range epic.SubIssues {
		if c.completed[num] {
			completed++
		}
	}
	c.mu.Unlock()

	var b strings.Builder
	fmt.Fprintf(&b, "Epic #%d: %s\n", epic.IssueNumber, epic.Title)
	fmt.Fprintf(&b, "  Progress: %d/%d sub-issues complete (%.0f%%)\n",
		completed, len(epic.SubIssues), pct)

	barWidth := 20
	filled := int(progress * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	b.WriteString("  [")
	for i := 0; i < filled; i++ {
		b.WriteRune('█')
	}
	for i := filled; i < barWidth; i++ {
		b.WriteRune('░')
	}
	b.WriteString("]\n")

	if next, err := c.NextIssue(epic); err == nil {
		fmt.Fprintf(&b, "  Next issue: #%d\n", next)
	} else {
		b.WriteString("  Next issue: all complete\n")
	}
	return b.String()
}

// nilLoader is the default no-op loader.
type nilLoader struct{}

func (nilLoader) LoadEpic(issueNumber int) (*Epic, error) {
	return nil, ErrEpicNotFound
}

func (nilLoader) LoadDependencies(issueNumber int) ([]int, error) {
	return nil, nil
}
