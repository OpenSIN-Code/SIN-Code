// SPDX-License-Identifier: MIT
// Purpose: deterministic aggregate stats over a slice of sin-dept markers.
// The Stats struct is intentionally a value type with sorted map slices; the
// downstream renderer (report.go) is byte-stable exactly because every
// aggregation step sorts its keys before producing output.
// Docs: sindept.doc.md
package sindept

import (
	"sort"
	"strings"
)

// Stats is the aggregate view of a marker set. It is the unit that the
// `sin-code debt stats` subcommand and the `report.go` markdown renderer
// operate on. Every map is materialized as `[]KV` for byte-determinism.
type Stats struct {
	// Total is the marker count (with and without upgrade).
	Total int
	// WithUpgrade counts markers that name an explicit upgrade clause.
	WithUpgrade int
	// WithoutUpgrade counts markers that do not — these are rot-risk
	// ("no-trigger") entries.
	WithoutUpgrade int

	// ByFile groups marker counts by absolute file path.
	ByFile []KV
	// ByReason groups counts by the free-text reason (trimmed, lowercased).
	ByReason []KV
	// ByLanguage groups counts by detected language tag.
	ByLanguage []KV
	// BySymbol groups counts by the best-effort symbol guess.
	BySymbol []KV

	// RotRisk lists the markers that have no upgrade clause — the rows
	// humans should look at first. It is pre-sorted by File then Line.
	RotRisk []Marker

	// Oldest is the marker with the smallest Line number (file path
	// tie-broken by sort). It is nil when the input is empty.
	Oldest *Marker

	// FilesScanned counts files visited during the walk (including files
	// with no markers). It is informational only — not part of the
	// rot-risk calculation.
	FilesScanned int
	// MarkersPerFile is Total / max(1, len(ByFile)).
	MarkersPerFile float64
}

// KV is a lexicographically-stable key+count pair.
type KV struct {
	Key   string
	Count int
}

// AggregateStats turns a slice of markers into a stable Stats value. The
// input slice can be in any order; everything downstream sorts by key.
//
// The contract is byte-stable: two calls with the same marker set produce
// identical Stats (and therefore identical Render output).
func AggregateStats(markers []Marker) Stats {
	s := Stats{Total: len(markers)}
	if len(markers) == 0 {
		return s
	}

	files := make(map[string]int)
	reasons := make(map[string]int)
	langs := make(map[string]int)
	symbols := make(map[string]int)

	for i := range markers {
		m := &markers[i]
		if m.HasUpg && m.Upgrade != "" {
			s.WithUpgrade++
		} else {
			s.WithoutUpgrade++
			s.RotRisk = append(s.RotRisk, *m)
		}
		files[m.File]++
		reasons[strings.ToLower(strings.TrimSpace(m.Reason))]++
		if m.Language != "" {
			langs[m.Language]++
		}
		if m.Symbol != "" {
			symbols[m.Symbol]++
		}
	}

	s.ByFile = sortedKV(files)
	s.MarkersPerFile = float64(s.Total) / float64(maxInt(1, len(s.ByFile)))
	s.ByReason = sortedKV(reasons)
	s.ByLanguage = sortedKV(langs)
	s.BySymbol = sortedKV(symbols)
	s.FilesScanned = len(s.ByFile)

	// Oldest = the lex-smallest (File, Line) marker.
	oldest := &markers[0]
	for i := range markers {
		if markers[i].File < oldest.File ||
			(markers[i].File == oldest.File && markers[i].Line < oldest.Line) {
			oldest = &markers[i]
		}
	}
	s.Oldest = oldest

	// Sort rot-risk lex-by (File, Line, Column).
	sort.Slice(s.RotRisk, func(i, j int) bool {
		if s.RotRisk[i].File != s.RotRisk[j].File {
			return s.RotRisk[i].File < s.RotRisk[j].File
		}
		if s.RotRisk[i].Line != s.RotRisk[j].Line {
			return s.RotRisk[i].Line < s.RotRisk[j].Line
		}
		return s.RotRisk[i].Column < s.RotRisk[j].Column
	})
	return s
}

// sortedKV converts a map into the lex-sorted []KV view.
//
// Two input maps with the same (key, count) pairs always emit the same
// []KV. The deterministic sort order keeps Render output stable.
func sortedKV(m map[string]int) []KV {
	out := make([]KV, 0, len(m))
	for k, v := range m {
		out = append(out, KV{Key: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// maxInt returns the larger of a and b. It is a local helper rather than
// the built-in `max` because that helper is Go 1.21+ only and we keep the
// package runnable on the minimum Go version the repo declares.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
