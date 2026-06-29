// SPDX-License-Identifier: MIT
// Purpose: Plan/Apply/Rollback orchestration. The engine that wires the
// loaders, the deterministic pass, and the optional LLM summarization.
// All on-disk side effects are gated behind Apply — Plan never writes.
// Step contract:
//  1. Plan(target, strategy, opts) -> (Plan, error)         // read-only
//  2. Apply(plan, opts)              -> (ApplyReport, error) // atomic
//  3. Rollback(snapshotID)           -> error                // restorative
//
// Atomicity guarantee: Apply writes a snapshot to a `.partial` file,
// renames it once fully fsync'd, then performs the destination rewrite
// (one per target). Rollback discovers a `.partial` (incomplete) and
// refuses to consume it to keep the user from restoring half a state.
//
// Lossless guarantee: drops[] + merged-source-hashes[] are persisted
// verbatim in the snapshot. Rollback restores them byte-for-byte.
package compress

import (
	"fmt"
	"time"
)

// BuildPlan is the public Plan() entry point. Reads the source
// surfaces, classifies entries, returns a Plan describing what would
// change if Apply were called. `--dry-run` stops here. The function is
// named BuildPlan (verb form) so the `Plan` type remains the noun.
func BuildPlan(target Target, strategy Strategy, paths Paths, opts PlanOptions) (Plan, error) {
	if !strategy.IsValid() {
		return Plan{}, fmt.Errorf("compress: unknown strategy %q (use: deterministic|llm|hybrid)", strategy)
	}
	if strategy == "" {
		strategy = StrategyDeterministic
	}
	targets, err := expandTargets(target)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{
		Target:   target,
		Strategy: strategy,
		Warnings: []string{},
		Paths:    paths,
	}
	now := opts.now().Format(time.RFC3339)
	p.CreatedAt = now
	for _, t := range targets {
		entries, warnings, err := load(t, paths)
		if err != nil {
			return Plan{}, fmt.Errorf("compress: load %s: %w", t, err)
		}
		set := normalize(entries, t)
		keeps, drops, warns := deterministic(set, opts)
		p.Warnings = append(p.Warnings, warnings...)
		p.Warnings = append(p.Warnings, warns...)
		// Hashes for keeps (already stable).
		p.Keeps = append(p.Keeps, keeps...)
		p.Drops = append(p.Drops, drops...)
		// Stats roll-up (per-target, single value for "all").
		p.Stats.OriginalBytes += set.originalBytes
		p.Stats.OriginalEntries += len(set.entries)
		p.Stats.Keeps += len(keeps)
		p.Stats.Drops += len(drops)
	}
	// LLM summarization step (StrategyLLM and StrategyHybrid).
	// Hybrid first runs deterministic above and only then asks the
	// LLM to compress the residual drops into a single merged entry.
	if strategy == StrategyLLM || strategy == StrategyHybrid {
		llm, err := NewLLMSummarizer(nil) // nil: defaults to env-resolved client
		if err != nil || !llm.Available() {
			p.Warnings = append(p.Warnings,
				"llm: no usable provider (set SIN_LLM_BASE_URL + key, or pass --no-llm); skipping llm pass")
		} else {
			merge, err := llm.MergeDrops(p.Drops, MergeOpts{TargetRatio: 0.5})
			if err == nil && merge != nil {
				p.Merges = append(p.Merges, *merge)
				p.Stats.Merges++
			}
		}
	}
	// Projected stats — what the final Apply would produce.
	p.Stats.ProjectedBytes = 0
	for _, k := range p.Keeps {
		p.Stats.ProjectedBytes += k.Bytes
	}
	for _, m := range p.Merges {
		p.Stats.ProjectedBytes += m.Bytes
	}
	p.Stats.ProjectedEntries = len(p.Keeps) + len(p.Merges)
	if p.Stats.OriginalBytes > 0 {
		p.Stats.ProjectedRatio = float64(p.Stats.ProjectedBytes) / float64(p.Stats.OriginalBytes)
	}
	// PlanHash is content-addressed across (entries + drops + merges).
	p.PlanHash = planHash(p)
	p.ID = idFor(p.Target, p.PlanHash)
	return p, nil
}

// Apply executes a Plan. Atomic-style: snapshot first (to .partial,
// then atomic rename), then target rewrites ordered by Plan.Stats.
// Returns an ApplyReport which the CLI renders.
func Apply(p Plan, paths Paths, opts ApplyOptions) (ApplyReport, error) {
	if opts.DryRun {
		return dryReport(p), nil
	}
	snapID, snapPath, err := writeSnapshot(p)
	if err != nil {
		return ApplyReport{}, fmt.Errorf("compress: snapshot: %w", err)
	}
	rep := ApplyReport{
		PlanID:        p.ID,
		SnapshotID:    snapID,
		SnapshotPath:  snapPath,
		AppliedAt:     time.Now().UTC().Format(time.RFC3339),
		OriginalBytes: p.Stats.OriginalBytes,
	}
	rep.PerTarget = make([]TargetReport, 0, 4)

	// Apply per target — we re-derive which entries to keep by
	// hashing the post-Plan keeps[] and looking them up in the source.
	for _, t := range AllTargets {
		if t != p.Target && p.Target != TargetAll {
			continue
		}
		tr, err := applyTarget(t, p, paths)
		if err != nil {
			// Restore from snapshot + surface error.
			_ = Rollback(snapID)
			return ApplyReport{}, fmt.Errorf("compress: apply %s: %w", t, err)
		}
		rep.PerTarget = append(rep.PerTarget, tr)
	}
	rep.KeptBytes = p.Stats.ProjectedBytes
	if p.Stats.OriginalBytes > 0 {
		rep.Ratio = float64(rep.KeptBytes) / float64(p.Stats.OriginalBytes)
	}
	return rep, nil
}

// dryReport is the in-memory ApplyReport returned for --dry-run. It
// does no file work; the caller (CLI) prints and exits.
func dryReport(p Plan) ApplyReport {
	rep := ApplyReport{
		PlanID:        p.ID,
		AppliedAt:     p.CreatedAt,
		OriginalBytes: p.Stats.OriginalBytes,
		KeptBytes:     p.Stats.ProjectedBytes,
		Ratio:         p.Stats.ProjectedRatio,
		Warnings:      p.Warnings,
	}
	rep.PerTarget = []TargetReport{{
		Target:        p.Target,
		BeforeEntries: p.Stats.OriginalEntries,
		AfterEntries:  p.Stats.ProjectedEntries,
		BeforeBytes:   p.Stats.OriginalBytes,
		AfterBytes:    p.Stats.ProjectedBytes,
	}}
	return rep
}
