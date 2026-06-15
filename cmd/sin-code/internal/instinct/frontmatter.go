// SPDX-License-Identifier: MIT
// Purpose: ECC-compatible YAML frontmatter <-> Markdown serialization.
// Format mirrors affaan-m/ecc continuous-learning-v2; the on-disk schema
// is documented in frontmatter.doc.md. We use the community-standard
// gopkg.in/yaml.v3 to stay out of the encoding business.
// Docs: frontmatter.doc.md
package instinct

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Marshal renders an instinct as Markdown with YAML frontmatter:
//
//	---
//	id: ...
//	trigger: ...
//	confidence: 0.7
//	---
//	# Title
//
//	## Action
//	...
//
//	## Evidence
//	- ...
func Marshal(i *Instinct) ([]byte, error) {
	fm, err := yaml.Marshal(i)
	if err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(fm)
	b.WriteString("---\n\n")
	b.WriteString("# " + titleFromTrigger(i.Trigger) + "\n\n")
	b.WriteString("## Action\n\n")
	b.WriteString(strings.TrimSpace(i.Action) + "\n\n")
	if len(i.Evidence) > 0 {
		b.WriteString("## Evidence\n\n")
		for _, e := range i.Evidence {
			b.WriteString("- " + strings.TrimSpace(e) + "\n")
		}
	}
	return b.Bytes(), nil
}

// Unmarshal parses an ECC-style instinct Markdown file.
func Unmarshal(data []byte) (*Instinct, error) {
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return nil, fmt.Errorf("missing frontmatter delimiter")
	}
	rest := strings.TrimPrefix(text, "---")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, fmt.Errorf("unterminated frontmatter")
	}
	fm := rest[:idx]
	body := rest[idx+len("\n---"):]

	var i Instinct
	if err := yaml.Unmarshal([]byte(fm), &i); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	i.Action, i.Evidence = parseBody(body)
	if i.Status == "" {
		i.recomputeStatus()
	}
	return &i, nil
}

func parseBody(body string) (action string, evidence []string) {
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	section := ""
	var actionLines []string
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "## "):
			section = strings.ToLower(strings.TrimSpace(trimmed[3:]))
			continue
		case strings.HasPrefix(trimmed, "# "):
			continue
		}
		switch section {
		case "action":
			if trimmed != "" {
				actionLines = append(actionLines, trimmed)
			}
		case "evidence":
			if strings.HasPrefix(trimmed, "- ") {
				evidence = append(evidence, strings.TrimSpace(trimmed[2:]))
			}
		}
	}
	return strings.TrimSpace(strings.Join(actionLines, " ")), evidence
}

func titleFromTrigger(trigger string) string {
	t := strings.TrimSpace(trigger)
	t = strings.TrimPrefix(t, "when ")
	if t == "" {
		return "Instinct"
	}
	return strings.ToUpper(t[:1]) + t[1:]
}
