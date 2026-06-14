// SPDX-License-Identifier: MIT
// Purpose: Spec merging and conflict resolution for version management.
// Supports three-way merges, field-level conflict detection, and deterministic resolution.
// Docs: internal/spec/merge.go.doc.md
package spec

import (
	"fmt"
	"time"
)

// MergeStrategy defines how conflicts are resolved during merge.
type MergeStrategy string

const (
	// StrategyTheirsTakePrecedence: Accept all changes from the incoming spec
	StrategyTheirs MergeStrategy = "theirs"
	// StrategyOursTakePrecedence: Keep all existing changes
	StrategyOurs MergeStrategy = "ours"
	// StrategyManual: Mark conflicts for manual resolution
	StrategyManual MergeStrategy = "manual"
	// StrategyNewest: Use most recently updated field
	StrategyNewest MergeStrategy = "newest"
)

// MergeConflict represents a single field-level conflict during merge.
type MergeConflict struct {
	Field       string
	Base        interface{} // Common ancestor value
	Ours        interface{} // Current value
	Theirs      interface{} // Incoming value
	Resolution  interface{} // Resolved value (if resolved)
	IsResolved  bool
	Strategy    MergeStrategy
}

// MergeResult holds the outcome of a three-way merge operation.
type MergeResult struct {
	Merged      *Spec
	Conflicts   []MergeConflict
	HasConflicts bool
	MergeTime   time.Time
	Successful  bool
}

// MergeSpecs performs a three-way merge of specs: base (common ancestor),
// ours (current), theirs (incoming). Conflicts are resolved per strategy.
func MergeSpecs(base, ours, theirs *Spec, strategy MergeStrategy) *MergeResult {
	result := &MergeResult{
		Conflicts: []MergeConflict{},
		MergeTime: time.Now(),
	}

	// Create merged spec starting from 'ours'
	merged := *ours
	merged.UpdatedAt = time.Now()
	merged.Version++

	// Helper to detect conflict
	isConflict := func(baseVal, oursVal, theirsVal interface{}) bool {
		return baseVal != oursVal && baseVal != theirsVal && oursVal != theirsVal
	}

	// Merge Title
	if isConflict(base.Title, ours.Title, theirs.Title) {
		conflict := MergeConflict{
			Field:      "title",
			Base:       base.Title,
			Ours:       ours.Title,
			Theirs:     theirs.Title,
			Strategy:   strategy,
		}
		if resolveConflict(strategy, &conflict) {
			merged.Title = conflict.Resolution.(string)
			conflict.IsResolved = true
		}
		result.Conflicts = append(result.Conflicts, conflict)
	} else if ours.Title != theirs.Title {
		merged.Title = theirs.Title
	}

	// Merge Description
	if isConflict(base.Description, ours.Description, theirs.Description) {
		conflict := MergeConflict{
			Field:      "description",
			Base:       base.Description,
			Ours:       ours.Description,
			Theirs:     theirs.Description,
			Strategy:   strategy,
		}
		if resolveConflict(strategy, &conflict) {
			merged.Description = conflict.Resolution.(string)
			conflict.IsResolved = true
		}
		result.Conflicts = append(result.Conflicts, conflict)
	} else if ours.Description != theirs.Description {
		merged.Description = theirs.Description
	}

	// Merge Goals
	if isConflict(base.Goals, ours.Goals, theirs.Goals) {
		conflict := MergeConflict{
			Field:      "goals",
			Base:       base.Goals,
			Ours:       ours.Goals,
			Theirs:     theirs.Goals,
			Strategy:   strategy,
		}
		if resolveConflict(strategy, &conflict) {
			merged.Goals = conflict.Resolution.(string)
			conflict.IsResolved = true
		}
		result.Conflicts = append(result.Conflicts, conflict)
	} else if ours.Goals != theirs.Goals {
		merged.Goals = theirs.Goals
	}

	// Merge Constraints
	if isConflict(base.Constraints, ours.Constraints, theirs.Constraints) {
		conflict := MergeConflict{
			Field:      "constraints",
			Base:       base.Constraints,
			Ours:       ours.Constraints,
			Theirs:     theirs.Constraints,
			Strategy:   strategy,
		}
		if resolveConflict(strategy, &conflict) {
			merged.Constraints = conflict.Resolution.(string)
			conflict.IsResolved = true
		}
		result.Conflicts = append(result.Conflicts, conflict)
	} else if ours.Constraints != theirs.Constraints {
		merged.Constraints = theirs.Constraints
	}

	// Merge Dependencies (list-based merge)
	if !equalStringSlices(ours.Dependencies, theirs.Dependencies) {
		merged.Dependencies = mergeStringSlices(ours.Dependencies, theirs.Dependencies)
	}

	// Merge Status
	if ours.Status != theirs.Status && base.Status == ours.Status {
		// Ours hasn't changed, use theirs
		merged.Status = theirs.Status
	} else if ours.Status != theirs.Status && base.Status != ours.Status {
		// Conflict: both sides changed
		conflict := MergeConflict{
			Field:      "status",
			Base:       base.Status,
			Ours:       ours.Status,
			Theirs:     theirs.Status,
			Strategy:   strategy,
		}
		if resolveConflict(strategy, &conflict) {
			merged.Status = conflict.Resolution.(SpecStatus)
			conflict.IsResolved = true
		}
		result.Conflicts = append(result.Conflicts, conflict)
	}

	result.Merged = &merged
	result.HasConflicts = len(result.Conflicts) > 0
	result.Successful = !result.HasConflicts || (result.HasConflicts && allResolved(result.Conflicts))

	return result
}

// resolveConflict applies merge strategy to a conflict and sets the Resolution field.
// Returns true if conflict was resolved, false if unresolved.
func resolveConflict(strategy MergeStrategy, conflict *MergeConflict) bool {
	switch strategy {
	case StrategyOurs:
		conflict.Resolution = conflict.Ours
		return true
	case StrategyTheirs:
		conflict.Resolution = conflict.Theirs
		return true
	case StrategyNewest:
		// For demonstration, theirs "wins" (in production, use timestamp metadata)
		conflict.Resolution = conflict.Theirs
		return true
	case StrategyManual:
		// Manual resolution requires user input; don't auto-resolve
		return false
	default:
		return false
	}
}

// allResolved checks if all conflicts in a list are resolved.
func allResolved(conflicts []MergeConflict) bool {
	for _, c := range conflicts {
		if !c.IsResolved {
			return false
		}
	}
	return true
}

// equalStringSlices compares two string slices for equality.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// mergeStringSlices performs set-union on two string slices.
func mergeStringSlices(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, s := range a {
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}

	for _, s := range b {
		if !seen[s] {
			result = append(result, s)
			seen[s] = true
		}
	}

	return result
}

// FastForward checks if a merge can be a fast-forward (theirs is ahead of ours).
func FastForward(ours, theirs *Spec) bool {
	// Fast-forward if theirs has higher version and newer timestamp
	return theirs.Version > ours.Version && theirs.UpdatedAt.After(ours.UpdatedAt)
}

// ConflictSummary returns a human-readable summary of conflicts.
func (mr *MergeResult) ConflictSummary() string {
	if !mr.HasConflicts {
		return "No conflicts"
	}
	return fmt.Sprintf("%d conflict(s): %d resolved, %d unresolved",
		len(mr.Conflicts),
		countResolved(mr.Conflicts),
		len(mr.Conflicts)-countResolved(mr.Conflicts),
	)
}

// countResolved returns the number of resolved conflicts.
func countResolved(conflicts []MergeConflict) int {
	count := 0
	for _, c := range conflicts {
		if c.IsResolved {
			count++
		}
	}
	return count
}

// String returns a detailed multi-line report of the merge result.
func (mr *MergeResult) String() string {
	var result string
	result = fmt.Sprintf("Merge Result: %v\n", mr.Successful)
	result += mr.ConflictSummary() + "\n"

	if mr.HasConflicts {
		result += "\nConflicts:\n"
		for i, c := range mr.Conflicts {
			result += fmt.Sprintf("  [%d] %s:\n", i+1, c.Field)
			result += fmt.Sprintf("      Ours:   %v\n", c.Ours)
			result += fmt.Sprintf("      Theirs: %v\n", c.Theirs)
			if c.IsResolved {
				result += fmt.Sprintf("      → Resolved: %v (via %s)\n", c.Resolution, c.Strategy)
			}
		}
	}

	return result
}
