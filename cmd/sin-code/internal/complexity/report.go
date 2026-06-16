// SPDX-License-Identifier: MIT
// Purpose: render complexity findings in ponytail text / JSON / markdown.
package complexity

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Report renders findings in the requested format.
func Report(findings []Finding, format string) (string, error) {
	switch format {
	case "json":
		return renderJSON(findings)
	case "markdown":
		return renderMarkdown(findings), nil
	case "text", "":
		return renderText(findings), nil
	default:
		return "", fmt.Errorf("unknown format: %s", format)
	}
}

// Summary returns the net line/dep counts and the closing phrase.
func Summary(findings []Finding) (netLines, netDeps int) {
	depSet := make(map[string]struct{})
	for _, f := range findings {
		netLines += f.LineCount
		for _, d := range f.DepsRemoved {
			depSet[d] = struct{}{}
		}
	}
	return netLines, len(depSet)
}

func renderText(findings []Finding) string {
	if len(findings) == 0 {
		return "Lean already. Ship."
	}
	var b strings.Builder
	for _, f := range findings {
		loc := fmt.Sprintf("%s:%d", f.Path, f.Line)
		if f.Line != f.EndLine {
			loc = fmt.Sprintf("%s:%d-%d", f.Path, f.Line, f.EndLine)
		}
		line := fmt.Sprintf("%s: %s. %s. [%s]", f.Tag, f.What, f.Replacement, loc)
		if f.ApprovedBy != "" {
			line += fmt.Sprintf(" (approved: sin-debt)")
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	netLines, netDeps := Summary(findings)
	b.WriteString(fmt.Sprintf("net: -%d lines, -%d deps possible.\n", netLines, netDeps))
	return b.String()
}

func renderJSON(findings []Finding) (string, error) {
	ranked := Rank(findings)
	netLines, netDeps := Summary(ranked)
	out := struct {
		Findings []Finding `json:"findings"`
		NetLines int       `json:"net_lines"`
		NetDeps  int       `json:"net_deps"`
		Status   string    `json:"status"`
	}{
		Findings: ranked,
		NetLines: netLines,
		NetDeps:  netDeps,
		Status:   status(ranked),
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func renderMarkdown(findings []Finding) string {
	if len(findings) == 0 {
		return "Lean already. Ship.\n"
	}
	var b strings.Builder
	b.WriteString("## Complexity review\n\n")
	b.WriteString("| Tag | What | Replacement | Location | Approved |\n")
	b.WriteString("| --- | ---- | ----------- | -------- | -------- |\n")
	for _, f := range findings {
		loc := fmt.Sprintf("%s:%d", f.Path, f.Line)
		if f.Line != f.EndLine {
			loc = fmt.Sprintf("%s:%d-%d", f.Path, f.Line, f.EndLine)
		}
		approved := ""
		if f.ApprovedBy != "" {
			approved = "sin-debt"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", f.Tag, f.What, f.Replacement, loc, approved)
	}
	netLines, netDeps := Summary(findings)
	fmt.Fprintf(&b, "\n**net: -%d lines, -%d deps possible.**\n", netLines, netDeps)
	return b.String()
}

func status(findings []Finding) string {
	if len(findings) == 0 {
		return "lean"
	}
	return "cuts-available"
}

// UniqueDeps returns the sorted unique dependency names that could be removed.
func UniqueDeps(findings []Finding) []string {
	set := make(map[string]struct{})
	for _, f := range findings {
		for _, d := range f.DepsRemoved {
			set[d] = struct{}{}
		}
	}
	var out []string
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
