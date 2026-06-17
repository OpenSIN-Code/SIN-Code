// SPDX-License-Identifier: MIT
// Purpose: deterministic snapshot/write/read of a CompareReport so
// the harness can produce byte-stable output for CI review (the
// caveman evals pattern, issue #171 §1.1). All maps are sorted by
// key, all slices are sorted by ID, and floats are normalised via
// the Cost() rounding helper in prices.go.
//
// The round-trip is "write JSON, read JSON, re-normalise, byte-equal
// after deterministic reorder". We do NOT depend on the global map
// iteration order of encoding/json — we replace every map with a
// flat pair-list at the struct boundary so the on-disk bytes are
// ordered.
//
// Docs: snapshot.doc.md
package evalharness

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
)

// SnapshotRow is one median row of the matrix — one per arm. Rows
// are sorted by ArmID so a written snapshot and a re-read snapshot
// diff cleanly. Median is taken across the per-case values (LOC
// requires `len(values)` to be odd-friendly; ties break in favour
// of the lower index, the caveman measure.py convention).
type SnapshotRow struct {
	ArmID            string  `json:"arm_id"`
	TotalCases       int     `json:"total_cases"`
	Passed           int     `json:"passed"`
	MedianLOC        int     `json:"median_loc"`
	MedianLatencyMS  int     `json:"median_latency_ms"`
	MedianUSD        float64 `json:"median_usd"`
	MedianTokens     int     `json:"median_tokens"`
	MedianScore      float64 `json:"median_score"`
	PassRate         float64 `json:"pass_rate"`
	WeightedScore    float64 `json:"weighted_score"`
	FirstToPassRate  float64 `json:"first_to_pass_rate,omitempty"`
	SkillName        string  `json:"skill_name,omitempty"`
	VerbosityLevel   string  `json:"verbosity,omitempty"`
	SystemPromptHash string  `json:"system_prompt_hash,omitempty"` // sha-like fingerprint; see header
}

// SnapshotHeader carries fixed metadata about a snapshot run so the
// comparator output is reproducible-by-rule. Generation timestamp
// is intentionally NOT recorded: a deterministic snapshot must
// diff cleanly between CI runs, even when they happen 30 days
// apart.
type SnapshotHeader struct {
	SetName       string `json:"set_name"`
	SinCodeVer    string `json:"sin_code_version"`
	SchemaVersion int    `json:"schema_version"`
	PromptHeader  string `json:"prompt_header,omitempty"` // e.g. "ponytail/benchmarks/README.md:34-58"
}

// Snapshot is the on-disk representation of one matrix run.
type Snapshot struct {
	Header   SnapshotHeader `json:"header"`
	Rows     []SnapshotRow  `json:"rows"`               // sorted by ArmID
	Warnings []string       `json:"warnings,omitempty"` // unknown pricing names, etc.
}

// SnapshotSchemaVersion is the byte-stable schema version. Bump when
// you add or rename a column; the comparator re-emits the snapshot
// and CI reviewers see a clean diff.
const SnapshotSchemaVersion = 1

// WriteSnapshot renders report into a Snapshot, then writes to w.
// Empty report produces an empty snapshot (no error) so the CLI can
// emit `"rows": []` instead of failing the run.
func WriteSnapshot(w io.Writer, report CompareReport, header SnapshotHeader) error {
	if w == nil {
		return errors.New("snapshot: nil writer")
	}
	snap := BuildSnapshot(report, header)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(snap)
}

// WriteSnapshotFile writes to path with 0644 mode. MkdirAll on the
// parent so deeply-nested output paths "just work".
func WriteSnapshotFile(path string, report CompareReport, header SnapshotHeader) error {
	if path == "" {
		return errors.New("snapshot: empty path")
	}
	dir := pathDir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil { // #nosec G301 — standard snapshot output mode
		return fmt.Errorf("snapshot: mkdir %s: %w", dir, err)
	}
	f, err := os.Create(path) // #nosec G304 — caller-controlled path, snapshot output
	if err != nil {
		return fmt.Errorf("snapshot: create %s: %w", path, err)
	}
	defer f.Close()
	return WriteSnapshot(f, report, header)
}

// LoadSnapshot reads a Snapshot from r. Strict — unknown schema
// versions are rejected. The comparator never reads a snapshot it
// didn't write its own; lints "data drift" early before the diff
// shows garbage.
func LoadSnapshot(r io.Reader) (Snapshot, error) {
	if r == nil {
		return Snapshot{}, errors.New("snapshot: nil reader")
	}
	var snap Snapshot
	if err := json.NewDecoder(r).Decode(&snap); err != nil {
		return Snapshot{}, fmt.Errorf("snapshot: decode: %w", err)
	}
	if snap.Header.SchemaVersion == 0 {
		snap.Header.SchemaVersion = SnapshotSchemaVersion
	}
	if snap.Header.SchemaVersion != SnapshotSchemaVersion {
		return Snapshot{}, fmt.Errorf("snapshot: schema version %d != %d",
			snap.Header.SchemaVersion, SnapshotSchemaVersion)
	}
	// Re-sort on read defensively against hand-edited files.
	sort.SliceStable(snap.Rows, func(i, j int) bool { return snap.Rows[i].ArmID < snap.Rows[j].ArmID })
	return snap, nil
}

// LoadSnapshotFile is the file wrapper.
func LoadSnapshotFile(path string) (Snapshot, error) {
	if path == "" {
		return Snapshot{}, errors.New("snapshot: empty path")
	}
	f, err := os.Open(path) // #nosec G304 — caller-controlled path, snapshot input
	if err != nil {
		return Snapshot{}, fmt.Errorf("snapshot: open %s: %w", path, err)
	}
	defer f.Close()
	return LoadSnapshot(f)
}

// BuildSnapshot converts a CompareReport into the on-disk snapshot.
// Rows are sorted by ArmID, totals accumulated from TotalsByArm.
// Per-case values are not stored individually — only the median row
// survives to keep the snapshot small (caveman evals/measure.py:
// "median across prompts").
func BuildSnapshot(report CompareReport, header SnapshotHeader) Snapshot {
	snap := Snapshot{Header: header, Rows: nil, Warnings: report.Warnings}
	if header.SchemaVersion == 0 {
		snap.Header.SchemaVersion = SnapshotSchemaVersion
	}
	ids := make([]string, 0, len(report.TotalsByArm))
	for id := range report.TotalsByArm {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t := report.TotalsByArm[id]
		row := SnapshotRow{
			ArmID:           id,
			TotalCases:      t.TotalCases,
			Passed:          t.Passed,
			PassRate:        t.PassRate(),
			WeightedScore:   t.WeightedScore,
			FirstToPassRate: t.FirstToPassRate,
			MedianLOC:       medianInt(t.LOC),
			MedianLatencyMS: medianInt(t.LatencyMS),
			MedianUSD:       medianFloat(t.USD),
			MedianTokens:    medianInt(t.Tokens),
			MedianScore:     medianFloat(t.Scores),
			SkillName:       t.SkillName,
			VerbosityLevel:  t.VerbosityLevel,
		}
		// Stable 8-char hash fingerprint of the system prompt — so
		// the diff can surface "skill body changed between runs"
		// even when the median numbers don't move.
		if t.SystemPrompt != "" {
			row.SystemPromptHash = shortHash(t.SystemPrompt)
		}
		snap.Rows = append(snap.Rows, row)
	}
	return snap
}

// pathDir is filepath.Dir except it tolerates non-trailing-slash
// "a/b" inputs by collapsing them sensibly.
func pathDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}

// medianInt returns the median across the integers; empty input
// returns 0. Even-length slices use the lower middle index by
// convention (matches caveman measure.py).
func medianInt(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]int(nil), xs...)
	sort.Ints(sorted)
	return sorted[len(sorted)/2]
}

// medianFloat mirrors medianInt for floats.
func medianFloat(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}

// shortHash is a 32-bit FNV-1a stable hash, rendered as 8 lower-
// hex chars. NOT cryptographic; just enough to surface "skill
// body changed" in a snapshot diff.
func shortHash(s string) string {
	var h uint32 = 0x811c9dc5
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 0x01000193
	}
	const hex = "0123456789abcdef"
	var out [8]byte
	for i := 7; i >= 0; i-- {
		out[i] = hex[h&0xf]
		h >>= 4
	}
	return string(out[:])
}

// DiffSnapshots produces a row-by-row delta. Missing-in-A rows come
// out as "added"; missing-in-B rows as "removed". Same ArmID with
// differing values is "changed". The output is sorted by ArmID so
// writing-and-reading again is byte-stable.
func DiffSnapshots(a, b Snapshot) ([]SnapshotRowDelta, error) {
	if a.Header.SchemaVersion != b.Header.SchemaVersion {
		return nil, fmt.Errorf("snapshot diff: schema %d != %d",
			a.Header.SchemaVersion, b.Header.SchemaVersion)
	}
	index := func(s Snapshot) map[string]SnapshotRow {
		m := make(map[string]SnapshotRow, len(s.Rows))
		for _, r := range s.Rows {
			m[r.ArmID] = r
		}
		return m
	}
	A := index(a)
	B := index(b)
	var deltas []SnapshotRowDelta
	ids := make([]string, 0, len(A)+len(B))
	for id := range A {
		ids = append(ids, id)
	}
	for id := range B {
		if _, ok := A[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		ra, hasA := A[id]
		rb, hasB := B[id]
		switch {
		case hasA && !hasB:
			deltas = append(deltas, SnapshotRowDelta{
				ArmID: id, Kind: "removed-A",
				Old: ra,
			})
		case !hasA && hasB:
			deltas = append(deltas, SnapshotRowDelta{
				ArmID: id, Kind: "added-B",
				New: rb,
			})
		case hasA && hasB:
			changes := zeroRow()
			changes.ArmID = id
			changes.PassRate = rb.PassRate - ra.PassRate
			changes.WeightedScore = rb.WeightedScore - ra.WeightedScore
			changes.FirstToPassRate = rb.FirstToPassRate - ra.FirstToPassRate
			changes.MedianLOC = rb.MedianLOC - ra.MedianLOC
			changes.MedianLatencyMS = rb.MedianLatencyMS - ra.MedianLatencyMS
			changes.MedianUSD = rb.MedianUSD - ra.MedianUSD
			changes.MedianTokens = rb.MedianTokens - ra.MedianTokens
			changes.MedianScore = rb.MedianScore - ra.MedianScore
			changes.SystemPromptHash = ra.SystemPromptHash + "->" + rb.SystemPromptHash
			kind := "changed"
			if ra.SystemPromptHash != "" && ra.SystemPromptHash != rb.SystemPromptHash {
				kind = "changed-skill-body"
			}
			deltas = append(deltas, SnapshotRowDelta{
				ArmID: id, Kind: kind, Old: ra, New: rb, Delta: changes,
			})
		}
	}
	return deltas, nil
}

// SnapshotRowDelta is one row of the diff.
type SnapshotRowDelta struct {
	ArmID string      `json:"arm_id"`
	Kind  string      `json:"kind"`
	Old   SnapshotRow `json:"old,omitempty"`
	New   SnapshotRow `json:"new,omitempty"`
	Delta SnapshotRow `json:"delta,omitempty"`
}

// zeroRow returns a row with ArmID set; everything else zero. Used
// by DiffSnapshots to avoid mutating rows directly.
func zeroRow() SnapshotRow {
	row := SnapshotRow{ArmID: "delta"}
	row.SystemPromptHash = ""
	return row
}
