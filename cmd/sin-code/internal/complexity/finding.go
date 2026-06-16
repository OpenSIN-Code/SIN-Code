// SPDX-License-Identifier: MIT
// Purpose: complexity finding model — ponytail 5-tag format.
package complexity

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
