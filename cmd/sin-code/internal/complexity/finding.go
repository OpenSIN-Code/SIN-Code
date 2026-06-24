// SPDX-License-Identifier: MIT
// Purpose: complexity finding model — ponytail 5-tag format.
package complexity

import "sort"

// Finding is one complexity cut suggested by the static reviewer.
type Finding struct {
	Tag         string   `json:"tag"`
	What        string   `json:"what"`
	Replacement string   `json:"replacement"`
	Path        string   `json:"path"`
	Line        int      `json:"line"`
	EndLine     int      `json:"end_line"`
	LineCount   int      `json:"line_count"`
	DepsRemoved []string `json:"deps_removed,omitempty"`
	ApprovedBy  string   `json:"approved_by,omitempty"`
}

// Tags used by the ponytail complexity review.
const (
	TagDelete = "delete"
	TagStdlib = "stdlib"
	TagNative = "native"
	TagYagni  = "yagni"
	TagShrink = "shrink"
)

// AllTags is the ordered set of ponytail tags.
var AllTags = []string{TagDelete, TagStdlib, TagNative, TagYagni, TagShrink}

// ── Rank (merged from rank.go) ───────────────────────────────────────────

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
