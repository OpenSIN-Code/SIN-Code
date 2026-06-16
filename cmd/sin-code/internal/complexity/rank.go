// SPDX-License-Identifier: MIT
// Purpose: rank findings by payoff (lines first, then removed dependencies).
package complexity

import "sort"

// Rank sorts findings by LineCount descending, then by unique dependency count
// descending, then by path/line for stable output.
func Rank(findings []Finding) []Finding {
	out := make([]Finding, len(findings))
	copy(out, findings)
	sort.Slice(out, func(i, j int) bool {
		if out[i].LineCount != out[j].LineCount {
			return out[i].LineCount > out[j].LineCount
		}
		ci, cj := len(out[i].DepsRemoved), len(out[j].DepsRemoved)
		if ci != cj {
			return ci > cj
		}
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out
}
