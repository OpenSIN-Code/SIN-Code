// SPDX-License-Identifier: MIT
// Purpose: PRP Markdown <-> Struct (frontmatter + body sections).
// Docs: parse.doc.md
package prp

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Marshal renders a PRP as Markdown with YAML frontmatter.
func Marshal(p *PRP) ([]byte, error) {
	fm, err := yaml.Marshal(p)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(fm)
	b.WriteString("---\n\n")
	b.WriteString("# " + p.Title + "\n\n")
	writeSection(&b, "Goal", p.Goal)
	writeSection(&b, "Context", p.Context)
	writeSection(&b, "Plan", p.Plan)
	writeSection(&b, "Acceptance Criteria", p.Acceptance)
	return b.Bytes(), nil
}

// Unmarshal parses a PRP Markdown file.
func Unmarshal(data []byte) (*PRP, error) {
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return nil, fmt.Errorf("missing frontmatter")
	}
	rest := strings.TrimPrefix(text, "---")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, fmt.Errorf("unterminated frontmatter")
	}
	var p PRP
	if err := yaml.Unmarshal([]byte(rest[:idx]), &p); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	body := rest[idx+len("\n---"):]
	sections := parseSections(body)
	p.Goal = firstNonEmpty(p.Goal, sections["goal"])
	p.Context = sections["context"]
	p.Plan = sections["plan"]
	p.Acceptance = sections["acceptance criteria"]
	return &p, nil
}

func writeSection(b *bytes.Buffer, title, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	b.WriteString("## " + title + "\n\n")
	b.WriteString(strings.TrimSpace(content) + "\n\n")
}

func parseSections(body string) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	cur := ""
	var buf []string
	flush := func() {
		if cur != "" {
			out[cur] = strings.TrimSpace(strings.Join(buf, "\n"))
		}
		buf = nil
	}
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "## ") {
			flush()
			cur = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			continue
		}
		if strings.HasPrefix(line, "# ") {
			continue
		}
		buf = append(buf, line)
	}
	flush()
	return out
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
