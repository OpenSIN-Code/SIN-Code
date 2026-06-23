// SPDX-License-Identifier: MIT
// Purpose: tests for autonomous research-report generation (issue #384).
// Race-clean (mandate M7), deterministic byte-stability for Slugify +
// Markdown output, hermetic — every dependency is a stub.
package autonomy

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestResearchReportGeneration(t *testing.T) {
	searcher := &StaticSearcher{
		Hits: []Source{
			{URL: "https://example.com/a", Title: "First", Snippet: "alpha beta"},
			{URL: "https://example.com/b", Title: "Second", Snippet: "gamma delta"},
			{URL: "https://example.com/c", Title: "Third", Snippet: "epsilon zeta"},
		},
	}
	body := "# Alpha Topic\n\n## Overview\nThis is the alpha topic with citations.\n\n## Detail\nMore detail here ([Source 1]). ([Source 2]) also relevant.\n\n## Sources\n[1] First — https://example.com/a\n[2] Second — https://example.com/b\n[3] Third — https://example.com/c\n"

	gen := NewGenerator(GeneratorConfig{MaxSources: 5}, searcher, nil, &StaticLLM{Reply: body})
	rep, err := gen.Generate(context.Background(), "Alpha Topic")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if rep == nil {
		t.Fatal("expected non-nil report")
	}
	if len(rep.Sources) != 3 {
		t.Fatalf("expected 3 sources, got %d: %+v", len(rep.Sources), rep.Sources)
	}
	if rep.Body == "" {
		t.Fatal("expected non-empty body")
	}
	if rep.Topic != "Alpha Topic" {
		t.Fatalf("topic echo mismatch: %q", rep.Topic)
	}
	if rep.Slug != "alpha-topic" {
		t.Fatalf("slug mismatch: %q", rep.Slug)
	}
}

func TestResearchReportSlug(t *testing.T) {
	cases := map[string]string{
		"Hello World":             "hello-world",
		"Go 1.23 Release Notes":   "go-1-23-release-notes",
		"  spaces around  ":       "spaces-around",
		"mixed/separators_here":   "mixed-separators-here",
		"dotted.path.style":       "dotted-path-style",
		"":                        "report",
		"Punctuation!@# overloaded": "punctuation-overloaded",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) want %q got %q", in, want, got)
		}
	}
	// Byte-stability: same input twice → same slug (regression guard)
	if a, b := Slugify("Foo Bar"), Slugify("Foo Bar"); a != b {
		t.Errorf("slugify not deterministic: %q vs %q", a, b)
	}
	// Truncation to 80 chars
	big := strings.Repeat("alpha-", 200) // 6×200=1200 chars; should hit 80 cap
	got := Slugify(big)
	if len(got) > 80 {
		t.Errorf("slug exceeds 80 chars: %d", len(got))
	}
	if strings.HasSuffix(got, "-") {
		t.Errorf("slug has trailing dash after truncation: %q", got)
	}
}

func TestResearchReportMarkdown(t *testing.T) {
	searcher := &StaticSearcher{
		Hits: []Source{
			{URL: "https://example.com/x", Title: "X", Snippet: "sample snippet"},
			{URL: "https://example.com/y", Title: "Y", Snippet: "more contents"},
		},
	}
	rep := &ResearchReport{
		Topic: "X",
		Sources: []Source{
			{URL: "https://example.com/x", Title: "X", Snippet: "sample snippet"},
			{URL: "https://example.com/y", Title: "Y", Snippet: "more contents"},
		},
		Body: "# X\n\n## A\nstuff.\n\n## Sources\n[1] X - https://example.com/x\n[2] Y - https://example.com/y\n",
		Slug: Slugify("X"),
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// hasHeader and at least one paragraph
	if !strings.HasPrefix(rep.Body, "# X") {
		t.Fatalf("body missing H1: %q", rep.Body)
	}
	// empty body must fail
	rep.Body = "  \n  "
	if err := rep.Validate(); err == nil {
		t.Fatal("expected validation failure on whitespace-only body")
	}
	// no header must fail
	rep.Body = "Just a paragraph with no headings."
	if err := rep.Validate(); err == nil {
		t.Fatal("expected validation failure on header-less body")
	}
	// nil repository must fail
	if err := (*ResearchReport)(nil).Validate(); err == nil {
		t.Fatal("nil Validate should fail")
	}
	// Generator integration: StaticLLM reply must pass validation
	llmBody := "# X\n\n## Intro\nhello ([Source 1]).\n\n## Sources\n[1] X - https://example.com/x\n[2] Y - https://example.com/y\n"
	gen := NewGenerator(GeneratorConfig{MaxSources: 5}, searcher, nil, &StaticLLM{Reply: llmBody})
	if _, err := gen.Generate(context.Background(), "X"); err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

// TestResearchReportWiringGuards covers M3: empty topic, no sources, no LLM,
// and ErrInvalid surface paths.
func TestResearchReportWiringGuards(t *testing.T) {
	// nil generator
	if _, err := (*Generator)(nil).Generate(context.Background(), "x"); err != ErrNotWired {
		t.Errorf("nil gen: want ErrNotWired, got %v", err)
	}
	// nil LLM
	g := NewGenerator(GeneratorConfig{}, &StaticSearcher{}, nil, nil)
	if _, err := g.Generate(context.Background(), "x"); err != ErrNotWired {
		t.Errorf("nil LLM: want ErrNotWired, got %v", err)
	}
	// empty topic
	g = NewGenerator(GeneratorConfig{}, &StaticSearcher{}, nil, &StaticLLM{})
	if _, err := g.Generate(context.Background(), "   "); err == nil {
		t.Error("empty topic: want error")
	}
	// LLM error propagates wrapped
	g = NewGenerator(GeneratorConfig{}, &StaticSearcher{Hits: []Source{{URL: "https://e.com/x", Title: "T"}}}, nil,
		&StaticLLM{Err: errStatic})
	if _, err := g.Generate(context.Background(), "phi"); err == nil {
		t.Error("LLM error: want propagated error")
	}
	// Searcher error propagates wrapped
	g = NewGenerator(GeneratorConfig{}, &StaticSearcher{Err: errStatic}, nil, &StaticLLM{})
	if _, err := g.Generate(context.Background(), "phi"); err == nil {
		t.Error("searcher error: want propagated error")
	}
	// Empty sources slice from searcher → no-sources error
	g = NewGenerator(GeneratorConfig{}, &StaticSearcher{}, nil, &StaticLLM{})
	if _, err := g.Generate(context.Background(), "phi"); err == nil {
		t.Error("empty sources: want error")
	}
}

var errStatic = staticErr("net: simulated failure")

type staticErr string

func (e staticErr) Error() string { return string(e) }

// TestResearchReportDedupRank verifies the dedupe + rank invariant:
// duplicate URLs collapse into one row; the longer snippet wins.
func TestResearchReportDedupRank(t *testing.T) {
	in := []Source{
		{URL: "https://a", Title: "Alpha", Snippet: "short"},
		{URL: "https://b", Title: "Beta", Snippet: "longer snippet"},
		{URL: "https://a", Title: "Alpha", Snippet: "longer than short"},
		{URL: "https://c", Title: "Gamma", Snippet: "mid"},
	}
	out := dedupeAndRank(in, 5)
	if len(out) != 3 {
		t.Fatalf("dedupe expected 3, got %d: %+v", len(out), out)
	}
	// URL "a" must keep the longer snippet
	var gotA *Source
	for i := range out {
		if out[i].URL == "https://a" {
			gotA = &out[i]
		}
	}
	if gotA == nil || gotA.Snippet != "longer than short" {
		t.Fatalf("url a snippet upgrade failed: %+v", gotA)
	}
	// First source ranked first (longest title wins on tie; sort by title length desc here that's "Alpha"=5, "Beta"=4, "Gamma"=5 alphabetical fallback)
	if out[0].Title != "Alpha" && out[0].Title != "Gamma" {
		t.Fatalf("unexpected first entry: %+v", out[0])
	}
	// Cap max=2 returns a prefix of length 2
	out = dedupeAndRank(in, 2)
	if len(out) != 2 {
		t.Fatalf("cap max=2 expected length 2, got %d", len(out))
	}
}

// TestResearchReportFrontmatter wraps the body when RequireFrontmatter is set.
func TestResearchReportFrontmatter(t *testing.T) {
	searcher := &StaticSearcher{
		Hits: []Source{
			{URL: "https://e.com/x", Title: "X"},
		},
	}
	fixed := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	gen := NewGenerator(GeneratorConfig{
		MaxSources:         5,
		RequireFrontmatter: true,
		Now:                func() time.Time { return fixed },
	}, searcher, nil, &StaticLLM{
		Reply: "# X\n\n## Intro\nhello ([Source 1]).\n\n## Sources\n[1] X - https://e.com/x\n",
	})
	rep, err := gen.Generate(context.Background(), "X")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.HasPrefix(rep.Body, "# X\n\n_Generated at 2026-06-18T") {
		t.Fatalf("frontmatter header missing or malformed: %q", rep.Body)
	}
}
