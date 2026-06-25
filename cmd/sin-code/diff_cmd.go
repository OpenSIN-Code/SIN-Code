// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second diff-related command is added, merge into a shared file
// Purpose: `sin-code diff` — git diff with complexity + sin-debt overlay.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/complexity"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sindept"
)

var (
	diffCached bool
	diffLast   bool
	diffStat   bool
	diffJSON   bool
)

// diffFileSummary holds per-file information for the diff overlay.
type diffFileSummary struct {
	Path               string               `json:"path"`
	LinesAdded         int                  `json:"lines_added"`
	LinesRemoved       int                  `json:"lines_removed"`
	ComplexityTags     []string             `json:"complexity_tags,omitempty"`
	ComplexityFindings []complexity.Finding `json:"complexity_findings,omitempty"`
	DebtMarkers        []sindept.Marker     `json:"debt_markers,omitempty"`
	Diff               string               `json:"-"`
}

// diffResult is the full enriched diff result.
type diffResult struct {
	Files              []diffFileSummary `json:"files"`
	LinesAdded         int               `json:"lines_added"`
	LinesRemoved       int               `json:"lines_removed"`
	ComplexityFindings int               `json:"complexity_findings"`
	NewDebtMarkers     int               `json:"new_debt_markers"`
}

// NewDiffCmd builds the `diff` cobra subcommand.
func NewDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Git diff with complexity + sin-debt overlay",
		Long: `sin-code diff shows a git diff enriched with complexity and sin-debt
information.

For each changed file:
  - Go files are analyzed with the ponytail complexity analyzer
  - All files are scanned for sin-debt markers
  - Lines with sin-debt markers get a ⚡ prefix
  - Files with complexity findings get a summary header showing the tags

Flags:
  --cached   show staged changes only (git diff --cached)
  --last     show last commit's diff (HEAD~1..HEAD)
  --stat     only show summary, not the full diff
  --json     structured JSON output

Examples:
  sin-code diff
  sin-code diff --cached
  sin-code diff --last
  sin-code diff --stat
  sin-code diff --json`,
		RunE: runDiff,
	}
	cmd.Flags().BoolVar(&diffCached, "cached", false, "Show staged changes only")
	cmd.Flags().BoolVar(&diffLast, "last", false, "Show last commit's diff (HEAD~1..HEAD)")
	cmd.Flags().BoolVar(&diffStat, "stat", false, "Only show summary, not the full diff")
	cmd.Flags().BoolVar(&diffJSON, "json", false, "Structured JSON output")
	internal.RegisterVersionCmd(cmd)
	return cmd
}

func runDiff(cmd *cobra.Command, _ []string) error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("diff: get working directory: %w", err)
	}

	if !inGitRepo(root) {
		return fmt.Errorf("diff: not a git repository (or any of the parent directories)")
	}

	diffArgs := buildDiffArgs()

	numstat, err := runGitInDir(root, append([]string{"diff", "--numstat"}, diffArgs...)...)
	if err != nil {
		return fmt.Errorf("diff: git diff --numstat: %w", err)
	}

	files := parseNumstat(numstat)
	if len(files) == 0 {
		out := cmd.OutOrStdout()
		if diffJSON {
			fmt.Fprint(out, `{"files":[],"lines_added":0,"lines_removed":0,"complexity_findings":0,"new_debt_markers":0}`)
			fmt.Fprintln(out)
		} else {
			fmt.Fprintln(out, "No changes.")
		}
		return nil
	}

	diffOutput, err := runGitInDir(root, append([]string{"diff"}, diffArgs...)...)
	if err != nil {
		return fmt.Errorf("diff: git diff: %w", err)
	}

	markers, err := complexity.ParseMarkers(root)
	if err != nil {
		markers = make(map[string][]complexity.Marker)
	}
	findings, err := complexity.Find(complexity.Options{
		Root:      root,
		MarkerMap: markers,
	})
	if err != nil {
		findings = nil
	}

	result := buildDiffResult(root, files, diffOutput, findings)

	for i := range result.Files {
		f := &result.Files[i]
		fpath := filepath.Join(root, f.Path)
		if _, err := os.Stat(fpath); err == nil {
			mk, err := sindept.ParseFile(fpath)
			if err == nil {
				f.DebtMarkers = mk
				result.NewDebtMarkers += len(mk)
			}
		}
	}

	out := cmd.OutOrStdout()

	if diffJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	useColor := isTerminal(os.Stdout)

	if diffStat {
		return renderDiffStat(out, result, useColor)
	}
	return renderDiffFull(out, result, useColor)
}

// inGitRepo reports whether root is inside a git working tree.
func inGitRepo(root string) bool {
	_, err := runGitInDir(root, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// buildDiffArgs assembles the extra args for git diff based on flags.
func buildDiffArgs() []string {
	if diffLast {
		return []string{"HEAD~1..HEAD"}
	}
	if diffCached {
		return []string{"--cached"}
	}
	return nil
}

// runGitInDir executes a git command in root and returns stdout as a string.
func runGitInDir(root string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = root
	out, err := c.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if err == exitErr {
			return "", fmt.Errorf("%s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// parseNumstat parses `git diff --numstat` output into file summaries.
// Each line is: <added>\t<removed>\t<path>
// Binary files show "-" for added/removed.
func parseNumstat(output string) []diffFileSummary {
	var files []diffFileSummary
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		f := diffFileSummary{
			Path: parts[2],
		}
		f.LinesAdded = parseNumstatCount(parts[0])
		f.LinesRemoved = parseNumstatCount(parts[1])
		files = append(files, f)
	}
	return files
}

// parseNumstatCount converts a numstat count field to int ("-" → 0).
func parseNumstatCount(s string) int {
	s = strings.TrimSpace(s)
	if s == "-" || s == "" {
		return 0
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// buildDiffResult assembles the full diffResult from parsed components.
func buildDiffResult(root string, files []diffFileSummary, diffOutput string, findings []complexity.Finding) *diffResult {
	fileDiffs := splitDiffByFile(diffOutput)
	diffByPath := make(map[string]string, len(fileDiffs))
	for path, d := range fileDiffs {
		diffByPath[path] = d
	}

	findingByPath := make(map[string][]complexity.Finding)
	totalFindings := 0
	for _, f := range findings {
		findingByPath[f.Path] = append(findingByPath[f.Path], f)
		totalFindings++
	}

	result := &diffResult{
		ComplexityFindings: totalFindings,
	}
	for i := range files {
		f := &files[i]
		f.Diff = diffByPath[f.Path]
		f.ComplexityFindings = findingByPath[f.Path]
		f.ComplexityTags = uniqueTags(f.ComplexityFindings)
		result.LinesAdded += f.LinesAdded
		result.LinesRemoved += f.LinesRemoved
	}
	result.Files = files
	return result
}

// uniqueTags returns the sorted unique tags from a slice of findings.
func uniqueTags(findings []complexity.Finding) []string {
	seen := make(map[string]bool)
	var tags []string
	for _, f := range findings {
		if !seen[f.Tag] {
			seen[f.Tag] = true
			tags = append(tags, f.Tag)
		}
	}
	return tags
}

// splitDiffByFile splits a full git diff output into per-file sections,
// keyed by the file path (as it appears in the b/ side of the diff).
func splitDiffByFile(diffOutput string) map[string]string {
	out := make(map[string]string)
	sections := strings.Split(diffOutput, "diff --git ")
	for _, sec := range sections {
		sec = strings.TrimSpace(sec)
		if sec == "" {
			continue
		}
		path := extractDiffPath(sec)
		if path != "" {
			out[path] = "diff --git " + sec + "\n"
		}
	}
	return out
}

// extractDiffPath extracts the file path from a diff section.
// It looks for the `+++ b/path` line, falling back to the header.
func extractDiffPath(section string) string {
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			return strings.TrimPrefix(line, "+++ b/")
		}
		if strings.HasPrefix(line, "+++ /dev/null") {
			continue
		}
	}
	parts := strings.SplitN(section, " ", 3)
	if len(parts) >= 2 {
		b := strings.TrimSpace(parts[1])
		b = strings.TrimPrefix(b, "b/")
		return b
	}
	return ""
}

// ── rendering ──────────────────────────────────────────────────────────

// lipgloss styles for color output (only applied when useColor is true).
var (
	diffPathStyle    = lipgloss.NewStyle().Bold(true)
	diffAddedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	diffRemovedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	diffDebtStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	diffSummaryStyle = lipgloss.NewStyle().Bold(true)
)

var diffTagStyles = map[string]lipgloss.Style{
	complexity.TagDelete: lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
	complexity.TagStdlib: lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
	complexity.TagNative: lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true),
	complexity.TagYagni:  lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true),
	complexity.TagShrink: lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true),
}

// renderDiffFull renders the enriched diff with inline annotations.
func renderDiffFull(w io.Writer, result *diffResult, useColor bool) error {
	for i := range result.Files {
		f := &result.Files[i]
		renderFileSection(w, f, useColor)
	}
	renderDiffSummary(w, result, useColor)
	return nil
}

// renderFileSection renders one file's diff with complexity and debt annotations.
func renderFileSection(w io.Writer, f *diffFileSummary, useColor bool) {
	pathLabel := f.Path
	if useColor {
		pathLabel = diffPathStyle.Render(f.Path)
	}

	// Complexity header
	if len(f.ComplexityTags) > 0 {
		tagParts := make([]string, 0, len(f.ComplexityTags))
		for _, tag := range f.ComplexityTags {
			label := "[" + tag + "]"
			if useColor {
				if s, ok := diffTagStyles[tag]; ok {
					label = s.Render(label)
				}
			}
			tagParts = append(tagParts, label)
		}
		fmt.Fprintf(w, "%s %s — %d finding(s)\n", pathLabel, strings.Join(tagParts, " "), len(f.ComplexityFindings))
	} else {
		fmt.Fprintf(w, "%s\n", pathLabel)
	}

	// Debt marker count
	if len(f.DebtMarkers) > 0 {
		debtLabel := fmt.Sprintf("⚡ %d sin-debt marker(s)", len(f.DebtMarkers))
		if useColor {
			debtLabel = diffDebtStyle.Render(debtLabel)
		}
		fmt.Fprintf(w, "  %s\n", debtLabel)
	}

	// Diff body with ⚡ annotations
	if f.Diff != "" {
		for _, line := range strings.Split(f.Diff, "\n") {
			if shouldAnnotateDebt(line) {
				content := line
				if useColor {
					content = diffDebtStyle.Render("⚡ ") + line
				} else {
					content = "⚡ " + line
				}
				fmt.Fprintln(w, content)
			} else {
				if useColor {
					line = colorizeDiffLine(line)
				}
				fmt.Fprintln(w, line)
			}
		}
	}
	fmt.Fprintln(w)
}

// shouldAnnotateDebt reports whether a diff line contains a sin-debt marker.
func shouldAnnotateDebt(line string) bool {
	return strings.Contains(line, "sin-debt:")
}

// colorizeDiffLine applies green/red coloring to diff lines.
func colorizeDiffLine(line string) string {
	if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
		return diffAddedStyle.Render(line)
	}
	if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
		return diffRemovedStyle.Render(line)
	}
	return line
}

// renderDiffStat renders only the summary table.
func renderDiffStat(w io.Writer, result *diffResult, useColor bool) error {
	header := fmt.Sprintf(" %d files changed, %d insertions(+), %d deletions(-)",
		len(result.Files), result.LinesAdded, result.LinesRemoved)
	if useColor {
		header = diffSummaryStyle.Render(header)
	}
	fmt.Fprintln(w, header)

	for i := range result.Files {
		f := &result.Files[i]
		line := fmt.Sprintf(" %s | %d +, %d -", f.Path, f.LinesAdded, f.LinesRemoved)
		if useColor {
			line = diffPathStyle.Render(f.Path) + fmt.Sprintf(" | %d +, %d -", f.LinesAdded, f.LinesRemoved)
		}
		if len(f.ComplexityTags) > 0 {
			line += "  " + strings.Join(f.ComplexityTags, ",")
		}
		if len(f.DebtMarkers) > 0 {
			line += fmt.Sprintf("  ⚡%d", len(f.DebtMarkers))
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w)
	renderDiffSummary(w, result, useColor)
	return nil
}

// renderDiffSummary prints the final summary block.
func renderDiffSummary(w io.Writer, result *diffResult, useColor bool) {
	label := "Summary"
	if useColor {
		label = diffSummaryStyle.Render(label)
	}
	fmt.Fprintf(w, "%s: %d file(s) | +%d -%d | complexity: %d | sin-debt: %d\n",
		label, len(result.Files), result.LinesAdded, result.LinesRemoved,
		result.ComplexityFindings, result.NewDebtMarkers)
}
