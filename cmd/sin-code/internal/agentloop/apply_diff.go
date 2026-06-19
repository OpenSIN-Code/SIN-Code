// SPDX-License-Identifier: MIT
// Purpose: Unified diff-based editing tool (issue #365). Parses standard
// unified diff patches, validates each hunk against the target content,
// and applies hunks to produce the modified content. The DiffApplier can
// be used by both chat and MCP surfaces for consistent diff application.
package agentloop

import (
	"errors"
	"fmt"
	"strings"
)

// DiffHunk represents a single hunk from a unified diff.
type DiffHunk struct {
	OldStart int    // 1-based start line in the old file (0 if no old lines)
	OldLines int    // number of lines in the old file
	NewStart int    // 1-based start line in the new file (0 if no new lines)
	NewLines int    // number of lines in the new file
	OldText  string // the original lines this hunk replaces
	NewText  string // the replacement lines
}

// DiffApplier applies unified diff patches to file content. It is safe
// for concurrent use of separate Apply calls (no shared mutable state).
type DiffApplier struct{}

// NewDiffApplier creates a new DiffApplier.
func NewDiffApplier() *DiffApplier {
	return &DiffApplier{}
}

// Apply parses and applies a unified diff string to the given content.
func (d *DiffApplier) Apply(content, diff string) (string, error) {
	hunks, err := ParseUnifiedDiff(diff)
	if err != nil {
		return "", err
	}
	if err := ValidateDiff(content, hunks); err != nil {
		return "", err
	}
	return ApplyDiff(content, hunks)
}

// ParseUnifiedDiff parses a standard unified diff string into a slice of
// DiffHunk. The expected format:
//
//	--- a/file
//	+++ b/file
//	@@ -oldStart,oldLines +newStart,newLines @@
//	 context line
//	-removed line
//	+added line
//
// Multiple hunk headers (@@ ... @@) are supported. File headers (---/+++)
// are optional and skipped. Lines starting with ' ' are context (kept in
// both OldText and NewText). Lines starting with '-' go into OldText only.
// Lines starting with '+' go into NewText only. A backslash-no-newline
// marker (\ No newline at end of file) is consumed and ignored.
func ParseUnifiedDiff(diff string) ([]DiffHunk, error) {
	if diff == "" {
		return nil, nil
	}

	lines := strings.Split(diff, "\n")
	// Remove trailing empty element from the final newline.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var hunks []DiffHunk
	var cur *DiffHunk

	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "---"):
			// File header (old) — skip.
			continue
		case strings.HasPrefix(line, "+++"):
			// File header (new) — skip.
			continue
		case strings.HasPrefix(line, "@@"):
			// Finalise previous hunk if any.
			if cur != nil {
				hunks = append(hunks, *cur)
			}
			h, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("diff line %d: %w", i+1, err)
			}
			// Reset line counts — they are counted from actual content lines.
			// The header's OldLines/NewLines are metadata, not authoritative.
			h.OldLines = 0
			h.NewLines = 0
			cur = &h
		case strings.HasPrefix(line, "\\"):
			// "\ No newline at end of file" marker — consume and ignore.
			continue
		case cur == nil:
			return nil, fmt.Errorf("diff line %d: content before first hunk header", i+1)
		case strings.HasPrefix(line, "-"):
			cur.OldText += line[1:] + "\n"
			cur.OldLines++
		case strings.HasPrefix(line, "+"):
			cur.NewText += line[1:] + "\n"
			cur.NewLines++
		case strings.HasPrefix(line, " "):
			ctx := line[1:] + "\n"
			cur.OldText += ctx
			cur.NewText += ctx
			cur.OldLines++
			cur.NewLines++
		case line == "":
			// Empty line in diff — treat as context blank line.
			ctx := "\n"
			cur.OldText += ctx
			cur.NewText += ctx
			cur.OldLines++
			cur.NewLines++
		default:
			return nil, fmt.Errorf("diff line %d: unexpected line %q", i+1, line)
		}
	}

	if cur != nil {
		hunks = append(hunks, *cur)
	}

	return hunks, nil
}

// parseHunkHeader parses a line like "@@ -10,5 +10,6 @@".
func parseHunkHeader(line string) (DiffHunk, error) {
	// Strip surrounding @@ ... @@ delimiters.
	inner := strings.TrimSpace(strings.TrimPrefix(line, "@@"))
	inner = strings.TrimSuffix(inner, "@@")
	inner = strings.TrimSpace(inner)

	parts := strings.Fields(inner)
	if len(parts) < 2 {
		return DiffHunk{}, fmt.Errorf("malformed hunk header: %q", line)
	}

	oldPart := strings.TrimPrefix(parts[0], "-")
	newPart := strings.TrimPrefix(parts[1], "+")

	oldStart, oldLines, err := parseRange(oldPart)
	if err != nil {
		return DiffHunk{}, fmt.Errorf("old range: %w", err)
	}
	newStart, newLines, err := parseRange(newPart)
	if err != nil {
		return DiffHunk{}, fmt.Errorf("new range: %w", err)
	}

	return DiffHunk{
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
	}, nil
}

// parseRange parses "start,count" or just "start" (count defaults to 1).
func parseRange(s string) (start, count int, err error) {
	if s == "" {
		return 0, 0, errors.New("empty range")
	}
	parts := strings.SplitN(s, ",", 2)
	start, err = atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	if len(parts) == 2 {
		count, err = atoi(parts[1])
		if err != nil {
			return 0, 0, err
		}
	} else {
		count = 1
	}
	return start, count, nil
}

func atoi(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty number")
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid digit %q in %q", c, s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// ValidateDiff verifies that all hunks' OldText matches the corresponding
// region in content. Hunk OldStart is 1-based. Returns an error if any
// hunk does not match.
func ValidateDiff(content string, hunks []DiffHunk) error {
	if len(hunks) == 0 {
		return nil
	}

	lines := strings.Split(content, "\n")
	// If content ends with newline, Split produces a trailing empty — remove it.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	for i, h := range hunks {
		if h.OldStart < 0 {
			return fmt.Errorf("hunk %d: invalid old start %d", i+1, h.OldStart)
		}
		if h.OldLines < 0 {
			return fmt.Errorf("hunk %d: invalid old line count %d", i+1, h.OldLines)
		}

		// Special case: hunk with 0 old lines is an insertion — always valid.
		if h.OldLines == 0 {
			continue
		}

		if h.OldStart == 0 {
			return fmt.Errorf("hunk %d: old start is 0 but old lines > 0", i+1)
		}

		startIdx := h.OldStart - 1 // convert to 0-based
		endIdx := startIdx + h.OldLines

		if startIdx >= len(lines) {
			return fmt.Errorf("hunk %d: old start %d beyond content (%d lines)", i+1, h.OldStart, len(lines))
		}
		if endIdx > len(lines) {
			return fmt.Errorf("hunk %d: old range %d-%d beyond content (%d lines)", i+1, h.OldStart, endIdx, len(lines))
		}

		expected := strings.Join(lines[startIdx:endIdx], "\n") + "\n"
		if h.OldText != expected {
			return fmt.Errorf("hunk %d: context mismatch at line %d", i+1, h.OldStart)
		}
	}

	return nil
}

// ApplyDiff applies validated hunks to content and returns the result.
// Hunks are applied in order. If a hunk has 0 old lines (pure insertion),
// the NewText is inserted at OldStart. The result always ends with a
// trailing newline if the original content did.
func ApplyDiff(content string, hunks []DiffHunk) (string, error) {
	if len(hunks) == 0 {
		return content, nil
	}

	hadTrailingNewline := strings.HasSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	if hadTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var result []string
	cursor := 0 // 0-based index into lines

	for i, h := range hunks {
		var startIdx int
		if h.OldLines == 0 {
			// Pure insertion: OldStart is the line *before* which to insert
			// (or at end if OldStart == len(lines)+1 or 0).
			if h.OldStart == 0 {
				startIdx = 0
			} else {
				startIdx = h.OldStart // insert before this line (1-based → 0-based is just OldStart-1, but for insert it's OldStart-1 as position)
				if startIdx > len(lines) {
					startIdx = len(lines)
				}
				if startIdx < 0 {
					startIdx = 0
				}
			}
		} else {
			startIdx = h.OldStart - 1
		}

		if startIdx < cursor {
			return "", fmt.Errorf("hunk %d: overlaps or precedes previous hunk (start %d < cursor %d)", i+1, startIdx, cursor)
		}

		// Copy unchanged lines before this hunk.
		result = append(result, lines[cursor:startIdx]...)

		// Append the new text lines (NewText already includes context + additions).
		newLines := splitLines(h.NewText)
		result = append(result, newLines...)

		// Advance cursor past the old region.
		if h.OldLines > 0 {
			cursor = startIdx + h.OldLines
		} else {
			cursor = startIdx
		}
	}

	// Copy remaining lines after the last hunk.
	result = append(result, lines[cursor:]...)

	out := strings.Join(result, "\n")
	if hadTrailingNewline {
		out += "\n"
	}
	return out, nil
}

// splitLines splits text into lines, dropping the trailing empty element
// produced by a final newline.
func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// GenerateDiff produces a unified diff string from old and new content.
// It uses a simple line-based algorithm that groups consecutive changes into
// one hunk. The output is valid for ApplyDiff and human-readable.
func GenerateDiff(oldContent, newContent string) string {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	// Find common prefix.
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}

	// Find common suffix after the prefix.
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}

	oldEnd := len(oldLines) - suffix
	newEnd := len(newLines) - suffix

	ctxBefore := 3
	if prefix < ctxBefore {
		ctxBefore = prefix
	}
	ctxAfter := 3
	if oldEnd+ctxAfter > len(oldLines) {
		ctxAfter = len(oldLines) - oldEnd
		if ctxAfter < 0 {
			ctxAfter = 0
		}
	}

	oldStart := prefix - ctxBefore + 1
	newStart := prefix - ctxBefore + 1
	oldLinesCount := ctxBefore + (oldEnd - prefix) + ctxAfter
	newLinesCount := ctxBefore + (newEnd - prefix) + ctxAfter

	var b strings.Builder
	fmt.Fprintf(&b, "--- a/file\n+++ b/file\n")
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldStart, oldLinesCount, newStart, newLinesCount)

	for i := prefix - ctxBefore; i < prefix; i++ {
		fmt.Fprintf(&b, " %s\n", oldLines[i])
	}
	for i := prefix; i < oldEnd; i++ {
		fmt.Fprintf(&b, "-%s\n", oldLines[i])
	}
	for i := prefix; i < newEnd; i++ {
		fmt.Fprintf(&b, "+%s\n", newLines[i])
	}
	for i := oldEnd; i < oldEnd+ctxAfter && i < len(oldLines); i++ {
		fmt.Fprintf(&b, " %s\n", oldLines[i])
	}

	return b.String()
}
