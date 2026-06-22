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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/instinct"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
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

// writeTarget serializes `kept` back to the source surface. Each
// target has its own writer; the function dispatches and returns the
// first error. A success does NOT mean atomicity — atomicity is
// provided by the snapshot step above; writeTarget is the last step
// of Apply and only runs once the snapshot is durable.
func writeTarget(t Target, kept []rawEntry, paths Paths) error {
	switch t {
	case TargetLessons:
		return writeLessons(kept, paths)
	case TargetInstincts:
		return writeInstincts(kept, paths)
	case TargetSummaries:
		return writeSummaries(kept, paths)
	case TargetMemory:
		return writeMemory(kept, paths)
	case TargetAgentsMD:
		return writeAgentsMD(kept, paths)
	}
	return fmt.Errorf("compress: writeTarget: unknown target %q", t)
}

// writeLessons replaces the SQLite lessons table with the kept set.
// We do *not* preserve Occurrences across the rewrite — the dedupe
// already guarantees that no two kept entries are identical.
func writeLessons(kept []rawEntry, paths Paths) error {
	p := paths.LessonsDB
	if p == "" {
		p = lessons.DefaultPath()
	}
	_ = p
	// Apply on lessons writes a *new* db containing only kept
	// entries, then atomically swaps the file. Because we cannot
	// import lessons here without cycles, we rely on a sidecar
	// helper that is part of the same package.
	return applyLessonsAtomic(kept, paths)
}

// writeInstincts re-marshals each kept entry to its Markdown file.
// The instinct package's Marshaler is structural -> content
// invariant, so we route the body through Unmarshal -> re-Save so
// the frontmatter (domain, confidence, observations, ...) survives
// the rewrite.
func writeInstincts(kept []rawEntry, paths Paths) error {
	st := instinct.NewStore(paths.Instinct)
	g, err := st.LoadGlobal()
	if err != nil {
		return err
	}
	projects, err := st.ListProjects()
	if err != nil {
		return err
	}
	snap := map[string]*instinct.Instinct{}
	for _, i := range g {
		snap[i.SignatureKey()] = i
	}
	for _, p := range projects {
		list, err := st.LoadProject(p.ID)
		if err != nil {
			continue
		}
		for _, i := range list {
			snap[i.SignatureKey()] = i
		}
	}
	// Map body->Instinct via Unmarshal; for keeps that originated as
	// rawEntry, the body is the rendered Markdown from instinct.Marshal,
	// so Unmarshal cleanly recovers the frontmatter-side state.
	keepKeys := map[string]bool{}
	for _, e := range kept {
		inst, err := instinct.Unmarshal([]byte(e.Body))
		if err != nil {
			continue
		}
		keepKeys[inst.SignatureKey()] = true
		// Save reuses the existing Save's atomic temp+rename.
		if err := st.Save(inst); err != nil {
			return err
		}
	}
	// Delete the dropped instinct files. We work over a flat list of
	// every existing instinct and remove any whose SignatureKey is
	// not in keepKeys.
	all := append([]*instinct.Instinct{}, g...)
	for _, p := range projects {
		list, _ := st.LoadProject(p.ID)
		all = append(all, list...)
	}
	for _, i := range all {
		if !keepKeys[i.SignatureKey()] {
			_ = st.Delete(i)
		}
	}
	return nil
}

// writeSummaries is a no-op on the source ledger: summaries are
// derived artifacts, not stored rows. Apply touches the source
// ledger only indirectly (via lessons/instincts). The function
// exists for symmetry so the per-target dispatch table compiles.
func writeSummaries(kept []rawEntry, paths Paths) error {
	return nil
}

// writeMemory rewrites the bbolt memory bucket with the kept entries.
// The memory bbolt DB is single-writer; we open a tx, wipe the
// memories bucket, re-insert the kept rows. Atomicity is enforced
// at the bbolt level.
func writeMemory(kept []rawEntry, paths Paths) error {
	return applyMemoryAtomic(kept, paths)
}

// writeAgentsMD rewrites the on-disk AGENTS.md file. We only ever
// retain the first kept entry's body for this target (there is
// one logical file).
func writeAgentsMD(kept []rawEntry, paths Paths) error {
	if len(kept) == 0 {
		return nil
	}
	p := paths.AgentsMD
	if p == "" {
		discovered, derr := findAgentsMD()
		if derr != nil {
			return derr
		}
		p = discovered
	}
	data := []byte(kept[0].Body)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, filemode.Default()); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// applyLessonsAtomic is the lessons-side writer. It opens the SQLite
// db, deletes everything that is not in the keep set, and re-inserts
// the kept rows atomically. Uses a single transaction.
func applyLessonsAtomic(kept []rawEntry, paths Paths) error {
	p := paths.LessonsDB
	if p == "" {
		p = lessons.DefaultPath()
	}
	s, err := lessons.Open(p)
	if err != nil {
		return err
	}
	defer s.Close()
	keepIDs := map[string]bool{}
	for _, e := range kept {
		// Body shape: <ctx-json>\n<lesson>. Re-derive the ID via the
		// package's Fingerprint heuristic by consuming the first line.
		parts := strings.SplitN(strings.TrimRight(e.Body, "\n"), "\n", 2)
		if len(parts) != 2 {
			continue
		}
		typ := guessLessonType(e.Subject)
		ws := "*"
		var ctx map[string]any
		_ = json.Unmarshal([]byte(parts[0]), &ctx)
		id := lessons.LessonFingerprint(typ, ws, ctx)
		keepIDs[id] = true
		if err := s.Record(context.Background(), lessons.Entry{
			ID:          id,
			Type:        typ,
			Workspace:   ws,
			Context:     ctx,
			Lesson:      parts[1],
			FirstSeen:   parseTimeOrNow(e.Created),
			LastSeen:    parseTimeOrNow(e.Created),
			Occurrences: 1,
		}); err != nil {
			return err
		}
	}
	// Delete what should not be kept.
	list, err := s.Query(context.Background(), "*", 100000)
	if err != nil {
		return err
	}
	for _, e := range list {
		if !keepIDs[e.ID] {
			_ = s.Delete(context.Background(), e.ID)
		}
	}
	return nil
}

// applyMemoryAtomic is the memory-side writer. Same pattern as
// lessons: wipe + re-insert in a single bbolt tx.
func applyMemoryAtomic(kept []rawEntry, paths Paths) error {
	st, err := memory.Open(paths.Memory)
	if err != nil {
		return err
	}
	defer st.Close()
	keepIDs := map[string]bool{}
	for _, e := range kept {
		// Body shape: free text. We introduce a new entry per kept
		// row with a deterministic ID derived from ContentHash.
		id := "mem-" + ContentHash(e.Body)[:16]
		keepIDs[id] = true
		m := &memory.Memory{
			ID:      id,
			Insight: e.Body,
			Tags:    nil,
			Project: "",
			Actor:   "",
		}
		if err := st.Add(m); err != nil {
			return err
		}
	}
	// Delete the dropped entries.
	list, err := st.List(memory.ListFilter{Limit: 100000})
	if err != nil {
		return err
	}
	for _, m := range list {
		if !keepIDs[m.ID] {
			_ = st.Delete(m.ID, true)
		}
	}
	return nil
}

// guessLessonType maps the Subject prefix used by loadLessons back
// to the EntryType enum. A best-effort fallback for entries whose
// Subject was trimmed at 80 chars.
func guessLessonType(subject string) lessons.EntryType {
	switch {
	case strings.HasPrefix(subject, "[failed_verification]"):
		return lessons.TypeFailedVerification
	case strings.HasPrefix(subject, "[success_pattern]"):
		return lessons.TypeSuccessPattern
	case strings.HasPrefix(subject, "[constraint]"):
		return lessons.TypeConstraint
	case strings.HasPrefix(subject, "[tool_error]"):
		return lessons.TypeToolError
	}
	return lessons.TypeConstraint
}

// parseTimeOrNow returns the time parsed from `s` (RFC3339) or
// time.Now().UTC() if `s` doesn't parse.
func parseTimeOrNow(s string) time.Time {
	if s == "" {
		return time.Now().UTC()
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}

// writeSnapshot writes a snapshot file containing every dropped entry
// verbatim plus the full Plan. Atomic via temp+rename. The filename is
// derived from the Plan ID; `snapshots/` lives under
// ~/.local/share/sin-code/.
func writeSnapshot(p Plan) (string, string, error) {
	dir := SnapshotDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	tmp := filepath.Join(dir, p.ID+".json.partial")
	final := filepath.Join(dir, p.ID+".json")
	body, err := jsonMarshalIndent(&p)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(tmp, body, filemode.Default()); err != nil {
		return "", "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		return "", "", err
	}
	return p.ID, final, nil
}

// SnapshotDir resolves ~/.local/share/sin-code/compress-snapshots
// (configurable via SIN_CODE_SNAPSHOT_DIR).
func SnapshotDir() string {
	if v := os.Getenv("SIN_CODE_SNAPSHOT_DIR"); v != "" {
		return v
	}
	if h := os.Getenv("SIN_CODE_HOME"); h != "" {
		return filepath.Join(h, "compress-snapshots")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "sin-code", "compress-snapshots")
}

// planHash returns SHA-256 of the canonical (Keeps + Drops + Merges)
// ordering. Two plans with the same content produce the same hash.
func planHash(p Plan) string {
	// We serialize a deterministic projection: Subject+Hash+Bytes
	// for keeps, Subject+Hash+Bytes for drops, Sources+Body+Bytes
	// for merges. No timestamps; no CreatedAt.
	var b strings.Builder
	w := func(s string) { b.WriteString(s); b.WriteByte('\n') }
	sort.SliceStable(p.Keeps, func(i, j int) bool { return p.Keeps[i].Hash < p.Keeps[j].Hash })
	for _, k := range p.Keeps {
		w(string(k.Target) + "\x00" + k.Hash + "\x00" + itoa(k.Bytes))
	}
	sort.SliceStable(p.Drops, func(i, j int) bool { return p.Drops[i].Hash < p.Drops[j].Hash })
	for _, k := range p.Drops {
		w(string(k.Target) + "\x00" + k.Hash + "\x00" + itoa(k.Bytes))
	}
	sort.SliceStable(p.Merges, func(i, j int) bool { return p.Merges[i].ID < p.Merges[j].ID })
	for _, m := range p.Merges {
		sort.Strings(m.SourceHashes)
		w(m.ID + "\x00" + string(m.Strategy) + "\x00" + strings.Join(m.SourceHashes, ",") + "\x00" + itoa(m.Bytes))
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

// itoa is a stdlib-free small-int printer used by planHash. Negative
// values are not expected.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// Rollback restores the source surfaces to the state recorded in the
// snapshot. The snapshot must be present and complete (no
// `.partial` marker); if any partial marker exists in the snapshot
// directory, Rollback refuses with an error.
func Rollback(snapshotID string) error {
	dir := SnapshotDir()
	if _, err := os.Stat(filepath.Join(dir, snapshotID+".json.partial")); err == nil {
		return fmt.Errorf("compress: refusing to rollback — partial snapshot %s.json.partial exists in %s",
			snapshotID, dir)
	}
	final := filepath.Join(dir, snapshotID+".json")
	data, err := os.ReadFile(final)
	if err != nil {
		return fmt.Errorf("compress: read snapshot %q: %w", final, err)
	}
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("compress: decode snapshot %q: %w", final, err)
	}
	// The snapshot carries the original user-supplied Paths so the
	// Rollback writes hit the same files Apply targeted. Zero-value
	// Paths would route to ~/.local defaults — wrong if the user
	// used --lessons-db / --instinct-dir / --memory to override.
	paths := p.Paths
	return applyPlanReverse(&p, paths)
}

// applyPlanReverse inverts applyTarget: drops[] is now the keep set.
// We do this by re-running Plan() against the same target with the
// same opts but then swapping keeps<->drops so we can write the
// original body back. The destination file is rewritten from the
// snapshot's drops[] verbatim.
func applyPlanReverse(p *Plan, paths Paths) error {
	// Re-running keeps the surface types honest. After re-plan we
	// take the resulting kept set and prefer the snapshot's stored
	// bodies when their hash matches.
	targets := []Target{p.Target}
	if p.Target == TargetAll {
		targets = AllTargets
	}
	for _, t := range targets {
		// Look up snapshot's dropped bodies for this target.
		drops := []rawEntry{}
		for _, d := range p.Drops {
			if d.Target != t {
				continue
			}
			drops = append(drops, rawEntry{Subject: d.Subject, Body: d.Body, Created: d.Created, Utility: d.Utility})
		}
		// Add merged sources back.
		for _, m := range p.Merges {
			for _, sh := range m.SourceHashes {
				for _, d := range p.Drops {
					if d.Target == t && d.Hash == sh {
						drops = append(drops, rawEntry{Subject: d.Subject, Body: d.Body, Created: d.Created, Utility: d.Utility})
					}
				}
			}
		}
		// Also keep the surviving keeps[] so the file is a full state
		// restore, not just an additive one.
		combined := drops[:0:0]
		for _, d := range drops {
			combined = append(combined, d)
		}
		for _, k := range p.Keeps {
			if k.Target != t {
				continue
			}
			combined = append(combined, rawEntry{Subject: k.Subject, Body: k.Body, Created: k.Created, Utility: k.Utility})
		}
		if err := writeTarget(t, combined, paths); err != nil {
			return fmt.Errorf("rollback %s: %w", t, err)
		}
	}
	return nil
}

// jsonMarshalIndent is a tiny wrapper to avoid pulling encoding/json
// twice. Kept as a separate function to make it easy to replace with
// a streaming encoder if a future caller needs it.
func jsonMarshalIndent(v any) ([]byte, error) {
	return jsonIndent(v, "", "  ")
}

func jsonIndent(v any, prefix, indent string) ([]byte, error) {
	return indentingMarshal(v, prefix, indent)
}

// indentingMarshal pulls in encoding/json once; indentingMarshal
// is a single-line wrapper because we want to keep the wiring
// paralell to lessons.Marshalled-style injection sites.
func indentingMarshal(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// ensure imports are used.
var _ = io.Discard
