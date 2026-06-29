// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when compaction is refactored
//
// Per-target apply logic — rewrites one target surface from the kept
// set plus any LLM merges. Targets are heterogeneous; each branch owns
// its source file format and is responsible for keeping the
// rolled-back-from-snapshot state trivially recoverable via Rollback.
package compress

import (
	"fmt"
	"strings"
	"time"
)

// applyTarget rewrites *one* target surface. Targets are heterogeneous
// — each branch owns its source file format and is responsible for
// keeping the rolled-back-from-snapshot state trivially recoverable via
// Rollback. We do nothing fancy on partial failure: an apply that
// fails to write the new state rolls back the snapshot to recover
// whatever was on disk before the apply attempt.
func applyTarget(t Target, p Plan, paths Paths) (TargetReport, error) {
	tr := TargetReport{Target: t}
	_, warnings, err := load(t, paths)
	if err != nil {
		return tr, fmt.Errorf("reload: %w", err)
	}
	_ = warnings
	// We re-load the *raw entries* by calling load() again with a
	// fresh result since load() returns warnings we collapse here.
	// In practice the load returns (entries, warnings, error); we
	// discard the warnings on re-entry.
	before, _, err := load(t, paths)
	if err != nil {
		return tr, fmt.Errorf("reload-entries: %w", err)
	}
	tr.BeforeEntries = len(before)
	tr.BeforeBytes = 0
	for _, e := range before {
		tr.BeforeBytes += len(e.Body)
	}
	keepHashes := hashesByTarget(t, p.Keeps)
	_ = mergesBySourceHash(t, p.Merges)
	kept := []rawEntry{}
	for _, e := range before {
		h := ContentHash(strings.TrimRight(e.Body, "\n"))
		if isMergedSource(t, h, p) {
			continue
		}
		_, keep := keepHashes[h]
		if keep {
			kept = append(kept, e)
		}
	}
	// Append merges whose source hashes are all from this target.
	for _, m := range p.Merges {
		all := true
		for _, sh := range m.SourceHashes {
			if !isFromTarget(t, sh, before) {
				all = false
				break
			}
		}
		if !all {
			continue
		}
		// Synthesize a rawEntry from the merge body so the per-target
		// writers can serialize uniformly.
		kept = append(kept, rawEntry{
			Subject: "[merge] " + m.ID,
			Body:    m.Body,
			Utility: 0.99,
			Created: time.Now().UTC().Format(time.RFC3339),
		})
	}
	// Each target writer knows how to serialize `kept` back to its
	// native format.
	if err := writeTarget(t, kept, paths); err != nil {
		return tr, fmt.Errorf("write: %w", err)
	}
	tr.AfterEntries = len(kept)
	tr.AfterBytes = 0
	for _, e := range kept {
		tr.AfterBytes += len(e.Body)
	}
	return tr, nil
}

// hashesByTarget returns a set of keep-hashes for a given target.
func hashesByTarget(t Target, keeps []PlanEntry) map[string]bool {
	out := map[string]bool{}
	for _, k := range keeps {
		if k.Target != t {
			continue
		}
		out[k.Hash] = true
	}
	return out
}

// mergesBySourceHash groups merges by their source-hash/target combo
// so applyTarget can mark source-hashes merged when iterating them.
func mergesBySourceHash(t Target, merges []PlanMerge) map[string]bool {
	out := map[string]bool{}
	for _, m := range merges {
		for _, sh := range m.SourceHashes {
			out[sh] = true
		}
	}
	return out
}

// isMergedSource reports whether the (target, hash) pair was consumed
// by one of the Plan's merges.
func isMergedSource(t Target, h string, p Plan) bool {
	for _, m := range p.Merges {
		for _, sh := range m.SourceHashes {
			if sh == h {
				return true
			}
		}
	}
	return false
}

// isFromTarget reports whether the given hash is the SHA-256 of any
// body in `before`. The target surface carries a unique list of
// entries; we identify membership by content hash.
func isFromTarget(t Target, h string, before []rawEntry) bool {
	for _, e := range before {
		if ContentHash(strings.TrimRight(e.Body, "\n")) == h {
			return true
		}
	}
	return false
}
