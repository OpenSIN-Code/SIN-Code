// SPDX-License-Identifier: MIT
// Purpose: minimal YAML frontmatter parser for SKILL.md files.
// Supports plain scalars, single/double-quoted strings, folded block
// scalars (>- and >) and literal block scalars (|- and |). Good enough
// for obra/superpowers metadata without pulling in gopkg.in/yaml.v3.
// Docs: superpowers.doc.md
package superpowers

import (
	"strings"
	"unicode"
)

// ParseFrontmatter extracts the leading `--- ... ---` block from a
// SKILL.md body and returns its key/value map. Both keys and values are
// returned with surrounding whitespace trimmed. If the body has no
// frontmatter, an empty map is returned and ok=false.
//
// Block-scalar semantics (matters for description):
//
//	>- / >    folded     — newlines become single spaces
//	|- / |    literal    — newlines are preserved verbatim
//
// All other newlines inside a block scalar are treated as content.
func ParseFrontmatter(body string) (map[string]string, bool) {
	out := make(map[string]string)
	// Frontmatter MUST start at byte 0 (with optional leading whitespace).
	trimmed := strings.TrimLeft(body, " \t")
	if !strings.HasPrefix(trimmed, "---") {
		return out, false
	}
	// Skip the opening "---" line, then scan until the closing "---".
	rest := trimmed[3:]
	// Strip a single newline right after the dashes.
	rest = strings.TrimPrefix(rest, "\r\n")
	rest = strings.TrimPrefix(rest, "\n")
	// Find the closing fence. It must appear at column 0 (ignoring leading
	// whitespace on the body), but obra/superpowers always uses column 0.
	end := indexClosingFence(rest)
	if end < 0 {
		return out, false
	}
	block := rest[:end]
	parseBlock(block, out)
	return out, true
}

// indexClosingFence returns the byte index of the next "---" line in body,
// or -1 if none. The fence must be at column 0 (possibly preceded by
// \r\n).
func indexClosingFence(body string) int {
	for i := 0; i < len(body); i++ {
		if body[i] != '-' {
			continue
		}
		// Must be at column 0 or right after a newline.
		if i != 0 && body[i-1] != '\n' {
			continue
		}
		// Now look for "---" followed by EOL or EOF.
		if i+3 > len(body) {
			return -1
		}
		if body[i] == '-' && body[i+1] == '-' && body[i+2] == '-' {
			after := i + 3
			if after == len(body) {
				return i
			}
			c := body[after]
			if c == '\n' || c == '\r' {
				return i
			}
		}
	}
	return -1
}

// parseBlock walks a frontmatter block and fills out. Lines look like:
//
//	name: value
//	name: "quoted value"
//	name: 'quoted value'
//	name: >-
//	  folded value
//	  continues here
//	name: |-
//	  literal value
//	  preserved
func parseBlock(block string, out map[string]string) {
	lines := strings.Split(block, "\n")
	i := 0
	for i < len(lines) {
		raw := lines[i]
		// Skip blank lines and comment-only lines (#).
		if strings.TrimSpace(raw) == "" || strings.HasPrefix(strings.TrimSpace(raw), "#") {
			i++
			continue
		}
		// A "key: value" line. The colon must NOT be inside quotes.
		key, val, hasVal, keyEnd := splitKeyValue(raw)
		if key == "" {
			i++
			continue
		}
		if !hasVal {
			// "key:" with empty value.
			out[strings.TrimSpace(key)] = ""
			i++
			continue
		}
		// Detect block scalars.
		head := strings.TrimRight(val, " \t")
		if strings.HasSuffix(head, ">") || strings.HasSuffix(head, ">-" ) ||
			strings.HasSuffix(head, "+") || strings.HasSuffix(head, "+-") {
			// Folded scalar.
			chomp := head[len(head)-1]
			stripTrailing := strings.HasSuffix(head, "-") || strings.HasSuffix(head, "+-")
			_ = chomp
			indent := leadingSpaces(raw)
			// Block may start on next line.
			startIdx := i + 1
			if keyEnd+1 < len(raw) && raw[keyEnd+1] != ' ' && raw[keyEnd+1] != '\t' {
				// Inline value (e.g. "key: >- inline") — not supported here;
				// we treat the rest of the line as a single line then keep
				// collecting indented continuations.
				startIdx = i + 1
			}
			collected, next := collectBlockLines(lines, startIdx, indent+1, stripTrailing)
			joined := strings.Join(collected, "\n")
			folded := foldScalar(joined, stripTrailing)
			out[strings.TrimSpace(key)] = folded
			i = next
			continue
		}
		if strings.HasSuffix(head, "|") || strings.HasSuffix(head, "|-") ||
			strings.HasSuffix(head, "+") || strings.HasSuffix(head, "+-") {
			indent := leadingSpaces(raw)
			startIdx := i + 1
			stripTrailing := strings.HasSuffix(head, "-") || strings.HasSuffix(head, "+-")
			collected, next := collectBlockLines(lines, startIdx, indent+1, stripTrailing)
			joined := strings.Join(collected, "\n")
			out[strings.TrimSpace(key)] = joined
			i = next
			continue
		}
		// Plain scalar: unquote, trim.
		out[strings.TrimSpace(key)] = unquote(strings.TrimSpace(val))
		i++
	}
}

// splitKeyValue parses a single frontmatter line. It returns:
//   - key:    the trimmed key (empty if no colon found)
//   - value:  the raw value (right of the colon, untouched — caller
//     decides how to interpret block scalars)
//   - hasVal: true if a colon was present
//   - colonIdx: the byte index of the colon in the original line
func splitKeyValue(line string) (key, value string, hasVal bool, colonIdx int) {
	// Skip leading whitespace for the key itself; we still report colonIdx
	// relative to the original line so indent detection works.
	trimmed := line
	leading := 0
	for leading < len(trimmed) && (trimmed[leading] == ' ' || trimmed[leading] == '\t') {
		leading++
	}
	trimmed = trimmed[leading:]
	colon := -1
	inS, inD := false, false
	for i, r := range trimmed {
		switch r {
		case '\'':
			if !inD {
				inS = !inS
			}
		case '"':
			if !inS {
				inD = !inD
			}
		case ':':
			if inS || inD {
				continue
			}
			// Colon must be followed by space, EOL, or end-of-line.
			next := byte(' ')
			if i+1 < len(trimmed) {
				next = trimmed[i+1]
			}
			if next == ' ' || next == '\t' || next == '\r' || next == '\n' || i == len(trimmed)-1 {
				colon = i
			}
		}
		if colon >= 0 {
			break
		}
	}
	if colon < 0 {
		return "", "", false, 0
	}
	keyPart := trimmed[:colon]
	valPart := trimmed[colon+1:]
	return keyPart, valPart, true, leading + colon
}

// leadingSpaces returns the count of leading space/tab runes in line.
func leadingSpaces(line string) int {
	for i, r := range line {
		if r != ' ' && r != '\t' {
			return i
		}
	}
	return len(line)
}

// unquote strips a single layer of matched ' or " quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// collectBlockLines returns the run of consecutive lines whose indent is
// strictly greater than baseIndent, plus the index of the first line that
// did NOT match (i.e. the next thing the outer loop should process).
// If stripTrailing is true (YAML "strip" chomping indicator: -), the
// trailing empty/blank lines are removed.
//
// YAML semantics: the block scalar's "indentation indicator" is the
// difference between the parent key's column and the first non-blank line
// of the block. We detect that indicator on the fly and strip it from
// every subsequent line.
func collectBlockLines(lines []string, start, baseIndent int, stripTrailing bool) ([]string, int) {
	var collected []string
	j := start
	blockIndent := -1
	for j < len(lines) {
		l := lines[j]
		// Blank line: include as content, keep scanning.
		if strings.TrimSpace(l) == "" {
			collected = append(collected, "")
			j++
			continue
		}
		ind := leadingSpaces(l)
		if ind <= baseIndent {
			break
		}
		if blockIndent < 0 {
			blockIndent = ind
		}
		stripped := l
		if len(stripped) >= blockIndent {
			stripped = stripped[blockIndent:]
		}
		collected = append(collected, stripped)
		j++
	}
	// Eat trailing blank lines if stripTrailing.
	if stripTrailing {
		for len(collected) > 0 && strings.TrimSpace(collected[len(collected)-1]) == "" {
			collected = collected[:len(collected)-1]
		}
	}
	return collected, j
}

// foldScalar converts a folded block scalar body to its string form:
//   - newlines become single spaces
//   - runs of multiple newlines ("\n\n") collapse to a single space too
//     (matches the YAML folded-scalar rule: blank line → space)
//   - leading/trailing whitespace is trimmed from the final result
func foldScalar(body string, stripTrailing bool) string {
	if stripTrailing {
		body = strings.TrimRight(body, " \t\n")
	}
	var b strings.Builder
	prevSpace := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '\n' || c == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			// Eat a paired \r\n.
			if c == '\r' && i+1 < len(body) && body[i+1] == '\n' {
				i++
			}
			continue
		}
		// Collapse multiple internal spaces/tabs into one.
		if c == ' ' || c == '\t' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteByte(c)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

// trimUnicode is a small helper used by tests / callers that want to
// collapse internal whitespace runs in a fold result.
func trimUnicode(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
}
