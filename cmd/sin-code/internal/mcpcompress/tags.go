// SPDX-License-Identifier: MIT
// tags.go carries the canonical ponytail tag set as parse-locked
// constants + a Set helper used by serve.go boundary validation.
//
// The ponytail tag set is:
//
//	delete | stdlib | native | yagni | shrink
//
// These five tokens are the default for `sin-code serve --compress-tools`
// and the default for the `serve.compress.tags` config key. The CLI
// user's --compress-tags flag accepts any subset.
//
// Note: Tag values are part of the public configuration API; renaming
// any is a breaking change (major bump).
package mcpcompress

import "strings"

// TagSet is a small wrapper around a sorted, deduplicated []Tag. The
// zero value is the empty set; construct with NewTagSet or FromList.
//
// TagSet is not concurrency-safe — it is meant for short-lived
// boundary checks at parse time, not long-lived shared state.
type TagSet struct {
	tags []Tag
}

// NewTagSet returns a TagSet containing exactly the given tags in
// the order they appear in DefaultTags (stable ordering).
// Unknown tags are silently dropped — call Valid first if you want
// strict validation.
func NewTagSet(tags []Tag) TagSet {
	known := map[Tag]bool{
		TagDelete: true,
		TagStdlib: true,
		TagNative: true,
		TagYagni:  true,
		TagShrink: true,
	}
	out := make([]Tag, 0, len(tags))
	for _, t := range tags {
		if known[t] {
			out = append(out, t)
		}
	}
	// Stable order: by DefaultTags index.
	rank := map[Tag]int{
		TagDelete: 0,
		TagStdlib: 1,
		TagNative: 2,
		TagYagni:  3,
		TagShrink: 4,
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && rank[out[j-1]] > rank[out[j]]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return TagSet{tags: out}
}

// FromCSV parses a comma-separated tag string ("delete,stdlib,native").
// Empty input yields the default tag set. Whitespace around tags
// is trimmed. Unknown tags are silently dropped.
//
// Byte-stable per input string. Pure.
func FromCSV(s string) TagSet {
	s = strings.TrimSpace(s)
	if s == "" {
		return NewTagSet(DefaultTags)
	}
	parts := strings.Split(s, ",")
	tags := make([]Tag, 0, len(parts))
	for _, p := range parts {
		t := Tag(strings.TrimSpace(p))
		if t != "" {
			tags = append(tags, t)
		}
	}
	return NewTagSet(tags)
}

// List returns the under-tag slice in declaration order.
func (t TagSet) List() []Tag {
	if len(t.tags) == 0 {
		return nil
	}
	out := make([]Tag, len(t.tags))
	copy(out, t.tags)
	return out
}

// Contains returns true if tag is in the set. O(n).
func (t TagSet) Contains(tag Tag) bool {
	for _, x := range t.tags {
		if x == tag {
			return true
		}
	}
	return false
}

// Size returns the number of tags in the set.
func (t TagSet) Size() int { return len(t.tags) }

// CSV returns the canonical comma-separated string for
// round-tripping through config files. Byte-stable per
// (TagSet value).
func (t TagSet) CSV() string {
	if len(t.tags) == 0 {
		return ""
	}
	var b strings.Builder
	for i, x := range t.tags {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(string(x))
	}
	return b.String()
}

// Empty reports whether the set has no entries.
func (t TagSet) Empty() bool { return len(t.tags) == 0 }

// Valid reports whether the tag is in the canonical ponytail set.
func Valid(t Tag) bool {
	switch t {
	case TagDelete, TagStdlib, TagNative, TagYagni, TagShrink:
		return true
	}
	return false
}
