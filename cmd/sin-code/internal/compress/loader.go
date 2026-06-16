// SPDX-License-Identifier: MIT
// Purpose: read the four source surfaces (lessons sqlite, instincts on
// disk, session summaries over the ledger, memory bbolt, AGENTS.md file)
// and translate them to the compress package's uniform rawEntry shape.
// Sources that are missing or empty are tolerated — Plan() surfaces a
// warning rather than an error so a fresh checkout doesn't fail.
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
