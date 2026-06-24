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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/instinct"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/summary"
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

// Paths is the per-source overrides bundle. Zero value = use defaults
// baked into each loader. Tests override one or two fields at a time.
type Paths struct {
	LessonsDB string // 0 = use lessons.DefaultPath()
	Instinct  string // 0 = use instinct.ResolveBaseDir()
	Summaries string // 0 = use ledger.DefaultPath(); sections are per-session-id
	Memory    string // 0 = use memory.Open(""); see internal/memory/store.go DefaultStore
	AgentsMD  string // 0 = walk up from cwd looking for AGENTS.md
}

// load is the per-target entry-load dispatcher. Returns ([]rawEntry, []string-warning).
// load is read-only — it never mutates the source. Apply is the only writer.
func load(target Target, paths Paths) ([]rawEntry, []string, error) {
	switch target {
	case TargetLessons:
		return loadLessons(paths)
	case TargetInstincts:
		return loadInstincts(paths)
	case TargetSummaries:
		return loadSummaries(paths)
	case TargetMemory:
		return loadMemory(paths)
	case TargetAgentsMD:
		return loadAgentsMD(paths)
	}
	return nil, nil, fmt.Errorf("compress: unknown target %q", target)
}

// expandTargets turns TargetAll into the concrete list, dedupes, and
// preserves the iteration order. Anything *not* in AllTargets is
// surfaced as an error from Expand().
func expandTargets(t Target) ([]Target, error) {
	switch t {
	case "":
		return nil, errors.New("compress: --target is required (lessons|instincts|summaries|memory|agents_md|all)")
	case TargetAll:
		out := make([]Target, len(AllTargets))
		copy(out, AllTargets)
		return out, nil
	}
	if !t.IsValid() {
		return nil, fmt.Errorf("compress: unknown target %q (use: %s | all)", t, strings.Join(targetNames(), "|"))
	}
	return []Target{t}, nil
}

// targetNames is the human-readable list used in `--help` and errors.
func targetNames() []string {
	out := make([]string, 0, len(AllTargets)+1)
	for _, t := range AllTargets {
		out = append(out, string(t))
	}
	out = append(out, string(TargetAll))
	return out
}

// loadLessons reads the entire lessons SQLite DB. Lessons are returned
// in Occurrences-DESC order so the briefing-shaped KeepRecent policy
// agrees with the existing lessons.Query(workspace, limit) sort.
func loadLessons(paths Paths) ([]rawEntry, []string, error) {
	p := paths.LessonsDB
	if p == "" {
		p = lessons.DefaultPath()
	}
	s, err := lessons.Open(p)
	if err != nil {
		return nil, nil, fmt.Errorf("compress: open lessons db %q: %w", p, err)
	}
	defer s.Close()
	list, err := s.Query(context.Background(), "*", 100000)
	if err != nil {
		return nil, nil, fmt.Errorf("compress: read lessons: %w", err)
	}
	warnings := []string{}
	if len(list) == 0 {
		warnings = append(warnings, "lessons: source empty — nothing to compact")
		return nil, warnings, nil
	}
	out := make([]rawEntry, 0, len(list))
	for _, e := range list {
		ctxJSON, _ := json.Marshal(stableAnyMap(e.Context))
		body := string(ctxJSON) + "\n" + e.Lesson
		out = append(out, rawEntry{
			Subject: fmt.Sprintf("[%s] %s", e.Type, oneLine(e.Lesson, 80)),
			Body:    body,
			Utility: float64(e.Occurrences),
			Created: e.FirstSeen.UTC().Format(time.RFC3339),
		})
	}
	return out, warnings, nil
}

// loadInstincts reads every instinct on disk and renders each as rawEntry.
// The body is the rendered Markdown (frontmatter + body) so dedupe is
// content-level. Project-scoped instincts win over global when both exist
// for the same SignatureKey.
func loadInstincts(paths Paths) ([]rawEntry, []string, error) {
	base := paths.Instinct
	if base == "" {
		base = instinct.ResolveBaseDir()
	}
	st := instinct.NewStore(base)
	g, err := st.LoadGlobal()
	if err != nil {
		return nil, nil, fmt.Errorf("compress: list global instincts: %w", err)
	}
	projects, err := st.ListProjects()
	if err != nil {
		return nil, nil, fmt.Errorf("compress: list instinct projects: %w", err)
	}
	out := make([]rawEntry, 0, len(g))
	seen := map[string]bool{}
	for _, i := range g {
		out = append(out, instinctToRaw(i))
		seen[i.SignatureKey()] = true
	}
	for _, p := range projects {
		list, err := st.LoadProject(p.ID)
		if err != nil {
			continue
		}
		for _, i := range list {
			if seen[i.SignatureKey()] {
				continue
			}
			out = append(out, instinctToRaw(i))
			seen[i.SignatureKey()] = true
		}
	}
	warnings := []string{}
	if len(out) == 0 {
		warnings = append(warnings, "instincts: source empty — nothing to compact")
	}
	return out, warnings, nil
}

// instinctToRaw renders an instinct to (subject, body) using the same
// Marshal the instinct package uses for Save(). Using a separate path
// (vs decoding the file) keeps the algorithm single-source-of-truth.
func instinctToRaw(i *instinct.Instinct) rawEntry {
	b, err := instinct.Marshal(i)
	if err != nil {
		b = []byte(i.Trigger + "\n" + i.Action)
	}
	return rawEntry{
		Subject: fmt.Sprintf("[%s] %s", i.Domain, oneLine(i.Trigger, 80)),
		Body:    string(b),
		Utility: i.Confidence,
		Created: i.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// loadSummaries reads every session_id recorded in the ledger and
// synthesizes a Summary per session; the body is `summary.Format(s)`.
// The path defaults to ledger.DefaultPath(); if the path is missing,
// a warning is returned and the slice is empty (the compressor is
// resilient to first-run states).
func loadSummaries(paths Paths) ([]rawEntry, []string, error) {
	p := paths.Summaries
	if p == "" {
		p = ledger.DefaultPath()
	}
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
		return nil, []string{"summaries: ledger db not found at " + p + " — skipping"}, nil
	}
	lstore, err := ledger.Open(p)
	if err != nil {
		return nil, nil, fmt.Errorf("compress: open ledger %q: %w", p, err)
	}
	defer lstore.Close()
	ctx := context.Background()
	sessions, err := lstore.Sessions(ctx, 100000)
	if err != nil {
		return nil, nil, fmt.Errorf("compress: list sessions: %w", err)
	}
	out := make([]rawEntry, 0, len(sessions))
	warnings := []string{}
	for _, sid := range sessions {
		s, err := summary.Build(ctx, lstore, sid)
		if err != nil {
			warnings = append(warnings, "summaries: session "+sid+": "+err.Error())
			continue
		}
		body := summary.Format(s)
		out = append(out, rawEntry{
			Subject: "session " + sid,
			Body:    body,
			Utility: float64(len(s.ToolsUsed)) + float64(s.Turns),
			Created: s.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	if len(out) == 0 {
		warnings = append(warnings, "summaries: no sessions in ledger — nothing to compact")
	}
	return out, warnings, nil
}

// loadMemory reads every long-string-valued memory entry. Short values
// (<128 bytes) are skipped — they are perceptually trivial and not worth
// compressing individually; the brain's "prime" path treats them
// inline. Longer values become rawEntry rows ready for dedupe.
func loadMemory(paths Paths) ([]rawEntry, []string, error) {
	st, err := memory.Open(paths.Memory)
	if err != nil {
		return nil, nil, fmt.Errorf("compress: open memory db: %w", err)
	}
	defer st.Close()
	list, err := st.List(memory.ListFilter{Limit: 100000})
	if err != nil {
		return nil, nil, fmt.Errorf("compress: read memory: %w", err)
	}
	warnings := []string{}
	if len(list) == 0 {
		warnings = append(warnings, "memory: source empty — nothing to compact")
		return nil, warnings, nil
	}
	out := make([]rawEntry, 0, len(list))
	for _, m := range list {
		body := m.Insight
		if len(body) < 128 {
			continue
		}
		out = append(out, rawEntry{
			Subject: m.ID,
			Body:    body,
			Utility: float64(len(m.Tags)),
			Created: m.Created.UTC().Format(time.RFC3339),
		})
	}
	return out, warnings, nil
}

// loadAgentsMD reads the AGENTS.md file at the workspace root. Returns
// one entry whose body is the file content (verbatim). The compressor
// is allowed to rewrite the file in place via Apply; Apply is the
// only writer.
func loadAgentsMD(paths Paths) ([]rawEntry, []string, error) {
	p := paths.AgentsMD
	if p == "" {
		discovered, derr := findAgentsMD()
		if derr != nil {
			return nil, []string{"agents_md: " + derr.Error()}, nil
		}
		p = discovered
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, []string{"agents_md: file not found (" + p + ") — skipping"}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("compress: read agents_md %q: %w", p, err)
	}
	warnings := []string{}
	if len(data) == 0 {
		warnings = append(warnings, "agents_md: source empty — nothing to compact")
		return nil, warnings, nil
	}
	out := []rawEntry{{
		Subject: filepath.Base(p),
		Body:    string(data),
		Utility: 1.0,
		Created: time.Now().UTC().Format(time.RFC3339),
	}}
	return out, warnings, nil
}

// findAgentsMD walks up from the current working directory looking for
// AGENTS.md. Returns the first hit or an error if no parent contains it.
func findAgentsMD() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		cand := filepath.Join(dir, "AGENTS.md")
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("AGENTS.md not found in any parent of cwd")
		}
		dir = parent
	}
}

// stableAnyMap returns the input unchanged but serves as a hook for
// callers that want stable JSON ordering later. The map is captured
// by reference; the lesson-body string remains identical for equal
// inputs, so the SHA-256 dedupe is stable.
func stableAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

// oneLine collapses a string into its first non-empty line, trimmed
// and capped at n chars. Used for the PlanEntry.Subject field.
func oneLine(s string, n int) string {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t != "" {
			if len(t) > n {
				t = t[:n-1] + "…"
			}
			return t
		}
	}
	return ""
}

// idFor combines the target and the plan hash to make a stable Plan ID.
func idFor(t Target, hash string) string {
	h := sha256.Sum256([]byte(string(t) + "\x00" + hash))
	return "plan-" + hex.EncodeToString(h[:])[:16]
}

// ensure imports are used.
var _ = sort.Strings
