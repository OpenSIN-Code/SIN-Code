// SPDX-License-Identifier: MIT
// Purpose: asset data model — agent/command/skill with YAML frontmatter +
// Markdown body. Port of ECC's asset shape in a clean-room Go reimplementation.
// Docs: asset.doc.md
package assets

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind distinguishes the asset families harvested from ECC.
type Kind string

const (
	KindAgent   Kind = "agent"
	KindCommand Kind = "command"
	KindSkill   Kind = "skill"
)

// Asset is a loaded Markdown asset (agent/command/skill).
type Asset struct {
	Kind         Kind     `yaml:"-"`
	Path         string   `yaml:"-"`
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Model        string   `yaml:"model,omitempty"`          // agents
	Tools        []string `yaml:"tools,omitempty"`          // agents
	Color        string   `yaml:"color,omitempty"`          // agents (cosmetic)
	Argument     string   `yaml:"argument-hint,omitempty"`  // commands
	AllowedTools []string `yaml:"allowed-tools,omitempty"`  // commands
	Domain       string   `yaml:"domain,omitempty"`
	Origin       string   `yaml:"origin,omitempty"` // attribution, e.g. "ECC"
	License      string   `yaml:"license,omitempty"`

	Body string `yaml:"-"` // markdown content after frontmatter (the prompt itself)
}

// ParseAsset reads frontmatter + body from a Markdown file's bytes.
func ParseAsset(kind Kind, path string, data []byte) (*Asset, error) {
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return nil, fmt.Errorf("%s: missing frontmatter", path)
	}
	rest := strings.TrimPrefix(text, "---")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, fmt.Errorf("%s: unterminated frontmatter", path)
	}
	fm := rest[:idx]
	body := strings.TrimSpace(rest[idx+len("\n---"):])

	a := &Asset{Kind: kind, Path: path}
	if err := yaml.Unmarshal([]byte(fm), a); err != nil {
		return nil, fmt.Errorf("%s: parse frontmatter: %w", path, err)
	}
	a.Body = body
	return a, nil
}

// Render reassembles the asset back into canonical Markdown (for re-export).
func (a *Asset) Render() ([]byte, error) {
	fm, err := yaml.Marshal(a)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(fm)
	b.WriteString("---\n\n")
	b.WriteString(a.Body)
	b.WriteString("\n")
	return b.Bytes(), nil
}
