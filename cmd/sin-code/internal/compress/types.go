// SPDX-License-Identifier: MIT
// Package compress implements deterministic + LLM-assisted compaction
// for SIN-Code's long-lived memory stores (lessons, instincts, summaries,
// AGENTS.md-shaped memory). Compaction is lossless: every dropped entry
// is preserved verbatim in a JSON snapshot under
// ~/.local/share/sin-code/compress-snapshots/<id>.json. Rollback restores
// the dropped entries byte-for-byte.
//
// Hard requirements (per AGENTS.md):
//   - Output is reproducible. The same input must always produce the
//     same Plan, the same hashes, the same ApplyReport. No wall-clock
//     randomness sneaks into the algorithm.
//   - Snapshots are atomic. A crash mid-write either leaves the
//     snapshot file intact and the live store untouched (safe to Rollback),
//     or leaves a `.partial` marker that Rollback refuses to consume.
//   - The package depends only on stdlib + already-imported internal
//     packages (lessons, instinct, summary, memory, llm). CGO_ENABLED=0
//     preserved (M2).
//   - Plan is read-only; Apply is the only mutator. The CLI wires the
//     the two steps so LLM-extracted Plans are visible to the user
//     before they touch disk (`--dry-run` stops at Plan).
//
// Issue: sin-code compress (#172).
package compress

import (
	"crypto/sha256"
	"encoding/hex"
)

// Target names the source surface the compressor reads from.
type Target string

const (
	TargetLessons   Target = "lessons"
	TargetInstincts Target = "instincts"
	TargetSummaries Target = "summaries"
	TargetMemory    Target = "memory"    // AGENTS.md-shaped long-valued memory entries
	TargetAgentsMD  Target = "agents_md" // the on-disk AGENTS.md file at the workspace root
	TargetAll       Target = "all"       // every target above
)

// AllTargets lists the concrete (non-aggregate) targets.
var AllTargets = []Target{
	TargetLessons,
	TargetInstincts,
	TargetSummaries,
	TargetMemory,
	TargetAgentsMD,
}

// IsValid reports whether t is a recognized Target (incl. "all").
func (t Target) IsValid() bool {
	switch t {
	case TargetLessons, TargetInstincts, TargetSummaries, TargetMemory, TargetAgentsMD, TargetAll:
		return true
	}
	return false
}

// Strategy selects the algorithmic family used to compute a Plan.
//
//   - StrategyDeterministic: SHA-256 dedupe + byte-budgeted keep-recent.
//     No network, no LLM. Reproducible across runs and machines.
//   - StrategyLLM: ask the configured llm.Client to merge N oldest
//     entries into one summary entry. Requires a configured provider;
//     if the client cannot chat (no key / offline), the Plan falls back
//     to deterministic for the affected scope and surfaces a warning.
//   - StrategyHybrid: deterministic pass first; only residual chunks
//     above the byte budget get summarized by the LLM.
type Strategy string

const (
	StrategyDeterministic Strategy = "deterministic"
	StrategyLLM           Strategy = "llm"
	StrategyHybrid        Strategy = "hybrid"
)

// IsValid reports whether s is one of the three recognized strategies.
func (s Strategy) IsValid() bool {
	switch s {
	case StrategyDeterministic, StrategyLLM, StrategyHybrid:
		return true
	}
	return false
}

// Plan is the read-only description of a compaction. It is what `--dry-run`
// emits and what `Apply` consumes. Plans are content-addressed: PlanHash
// is computed over Entries[]+Drops[]+Merges[] without timestamps, so two
// Plans built from identical inputs but at different wall-clock times have
// the same PlanHash. Useful for the regression test suite.
type Plan struct {
	ID        string   `json:"id"`
	Target    Target   `json:"target"`
	Strategy  Strategy `json:"strategy"`
	CreatedAt string   `json:"created_at"`
	PlanHash  string   `json:"plan_hash"`

	// Original / Kept counts by target. Used for the CLI ratio table.
	Stats PlanStats `json:"stats"`

	// Entries to keep verbatim, in final order. Order is deterministic
	// (utility score desc, then by entry hash asc for ties — a stable
	// sort tiebreaker). Hash is the SHA-256 of the canonical body.
	Keeps []PlanEntry `json:"keeps"`

	// Entries to remove. After Apply, the removed bodies live on disk
	// under SnapshotPath (sha256 keyed) — Apply is therefore lossless.
	Drops []PlanEntry `json:"drops"`

	// Merges carry LLM-produced replacements: each Merge maps a set of
	// `SourceHashes` to a single new entry whose `Hash` is its canonical
	// SHA-256 after Apply. Markdown-formatted, with preservation markers.
	Merges []PlanMerge `json:"merges,omitempty"`

	// Paths is the snapshot of the source-overrides the caller used
	// during BuildPlan/Apply. Rollback uses it to find the same DB
	// files; without it, Rollback would fall back to defaults and
	// the user could roll back to the wrong file.
	Paths Paths `json:"paths,omitempty"`

	// Warnings are non-fatal advisories surfaced by --dry-run.
	Warnings []string `json:"warnings,omitempty"`
}

// PlanStats is the per-target breakdown surfaced by `--dry-run`.
type PlanStats struct {
	Keeps            int     `json:"keeps"`
	Drops            int     `json:"drops"`
	Merges           int     `json:"merges"`
	OriginalEntries  int     `json:"original_entries"`
	OriginalBytes    int     `json:"original_bytes"`
	ProjectedEntries int     `json:"projected_entries"`
	ProjectedBytes   int     `json:"projected_bytes"`
	ProjectedRatio   float64 `json:"projected_ratio"` // kept_bytes / original_bytes
}

// PlanEntry is the entry-shaped view a Plan exposes. Hash is canonical.
type PlanEntry struct {
	Hash    string  `json:"hash"`
	Target  Target  `json:"target"`
	Subject string  `json:"subject"` // human-readable label (lesson text, instinct id, summary title)
	Body    string  `json:"body"`    // raw entry body, normalized for hashing
	Bytes   int     `json:"bytes"`
	Utility float64 `json:"utility"`
	Created string  `json:"created"`
}

// PlanMerge describes one LLM-driven merge: a list of source hashes that
// map to a synthesized replacement body.
type PlanMerge struct {
	ID           string   `json:"id"`
	Strategy     Strategy `json:"strategy"`
	SourceHashes []string `json:"source_hashes"`
	Body         string   `json:"body"`
	Bytes        int      `json:"bytes"`
}

// ApplyOptions is the Apply input that pairs a Plan with execution knobs.
type ApplyOptions struct {
	// Reason is recorded in the snapshot manifest so audit logs can
	// correlate a compaction with the originating CLI invocation.
	Reason string

	// MinInterval prevents aggressive back-to-back Apply calls when
	// the user re-runs the CLI in a tight loop. Zero means "no gate";
	// the default Plan() caller passes zero.
	MinIntervalSeconds int

	// DryRun, if true, returns a successful ApplyReport with no
	// on-disk side effects. Useful for the CLI's `--dry-run`.
	DryRun bool
}

// ApplyReport is the emit-from-Apply result. Includes per-target byte
// deltas, the snapshot ID written (if any), and surviving entry hashes
// so the CLI can render "what changed" tables.
type ApplyReport struct {
	PlanID        string         `json:"plan_id"`
	SnapshotID    string         `json:"snapshot_id,omitempty"`
	SnapshotPath  string         `json:"snapshot_path,omitempty"`
	AppliedAt     string         `json:"applied_at"`
	OriginalBytes int            `json:"original_bytes"`
	KeptBytes     int            `json:"kept_bytes"`
	Ratio         float64        `json:"ratio"`
	PerTarget     []TargetReport `json:"per_target"`
	Warnings      []string       `json:"warnings,omitempty"`
}

// TargetReport is one row of the per-target breakdown.
type TargetReport struct {
	Target        Target `json:"target"`
	BeforeEntries int    `json:"before_entries"`
	AfterEntries  int    `json:"after_entries"`
	BeforeBytes   int    `json:"before_bytes"`
	AfterBytes    int    `json:"after_bytes"`
	SnapshotID    string `json:"snapshot_id,omitempty"`
}

// ContentHash returns the canonical SHA-256 hash for a body. Whitespace
// at end-of-line is trimmed, byte-order is preserved (no Unicode
// normalization — the bodies are technical text, NFC changes are
// surprising).
func ContentHash(body string) string {
	h := sha256.Sum256([]byte(body))
	return hex.EncodeToString(h[:])
}

// shortHash returns the first 16 hex chars of ContentHash. Used for
// in-memory keys and snapshot file IDs.
func shortHash(body string) string {
	return ContentHash(body)[:16]
}
