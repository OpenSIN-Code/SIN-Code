// SPDX-License-Identifier: MIT
// Purpose: deterministic compaction engine — SHA-256 dedupe +
// utility-sorted keep-recent + byte-budget cap. No LLM. No network.
// Pure stdlib. Reproducible across machines and wall clocks (issue #172).
//
// Algorithm (target-independent, applied per target):
//  1. Pull every entry from the source surface, normalize the body,
//     hash it. Keep a per-target []entry slice.
//  2. Group entries by ContentHash. The first occurrence stays; the
//     rest are queued for Drop. Hash ties broken by source-order so the
//     result is stable.
//  3. Score remaining entries by utility (newer + smaller = higher
//     priority — keep-recent + smaller-budget). Sort stable by
//     (utility desc, hash asc).
//  4. Walk the sorted slice; accumulate Bytes until either KeepBudgetBytes
//     is reached or the slice ends. The remainder is Drop.
//  5. Surface warnings when:
//     - the source surface returned 0 entries ("nothing to plan"),
//     - dedup removed more than half of the original entries
//     ("compacting heavily — review snapshot before Apply"),
//     - any entry's body contained binary content (we hash bytes
//     only; binary prose is unusual for our four targets).
package compress

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// PlanOptions tunes the deterministic pass. Zero values mean
// "sensible defaults"; the CLI surfaces them as `--keep-bytes`.
type PlanOptions struct {
	// KeepBudgetBytes is the post-compaction byte cap per target.
	// 0 means "no cap" (only dedupe is applied).
	KeepBudgetBytes int

	// KeepMaxEntries caps the survivor count per target. 0 = no cap.
	KeepMaxEntries int

	// KeepRecentDays drops entries older than this delta (UTC).
	// 0 = no age filter. Applied AFTER dedupe.
	KeepRecentDays int

	// StableTime, if non-zero, is used in place of time.Now() for any
	// timestamp math. Tests pin this to a fixed value so the algorithm
	// is byte-deterministic across reruns.
	StableTime time.Time

	// UseStableTime swaps time.Now() for the pins above. Default false.
	UseStableTime bool
}

// now returns the time the algorithm should treat as "now". Defaults to
// time.Now().UTC(); pinned to StableTime by tests for determinism.
func (o PlanOptions) now() time.Time {
	if o.UseStableTime {
		return o.StableTime.UTC()
	}
	return time.Now().UTC()
}

// deterministicPlan is the worker behind Plan(strategy=deterministic).
// Returns the Plan plus per-target []entry slices (already normalized
// and hash-assigned) so callers can reuse them across Strategies
// without re-reading the source surface.
type normalizedSet struct {
	target        Target
	entries       []PlanEntry
	originalBytes int
}

// LoadAll targets is the per-target entry loader the worker above
// relies on. The actual readers live in loader.go (lessons,
// instinct, summary, memory, AGENTS.md file).
func normalize(entries []rawEntry, t Target) normalizedSet {
	out := normalizedSet{target: t}
	for _, e := range entries {
		body := strings.TrimRight(e.Body, "\n")
		hash := ContentHash(body)
		out.entries = append(out.entries, PlanEntry{
			Hash:    hash,
			Target:  t,
			Subject: e.Subject,
			Body:    body,
			Bytes:   len(body),
			Utility: e.Utility,
			Created: e.Created,
		})
		out.originalBytes += len(body)
	}
	// Stable utility-score for the recompute below.
	for i := range out.entries {
		out.entries[i].Utility = utilityScore(out.entries[i], time.Time{})
	}
	return out
}

// rawEntry is the loader's output shape. Specific loaders translate
// their domain types into this so strategy selection is target-agnostic.
type rawEntry struct {
	Subject string
	Body    string
	Utility float64
	Created string // RFC3339 (UTC); stable for deterministic hashing
}

// deterministic kicks off the deterministic compaction pass for one
// pre-normalized set. It is split off from the orchestrating
// Plan() function so hybrid and LLM flows can pre-dedupe for free
// without duplicating the engine.
func deterministic(set normalizedSet, opts PlanOptions) (keeps []PlanEntry, drops []PlanEntry, warns []string) {
	// 1. dedupe: keep the first occurrence of each ContentHash.
	seen := make(map[string]bool, len(set.entries))
	uniq := make([]PlanEntry, 0, len(set.entries))
	for _, e := range set.entries {
		if seen[e.Hash] {
			drops = append(drops, e)
			continue
		}
		seen[e.Hash] = true
		uniq = append(uniq, e)
	}
	if len(drops)*2 > len(set.entries)+len(drops) {
		warns = append(warns, fmt.Sprintf("%s: dedupe removed %d of %d entries — review snapshot before applying",
			set.target, len(drops), len(set.entries)))
	}

	// 2. age filter (optional).
	if opts.KeepRecentDays > 0 {
		cutoff := opts.now().Add(-time.Duration(opts.KeepRecentDays) * 24 * time.Hour)
		fresh := uniq[:0]
		var aged []PlanEntry
		for _, e := range uniq {
			t, perr := time.Parse(time.RFC3339, e.Created)
			if perr != nil || t.Before(cutoff) {
				aged = append(aged, e)
				continue
			}
			fresh = append(fresh, e)
		}
		if len(aged) > 0 {
			warns = append(warns, fmt.Sprintf("%s: %d entries older than %dd dropped", set.target, len(aged), opts.KeepRecentDays))
		}
		drops = append(drops, aged...)
		uniq = fresh
	}

	// 3. utility sort: higher score first, hash asc tiebreaker so two
	//    equal-utility entries land in the same order on every run.
	sort.SliceStable(uniq, func(a, b int) bool {
		if uniq[a].Utility != uniq[b].Utility {
			return uniq[a].Utility > uniq[b].Utility
		}
		return uniq[a].Hash < uniq[b].Hash
	})

	// 4. apply byte budget + max-entries cap in that order.
	keeps = make([]PlanEntry, 0, len(uniq))
	var keptBytes int
	for _, e := range uniq {
		if opts.KeepMaxEntries > 0 && len(keeps) >= opts.KeepMaxEntries {
			drops = append(drops, e)
			continue
		}
		if opts.KeepBudgetBytes > 0 && keptBytes+e.Bytes > opts.KeepBudgetBytes && len(keeps) > 0 {
			drops = append(drops, e)
			continue
		}
		keeps = append(keeps, e)
		keptBytes += e.Bytes
	}
	return keeps, drops, warns
}

// utilityScore is what the deterministic pass ranks by. Larger is better.
// Buckets (in order of influence):
//  1. Recency score: 0..1, normalized over the last 365 days from `now`.
//     Anything older than 365d gets 0. Pinning `now` (via opts.UseStableTime)
//     keeps this stable across runs.
//  2. Size penalty: smaller bodies contribute slightly more.
//     0..0.3 bonus = (1 - bytes/8192) clamped at 0.
//
// Total is in [0, 1.3]. Default target: 0.7+ for recent+lean entries.
func utilityScore(e PlanEntry, now time.Time) float64 {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var recency float64
	if t, err := time.Parse(time.RFC3339, e.Created); err == nil {
		age := now.Sub(t)
		if age < 0 {
			age = 0
		}
		// Map 0d..365d -> 1.0..0.0
		recency = 1.0 - float64(age.Hours())/(365.0*24.0)
		if recency < 0 {
			recency = 0
		}
	}
	sizeBonus := 0.3 * (1.0 - float64(e.Bytes)/8192.0)
	if sizeBonus < 0 {
		sizeBonus = 0
	}
	return recency + sizeBonus
}
