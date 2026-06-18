// SPDX-License-Identifier: MIT
// Purpose: tests for the research report generator (issue #384). The
// generator is a pure function over its inputs, so these tests are
// deterministic and race-free (M7).
package autonomy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReportGeneratorBasic(t *testing.T) {
	g := NewReportGenerator()
	r, err := g.Generate("JWT Auth Research", []ReportSection{
		{Title: "Background", Content: "JWT is a token format. It has three parts.", Sources: []string{"https://jwt.io", "https://rfc-editor.org/rfc7519"}},
		{Title: "Findings", Content: "Most libraries handle verification correctly.", Sources: []string{"https://jwt.io"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if r.Title != "JWT Auth Research" {
		t.Errorf("title = %q", r.Title)
	}
	if !strings.HasPrefix(r.Abstract, "JWT is a token format.") {
		t.Errorf("abstract = %q", r.Abstract)
	}
	if len(r.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(r.Sections))
	}
	// dedupe: jwt.io appears twice, should appear once.
	wantRefs := []string{"https://jwt.io", "https://rfc-editor.org/rfc7519"}
	if len(r.References) != len(wantRefs) {
		t.Fatalf("references = %v, want %v", r.References, wantRefs)
	}
	for i, w := range wantRefs {
		if r.References[i] != w {
			t.Errorf("reference[%d] = %q, want %q", i, r.References[i], w)
		}
	}
	if r.GeneratedAt.IsZero() {
		t.Error("GeneratedAt should be set")
	}
	if !r.GeneratedAt.Location().Equal(time.UTC) && r.GeneratedAt.Location().String() != "UTC" {
		t.Errorf("GeneratedAt should be UTC, got %s", r.GeneratedAt.Location())
	}
}

func TestReportGeneratorEmptyTitleRejected(t *testing.T) {
	g := NewReportGenerator()
	_, err := g.Generate("   ", []ReportSection{{Title: "S1", Content: "c"}})
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("expected title-required error, got %v", err)
	}
}

func TestReportGeneratorNoSectionsRejected(t *testing.T) {
	g := NewReportGenerator()
	_, err := g.Generate("T", nil)
	if err == nil || !strings.Contains(err.Error(), "section") {
		t.Fatalf("expected section-required error, got %v", err)
	}
}

func TestReportGeneratorEmptySectionTitleRejected(t *testing.T) {
	g := NewReportGenerator()
	_, err := g.Generate("T", []ReportSection{{Title: "", Content: "c"}})
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("expected empty-section-title error, got %v", err)
	}
}

func TestReportGeneratorMarkdownRendering(t *testing.T) {
	g := NewReportGenerator()
	r, err := g.Generate("T", []ReportSection{
		{Title: "Intro", Content: "Hello world.", Sources: []string{"https://a"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	md := g.ToMarkdown(r)
	if !strings.HasPrefix(md, "# T") {
		t.Errorf("markdown should start with title: %q", md)
	}
	if !strings.Contains(md, "**Abstract:**") {
		t.Errorf("markdown missing abstract: %q", md)
	}
	if !strings.Contains(md, "## Intro") {
		t.Errorf("markdown missing section header: %q", md)
	}
	if !strings.Contains(md, "Hello world.") {
		t.Errorf("markdown missing section content: %q", md)
	}
	if !strings.Contains(md, "## References") {
		t.Errorf("markdown missing references header: %q", md)
	}
	if !strings.Contains(md, "1. https://a") {
		t.Errorf("markdown missing numbered reference: %q", md)
	}
}

func TestReportGeneratorMarkdownNoReferences(t *testing.T) {
	g := NewReportGenerator()
	r, err := g.Generate("T", []ReportSection{{Title: "S", Content: "c"}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	md := g.ToMarkdown(r)
	if !strings.Contains(md, "_None_") {
		t.Errorf("markdown should show _None_ for empty refs: %q", md)
	}
}

func TestReportGeneratorJSONRoundTrip(t *testing.T) {
	g := NewReportGenerator()
	original, err := g.Generate("T", []ReportSection{
		{Title: "S1", Content: "content one.", Sources: []string{"https://x"}},
		{Title: "S2", Content: "content two!", Sources: []string{"https://y", "https://x"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	data, err := g.ToJSON(original)
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	if !strings.Contains(string(data), "\"Title\": \"T\"") {
		t.Errorf("json missing title: %s", data)
	}
	var round ResearchReport
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if round.Title != original.Title {
		t.Errorf("title round-trip = %q", round.Title)
	}
	if len(round.Sections) != len(original.Sections) {
		t.Errorf("sections round-trip = %d, want %d", len(round.Sections), len(original.Sections))
	}
	if len(round.References) != len(original.References) {
		t.Errorf("refs round-trip = %d, want %d", len(round.References), len(original.References))
	}
}

func TestReportGeneratorAbstractFallbackForEmptyContent(t *testing.T) {
	g := NewReportGenerator()
	r, err := g.Generate("T", []ReportSection{
		{Title: "Alpha", Content: "", Sources: nil},
		{Title: "Beta", Content: "later", Sources: nil},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(r.Abstract, "Alpha") || !strings.Contains(r.Abstract, "Beta") {
		t.Errorf("abstract fallback should list section titles: %q", r.Abstract)
	}
}
