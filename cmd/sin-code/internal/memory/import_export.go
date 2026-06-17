// SPDX-License-Identifier: MIT
// Purpose: import/export between memory store and external formats —
// MEMORY.md (Claude-style) and ECC instinct JSON (issue #357).
package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// MEMORY.md export/import
// ---------------------------------------------------------------------------

// ExportToMEMORYMD writes memories in Claude-style MEMORY.md format,
// grouped by tag. The output is deterministic (tags and entries are
// sorted) so repeated exports of the same data produce byte-identical
// files.
func ExportToMEMORYMD(memories []Memory, path string) error {
	var b strings.Builder
	b.WriteString("# Memory\n\n")

	groups := groupByTagValue(memories)

	for _, tag := range sortedTagKeys(groups) {
		b.WriteString("## Tag: ")
		b.WriteString(tag)
		b.WriteString("\n")
		entries := groups[tag]
		sort.Slice(entries, func(i, j int) bool { return entries[i].Insight < entries[j].Insight })
		for _, m := range entries {
			b.WriteString("- ")
			b.WriteString(m.Insight)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// ImportFromMEMORYMD parses a Claude-style MEMORY.md file and returns
// the memories as Memory structs. Tags are extracted from "## Tag: X"
// headers; content from "- text" list items.
func ImportFromMEMORYMD(path string) ([]Memory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("import: read %s: %w", path, err)
	}
	return parseMEMORYMD(string(raw)), nil
}

func parseMEMORYMD(content string) []Memory {
	var out []Memory
	currentTag := ""

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## Tag: ") {
			currentTag = strings.TrimSpace(strings.TrimPrefix(trimmed, "## Tag: "))
			continue
		}

		if strings.HasPrefix(trimmed, "- ") {
			insight := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			if insight == "" {
				continue
			}
			m := Memory{
				Insight: insight,
				Created: time.Now().UTC(),
				Updated: time.Now().UTC(),
			}
			if currentTag != "" {
				m.Tags = []string{currentTag}
			}
			out = append(out, m)
		}
	}
	return out
}

func groupByTagValue(memories []Memory) map[string][]Memory {
	groups := map[string][]Memory{}
	for _, m := range memories {
		tags := m.Tags
		if len(tags) == 0 {
			tags = []string{"untagged"}
		}
		for _, t := range tags {
			groups[t] = append(groups[t], m)
		}
	}
	return groups
}

func sortedTagKeys(m map[string][]Memory) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---------------------------------------------------------------------------
// ECC instinct JSON export/import
// ---------------------------------------------------------------------------

// InstinctFormat is the JSON wire format for ECC instinct interchange.
type InstinctFormat struct {
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
	Scope      string  `json:"scope"`
	Source     string  `json:"source"`
}

// ExportToInstinct writes instincts in ECC instinct JSON format.
// Only the four canonical fields (Content, Confidence, Scope, Source)
// are serialized — internal fields like ID and timestamps are omitted.
func ExportToInstinct(instincts []Instinct, path string) error {
	formats := make([]InstinctFormat, 0, len(instincts))
	for _, inst := range instincts {
		formats = append(formats, InstinctFormat{
			Content:    inst.Content,
			Confidence: inst.Confidence,
			Scope:      inst.Scope,
			Source:     inst.Source,
		})
	}
	raw, err := json.MarshalIndent(formats, "", "  ")
	if err != nil {
		return fmt.Errorf("export instinct: marshal: %w", err)
	}
	return os.WriteFile(path, raw, 0o644)
}

// ImportFromInstinct reads an ECC instinct JSON file and returns
// the instincts as Instinct structs. Internal fields (ID, timestamps)
// are generated during import.
func ImportFromInstinct(path string) ([]Instinct, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("import instinct: read %s: %w", path, err)
	}
	var formats []InstinctFormat
	if err := json.Unmarshal(raw, &formats); err != nil {
		return nil, fmt.Errorf("import instinct: unmarshal: %w", err)
	}
	now := time.Now().UTC()
	out := make([]Instinct, 0, len(formats))
	for _, f := range formats {
		inst := Instinct{
			ID:         instinctID(f.Content),
			Content:    f.Content,
			Confidence: f.Confidence,
			Scope:      f.Scope,
			Source:     f.Source,
			CreatedAt:  now,
		}
		if inst.Scope == "" {
			inst.Scope = "project"
		}
		if inst.Source == "" {
			inst.Source = "observation"
		}
		out = append(out, inst)
	}
	return out, nil
}
