// SPDX-License-Identifier: MIT
// Purpose: ReportGenerator — autonomous research report generation
// (issue #384). The autonomy daemon collects ResearchResults from one
// or more ResearchSources (issue #387) and assembles them into a
// structured ResearchReport with sections, deduplicated references, and
// an abstract. The report can be rendered to Markdown (for human review
// / PR descriptions / docs) or JSON (for machine consumption / ledger
// storage).
//
// ReportGenerator holds no mutable state and is safe for concurrent use
// (M7): Generate / ToMarkdown / ToJSON are all pure functions over their
// inputs. The template field stores an optional Markdown skeleton; the
// default template is used when NewReportGenerator is called.
package autonomy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ReportSection is one titled block of a research report. Content is
// free-form prose (typically Markdown); Sources is the list of citation
// URLs / DOIs the section draws from.
type ReportSection struct {
	Title   string
	Content string
	Sources []string
}

// ResearchReport is the assembled output of ReportGenerator.Generate.
// References is the deduplicated union of every section's Sources,
// ordered by first appearance. GeneratedAt is the UTC timestamp of
// assembly.
type ResearchReport struct {
	Title        string
	Abstract     string
	Sections     []ReportSection
	References   []string
	GeneratedAt  time.Time
}

// ReportGenerator assembles ResearchReports from sections and renders
// them to Markdown or JSON. It is stateless and safe for concurrent use.
type ReportGenerator struct {
	template string
}

// defaultReportTemplate is the Markdown skeleton used when no custom
// template is supplied. Placeholders {{.Title}}, {{.Abstract}}, and
// {{.Body}} are substituted by ToMarkdown.
const defaultReportTemplate = `# {{.Title}}

{{.Abstract}}

{{.Body}}

## References

{{.References}}
`

// NewReportGenerator returns a ReportGenerator using the default
// Markdown template.
func NewReportGenerator() *ReportGenerator {
	return &ReportGenerator{template: defaultReportTemplate}
}

// Generate assembles a ResearchReport from a title and a list of
// sections. It derives a one-paragraph abstract from the first section's
// content (first sentence, or the whole content if no sentence
// terminator is present), deduplicates references across all sections
// preserving first-appearance order, and stamps GeneratedAt with the
// current UTC time. At least one section with a non-empty title is
// required.
func (g *ReportGenerator) Generate(title string, sections []ReportSection) (*ResearchReport, error) {
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("research_report: title is required")
	}
	if len(sections) == 0 {
		return nil, errors.New("research_report: at least one section is required")
	}
	for i, s := range sections {
		if strings.TrimSpace(s.Title) == "" {
			return nil, fmt.Errorf("research_report: section %d has empty title", i)
		}
	}

	abstract := deriveAbstract(sections)
	refs := dedupeReferences(sections)

	return &ResearchReport{
		Title:       title,
		Abstract:    abstract,
		Sections:    sections,
		References:  refs,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// ToMarkdown renders r as a Markdown document using the generator's
// template. The body is the concatenation of all sections as
// "## <Title>\n\n<Content>\n" blocks. References are listed as a
// numbered list. The output is byte-stable for a given report except
// for the GeneratedAt timestamp, which is rendered in RFC3339 form.
func (g *ReportGenerator) ToMarkdown(r *ResearchReport) string {
	if r == nil {
		return ""
	}
	var body strings.Builder
	for _, s := range r.Sections {
		body.WriteString("## ")
		body.WriteString(s.Title)
		body.WriteString("\n\n")
		body.WriteString(strings.TrimRight(s.Content, "\n"))
		body.WriteString("\n\n")
	}
	var refs strings.Builder
	for i, ref := range r.References {
		fmt.Fprintf(&refs, "%d. %s\n", i+1, ref)
	}
	if len(r.References) == 0 {
		refs.WriteString("_None_\n")
	}

	tmpl := g.template
	if tmpl == "" {
		tmpl = defaultReportTemplate
	}
	out := strings.ReplaceAll(tmpl, "{{.Title}}", r.Title)
	out = strings.ReplaceAll(out, "{{.Abstract}}", abstractBlock(r.Abstract))
	out = strings.ReplaceAll(out, "{{.Body}}", strings.TrimRight(body.String(), "\n"))
	out = strings.ReplaceAll(out, "{{.References}}", strings.TrimRight(refs.String(), "\n"))
	if !r.GeneratedAt.IsZero() {
		out = strings.ReplaceAll(out, "{{.GeneratedAt}}", r.GeneratedAt.Format(time.RFC3339))
	}
	return out
}

// ToJSON serializes r as indented JSON. The JSON form is the canonical
// machine-readable representation used by the ledger and downstream
// consumers.
func (g *ReportGenerator) ToJSON(r *ResearchReport) ([]byte, error) {
	if r == nil {
		return nil, errors.New("research_report: report is nil")
	}
	return json.MarshalIndent(r, "", "  ")
}

// deriveAbstract builds a one-paragraph abstract from the sections. It
// takes the first sentence of the first section's content; if no
// sentence terminator is found, it uses the trimmed content (capped at
// a reasonable length). When the first section has no content, it falls
// back to a generic sentence naming the section titles.
func deriveAbstract(sections []ReportSection) string {
	if len(sections) == 0 {
		return ""
	}
	first := strings.TrimSpace(sections[0].Content)
	if first == "" {
		titles := make([]string, 0, len(sections))
		for _, s := range sections {
			titles = append(titles, s.Title)
		}
		return "This report covers: " + strings.Join(titles, ", ") + "."
	}
	// First sentence: up to the first '.', '!', or '?'.
	for i, r := range first {
		if r == '.' || r == '!' || r == '?' {
			return first[:i+1]
		}
	}
	const maxLen = 280
	if len(first) > maxLen {
		return first[:maxLen] + "..."
	}
	return first
}

// abstractBlock formats the abstract as a Markdown block. An empty
// abstract yields an italic placeholder so the rendered template never
// has a dangling header.
func abstractBlock(abstract string) string {
	abstract = strings.TrimSpace(abstract)
	if abstract == "" {
		return "_No abstract provided._"
	}
	return "**Abstract:** " + abstract
}

// dedupeReferences collects every source from every section in order,
// dropping duplicates (case-sensitive) and preserving first-appearance
// order. Empty/whitespace-only entries are skipped.
func dedupeReferences(sections []ReportSection) []string {
	seen := make(map[string]struct{})
	refs := make([]string, 0)
	for _, s := range sections {
		for _, src := range s.Sources {
			src = strings.TrimSpace(src)
			if src == "" {
				continue
			}
			if _, ok := seen[src]; ok {
				continue
			}
			seen[src] = struct{}{}
			refs = append(refs, src)
		}
	}
	return refs
}
