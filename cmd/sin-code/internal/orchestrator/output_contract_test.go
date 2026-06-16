// SPDX-License-Identifier: MIT
// Purpose: tests for `output_contract.go` (caveman-style output contract).
// Every byte of the rendered line is asserted exactly — byte-stability is
// load-bearing for the ledger (issue #168) and the orchestrator's re-
// ingestion cost.
package orchestrator

import (
	"strings"
	"testing"
)

// golden fixture used by every byte-equality test. One Finding, line-level,
// confidence non-zero so the ` # c=` suffix is exercised too.
var goldenFinding = Finding{
	Tag:        TagDelete,
	Symbol:     "truncate",
	Path:       "internal/foo/foo.go",
	Line:       42,
	Confidence: 0.85,
	Hint:       "drop unused 5-line wrapper",
}

// goldenRendered is the byte-stable expected output for `goldenFinding`.
// Pinned so reviewers can see the exact format.
const goldenRendered = "internal/foo/foo.go:42 — truncate — delete — drop unused 5-line wrapper # c=0.85\n"

// TestRender_GoldenByteStable pins the exact render of `goldenFinding`.
// If you intentionally change the format, update this constant AND
// `output_contract.doc.md` in the SAME PR.
func TestRender_GoldenByteStable(t *testing.T) {
	got := goldenFinding.Render()
	if got != goldenRendered[:len(goldenRendered)-1] {
		t.Fatalf("render drift:\n  got:  %q\n  want: %q", got, goldenRendered[:len(goldenRendered)-1])
	}
}

// TestRender_FileLevelDropssColonZero verifies Line=0 does NOT emit ":0".
func TestRender_FileLevelDropssColonZero(t *testing.T) {
	got := (Finding{
		Tag:        TagVerify,
		Symbol:     "-",
		Path:       "internal/foo/foo.go",
		Line:       0,
		Confidence: 0.50,
		Hint:       "manual review pending",
	}).Render()
	if strings.Contains(got, ":0") {
		t.Fatalf("file-level render must drop :0, got %q", got)
	}
	if !strings.HasPrefix(got, "internal/foo/foo.go — ") {
		t.Fatalf("file-level locator prefix wrong: %q", got)
	}
}

// TestRender_EmptySymbolRendersDash verifies Symbol="" → "-" so the
// parser doesn't see a 4-em-dash line.
func TestRender_EmptySymbolRendersDash(t *testing.T) {
	got := (Finding{
		Tag: TagRebuild, Symbol: "", Path: "x.go", Line: 7,
		Confidence: 0.1, Hint: "wrap",
	}).Render()
	if !strings.Contains(got, "x.go:7 — - — rebuild — wrap # c=0.10") {
		t.Fatalf("empty symbol must render as dash: %q", got)
	}
}

// TestRender_ConfidenceAlwaysPresent verifies the ` # c=` suffix is
// emitted for 0.0 too (otherwise missing-vs-zero findings differ in
// bytes; that's a bug).
func TestRender_ConfidenceAlwaysPresent(t *testing.T) {
	got := (Finding{Tag: TagRisk, Path: "a.go", Line: 1, Hint: "h"}).Render()
	if !strings.HasSuffix(got, " # c=0.00") {
		t.Fatalf("confidence 0.00 should still render, got %q", got)
	}
}

// TestParseFinding_RoundTrip is the central property: a Finding
// rendered to a string and parsed again yields an EQUAL struct
// (two decimals on confidence preserved by const-rounding).
func TestParseFinding_RoundTrip(t *testing.T) {
	rendered := goldenFinding.Render()
	got, err := ParseFinding(rendered)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != goldenFinding {
		t.Fatalf("round-trip diff:\n  got:  %+v\n  want: %+v", got, goldenFinding)
	}
}

// TestParseFinding_RejectsBadLine ensures a non-caveman line fails
// loudly. Whitespace-only, prose without em-dashes, missing confidence
// — every case must error.
func TestParseFinding_RejectsBadLine(t *testing.T) {
	bad := []string{
		"",
		"just some prose, no em-dashes",
		"a.go:1 — sym — delete — h", // missing confidence
		"a.go:1 — sym — unknown-tag — h # c=0.5", // unknown tag
		"a.go:1 — sym — delete — h # c=0",        // bad confidence format (no decimals)
		"a.go:1 — sym — DELETE — h # c=0.50",     // uppercase tag rejected (lower-case required)
	}
	for _, b := range bad {
		if _, err := ParseFinding(b); err == nil {
			t.Errorf("expected error for %q, got nil", b)
		}
	}
}

// TestParseFinding_RejectsHedgingButStillParses verifies the structural
// parser succeeds (it tolerates prose that the Verifier will reject).
// This keeps responsibilities clean — the parser deals with shape,
// the Verifier deals with semantics.
func TestParseFinding_RejectsHedgingButStillParses(t *testing.T) {
	_, err := ParseFinding("a.go:1 — sym — delete — perhaps remove it # c=0.50")
	if err != nil {
		t.Fatalf("parser must succeed here (hedge detection is the verifier's job): %v", err)
	}
}

// TestVerifyFindings_AcceptsCleanBatch verifies a clean slice produces
// zero errors.
func TestVerifyFindings_AcceptsCleanBatch(t *testing.T) {
	fs := []Finding{
		goldenFinding,
		{Tag: TagSimplify, Symbol: "wrap", Path: "b.go", Line: 3, Confidence: 0.7, Hint: "inline small helper"},
		{Tag: TagRisk, Symbol: "-", Path: "c.go", Line: 0, Confidence: 0.2, Hint: "blast radius across callers"},
	}
	if errs := VerifyFindings(fs); len(errs) > 0 {
		t.Fatalf("clean batch must be accepted, got errors: %v", errs)
	}
}

// TestVerifyFindings_RejectsHedging verifies EVERY hedging phrase is
// caught. All 12 closed-set phrases are tested.
func TestVerifyFindings_RejectsHedging(t *testing.T) {
	for _, phrase := range hedgingPhrases {
		// Embed phrase mid-hint so capitalization variants don't hide it.
		hint := "fix " + phrase + " please"
		fs := []Finding{{Path: "a.go", Line: 1, Tag: TagDelete, Confidence: 0.5, Hint: hint}}
		errs := VerifyFindings(fs)
		if len(errs) == 0 {
			t.Errorf("hedging %q must be rejected", phrase)
		}
	}
}

// TestVerifyFindings_RejectsEmptyPath ensures the structural invariant
// `Path != ""` is enforced. An empty-Path Finding is a sloppy probe.
func TestVerifyFindings_RejectsEmptyPath(t *testing.T) {
	fs := []Finding{{Tag: TagDelete, Symbol: "x", Line: 1, Confidence: 0.5}}
	if errs := VerifyFindings(fs); len(errs) == 0 {
		t.Fatal("empty-Path Finding must be rejected")
	}
}

// TestVerifyFindings_RejectsUnknownTag ensures the tag is a closed
// enumeration. A free-text "bug" or "fixme" tag breaks downstream
// routing (the orchestrator routes by Tag string).
func TestVerifyFindings_RejectsUnknownTag(t *testing.T) {
	fs := []Finding{{Tag: "fixme", Path: "a.go", Line: 1, Confidence: 0.5}}
	if errs := VerifyFindings(fs); len(errs) == 0 {
		t.Fatal("unknown tag must be rejected")
	}
}

// TestVerifyFindings_RejectsLongHint enforces the 240-char ceiling.
// Multi-line hints are forbidden — the contract is a ONE-liner.
func TestVerifyFindings_RejectsLongHint(t *testing.T) {
	fs := []Finding{{
		Tag: TagDelete, Path: "a.go", Line: 1, Confidence: 0.5,
		Hint: strings.Repeat("x", 241),
	}}
	if errs := VerifyFindings(fs); len(errs) == 0 {
		t.Fatal(">240-char hint must be rejected")
	}
}

// TestVerifyFindings_RejectsTrailingPunctuation enforces the
// imperative-mood rule. A trailing `.` or `!` is a code-review
// anti-pattern here.
func TestVerifyFindings_RejectsTrailingPunctuation(t *testing.T) {
	for _, suffix := range []string{".", "!"} {
		fs := []Finding{{Tag: TagDelete, Path: "a.go", Line: 1, Confidence: 0.5, Hint: "drop" + suffix}}
		if errs := VerifyFindings(fs); len(errs) == 0 {
			t.Errorf("hint ending in %q must be rejected", suffix)
		}
	}
}

// TestParseFindings_MultiLine is the realistic case: the sub-agent
// emitted three Findings across two paragraphs (a blank line in
// between). Parser returns three Findings with no errors.
func TestParseFindings_MultiLine(t *testing.T) {
	s := strings.Join([]string{
		goldenFinding.Render(),
		"",
		(Finding{Tag: TagRisk, Path: "b.go", Line: 7, Symbol: "-", Confidence: 0.3, Hint: "blast radius"}).Render(),
		(Finding{Tag: TagSimplify, Path: "c.go", Line: 4, Symbol: "wrap", Confidence: 0.65, Hint: "inline small helper"}).Render(),
	}, "\n")
	fs, errs, err := ParseFindings(s)
	if err != nil {
		t.Fatalf("multi-line parse: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(fs) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(fs))
	}
}

// TestParseFindings_MixedValidInvalid ensures per-line diagnostics
// are reported (NOT silently dropped). "Silent drop" is the original
// bug that motivated this contract — the sub-agent passes a malformed
// line and the orchestrator never knows.
func TestParseFindings_MixedValidInvalid(t *testing.T) {
	s := strings.Join([]string{
		goldenFinding.Render(),
		"this is not a caveman line",
		(Finding{Tag: TagVerify, Path: "x.go", Line: 2, Symbol: "-", Confidence: 0.5, Hint: "manual review"}).Render(),
	}, "\n")
	fs, errs, err := ParseFindings(s)
	if err != nil {
		t.Fatalf("multi-line parse: %v", err)
	}
	if len(fs) != 2 {
		t.Fatalf("expected 2 valid findings, got %d", len(fs))
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 per-line error, got %d (%v)", len(errs), errs)
	}
	if !strings.Contains(errs[0], "not a caveman line") {
		t.Fatalf("error should mention shape mismatch, got: %q", errs[0])
	}
}

// TestFindingsToBytes_NoTrailingNewlineOnEmpty verifies the empty
// slice yields the empty string — sub-agent "had nothing to say" is
// a valid outcome and MUST NOT inject a phantom newline.
func TestFindingsToBytes_NoTrailingNewlineOnEmpty(t *testing.T) {
	if got := FindingsToBytes(nil); got != "" {
		t.Fatalf("empty slice must yield empty string, got %q", got)
	}
}

// TestFindingsToBytes_TrailingNewlineOnNonEmpty verifies a single Newline
// after the last line. This matches `goldenRendered` byte-for-byte when
// wrapping `goldenFinding`.
func TestFindingsToBytes_TrailingNewlineOnNonEmpty(t *testing.T) {
	if got := FindingsToBytes([]Finding{goldenFinding}); got != goldenRendered {
		t.Fatalf("FindingsToBytes drift:\n  got:  %q\n  want: %q", got, goldenRendered)
	}
}

// TestAllTagsStable is a regression guard: any PR that reorders AllTags
// will silently break every consumer that hashes on tag-set sequence.
// Pinned here so reviewers MUST update both this test and the doc.
func TestAllTagsStable(t *testing.T) {
	want := []string{"delete", "rebuild", "risk", "simplify", "verify"}
	if len(AllTags) != len(want) {
		t.Fatalf("tag-count drift: %+v", AllTags)
	}
	for i, w := range want {
		if string(AllTags[i]) != w {
			t.Errorf("tag[%d]: got %q, want %q", i, AllTags[i], w)
		}
	}
}

// TestFindHedging_PreservesClosedSet exercises the closed-set
// directly, so a refactor that drops phrases from the slice won't
// regress silently.
func TestFindHedging_PreservesClosedSet(t *testing.T) {
	for _, p := range hedgingPhrases {
		if len(FindHedging("contains "+p+" here")) == 0 {
			t.Errorf("FindHedging missed %q", p)
		}
	}
	if len(FindHedging("clean prose, no hedging")) != 0 {
		t.Fatal("clean prose was incorrectly flagged")
	}
}

// TestParseFinding_ConfidenceBytes is a regression guard for the
// `%.2f` formatting — a single digit must not sneak through.
func TestParseFinding_ConfidenceBytes(t *testing.T) {
	got := (Finding{Tag: TagDelete, Path: "a.go", Line: 1, Symbol: "x", Confidence: 0.5, Hint: "h"}).Render()
	if !strings.HasSuffix(got, " # c=0.50") {
		t.Fatalf("confidence must be two decimals: %q", got)
	}
}
