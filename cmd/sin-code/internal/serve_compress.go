// SPDX-License-Identifier: MIT
// Purpose: serve — MCP tool-description compression statistics (issue #173).
// sin-debt: shrink, upgrade: when a second compress-related function is needed, merge into a shared file
package internal

import (
	"fmt"
	"os"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpcompress"
)

// printCompressionStats writes per-tool byte budgets to w. Output is a
// left-aligned text table — deterministic across runs (no time, no
// timestamp). Used by --print-stats.
//
// The "active rules" line makes the active pipeline transparent so the
// caller can correlate the savings with the ruleset they asked for.
func printCompressionStats(w *os.File, p mcpcompress.Pipeline, stats []mcpcompress.Stats) {
	var totalOrig, totalComp int
	for _, s := range stats {
		totalOrig += s.Original
		totalComp += s.Compressed
	}
	saved := totalOrig - totalComp
	if saved < 0 {
		saved = 0
	}
	ratio := 0.0
	if totalOrig > 0 {
		ratio = float64(saved) / float64(totalOrig)
	}
	rules := make([]string, len(p))
	for i, r := range p {
		rules[i] = r.Name()
	}
	fmt.Fprintf(w, "mcpcompress: active rules = [%s]\n", strings.Join(rules, ","))
	fmt.Fprintf(w, "mcpcompress: ponytail tags = [%s]\n\n", strings.Join(tagNames(p.Tags()), ","))
	fmt.Fprintf(w, "  %-32s  %7s  %7s  %7s  %6s\n", "tool", "orig", "comp", "saved", "ratio")
	fmt.Fprintf(w, "  %-32s  %7s  %7s  %7s  %6s\n", strings.Repeat("-", 32), "------", "------", "------", "------")
	for _, s := range stats {
		fmt.Fprintf(w, "  %-32s  %7d  %7d  %7d  %5.1f%%\n",
			s.Name, s.Original, s.Compressed, s.BytesSaved, 100*s.Ratio)
	}
	fmt.Fprintf(w, "  %-32s  %7s  %7s  %7s  %6s\n", strings.Repeat("-", 32), "------", "------", "------", "------")
	fmt.Fprintf(w, "  %-32s  %7d  %7d  %7d  %5.1f%%\n",
		"TOTAL", totalOrig, totalComp, saved, 100*ratio)
}

func tagNames(tags []mcpcompress.Tag) []string {
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = string(t)
	}
	return out
}
