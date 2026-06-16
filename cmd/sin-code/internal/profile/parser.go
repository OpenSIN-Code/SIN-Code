// SPDX-License-Identifier: MIT
// Marker-fence primitives for the profile renderer.
//
// # Covenant with internal/skilldist (issue #169)
//
// These primitives produce **byte-identical** output to skilldist for
// the same `<skill>` value. skilldist's `BeginMarker` / `EndMarker` /
// `RenderBlock` / `ParseMarkers` are the source of truth; we keep the
// profile package self-contained so this package compiles in worktrees
// that do not yet have skilldist, but the marker fences we emit are
// drop-in compatible.
//
// If skilldist's contract ever changes, fix the divergence in the
// same PR — agents downstream depend on a fixed grep anchor.
package profile

import (
	"fmt"
	"strings"
)

// MarkerPrefix is the begin/end marker family. We use the same
// "SIN-CODE-SKILL" anchor skilldist uses so a downstream parser that
// searches for a marker-fenced block finds both skill bundles and
// profile rules with one regex.
const MarkerPrefix = "SIN-CODE-SKILL"

// BeginMarker returns the exact begin-marker line for one (target, skill)
// pair. Always ASCII; tests pin the exact bytes.
func BeginMarker(skill string) string {
	//nolint:gocritic // exported name; kept literal so test pins match
	return fmt.Sprintf("<!-- %s-START: %s -->", MarkerPrefix, skill)
}

// EndMarker returns the exact end-marker line. The trailing whitespace
// before the skill name is intentional visual alignment per skilldist's
// contract; ParseMarkers strips it on lookup so a regex-based scanner
// outside this package still finds both ends.
func EndMarker(skill string) string {
	return fmt.Sprintf("<!-- %s-END:   %s -->", MarkerPrefix, skill)
}

// RenderBlock produces the marker-fenced body that profile's writers
// emit for one (target, skill) pair.
//
// The output always ends with exactly one trailing `\n`. The body is
// stripped of its own trailing whitespace so the END marker sits flush.
//
// RenderBlock is byte-stable for a given (skill, body) pair: a second
// invocation with the same bytes produces exactly the same output. This
// is the contract the verify-gate depends on; tests pin the exact
// bytes.
func RenderBlock(skill, body string) string {
	body = strings.TrimRight(body, "\n")
	body = strings.TrimRight(body, "\r")
	return fmt.Sprintf("%s\n# Skill: %s\n\n%s\n%s\n",
		BeginMarker(skill),
		skill,
		body,
		EndMarker(skill),
	)
}

// ParseResult is the structured outcome of ParseMarkers; consumers
// usually only care about (Body, OK). Prefix and Suffix are the bytes
// before and after the matched block, returned so a writer can
// reconstruct the file on update.
//
//	OK=true   — the fenced block for `skill` exists; Body is the inner
//	            block lines WITHOUT the marker lines.
//	OK=false  — no fenced block was found; Prefix is the full input
//	            and Body / Suffix are empty.
type ParseResult struct {
	Prefix string
	Body   string
	Suffix string
	OK     bool
}

// ParseMarkers scans `content` and returns the parsed envelope around
// the marker fence for `skill`. The scan is line-based and tolerant of
// trailing whitespace on the END line (the BEGIN line is exact). Body
// is returned verbatim.
//
// If the BEGIN line is found but the END line is missing, ParseResult.OK
// is false and Prefix is the full input — a half-opened fence is
// treated as if the block were absent, so the writer emits a clean
// block rather than producing a malformed file.
//
// CRLF is normalized to LF before parsing, so files written on Windows
// devices still parse cleanly.
func ParseMarkers(content, skill string) ParseResult {
	begin := BeginMarker(skill)
	end := EndMarker(skill)

	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	beginIdx := -1
	for i, ln := range lines {
		if strings.TrimRight(ln, " \t") == begin {
			beginIdx = i
			break
		}
	}
	if beginIdx < 0 {
		return ParseResult{Prefix: content}
	}
	endIdx := -1
	for i := beginIdx + 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == end {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return ParseResult{Prefix: content}
	}

	body := strings.Join(lines[beginIdx+1:endIdx], "\n")
	prefix := strings.Join(lines[:beginIdx], "\n")
	if beginIdx == 0 {
		prefix = ""
	}
	suffix := strings.Join(lines[endIdx+1:], "\n")

	return ParseResult{Prefix: prefix, Body: body, Suffix: suffix, OK: true}
}
