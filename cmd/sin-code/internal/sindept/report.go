// SPDX-License-Identifier: MIT
// Purpose: byte-deterministic markdown report renderer for sin-dept marker
// sets. Render produces the same bytes for two Stats that aggregate the
// same marker set, regardless of the order in which files were walked or
// goroutines executed.
//
// Docs: sindept.doc.md
package sindept

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ReportSection enumerates the optional sections Render* funcs can emit.
// They are pre-sorted (lexicographic) so the output is deterministic.
type ReportSection string

const (
	SectionSummary  ReportSection = "summary"
	SectionRotRisk  ReportSection = "rot_risk"
	SectionByFile   ReportSection = "by_file"
	SectionByReason ReportSection = "by_reason"
	SectionByLang   ReportSection = "by_lang"
	SectionBySymbol ReportSection = "by_symbol"
	SectionMarkers  ReportSection = "markers"
)

// DefaultSections is the lex-ordered set emitted by Render and used by
// `sin-code debt stats`. Hard-coded so the per-file report and the
// aggregate dashboard stay byte-identical for the same input.
func DefaultSections() []ReportSection {
	return []ReportSection{
		SectionByFile,
		SectionByLang,
		SectionByReason,
		SectionRotRisk,
		SectionSummary,
	}
}

// FormatVersion is the canonical string embedded in every rendered report.
// Bumping it (and updating tests) is on the table when the schema changes.
const FormatVersion = "sin-debt/v1"

// Header is the single line printed at the top of every report. It is
// kept on one line so `rg "sin-debt/v1"` reliably finds a report even
// when it is concatenated with PR noise.
func Header() string {
	return fmt.Sprintf("# sin-debt report (%s)\n", FormatVersion)
}

// RenderStats writes the canonical markdown report for `s` to `w`. The
// output is byte-deterministic: aggregates were already sorted by
// AggregateStats; rendering prints them in a fixed order; floats are
// formatted with a fixed precision.
func RenderStats(w io.Writer, s Stats, sections []ReportSection) {
	if sections == nil {
		sections = DefaultSections()
	}
	fmt.Fprint(w, Header())

	for _, sec := range sections {
		switch sec {
		case SectionSummary:
			renderSummary(w, s)
		case SectionRotRisk:
			renderRotRisk(w, s)
		case SectionByFile:
			renderKVTable(w, "By file", s.ByFile, s.Total)
		case SectionByReason:
			renderKVTable(w, "By reason", s.ByReason, s.Total)
		case SectionByLang:
			renderKVTable(w, "By language", s.ByLanguage, s.Total)
		case SectionBySymbol:
			renderKVTable(w, "By symbol", s.BySymbol, s.Total)
		}
	}
}

// RenderStatsString is the stringly-typed wrapper used by tests and the
// `debt stats` subcommand. Equivalent to RenderStats(&buf, s, nil).
func RenderStatsString(s Stats) string {
	var b strings.Builder
	RenderStats(&b, s, nil)
	return b.String()
}

// RenderListString writes a marker list as a markdown table — one row
// per marker. The marker slice must already be sorted by File/Line/Column
// (ParseDir guarantees this) for the report to be deterministic.
func RenderListString(markers []Marker) string {
	var b strings.Builder
	fmt.Fprint(&b, Header())
	fmt.Fprintln(&b, "## Markers")
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "| file | line | symbol | reason | upgrade | rot |")
	fmt.Fprintln(&b, "|------|------|--------|--------|---------|-----|")
	for _, m := range markers {
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s |\n",
			escapeCell(m.File),
			m.Line,
			escapeCell(m.Symbol),
			escapeCell(m.Reason),
			escapeCell(m.Upgrade),
			rotCell(m),
		)
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "_%d markers total. %d with upgrade, %d in rot-risk._\n",
		len(markers), countWithUpgrade(markers), countNoUpgrade(markers))
	return b.String()
}

func renderSummary(w io.Writer, s Stats) {
	fmt.Fprintln(w, "## Summary")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "- **Total markers:** %d\n", s.Total)
	fmt.Fprintf(w, "- **With upgrade:** %d\n", s.WithUpgrade)
	fmt.Fprintf(w, "- **Without upgrade (rot-risk):** %d\n", s.WithoutUpgrade)
	fmt.Fprintf(w, "- **Files scanned:** %d\n", s.FilesScanned)
	fmt.Fprintf(w, "- **Markers per file:** %s\n", formatFloat(s.MarkersPerFile))
	if s.Oldest != nil {
		fmt.Fprintf(w, "- **Oldest marker:** %s:%d (%s)\n",
			s.Oldest.File, s.Oldest.Line, s.Oldest.Reason)
	}
	fmt.Fprintln(w)
}

func renderRotRisk(w io.Writer, s Stats) {
	if s.WithoutUpgrade == 0 {
		return
	}
	fmt.Fprintln(w, "## Rot-risk markers (no upgrade clause)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| file | line | symbol | reason |")
	fmt.Fprintln(w, "|------|------|--------|--------|")
	for _, m := range s.RotRisk {
		fmt.Fprintf(w, "| %s | %d | %s | %s |\n",
			escapeCell(m.File), m.Line,
			escapeCell(m.Symbol), escapeCell(m.Reason))
	}
	fmt.Fprintln(w)
}

func renderKVTable(w io.Writer, title string, rows []KV, total int) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(w, "## %s\n\n", title)
	fmt.Fprintln(w, "| key | count | share |")
	fmt.Fprintln(w, "|-----|-------|-------|")
	for _, kv := range rows {
		share := 0.0
		if total > 0 {
			share = float64(kv.Count) / float64(total) * 100
		}
		fmt.Fprintf(w, "| %s | %d | %s%% |\n",
			escapeCell(kv.Key), kv.Count, formatFloat(share))
	}
	fmt.Fprintln(w)
}

// formatFloat keeps precision fixed at one decimal so two reports over
// the same Stats cannot drift by sub-cent randoms in floating-point math.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// escapeCell collapses newlines/tabs/pipes so markdown table cells stay
// well-formed. Deterministic: every rune that would break the cell is
// replaced with a literal space.
func escapeCell(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "|", "/")
	return strings.TrimSpace(s)
}

func rotCell(m Marker) string {
	if m.HasUpg && m.Upgrade != "" {
		return "ok"
	}
	return "rot"
}

func countWithUpgrade(ms []Marker) int {
	n := 0
	for _, m := range ms {
		if m.HasUpg && m.Upgrade != "" {
			n++
		}
	}
	return n
}

func countNoUpgrade(ms []Marker) int {
	return len(ms) - countWithUpgrade(ms)
}
