// SPDX-License-Identifier: MIT
// Purpose: hashline — content-hash anchored line addressing shared by read and edit.
// An anchor "12:ab34cd56" means "line 12, whose content hashes to ab34cd56".
// Edits validate anchors against current content and tolerate small drift,
// eliminating the stale-line-number failure mode of classic line-based editors.
// Docs: cmd/sin-code/internal/hashline.doc.md
package internal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const HashLineLen = 8
const DefaultDriftWindow = 25

func LineHash(line string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(line, " \t\r")))
	return hex.EncodeToString(sum[:])[:HashLineLen]
}

type Anchor struct {
	Line int    `json:"line"`
	Hash string `json:"hash"`
}

func (a Anchor) String() string { return fmt.Sprintf("%d:%s", a.Line, a.Hash) }

func ParseAnchor(s string) (Anchor, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return Anchor{}, fmt.Errorf("invalid anchor %q: want LINE:HASH (e.g. 12:ab34cd56)", s)
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil || n < 1 {
		return Anchor{}, fmt.Errorf("invalid anchor line %q: must be a positive integer", parts[0])
	}
	h := strings.ToLower(parts[1])
	if len(h) != HashLineLen {
		return Anchor{}, fmt.Errorf("invalid anchor hash %q: want %d hex chars", parts[1], HashLineLen)
	}
	return Anchor{Line: n, Hash: h}, nil
}

func SplitLines(content string) (lines []string, trailingNewline bool) {
	trailingNewline = strings.HasSuffix(content, "\n")
	if content == "" {
		return []string{}, false
	}
	lines = strings.Split(content, "\n")
	if trailingNewline {
		lines = lines[:len(lines)-1]
	}
	return lines, trailingNewline
}

func JoinLines(lines []string, trailingNewline bool) string {
	s := strings.Join(lines, "\n")
	if trailingNewline && len(lines) > 0 {
		s += "\n"
	}
	return s
}

func ResolveAnchor(lines []string, a Anchor, driftWindow int) (idx int, drift int, err error) {
	want := strings.ToLower(a.Hash)
	at := a.Line - 1
	if at >= 0 && at < len(lines) && LineHash(lines[at]) == want {
		return at, 0, nil
	}
	for d := 1; d <= driftWindow; d++ {
		if i := at - d; i >= 0 && i < len(lines) && LineHash(lines[i]) == want {
			return i, -d, nil
		}
		if i := at + d; i >= 0 && i < len(lines) && LineHash(lines[i]) == want {
			return i, d, nil
		}
	}
	currently := "<out of range>"
	if at >= 0 && at < len(lines) {
		currently = fmt.Sprintf("%s (%q)", LineHash(lines[at]), truncateLine(lines[at], 60))
	}
	return 0, 0, fmt.Errorf(
		"anchor %s not found within ±%d lines: line %d currently hashes to %s — re-run 'sin-code read --mode hashline' to refresh anchors",
		a.String(), driftWindow, a.Line, currently)
}

func FormatHashlines(lines []string, start int) string {
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%d:%s|%s\n", start+i, LineHash(line), line)
	}
	return b.String()
}

func truncateLine(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\u2026"
}
