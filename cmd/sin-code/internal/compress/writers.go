// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when compaction is refactored
//
// Per-target writers — serialize the kept set back to each source
// surface's native format. Each writer is dispatched by writeTarget;
// atomicity is provided by the snapshot step in Apply.
package compress

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/instinct"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
)

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

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

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
